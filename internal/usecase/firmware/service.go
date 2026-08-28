// Package firmware menangani upload firmware sungguhan ke object storage
// (MinIO/S3-compatible, ROADMAP.md Fase 2) dan penjadwalan upgrade (TECH.md
// §7, FR-19/FR-20/FR-21). Handler HTTP hanya mem-parsing multipart request
// dan meneruskan io.Reader mentah ke usecase ini — keputusan penamaan object
// key, perhitungan checksum, dan orkestrasi upload/cleanup adalah business
// logic yang sengaja ditaruh di sini (bukan di delivery/http, CLAUDE.md).
package firmware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// presignedDownloadExpiry — masa berlaku URL presigned MinIO yang dikirim
// sebagai URL Download RPC CWMP. 1 jam cukup untuk CPE mulai mengunduh
// (TR-069 Download RPC device biasanya mulai unduh segera setelah task
// diterima) — keputusan produk yang sudah disepakati, lihat
// domain.ObjectStorage.
const presignedDownloadExpiry = 1 * time.Hour

type Service struct {
	files    domain.FirmwareFileRepository
	jobs     domain.FirmwareUpgradeJobRepository
	rollouts domain.FirmwareRolloutBatchRepository
	devices  domain.DeviceRepository
	tasks    domain.TaskCreator
	refs     domain.RefRepository
	activity domain.ActivityLogRepository
	storage  domain.ObjectStorage
}

func NewService(
	files domain.FirmwareFileRepository,
	jobs domain.FirmwareUpgradeJobRepository,
	rollouts domain.FirmwareRolloutBatchRepository,
	devices domain.DeviceRepository,
	tasks domain.TaskCreator,
	refs domain.RefRepository,
	activity domain.ActivityLogRepository,
	storage domain.ObjectStorage,
) *Service {
	return &Service{files: files, jobs: jobs, rollouts: rollouts, devices: devices, tasks: tasks, refs: refs, activity: activity, storage: storage}
}

// requireDeviceTenantScope — RBAC scope tenant (CLAUDE.md), dipakai tiap kali
// usecase ini mengakses data ter-scope ke satu device.
func requireDeviceTenantScope(actor domain.Actor, deviceTenantID *uint64) error {
	if actor.IsSuperadmin() {
		return nil
	}
	if deviceTenantID == nil || actor.TenantID == nil || *actor.TenantID != *deviceTenantID {
		return domain.ErrForbidden
	}
	return nil
}

// UploadFirmwareInput membawa isi file firmware sungguhan (io.Reader mentah
// dari multipart request, sudah dibuka handler) — bukan lagi path/checksum
// string bebas dari client. FileSize HARUS akurat (dipakai MinIO PutObject);
// handler mengisinya dari *multipart.FileHeader.Size yang dihitung Go dari
// byte yang benar-benar diterima, bukan klaim client.
type UploadFirmwareInput struct {
	VendorID      uint64
	DeviceModelID *uint64
	Version       string
	FileName      string
	File          io.Reader
	FileSize      int64
	ContentType   string
	ReleaseNotes  *string
}

// UploadFirmware meng-upload isi file ke object storage lalu mencatat
// metadatanya. Checksum SHA-256 SELALU dihitung di server dari isi file yang
// benar-benar diterima (streaming lewat io.TeeReader saat upload) — nilai
// checksum dari client (kalau ada) tidak pernah dipercaya (FR-19).
func (s *Service) UploadFirmware(ctx context.Context, actor domain.Actor, in UploadFirmwareInput) (*domain.FirmwareFile, error) {
	if in.File == nil {
		return nil, fmt.Errorf("%w: file firmware wajib diisi", domain.ErrInvalidInput)
	}
	if in.VendorID == 0 {
		return nil, fmt.Errorf("%w: vendor_id wajib diisi", domain.ErrInvalidInput)
	}
	if strings.TrimSpace(in.Version) == "" {
		return nil, fmt.Errorf("%w: version wajib diisi", domain.ErrInvalidInput)
	}
	if in.FileSize <= 0 || in.FileSize > domain.MaxFirmwareFileSizeBytes {
		return nil, fmt.Errorf("%w: ukuran file firmware tidak valid atau melebihi batas maksimum (%d byte)", domain.ErrInvalidInput, domain.MaxFirmwareFileSizeBytes)
	}

	objectKey := buildFirmwareObjectKey(in.VendorID, in.FileName)
	contentType := in.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	hasher := sha256.New()
	tee := io.TeeReader(in.File, hasher)
	if err := s.storage.Upload(ctx, objectKey, tee, in.FileSize, contentType); err != nil {
		return nil, fmt.Errorf("firmware: upload ke object storage gagal: %w", err)
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))
	size := uint64(in.FileSize)

	f := &domain.FirmwareFile{
		FirmwareUUID:   uuid.NewString(),
		VendorID:       in.VendorID,
		DeviceModelID:  in.DeviceModelID,
		Version:        in.Version,
		FileName:       in.FileName,
		StorageKey:     objectKey,
		FileSizeBytes:  &size,
		ChecksumSHA256: &checksum,
		ReleaseNotes:   in.ReleaseNotes,
		IsActive:       true,
		Audit:          domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if err := s.files.Create(ctx, f); err != nil {
		// Cleanup: file sudah ter-upload tapi metadata gagal disimpan — jangan
		// tinggalkan objek yatim di bucket.
		_ = s.storage.Delete(ctx, objectKey)
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "UPLOAD_FIRMWARE", EntityType: "firmware_file", EntityID: &f.ID,
	})
	return f, nil
}

