package domain

import (
	"context"
	"time"
)

type FirmwareFile struct {
	ID            uint64  `db:"id" json:"id"`
	FirmwareUUID  string  `db:"firmware_uuid" json:"firmware_uuid"`
	VendorID      uint64  `db:"vendor_id" json:"vendor_id"`
	DeviceModelID *uint64 `db:"device_model_id" json:"device_model_id"`
	Version       string  `db:"version" json:"version"`
	FileName      string  `db:"file_name" json:"file_name"`
	// StorageKey adalah object key di object storage (MinIO/S3-compatible),
	// BUKAN URL — presigned URL digenerate on-demand tiap dibutuhkan lewat
	// ObjectStorage.PresignedGetURL (jangan simpan URL presigned, cepat
	// kedaluwarsa). Kolom DB: storage_key (di-rename dari file_path lama
	// yang dulunya string bebas dari client, lihat migrations/0007).
	StorageKey     string  `db:"storage_key" json:"storage_key"`
	FileSizeBytes  *uint64 `db:"file_size_bytes" json:"file_size_bytes"`
	ChecksumSHA256 *string `db:"checksum_sha256" json:"checksum_sha256"`
	ReleaseNotes   *string `db:"release_notes" json:"release_notes"`
	IsActive       bool    `db:"is_active" json:"is_active"`
	Audit
}

// MaxFirmwareFileSizeBytes adalah batas ukuran file firmware yang diterima
// endpoint upload (256MB — generous margin, firmware CPE residential/ONT
// biasanya beberapa puluh MB). Dipakai handler (guard sebelum baca body ke
// memory) dan usecase (defense-in-depth).
const MaxFirmwareFileSizeBytes int64 = 256 << 20

type FirmwareFileRepository interface {
	Create(ctx context.Context, f *FirmwareFile) error
	GetByID(ctx context.Context, id uint64) (*FirmwareFile, error)
	GetByUUID(ctx context.Context, uuid string) (*FirmwareFile, error)
	ListByVendor(ctx context.Context, vendorID uint64, p Pagination) ([]FirmwareFile, int, error)
	SoftDelete(ctx context.Context, id, deletedBy uint64) error
}

// FirmwareUpgradeJob menghubungkan firmware ke device dan task Download-nya
// (lihat TECH.md §7). RolloutBatchID/WaveNumber (migrations/0011) opsional --
// diisi bila job ini dibuat sbg bagian dari FirmwareRolloutBatch (canary/
// staged rollout), NULL utk job ScheduleUpgrade satuan (perilaku lama,
// tidak berubah).
type FirmwareUpgradeJob struct {
	ID         uint64 `db:"id" json:"id"`
	JobUUID    string `db:"job_uuid" json:"job_uuid"`
	DeviceID   uint64 `db:"device_id" json:"device_id"`
	FirmwareID uint64 `db:"firmware_id" json:"firmware_id"`
	// RolloutBatchID — FK opsional ke firmware_rollout_batches.
	RolloutBatchID *uint64 `db:"rollout_batch_id" json:"rollout_batch_id"`
	// WaveNumber — wave ke berapa (dalam RolloutBatchID) job ini dibuat,
	// dipakai menghitung success/failure rate PER WAVE (lihat
	// FirmwareUpgradeJobRepository.CountByRolloutWave).
	WaveNumber   *uint32    `db:"wave_number" json:"wave_number"`
	TaskID       *uint64    `db:"task_id" json:"task_id"`
	TaskStatusID uint64     `db:"task_status_id" json:"task_status_id"`
	FromVersion  *string    `db:"from_version" json:"from_version"`
	ToVersion    *string    `db:"to_version" json:"to_version"`
	ScheduledAt  *time.Time `db:"scheduled_at" json:"scheduled_at"`
	StartedAt    *time.Time `db:"started_at" json:"started_at"`
	CompletedAt  *time.Time `db:"completed_at" json:"completed_at"`
	ErrorMessage *string    `db:"error_message" json:"error_message"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	CreatedBy    *uint64    `db:"created_by" json:"created_by"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
	UpdatedBy    *uint64    `db:"updated_by" json:"updated_by"`
}

