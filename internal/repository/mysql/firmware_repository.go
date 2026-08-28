package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// ---- FirmwareFile ----

type firmwareFileRepository struct{ db *sqlx.DB }

func NewFirmwareFileRepository(db *sqlx.DB) domain.FirmwareFileRepository {
	return &firmwareFileRepository{db: db}
}

func (r *firmwareFileRepository) Create(ctx context.Context, f *domain.FirmwareFile) error {
	now := time.Now()
	f.CreatedAt, f.UpdatedAt = now, now
	const q = `INSERT INTO firmware_files
		(firmware_uuid, vendor_id, device_model_id, version, file_name, storage_key, file_size_bytes,
		 checksum_sha256, release_notes, is_active, created_at, updated_at, created_by)
		VALUES (:firmware_uuid, :vendor_id, :device_model_id, :version, :file_name, :storage_key, :file_size_bytes,
		 :checksum_sha256, :release_notes, :is_active, :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, f)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	f.ID = uint64(id)
	return nil
}

func (r *firmwareFileRepository) GetByID(ctx context.Context, id uint64) (*domain.FirmwareFile, error) {
	var f domain.FirmwareFile
	if err := r.db.GetContext(ctx, &f, `SELECT * FROM firmware_files WHERE id = ? AND is_deleted = 0`, id); err != nil {
		return nil, translateErr(err)
	}
	return &f, nil
}

func (r *firmwareFileRepository) GetByUUID(ctx context.Context, uuid string) (*domain.FirmwareFile, error) {
	var f domain.FirmwareFile
	err := r.db.GetContext(ctx, &f, `SELECT * FROM firmware_files WHERE firmware_uuid = ? AND is_deleted = 0`, uuid)
	if err != nil {
		return nil, translateErr(err)
	}
	return &f, nil
}

func (r *firmwareFileRepository) ListByVendor(ctx context.Context, vendorID uint64, p domain.Pagination) ([]domain.FirmwareFile, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM firmware_files WHERE vendor_id = ? AND is_deleted = 0`, vendorID); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.FirmwareFile
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM firmware_files WHERE vendor_id = ? AND is_deleted = 0 ORDER BY id DESC LIMIT ? OFFSET ?`,
		vendorID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *firmwareFileRepository) SoftDelete(ctx context.Context, id, deletedBy uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE firmware_files SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now(), deletedBy, id)
	return translateErr(err)
}

// ---- FirmwareUpgradeJob ----

type firmwareUpgradeJobRepository struct{ db *sqlx.DB }

func NewFirmwareUpgradeJobRepository(db *sqlx.DB) domain.FirmwareUpgradeJobRepository {
	return &firmwareUpgradeJobRepository{db: db}
}

func (r *firmwareUpgradeJobRepository) Create(ctx context.Context, j *domain.FirmwareUpgradeJob) error {
	now := time.Now()
	j.CreatedAt, j.UpdatedAt = now, now
	const q = `INSERT INTO firmware_upgrade_jobs
		(job_uuid, device_id, firmware_id, rollout_batch_id, wave_number, task_id, task_status_id, from_version, to_version, scheduled_at, created_at, updated_at, created_by)
		VALUES (:job_uuid, :device_id, :firmware_id, :rollout_batch_id, :wave_number, :task_id, :task_status_id, :from_version, :to_version, :scheduled_at, :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, j)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	j.ID = uint64(id)
	return nil
}

func (r *firmwareUpgradeJobRepository) GetByID(ctx context.Context, id uint64) (*domain.FirmwareUpgradeJob, error) {
	var j domain.FirmwareUpgradeJob
	if err := r.db.GetContext(ctx, &j, `SELECT * FROM firmware_upgrade_jobs WHERE id = ?`, id); err != nil {
		return nil, translateErr(err)
	}
	return &j, nil
}

func (r *firmwareUpgradeJobRepository) GetByTaskID(ctx context.Context, taskID uint64) (*domain.FirmwareUpgradeJob, error) {
	var j domain.FirmwareUpgradeJob
	if err := r.db.GetContext(ctx, &j, `SELECT * FROM firmware_upgrade_jobs WHERE task_id = ?`, taskID); err != nil {
		return nil, translateErr(err)
	}
	return &j, nil
}