// buildFirmwareObjectKey menghasilkan object key yang tidak gampang ditebak
// dan tidak bisa collision/overwrite tak sengaja (UUID + nama asli sebagai
// bagian akhir, bukan satu-satunya bagian). filepath.Base + penyaringan
// karakter mencegah path traversal lewat nama file yang dikontrol client
// (mis. "../../etc/passwd" atau "..\\..\\secret").
func buildFirmwareObjectKey(vendorID uint64, fileName string) string {
	base := filepath.Base(fileName)
	if base == "" || base == "." || base == ".." || base == string(filepath.Separator) {
		base = "firmware.bin"
	}
	base = sanitizeObjectKeyPart(base)
	if base == "" {
		base = "firmware.bin"
	}
	// Batasi panjang bagian nama file -- ini dikontrol penuh oleh client lewat
	// header Content-Disposition, tanpa batas nama file yang sangat panjang
	// bisa membuat object key melebihi lebar kolom firmware_files.storage_key
	// (VARCHAR(500)) atau limit key S3 (~1024 byte). Endpoint ini superadmin-
	// only jadi bukan vektor eksploitasi eksternal, tapi dibatasi utk
	// robustness (temuan acs-security-reviewer, review fitur MinIO).
	const maxFileNamePartLen = 100
	if len(base) > maxFileNamePartLen {
		base = base[:maxFileNamePartLen]
	}
	return fmt.Sprintf("firmware/%d/%s_%s", vendorID, uuid.NewString(), base)
}

// sanitizeObjectKeyPart membatasi nama file ke karakter aman untuk object
// key S3-compatible (alfanumerik, titik, strip, underscore) — karakter lain
// (termasuk "/", "\\", null byte) diganti underscore.
func sanitizeObjectKeyPart(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "._")
}

func (s *Service) Get(ctx context.Context, id uint64) (*domain.FirmwareFile, error) {
	return s.files.GetByID(ctx, id)
}

func (s *Service) ListByVendor(ctx context.Context, vendorID uint64, p domain.Pagination) ([]domain.FirmwareFile, int, error) {
	return s.files.ListByVendor(ctx, vendorID, p)
}

// ScheduleUpgrade membuat firmware_upgrade_jobs + task Download (RPC CWMP
// standar FileType=1 Firmware Upgrade Image, lihat TECH.md §7). Dipakai jalur
// manual (satu device) DAN sbg domain.FirmwareScheduler oleh
// usecase/provisioning (aksi FirmwareFileID pada ZeroTouchRule).
func (s *Service) ScheduleUpgrade(ctx context.Context, actor domain.Actor, deviceID, firmwareID uint64, scheduledAt *time.Time) (*domain.FirmwareUpgradeJob, error) {
	return s.scheduleUpgradeJob(ctx, actor, deviceID, firmwareID, scheduledAt, nil, nil)
}