// FirmwareRolloutWaveStatusCount — agregasi jumlah job per task_status_id
// dalam satu batch/wave (dipakai usecase orchestration -- BELUM
// diimplementasikan di sini -- utk menghitung failure rate sebelum
// memutuskan lanjut/pause wave berikutnya). Pola sama dengan
// TaskStatusCount di domain/task.go.
type FirmwareRolloutWaveStatusCount struct {
	TaskStatusID uint64 `db:"task_status_id" json:"task_status_id"`
	Count        int    `db:"cnt" json:"count"`
}

type FirmwareUpgradeJobRepository interface {
	Create(ctx context.Context, j *FirmwareUpgradeJob) error
	GetByID(ctx context.Context, id uint64) (*FirmwareUpgradeJob, error)
	GetByTaskID(ctx context.Context, taskID uint64) (*FirmwareUpgradeJob, error)
	ListByDevice(ctx context.Context, deviceID uint64, p Pagination) ([]FirmwareUpgradeJob, int, error)
	// ListByRolloutBatch — daftar job milik satu batch, opsional difilter ke
	// satu wave tertentu (waveNumber nil = seluruh wave dalam batch tsb).
	ListByRolloutBatch(ctx context.Context, batchID uint64, waveNumber *uint32, p Pagination) ([]FirmwareUpgradeJob, int, error)
	// CountByRolloutWave — agregasi GROUP BY task_status_id utk job dalam
	// satu batch, opsional difilter ke satu wave (waveNumber nil = seluruh
	// wave dalam batch tsb) -- dasar perhitungan failure rate orchestration.
	//
	// [BUG KRITIS DIPERBAIKI] JOIN ke tasks.task_status_id (status LIVE),
	// BUKAN firmware_upgrade_jobs.task_status_id (salinan statis yang HANYA
	// diperbarui HandleTransferComplete saat CPE benar-benar mengirim RPC
	// TransferComplete) -- device yang timeout/offline sebelum sempat
	// TransferComplete sebelumnya membuat kolom itu macet PENDING SELAMANYA,
	// sehingga wave (dan seluruh batch) tidak pernah dianggap "tuntas" oleh
	// AdvanceRollout, walau task-nya sendiri sudah TIMEOUT lewat sweeper
	// TimeoutStaleSent yang sudah ada. Job yang task-nya MASIH non-terminal
	// (PENDING/QUEUED/SENT) TAPI sudah lebih tua dari staleAfter (device
	// benar-benar tidak pernah kembali online sama sekali utk memulai sesi
	// baru -- kasus yang TimeoutStaleSent sendiri tidak tangani krn itu
	// hanya utk task yang SUDAH SENT dalam satu sesi) diklasifikasikan
	// sbg timeoutStatusID secara efektif di hasil agregasi ini, supaya wave
	// tetap bisa maju/dievaluasi gagal alih-alih menunggu selamanya.
	CountByRolloutWave(ctx context.Context, batchID uint64, waveNumber *uint32, staleAfter time.Time, timeoutStatusID uint64) ([]FirmwareRolloutWaveStatusCount, error)
	UpdateStatus(ctx context.Context, id, taskStatusID uint64, errMsg *string) error
	MarkStarted(ctx context.Context, id uint64, startedAt time.Time) error
	MarkCompleted(ctx context.Context, id uint64, toVersion string, completedAt time.Time) error
}

// FirmwareScheduler dipakai usecase/provisioning untuk menjadwalkan firmware
// push sbg salah satu aksi ZeroTouchRule (FirmwareFileID, migrations/0009)
// tanpa provisioning perlu bergantung langsung pada package usecase/firmware
// (menghindari import cycle, pola sama seperti TaskEnqueuer di domain/task.go).
// Diimplementasikan oleh usecase/firmware.Service.ScheduleUpgrade tanpa
// perubahan signature (sudah cocok persis).
type FirmwareScheduler interface {
	ScheduleUpgrade(ctx context.Context, actor Actor, deviceID, firmwareID uint64, scheduledAt *time.Time) (*FirmwareUpgradeJob, error)
}

