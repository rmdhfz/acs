package file

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"acs/internal/domain"
)

type Service struct {
	files    domain.FileRepository
	storage  domain.ObjectStorage
	activity domain.ActivityLogRepository
	// quota — opsional (WithStorageQuota): batas total ukuran berkas per tenant. nil = tak dibatasi.
	quota domain.TenantStorageQuotaRepository
}

// WithStorageQuota mengaktifkan batas total ukuran berkas per tenant pada UploadFile.
func (s *Service) WithStorageQuota(q domain.TenantStorageQuotaRepository) *Service {
	s.quota = q
	return s
}

// requireStorageCapacity menolak unggahan sebesar size byte bila total berkas aktif tenant akan melampaui batasnya.
// Cek-lalu-tulis (tanpa kunci): unggahan serentak dapat melampaui batas sebesar beberapa berkas — kuota lunak, bukan
// batas keamanan keras. Gagal-tertutup bila batas tidak dapat dibaca.
func (s *Service) requireStorageCapacity(ctx context.Context, tenantID *uint64, size uint64) error {
	if s.quota == nil || tenantID == nil {
		return nil
	}
	max, used, err := s.quota.StorageUsage(ctx, *tenantID)
	if err != nil {
		return err
	}
	if max == nil {
		return nil
	}
	if used >= *max || size > *max-used {
		return fmt.Errorf("%w: kuota penyimpanan tenant tercapai (terpakai %d dari %d byte)", domain.ErrQuotaExceeded, used, *max)
	}
	return nil
}

func NewService(
	files domain.FileRepository,
	storage domain.ObjectStorage,
	activity domain.ActivityLogRepository,
) *Service {
	return &Service{files: files, storage: storage, activity: activity}
}

type UploadFileInput struct {
	TenantID      *uint64
	VendorID      *uint64
	DeviceModelID *uint64
	FileType      domain.FileType
	Version       *string
	FileName      string
	File          io.Reader
	FileSize      int64
	ContentType   string
}

func (s *Service) UploadFile(ctx context.Context, actor domain.Actor, in UploadFileInput) (*domain.File, error) {
	if in.File == nil {
		return nil, fmt.Errorf("%w: file wajib diisi", domain.ErrInvalidInput)
	}
	switch in.FileType {
	case domain.FileTypeFirmware, domain.FileTypeWebContent, domain.FileTypeVendorConfig,
		domain.FileTypeTone, domain.FileTypeRinger, domain.FileTypeVendorLog:
	default:
		return nil, fmt.Errorf("%w: file_type tidak dikenal (gunakan salah satu: 1 Firmware Upgrade Image, 2 Web Content, 3 Vendor Configuration File, 4 Tone File, 5 Ringer Melody File, Vendor Log File)", domain.ErrInvalidInput)
	}
	if !actor.IsSuperadmin() && actor.TenantID == nil {
		return nil, domain.ErrForbidden
	}
	if in.FileSize <= 0 || in.FileSize > domain.MaxFirmwareFileSizeBytes {
		return nil, fmt.Errorf("%w: ukuran file tidak valid", domain.ErrInvalidInput)
	}

	if in.TenantID != nil && !actor.IsSuperadmin() && (actor.TenantID == nil || *actor.TenantID != *in.TenantID) {
		return nil, domain.ErrForbidden
	}

	if err := s.requireStorageCapacity(ctx, in.TenantID, uint64(in.FileSize)); err != nil {
		return nil, err
	}

	objectKey := buildObjectKey(in.FileType, in.FileName)
	contentType := in.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	hasher := sha256.New()
	tee := io.TeeReader(in.File, hasher)
	if err := s.storage.Upload(ctx, objectKey, tee, in.FileSize, contentType); err != nil {
		return nil, fmt.Errorf("file: upload ke object storage gagal: %w", err)
	}
	// checksum := hex.EncodeToString(hasher.Sum(nil)) // Bisa disimpan jika ada field checksum

	size := uint64(in.FileSize)

	f := &domain.File{
		FileUUID:      uuid.NewString(),
		TenantID:      in.TenantID,
		VendorID:      in.VendorID,
		DeviceModelID: in.DeviceModelID,
		FileType:      in.FileType,
		Version:       in.Version,
		FileName:      in.FileName,
		StorageKey:    objectKey,
		FileSizeBytes: size,
		Audit:         domain.Audit{CreatedBy: actor.UserIDPtr()},
	}

	if err := s.files.Create(ctx, f); err != nil {
		_ = s.storage.Delete(ctx, objectKey)
		return nil, err
	}

	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "UPLOAD_FILE", EntityType: "file", EntityID: &f.ID,
	})
	return f, nil
}

func buildObjectKey(fileType domain.FileType, fileName string) string {
	base := filepath.Base(fileName)
	if base == "" || base == "." || base == ".." || base == string(filepath.Separator) {
		base = "file.bin"
	}
	base = sanitizeObjectKeyPart(base)
	if base == "" {
		base = "file.bin"
	}
	const maxFileNamePartLen = 100
	if len(base) > maxFileNamePartLen {
		base = base[:maxFileNamePartLen]
	}
	// Map type ke prefix folder
	folder := "generic"
	switch fileType {
	case domain.FileTypeFirmware:
		folder = "firmware"
	case domain.FileTypeVendorConfig:
		folder = "config"
	case domain.FileTypeVendorLog:
		folder = "logs"
	case domain.FileTypeWebContent:
		folder = "web"
	}
	return fmt.Sprintf("files/%s/%s_%s", folder, uuid.NewString(), base)
}

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

func (s *Service) List(ctx context.Context, actor domain.Actor, p domain.Pagination) ([]domain.File, int, error) {
	// Non-superadmin tanpa tenant tidak boleh melihat apa pun (tenantID nil =
	// "semua file" di repository).
	if !actor.IsSuperadmin() && actor.TenantID == nil {
		return nil, 0, domain.ErrForbidden
	}
	tenantID := actor.TenantID
	if actor.IsSuperadmin() {
		// Superadmin melihat file global (nil) juga, ini disederhanakan
		// Query list file repository mendukung (tenant_id = ? OR tenant_id IS NULL)
		// Tapi kalau query API mengirim tenant_id spesifik, harusnya pakai argumen.
		// Untuk sementara:
	}
	return s.files.List(ctx, tenantID, p)
}

func (s *Service) Delete(ctx context.Context, actor domain.Actor, id uint64) error {
	f, err := s.files.GetByID(ctx, id)
	if err != nil {
		return err
	}
	// File global (tenant_id NULL) hanya boleh dihapus SUPERADMIN; file tenant hanya
	// oleh tenant pemiliknya. (Sebelumnya file global lolos cek dan bisa dihapus
	// ADMIN tenant mana pun.)
	if !actor.IsSuperadmin() && (f.TenantID == nil || actor.TenantID == nil || *actor.TenantID != *f.TenantID) {
		return domain.ErrForbidden
	}

	if err := s.files.Delete(ctx, id); err != nil {
		return err
	}
	// Hapus juga objeknya: tanpa ini siklus unggah-hapus menumpuk objek yatim di bucket bersama sementara kuota (yang hanya
	// menghitung baris aktif) tetap kosong. Best-effort setelah soft delete — kegagalan hanya meninggalkan objek yatim.
	_ = s.storage.Delete(ctx, f.StorageKey)

	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "DELETE_FILE", EntityType: "file", EntityID: &id,
	})
	return nil
}