// scheduleUpgradeJob adalah inti ScheduleUpgrade, ditambah rolloutBatchID/
// waveNumber opsional (diisi non-nil HANYA oleh advanceRolloutWave saat
// membuat job sbg bagian satu wave canary rollout, ROADMAP.md perbandingan
// GenieACS — lihat migrations/0011).
func (s *Service) scheduleUpgradeJob(ctx context.Context, actor domain.Actor, deviceID, firmwareID uint64, scheduledAt *time.Time, rolloutBatchID *uint64, waveNumber *uint32) (*domain.FirmwareUpgradeJob, error) {
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if err := requireDeviceTenantScope(actor, dev.TenantID); err != nil {
		return nil, err
	}
	fw, err := s.files.GetByID(ctx, firmwareID)
	if err != nil {
		return nil, err
	}

	// Presigned URL MinIO LANGSUNG (bukan proxy lewat ACS) — keputusan
	// produk yang sudah disepakati, lihat domain.ObjectStorage. Digenerate
	// on-demand di sini (bukan disimpan) karena masa berlakunya pendek.
	downloadURL, err := s.storage.PresignedGetURL(ctx, fw.StorageKey, presignedDownloadExpiry)
	if err != nil {
		return nil, fmt.Errorf("firmware: gagal membuat presigned URL: %w", err)
	}
	t, err := s.tasks.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID: deviceID,
		TaskType: domain.TaskTypeDownload,
		Priority: 3,
		Parameters: map[string]interface{}{
			"file_type":        "1 Firmware Upgrade Image",
			"url":              downloadURL,
			"checksum_sha256":  fw.ChecksumSHA256,
			"target_file_name": fw.FileName,
		},
		ScheduledAt: scheduledAt,
	})
	if err != nil {
		return nil, err
	}

	pendingStatus, err := s.refs.GetByCode(ctx, domain.RefTableTaskStatus, domain.TaskStatusPending)
	if err != nil {
		return nil, err
	}

	job := &domain.FirmwareUpgradeJob{
		JobUUID:        uuid.NewString(),
		DeviceID:       deviceID,
		FirmwareID:     firmwareID,
		RolloutBatchID: rolloutBatchID,
		WaveNumber:     waveNumber,
		TaskID:         &t.ID,
		TaskStatusID:   pendingStatus.ID,
		FromVersion:    dev.SoftwareVersion,
		ToVersion:      &fw.Version,
		ScheduledAt:    scheduledAt,
		CreatedBy:      actor.UserIDPtr(),
	}
	if err := s.jobs.Create(ctx, job); err != nil {
		return nil, err
	}

	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "SCHEDULE_FIRMWARE_UPGRADE", EntityType: "firmware_upgrade_job", EntityID: &job.ID,
	})
	return job, nil
}

func (s *Service) ListJobsByDevice(ctx context.Context, actor domain.Actor, deviceID uint64, p domain.Pagination) ([]domain.FirmwareUpgradeJob, int, error) {
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return nil, 0, err
	}
	if err := requireDeviceTenantScope(actor, dev.TenantID); err != nil {
		return nil, 0, err
	}
	return s.jobs.ListByDevice(ctx, deviceID, p)
}

// HandleTransferComplete di-panggil usecase/session saat CPE mengirim RPC
// TransferComplete (event 7) berkorelasi dengan task Download milik job ini
// (TECH.md §7) — meng-update status job & software_version device bila sukses.
func (s *Service) HandleTransferComplete(ctx context.Context, taskID uint64, success bool, errMsg string) error {
	job, err := s.jobs.GetByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	if success {
		completedStatus, err := s.refs.GetByCode(ctx, domain.RefTableTaskStatus, domain.TaskStatusCompleted)
		if err != nil {
			return err
		}
		now := time.Now()
		toVersion := ""
		if job.ToVersion != nil {
			toVersion = *job.ToVersion
		}
		if err := s.jobs.MarkCompleted(ctx, job.ID, toVersion, now); err != nil {
			return err
		}
		if err := s.jobs.UpdateStatus(ctx, job.ID, completedStatus.ID, nil); err != nil {
			return err
		}
		if dev, err := s.devices.GetByID(ctx, job.DeviceID); err == nil {
			dev.SoftwareVersion = job.ToVersion
			_ = s.devices.Update(ctx, dev)
		}
		return nil
	}
	failedStatus, err := s.refs.GetByCode(ctx, domain.RefTableTaskStatus, domain.TaskStatusFailed)
	if err != nil {
		return err
	}
	return s.jobs.UpdateStatus(ctx, job.ID, failedStatus.ID, &errMsg)
}

// ---- Firmware Rollout Batch (canary/staged rollout, migrations/0011,
// perbandingan kapabilitas GenieACS) — TECH.md §7: upgrade firmware adalah
// operasi berisiko tinggi (bisa mem-brick device di lokasi pelanggan yang
// sulit dijangkau), jadi populasi besar SEBAIKNYA di-upgrade bertahap per
// wave, bukan sekaligus, dengan gerbang failure-rate di antara wave. ----