// FirmwareRolloutBatch — rencana canary/staged rollout firmware ke populasi
// device bertahap per wave (migrations/0011; TECH.md perbandingan GenieACS).
// Populasi target dipilih via VendorID/DeviceModelID -- bentuk filter yang
// SAMA dgn domain.DeviceFilter yang sudah dipakai listing device lain
// (domain/device.go), supaya usecase orchestration (BELUM diimplementasikan
// di sini) tinggal reuse DeviceRepository.List utk resolve populasi.
// Individual per-device job TETAP di FirmwareUpgradeJob (RolloutBatchID di
// atas) -- tidak menduplikasi device-tracking di sini.
type FirmwareRolloutBatch struct {
	ID                    uint64  `db:"id" json:"id"`
	BatchUUID             string  `db:"batch_uuid" json:"batch_uuid"`
	TenantID              *uint64 `db:"tenant_id" json:"tenant_id"` // NULL = rollout lintas-tenant/global
	FirmwareFileID        uint64  `db:"firmware_file_id" json:"firmware_file_id"`
	VendorID              *uint64 `db:"vendor_id" json:"vendor_id"`             // kriteria seleksi populasi, NULL = tidak difilter vendor
	DeviceModelID         *uint64 `db:"device_model_id" json:"device_model_id"` // kriteria seleksi populasi, NULL = tidak difilter model
	WavePercentage        uint8   `db:"wave_percentage" json:"wave_percentage"`
	MaxFailureRatePercent uint8   `db:"max_failure_rate_percent" json:"max_failure_rate_percent"`
	// CurrentWave — bookkeeping progres wave, diperbarui usecase
	// orchestration (mis. wave-advancement) yang MENYUSUL, bukan bagian dari
	// perubahan ini.
	CurrentWave uint32     `db:"current_wave" json:"current_wave"`
	StatusID    uint64     `db:"status_id" json:"status_id"`
	Notes       *string    `db:"notes" json:"notes"`
	ScheduledAt *time.Time `db:"scheduled_at" json:"scheduled_at"`
	StartedAt   *time.Time `db:"started_at" json:"started_at"`
	CompletedAt *time.Time `db:"completed_at" json:"completed_at"`
	Audit
}

type FirmwareRolloutBatchRepository interface {
	Create(ctx context.Context, b *FirmwareRolloutBatch) error
	GetByID(ctx context.Context, id uint64) (*FirmwareRolloutBatch, error)
	GetByUUID(ctx context.Context, uuid string) (*FirmwareRolloutBatch, error)
	List(ctx context.Context, tenantID *uint64, p Pagination) ([]FirmwareRolloutBatch, int, error)
	// Update menyimpan seluruh field yang bisa berubah setelah dibuat --
	// wave_percentage/max_failure_rate_percent/notes (edit operator), serta
	// current_wave/status_id/started_at/completed_at (bookkeeping progres
	// yang akan digerakkan usecase orchestration).
	Update(ctx context.Context, b *FirmwareRolloutBatch) error
	SoftDelete(ctx context.Context, id, deletedBy uint64) error
	// RunWithAdvanceLock — mutual exclusion ANTAR INSTANCE acsd (app server
	// stateless, TECH.md §9) memakai MariaDB named lock (GET_LOCK/
	// RELEASE_LOCK dipegang SATU koneksi DB yang sama sepanjang fn berjalan
	// -- bukan mutex Go proses, yang tidak berlaku lintas instance; dan
	// bukan pasangan Try/Release method terpisah, karena sqlx.DB adalah
	// connection POOL -- panggilan GetContext/ExecContext biasa yang
	// terpisah bisa jatuh ke koneksi fisik berbeda tiap kali, sehingga
	// RELEASE_LOCK dari koneksi lain TIDAK melepas lock yang di-acquire
	// lewat koneksi lain sebelumnya). Mencegah AdvanceRollout dipanggil
	// bersamaan utk batch yang sama (sweeper periodik DAN operator klik
	// "advance" manual bisa race) menghasilkan wave duplikat/device
	// dijadwalkan dua kali (temuan acs-security-reviewer). fn TETAP bebas
	// memakai repository lain apa pun lewat pool biasa -- lock ini murni
	// gerbang mutual-exclusion, bukan transaksi data.
	//
	// ran=false berarti fn TIDAK dijalankan sama sekali (pemanggil lain
	// sedang memegang lock batch ini) -- BUKAN error, murni "belum
	// giliranmu", aman dicoba lagi nanti (sweeper 1 menit berikutnya, atau
	// klik ulang operator).
	RunWithAdvanceLock(ctx context.Context, batchID uint64, fn func(ctx context.Context) error) (ran bool, err error)
}