func (r *firmwareUpgradeJobRepository) ListByDevice(ctx context.Context, deviceID uint64, p domain.Pagination) ([]domain.FirmwareUpgradeJob, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM firmware_upgrade_jobs WHERE device_id = ?`, deviceID); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.FirmwareUpgradeJob
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM firmware_upgrade_jobs WHERE device_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		deviceID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

// ListByRolloutBatch — daftar job milik satu batch, opsional difilter ke
// satu wave (waveNumber nil = seluruh wave dalam batch tsb). Dipakai usecase
// orchestration rollout (BELUM diimplementasikan di sini) utk menampilkan
// progres per wave.
func (r *firmwareUpgradeJobRepository) ListByRolloutBatch(ctx context.Context, batchID uint64, waveNumber *uint32, p domain.Pagination) ([]domain.FirmwareUpgradeJob, int, error) {
	where := "rollout_batch_id = ?"
	args := []interface{}{batchID}
	if waveNumber != nil {
		where += " AND wave_number = ?"
		args = append(args, *waveNumber)
	}
	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM firmware_upgrade_jobs WHERE "+where, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.FirmwareUpgradeJob
	args = append(args, p.Limit(), p.Offset())
	err := r.db.SelectContext(ctx, &rows,
		"SELECT * FROM firmware_upgrade_jobs WHERE "+where+" ORDER BY id DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

// CountByRolloutWave — lihat komentar lengkap di domain/firmware.go soal
// bug kritis yang diperbaiki (JOIN ke status LIVE tasks.task_status_id,
// bukan salinan statis firmware_upgrade_jobs.task_status_id yang macet
// selamanya utk device yang timeout/offline sebelum TransferComplete).
// Job non-terminal (PENDING/QUEUED/SENT, dicocokkan via kode di ref_task_status
// -- bukan hardcode ID -- supaya tidak bergantung urutan seed) yang sudah
// lebih tua dari staleAfter diklasifikasikan efektif sbg timeoutStatusID.
func (r *firmwareUpgradeJobRepository) CountByRolloutWave(ctx context.Context, batchID uint64, waveNumber *uint32, staleAfter time.Time, timeoutStatusID uint64) ([]domain.FirmwareRolloutWaveStatusCount, error) {
	where := "j.rollout_batch_id = ?"
	args := []interface{}{}
	if waveNumber != nil {
		where += " AND j.wave_number = ?"
	}
	q := `SELECT
			CASE
				WHEN s.code IN ('PENDING', 'QUEUED', 'SENT') AND j.created_at < ? THEN ?
				ELSE t.task_status_id
			END AS task_status_id,
			COUNT(*) AS cnt
		FROM firmware_upgrade_jobs j
		JOIN tasks t ON t.id = j.task_id
		JOIN ref_task_status s ON s.id = t.task_status_id
		WHERE ` + where + `
		GROUP BY task_status_id`
	args = append(args, staleAfter, timeoutStatusID, batchID)
	if waveNumber != nil {
		args = append(args, *waveNumber)
	}
	var rows []domain.FirmwareRolloutWaveStatusCount
	if err := r.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *firmwareUpgradeJobRepository) UpdateStatus(ctx context.Context, id, taskStatusID uint64, errMsg *string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE firmware_upgrade_jobs SET task_status_id = ?, error_message = ? WHERE id = ?`, taskStatusID, errMsg, id)
	return translateErr(err)
}

func (r *firmwareUpgradeJobRepository) MarkStarted(ctx context.Context, id uint64, startedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE firmware_upgrade_jobs SET started_at = ? WHERE id = ?`, startedAt, id)
	return translateErr(err)
}

func (r *firmwareUpgradeJobRepository) MarkCompleted(ctx context.Context, id uint64, toVersion string, completedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE firmware_upgrade_jobs SET to_version = ?, completed_at = ? WHERE id = ?`, toVersion, completedAt, id)
	return translateErr(err)
}

// ---- FirmwareRolloutBatch ----
// (migrations/0011) — data layer utk canary/staged rollout firmware.
// TIDAK ada logic wave-advancement/failure-rate-gating di sini (usecase
// orchestration yang menyusul) -- murni CRUD + query agregat lewat
// FirmwareUpgradeJobRepository.CountByRolloutWave di atas.

type firmwareRolloutBatchRepository struct{ db *sqlx.DB }

func NewFirmwareRolloutBatchRepository(db *sqlx.DB) domain.FirmwareRolloutBatchRepository {
	return &firmwareRolloutBatchRepository{db: db}
}