// requireRolloutTenantScope — pola sama persis dgn requireDeviceTenantScope
// di atas (file ini sengaja tidak impor usecase/auth, ikut konvensi yg sudah
// ada di firmware/service.go).
func requireRolloutTenantScope(actor domain.Actor, batchTenantID *uint64) error {
	if actor.IsSuperadmin() {
		return nil
	}
	if batchTenantID == nil || actor.TenantID == nil || *actor.TenantID != *batchTenantID {
		return domain.ErrForbidden
	}
	return nil
}

type CreateRolloutBatchInput struct {
	TenantID              *uint64
	FirmwareFileID        uint64
	VendorID              *uint64
	DeviceModelID         *uint64
	WavePercentage        uint8
	MaxFailureRatePercent uint8
	Notes                 *string
	ScheduledAt           *time.Time
}

// CreateRolloutBatch membuat rencana rollout lalu LANGSUNG memicu wave
// pertama (operator tidak perlu panggilan terpisah untuk "mulai") — panggilan
// lanjutan ke AdvanceRollout (manual via REST, atau sweep periodik
// cmd/acsd) yang meneruskan ke wave berikutnya setelah wave berjalan selesai.
func (s *Service) CreateRolloutBatch(ctx context.Context, actor domain.Actor, in CreateRolloutBatchInput) (*domain.FirmwareRolloutBatch, error) {
	// TenantID nil (global/lintas-tenant) hanya boleh dibuat superadmin --
	// pola sama seperti resource lintas-tenant lain di codebase ini (mis.
	// profile provisioning global).
	if in.TenantID == nil {
		if !actor.IsSuperadmin() {
			return nil, domain.ErrForbidden
		}
	} else if err := requireRolloutTenantScope(actor, in.TenantID); err != nil {
		return nil, err
	}
	if _, err := s.files.GetByID(ctx, in.FirmwareFileID); err != nil {
		return nil, err
	}
	if in.WavePercentage == 0 || in.WavePercentage > 100 {
		return nil, fmt.Errorf("%w: wave_percentage harus 1-100", domain.ErrInvalidInput)
	}
	if in.MaxFailureRatePercent > 100 {
		return nil, fmt.Errorf("%w: max_failure_rate_percent harus 0-100", domain.ErrInvalidInput)
	}

	pendingStatus, err := s.refs.GetByCode(ctx, domain.RefTableFirmwareRolloutStatus, domain.FirmwareRolloutStatusPending)
	if err != nil {
		return nil, err
	}
	batch := &domain.FirmwareRolloutBatch{
		BatchUUID:             uuid.NewString(),
		TenantID:              in.TenantID,
		FirmwareFileID:        in.FirmwareFileID,
		VendorID:              in.VendorID,
		DeviceModelID:         in.DeviceModelID,
		WavePercentage:        in.WavePercentage,
		MaxFailureRatePercent: in.MaxFailureRatePercent,
		StatusID:              pendingStatus.ID,
		Notes:                 in.Notes,
		ScheduledAt:           in.ScheduledAt,
		Audit:                 domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if err := s.rollouts.Create(ctx, batch); err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "CREATE_FIRMWARE_ROLLOUT_BATCH", EntityType: "firmware_rollout_batch", EntityID: &batch.ID,
	})
	// Wave pertama dipicu langsung dgn actor pembuat batch (kecuali ada jadwal)
	// -- kalau gagal (mis. tidak ada device yg cocok kriteria), batch TETAP tersimpan
	// berstatus PENDING, bukan dianggap gagal seutuhnya (operator bisa lihat
	// & coba AdvanceRollout lagi manual, mis. setelah menambah device).
	if batch.ScheduledAt == nil || !batch.ScheduledAt.After(time.Now()) {
		if advanced, err := s.AdvanceRollout(ctx, batch.ID); err == nil {
			batch = advanced
		}
	}
	return batch, nil
}

func (s *Service) GetRolloutBatch(ctx context.Context, actor domain.Actor, id uint64) (*domain.FirmwareRolloutBatch, error) {
	b, err := s.rollouts.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if b.TenantID == nil {
		if !actor.IsSuperadmin() {
			return nil, domain.ErrForbidden
		}
	} else if err := requireRolloutTenantScope(actor, b.TenantID); err != nil {
		return nil, err
	}
	return b, nil
}

// ListRolloutBatches — queryTenantID cuma efektif utk superadmin (pola sama
// dgn List lain di codebase ini); non-superadmin selalu dipaksa ke tenant
// sendiri, ditolak (bukan fail-open) kalau actor.TenantID kosong.
func (s *Service) ListRolloutBatches(ctx context.Context, actor domain.Actor, queryTenantID *uint64, p domain.Pagination) ([]domain.FirmwareRolloutBatch, int, error) {
	tenantID := queryTenantID
	if !actor.IsSuperadmin() {
		if actor.TenantID == nil {
			return nil, 0, domain.ErrForbidden
		}
		tenantID = actor.TenantID
	}
	return s.rollouts.List(ctx, tenantID, p)
}

