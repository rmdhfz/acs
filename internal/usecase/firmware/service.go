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
	devices  domain.DeviceRepository
	tasks    domain.TaskCreator
	refs     domain.RefRepository
	activity domain.ActivityLogRepository
	storage  domain.ObjectStorage
}

func NewService(
	files domain.FirmwareFileRepository,
	jobs domain.FirmwareUpgradeJobRepository,
	devices domain.DeviceRepository,
	tasks domain.TaskCreator,
	refs domain.RefRepository,
	activity domain.ActivityLogRepository,
	storage domain.ObjectStorage,
) *Service {
	return &Service{files: files, jobs: jobs, devices: devices, tasks: tasks, refs: refs, activity: activity, storage: storage}
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
// standar FileType=1 Firmware Upgrade Image, lihat TECH.md §7).
func (s *Service) ScheduleUpgrade(ctx context.Context, actor domain.Actor, deviceID, firmwareID uint64, scheduledAt *time.Time) (*domain.FirmwareUpgradeJob, error) {
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
		JobUUID:      uuid.NewString(),
		DeviceID:     deviceID,
		FirmwareID:   firmwareID,
		TaskID:       &t.ID,
		TaskStatusID: pendingStatus.ID,
		FromVersion:  dev.SoftwareVersion,
		ToVersion:    &fw.Version,
		ScheduledAt:  scheduledAt,
		CreatedBy:    actor.UserIDPtr(),
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