func (r *firmwareRolloutBatchRepository) Create(ctx context.Context, b *domain.FirmwareRolloutBatch) error {
	now := time.Now()
	b.CreatedAt, b.UpdatedAt = now, now
	const q = `INSERT INTO firmware_rollout_batches
		(batch_uuid, tenant_id, firmware_file_id, vendor_id, device_model_id, wave_percentage,
		 max_failure_rate_percent, current_wave, status_id, notes, scheduled_at, started_at, completed_at,
		 created_at, updated_at, created_by)
		VALUES (:batch_uuid, :tenant_id, :firmware_file_id, :vendor_id, :device_model_id, :wave_percentage,
		 :max_failure_rate_percent, :current_wave, :status_id, :notes, :scheduled_at, :started_at, :completed_at,
		 :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, b)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	b.ID = uint64(id)
	return nil
}

func (r *firmwareRolloutBatchRepository) GetByID(ctx context.Context, id uint64) (*domain.FirmwareRolloutBatch, error) {
	var b domain.FirmwareRolloutBatch
	if err := r.db.GetContext(ctx, &b, `SELECT * FROM firmware_rollout_batches WHERE id = ? AND is_deleted = 0`, id); err != nil {
		return nil, translateErr(err)
	}
	return &b, nil
}

func (r *firmwareRolloutBatchRepository) GetByUUID(ctx context.Context, uuid string) (*domain.FirmwareRolloutBatch, error) {
	var b domain.FirmwareRolloutBatch
	err := r.db.GetContext(ctx, &b, `SELECT * FROM firmware_rollout_batches WHERE batch_uuid = ? AND is_deleted = 0`, uuid)
	if err != nil {
		return nil, translateErr(err)
	}
	return &b, nil
}

// List — tenantID nil = lintas seluruh tenant TERMASUK batch global
// (HANYA valid dipanggil dgn actor superadmin, lihat usecase.ListRolloutBatches).
// tenantID non-nil SENGAJA TIDAK ikut menampilkan batch global (tenant_id
// IS NULL) -- BEDA dari pola provisioning_profiles/zero_touch_rules yang
// memang mengizinkan tenant biasa membaca "template global" sbg desain
// (FR-17, precondition/aksi read-only). Firmware rollout batch adalah AKSI
// OPERASIONAL AKTIF (bisa memuat notes, dan pemicu push firmware nyata ke
// device) berbeda profil sensitivitas dari template baca-saja -- GetRolloutBatch
// sudah menolak non-superadmin membaca batch global satu-per-satu; List HARUS
// konsisten, bukan membocorkannya lewat endpoint listing (temuan
// acs-security-reviewer).
func (r *firmwareRolloutBatchRepository) List(ctx context.Context, tenantID *uint64, p domain.Pagination) ([]domain.FirmwareRolloutBatch, int, error) {
	where := "is_deleted = 0"
	args := []interface{}{}
	if tenantID != nil {
		where += " AND tenant_id = ?"
		args = append(args, *tenantID)
	}
	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM firmware_rollout_batches WHERE "+where, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.FirmwareRolloutBatch
	args = append(args, p.Limit(), p.Offset())
	err := r.db.SelectContext(ctx, &rows,
		"SELECT * FROM firmware_rollout_batches WHERE "+where+" ORDER BY id DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

// Update menyimpan seluruh field yang bisa berubah setelah dibuat (lihat
// komentar interface di domain/firmware.go) -- satu method generik dipakai
// baik utk edit operator (wave_percentage dsb) maupun bookkeeping progres
// (current_wave/status_id/started_at/completed_at) yang akan digerakkan
// usecase orchestration.
func (r *firmwareRolloutBatchRepository) Update(ctx context.Context, b *domain.FirmwareRolloutBatch) error {
	const q = `UPDATE firmware_rollout_batches SET
		wave_percentage = :wave_percentage, max_failure_rate_percent = :max_failure_rate_percent,
		current_wave = :current_wave, status_id = :status_id, notes = :notes, scheduled_at = :scheduled_at,
		started_at = :started_at, completed_at = :completed_at, updated_by = :updated_by
		WHERE id = :id AND is_deleted = 0`
	_, err := r.db.NamedExecContext(ctx, q, b)
	return translateErr(err)
}

func (r *firmwareRolloutBatchRepository) SoftDelete(ctx context.Context, id, deletedBy uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE firmware_rollout_batches SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now(), deletedBy, id)
	return translateErr(err)
}

// rolloutAdvanceLockName -- named lock MariaDB per-batch (bukan per-tabel),
// lihat komentar interface di domain/firmware.go.
func rolloutAdvanceLockName(batchID uint64) string {
	return fmt.Sprintf("acs_firmware_rollout_advance_%d", batchID)
}

// RunWithAdvanceLock -- lihat komentar lengkap di domain/firmware.go soal
// kenapa GET_LOCK/RELEASE_LOCK WAJIB dipegang satu koneksi fisik yang sama
// (bukan lewat *sqlx.DB pool biasa yang bisa lompat koneksi tiap panggilan).
// db.Connx mengambil SATU koneksi didedikasikan dari pool utk seluruh durasi
// fungsi ini, dikembalikan (Close, bukan menutup koneksi fisik -- cuma
// kembali ke pool) lewat defer di akhir.
func (r *firmwareRolloutBatchRepository) RunWithAdvanceLock(ctx context.Context, batchID uint64, fn func(ctx context.Context) error) (bool, error) {
	conn, err := r.db.Connx(ctx)
	if err != nil {
		return false, translateErr(err)
	}
	defer conn.Close()

	lockName := rolloutAdvanceLockName(batchID)
	var acquired sql.NullInt64
	// Timeout 0 = non-blocking: langsung kembali 0 kalau lock sedang dipegang
	// koneksi lain, bukan menunggu.
	if err := conn.GetContext(ctx, &acquired, "SELECT GET_LOCK(?, 0)", lockName); err != nil {
		return false, translateErr(err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return false, nil
	}
	defer func() {
		// context.Background() sengaja dipakai (bukan ctx pemanggil) supaya
		// lock TETAP dilepas walau ctx pemanggil sudah dibatalkan/timeout --
		// lock yang tidak dilepas mengunci batch ini sampai koneksi ini
		// ditutup total (semantik GET_LOCK per-sesi DB MariaDB).
		_, _ = conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", lockName)
	}()

	if err := fn(ctx); err != nil {
		return true, err
	}
	return true, nil
}