// CancelRolloutBatch menghentikan rollout secara permanen (STATUS CANCELLED,
// BUKAN soft-delete baris — batch tetap harus terlihat di histori/audit).
// Job yang SUDAH terlanjur dibuat pada wave berjalan TIDAK dibatalkan di sini
// (task Download yang sudah terkirim ke device biarkan selesai wajar) --
// yang dicegah CancelRolloutBatch adalah wave BERIKUTNYA, bukan menarik balik
// yang sudah berjalan.
func (s *Service) CancelRolloutBatch(ctx context.Context, actor domain.Actor, id uint64) error {
	b, err := s.rollouts.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if b.TenantID == nil {
		if !actor.IsSuperadmin() {
			return domain.ErrForbidden
		}
	} else if err := requireRolloutTenantScope(actor, b.TenantID); err != nil {
		return err
	}
	cancelledStatus, err := s.refs.GetByCode(ctx, domain.RefTableFirmwareRolloutStatus, domain.FirmwareRolloutStatusCancelled)
	if err != nil {
		return err
	}
	b.StatusID = cancelledStatus.ID
	b.UpdatedBy = actor.UserIDPtr()
	if err := s.rollouts.Update(ctx, b); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "CANCEL_FIRMWARE_ROLLOUT_BATCH", EntityType: "firmware_rollout_batch", EntityID: &id,
	})
	return nil
}

// terminalFailureStatuses — status task yang dihitung sbg "gagal" untuk
// failure-rate gating (BUKAN cuma FAILED — TIMEOUT dan CANCELLED juga berarti
// device TIDAK berhasil naik firmware pada wave ini).
var terminalFailureStatuses = []string{domain.TaskStatusFailed, domain.TaskStatusTimeout, domain.TaskStatusCancelled}

// AdvanceRollout adalah satu-satunya titik masuk orkestrasi wave: dipanggil
// manual (REST, operator ingin memaksa cek sekarang) MAUPUN oleh sweep
// periodik (cmd/acsd, mirip pola TimeoutStaleSent di usecase/task). Idempotent
// dan aman dipanggil berkali-kali kapan pun — hanya benar-benar mengubah
// state kalau wave berjalan sudah tuntas (semua job berstatus akhir).
// AdvanceRollout -- lihat domain.FirmwareRolloutBatchRepository.RunWithAdvanceLock
// soal kenapa seluruh logika di bawah dibungkus lock advisory per-batch
// (mencegah sweeper periodik & klik manual operator, atau dua instance acsd,
// race membuat wave/device dijadwalkan dobel -- temuan acs-security-reviewer).
// !ran (lock sedang dipegang pemanggil lain) BUKAN error -- kembalikan state
// batch apa adanya, aman dicoba lagi nanti.
func (s *Service) AdvanceRollout(ctx context.Context, batchID uint64) (*domain.FirmwareRolloutBatch, error) {
	var result *domain.FirmwareRolloutBatch
	ran, err := s.rollouts.RunWithAdvanceLock(ctx, batchID, func(ctx context.Context) error {
		b, err := s.advanceRolloutLocked(ctx, batchID)
		result = b
		return err
	})
	if err != nil {
		return nil, err
	}
	if !ran {
		return s.rollouts.GetByID(ctx, batchID)
	}
	return result, nil
}

// advanceRolloutLocked adalah isi AdvanceRollout sesungguhnya, DIJAMIN
// pemanggil TUNGGAL berkat RunWithAdvanceLock di atas.
func (s *Service) advanceRolloutLocked(ctx context.Context, batchID uint64) (*domain.FirmwareRolloutBatch, error) {
	batch, err := s.rollouts.GetByID(ctx, batchID)
	if err != nil {
		return nil, err
	}
	pendingStatus, err := s.refs.GetByCode(ctx, domain.RefTableFirmwareRolloutStatus, domain.FirmwareRolloutStatusPending)
	if err != nil {
		return nil, err
	}
	inProgressStatus, err := s.refs.GetByCode(ctx, domain.RefTableFirmwareRolloutStatus, domain.FirmwareRolloutStatusInProgress)
	if err != nil {
		return nil, err
	}
	// Hanya batch PENDING (belum pernah punya wave) atau IN_PROGRESS (wave
	// sebelumnya sedang berjalan/baru tuntas) yang diproses -- PAUSED_
	// FAILURE_THRESHOLD/COMPLETED/CANCELLED SENGAJA tidak diapa-apakan di
	// sini (perlu tindakan eksplisit operator, bukan otomatis lanjut lagi).
	if batch.StatusID != pendingStatus.ID && batch.StatusID != inProgressStatus.ID {
		return batch, nil
	}

	if batch.CurrentWave > 0 {
		wave := batch.CurrentWave
		statusIDByCode, err := s.taskStatusIDsByCode(ctx, append([]string{
			domain.TaskStatusPending, domain.TaskStatusQueued, domain.TaskStatusSent, domain.TaskStatusCompleted,
		}, terminalFailureStatuses...))
		if err != nil {
			return nil, err
		}
		// staleWaveJobAge -- job non-terminal yang task-nya lebih tua dari ini
		// diperlakukan efektif TIMEOUT oleh CountByRolloutWave (lihat komentar
		// lengkap di domain/firmware.go: bug kritis ditemukan review keamanan --
		// device yang tidak PERNAH kembali online sama sekali membuat wave
		// macet selamanya kalau bergantung murni pada TransferComplete/
		// TimeoutStaleSent, yang hanya menangani task yang SUDAH SENT dalam
		// satu sesi). 24 jam dipilih sbg default aman: device ISP yang benar-
		// benar mati akan absen dari SEMUA Inform (termasuk periodic) jauh
		// lebih dari 24 jam, sementara device yang hanya offline sesaat
		// (restart router pelanggan, dst.) akan sempat Inform lagi & task-nya
		// diproses normal jauh sebelum ambang ini tercapai.
		const staleWaveJobAge = 24 * time.Hour
		counts, err := s.jobs.CountByRolloutWave(ctx, batch.ID, &wave, time.Now().Add(-staleWaveJobAge), statusIDByCode[domain.TaskStatusTimeout])
		if err != nil {
			return nil, err
		}
		var total, inFlight, failed int
		for _, c := range counts {
			total += c.Count
			switch c.TaskStatusID {
			case statusIDByCode[domain.TaskStatusPending], statusIDByCode[domain.TaskStatusQueued], statusIDByCode[domain.TaskStatusSent]:
				inFlight += c.Count
			case statusIDByCode[domain.TaskStatusFailed], statusIDByCode[domain.TaskStatusTimeout], statusIDByCode[domain.TaskStatusCancelled]:
				failed += c.Count
			}
		}
		if inFlight > 0 {
			// Wave berjalan belum tuntas -- belum waktunya memutuskan apa pun.
			return batch, nil
		}
		if total > 0 {
			failureRatePercent := failed * 100 / total
			if failureRatePercent > int(batch.MaxFailureRatePercent) {
				pausedStatus, err := s.refs.GetByCode(ctx, domain.RefTableFirmwareRolloutStatus, domain.FirmwareRolloutStatusPausedFailureThreshold)
				if err != nil {
					return nil, err
				}
				batch.StatusID = pausedStatus.ID
				if err := s.rollouts.Update(ctx, batch); err != nil {
					return nil, err
				}
				desc := fmt.Sprintf("wave %d: failure rate %d%% melebihi batas %d%%", wave, failureRatePercent, batch.MaxFailureRatePercent)
				_ = s.activity.Record(ctx, &domain.ActivityLog{
					TenantID: batch.TenantID, Action: "FIRMWARE_ROLLOUT_PAUSED", EntityType: "firmware_rollout_batch", EntityID: &batch.ID,
					Description: &desc,
				})
				return batch, nil
			}
		}
	}

	targets, err := s.resolveRolloutWaveTargets(ctx, batch)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		completedStatus, err := s.refs.GetByCode(ctx, domain.RefTableFirmwareRolloutStatus, domain.FirmwareRolloutStatusCompleted)
		if err != nil {
			return nil, err
		}
		batch.StatusID = completedStatus.ID
		now := time.Now()
		batch.CompletedAt = &now
		if err := s.rollouts.Update(ctx, batch); err != nil {
			return nil, err
		}
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			TenantID: batch.TenantID, Action: "FIRMWARE_ROLLOUT_COMPLETED", EntityType: "firmware_rollout_batch", EntityID: &batch.ID,
		})
		return batch, nil
	}

	// wave dihitung sbg CANDIDATE dulu (belum di-commit ke batch.CurrentWave)
	// -- lihat penanganan succeeded==0 di bawah: kalau TIDAK ADA device yang
	// berhasil dijadwalkan sama sekali (mis. kuota task queue tenant habis,
	// bukan masalah per-device), CurrentWave TIDAK boleh maju dan batch harus
	// PAUSED, bukan diam-diam retry wave yang sama tiap menit selamanya lewat
	// sweeper tanpa pernah terlihat operator (bug ditemukan review kode).
	wave := batch.CurrentWave + 1
	actor := s.rolloutSystemActor(batch)
	succeeded := 0
	for _, dev := range targets {
		if _, err := s.scheduleUpgradeJob(ctx, actor, dev.ID, batch.FirmwareFileID, nil, &batch.ID, &wave); err != nil {
			// Best-effort per device -- device yang gagal dijadwalkan TIDAK
			// dapat baris job (tidak ikut "sudah ditarget"), jadi otomatis
			// dicoba lagi pada wave berikutnya oleh resolveRolloutWaveTargets.
			continue
		}
		succeeded++
	}
	if succeeded == 0 {
		// Semua percobaan jadwal gagal (mis. kuota task queue tenant penuh) --
		// PAUSE, bukan lanjut retry tak berujung tanpa operator pernah tahu.
		pausedStatus, err := s.refs.GetByCode(ctx, domain.RefTableFirmwareRolloutStatus, domain.FirmwareRolloutStatusPausedFailureThreshold)
		if err != nil {
			return nil, err
		}
		batch.StatusID = pausedStatus.ID
		if err := s.rollouts.Update(ctx, batch); err != nil {
			return nil, err
		}
		desc := fmt.Sprintf("wave %d: 0/%d device berhasil dijadwalkan -- kemungkinan kuota task queue tenant penuh atau masalah sistemik lain, bukan kegagalan device", wave, len(targets))
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			TenantID: batch.TenantID, Action: "FIRMWARE_ROLLOUT_PAUSED", EntityType: "firmware_rollout_batch", EntityID: &batch.ID,
			Description: &desc,
		})
		return batch, nil
	}
	batch.CurrentWave = wave
	batch.StatusID = inProgressStatus.ID
	if batch.StartedAt == nil {
		now := time.Now()
		batch.StartedAt = &now
	}
	if err := s.rollouts.Update(ctx, batch); err != nil {
		return nil, err
	}
	desc := fmt.Sprintf("wave %d: %d/%d device berhasil dijadwalkan", wave, succeeded, len(targets))
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		TenantID: batch.TenantID, Action: "FIRMWARE_ROLLOUT_WAVE_STARTED", EntityType: "firmware_rollout_batch", EntityID: &batch.ID,
		Description: &desc,
	})
	return batch, nil
}

// SweepRolloutBatches mengevaluasi SEMUA rollout batch berstatus PENDING/
// IN_PROGRESS lintas tenant — dipanggil sweeper periodik cmd/acsd (pola sama
// dgn task.Service.TimeoutStaleSent), aman dijalankan dari instance manapun
// (TECH.md §9, app server stateless) krn AdvanceRollout idempotent.
func (s *Service) SweepRolloutBatches(ctx context.Context) (int, error) {
	pendingStatus, err := s.refs.GetByCode(ctx, domain.RefTableFirmwareRolloutStatus, domain.FirmwareRolloutStatusPending)
	if err != nil {
		return 0, err
	}
	inProgressStatus, err := s.refs.GetByCode(ctx, domain.RefTableFirmwareRolloutStatus, domain.FirmwareRolloutStatusInProgress)
	if err != nil {
		return 0, err
	}
	advanced, page, fetched := 0, 1, 0
	for {
		batches, total, err := s.rollouts.List(ctx, nil, domain.Pagination{Page: page, PageSize: 200})
		if err != nil {
			return advanced, err
		}
		for _, b := range batches {
			if b.StatusID != pendingStatus.ID && b.StatusID != inProgressStatus.ID {
				continue
			}
			if b.StatusID == pendingStatus.ID && b.ScheduledAt != nil && b.ScheduledAt.After(time.Now()) {
				continue // belum jadwalnya
			}
			if _, err := s.AdvanceRollout(ctx, b.ID); err != nil {
				continue // best-effort -- dicoba lagi sweep berikutnya
			}
			advanced++
		}
		fetched += len(batches)
		if fetched >= total || len(batches) == 0 {
			break
		}
		page++
	}
	return advanced, nil
}

// rolloutSystemActor — batch bertenant (TenantID != nil) diproses sbg actor
// tenant tsb (cukup utk lolos requireDeviceTenantScope di scheduleUpgradeJob,
// TIDAK perlu role apa pun, pola sama dgn systemActor ZTP di usecase/session);
// batch global (TenantID nil) BUTUH actor superadmin krn populasinya lintas
// tenant.
func (s *Service) rolloutSystemActor(batch *domain.FirmwareRolloutBatch) domain.Actor {
	if batch.TenantID != nil {
		return domain.Actor{TenantID: batch.TenantID}
	}
	return domain.Actor{Roles: []string{domain.RoleSuperadmin}}
}

// taskStatusIDsByCode — resolve sekumpulan kode ref_task_status ke ID sekali
// jalan (dipanggil per AdvanceRollout, bukan per device -- jumlah kode tetap
// & kecil).
func (s *Service) taskStatusIDsByCode(ctx context.Context, codes []string) (map[string]uint64, error) {
	out := make(map[string]uint64, len(codes))
	for _, code := range codes {
		ref, err := s.refs.GetByCode(ctx, domain.RefTableTaskStatus, code)
		if err != nil {
			return nil, err
		}
		out[code] = ref.ID
	}
	return out, nil
}

// resolveRolloutWaveTargets menghitung populasi device yang cocok kriteria
// batch (Vendor/DeviceModel/Tenant, form sama dgn domain.DeviceFilter),
// mengecualikan yang SUDAH pernah ditarget wave manapun pada batch ini, lalu
// mengembalikan potongan sejumlah wave_percentage% dari TOTAL populasi asli
// (bukan dari sisa yang belum ditarget) — supaya ukuran tiap wave stabil dan
// dapat diprediksi operator, wave terakhir otomatis mendapat sisa
// pembulatan. Baik populasi maupun daftar job SUDAH-ditarget diambil dgn
// paginasi penuh (bukan cuma halaman pertama) supaya benar utk populasi besar
// (ROADMAP.md: "ribuan hingga puluhan ribu CPE").
func (s *Service) resolveRolloutWaveTargets(ctx context.Context, batch *domain.FirmwareRolloutBatch) ([]domain.Device, error) {
	population, err := s.listAllDevices(ctx, domain.DeviceFilter{
		TenantID: batch.TenantID, VendorID: batch.VendorID, DeviceModelID: batch.DeviceModelID,
	})
	if err != nil {
		return nil, err
	}
	if len(population) == 0 {
		return nil, nil
	}
	targeted, err := s.listAllRolloutJobDeviceIDs(ctx, batch.ID)
	if err != nil {
		return nil, err
	}
	untargeted := make([]domain.Device, 0, len(population))
	for _, dev := range population {
		if !targeted[dev.ID] {
			untargeted = append(untargeted, dev)
		}
	}
	if len(untargeted) == 0 {
		return nil, nil
	}
	chunkSize := len(population) * int(batch.WavePercentage) / 100
	if chunkSize < 1 {
		chunkSize = 1
	}
	if chunkSize > len(untargeted) {
		chunkSize = len(untargeted)
	}
	return untargeted[:chunkSize], nil
}

// listAllDevices — paginasi penuh DeviceRepository.List (dibatasi
// domain.Pagination.Limit() 200/halaman) supaya benar utk populasi > 200 device.
func (s *Service) listAllDevices(ctx context.Context, f domain.DeviceFilter) ([]domain.Device, error) {
	var all []domain.Device
	page := 1
	for {
		rows, total, err := s.devices.List(ctx, f, domain.Pagination{Page: page, PageSize: 200})
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
		if len(all) >= total || len(rows) == 0 {
			break
		}
		page++
	}
	return all, nil
}

// listAllRolloutJobDeviceIDs — paginasi penuh job lintas SELURUH wave suatu
// batch (waveNumber nil), dikembalikan sbg set device_id yg sudah pernah
// ditarget.
func (s *Service) listAllRolloutJobDeviceIDs(ctx context.Context, batchID uint64) (map[uint64]bool, error) {
	set := make(map[uint64]bool)
	page, fetched := 1, 0
	for {
		rows, total, err := s.jobs.ListByRolloutBatch(ctx, batchID, nil, domain.Pagination{Page: page, PageSize: 200})
		if err != nil {
			return nil, err
		}
		for _, j := range rows {
			set[j.DeviceID] = true
		}
		fetched += len(rows)
		// fetched dibandingkan ke total BARIS job (bisa > jumlah device unik
		// kalau satu device pernah gagal dijadwalkan ulang di wave lain),
		// BUKAN len(set) -- supaya paginasi berhenti tepat waktu.
		if fetched >= total || len(rows) == 0 {
			break
		}
		page++
	}
	return set, nil
}
