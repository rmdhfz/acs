package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	if in.FileType == "" {
		return nil, fmt.Errorf("%w: file_type wajib diisi", domain.ErrInvalidInput)
	}
	if in.FileSize <= 0 || in.FileSize > domain.MaxFirmwareFileSizeBytes {
		return nil, fmt.Errorf("%w: ukuran file tidak valid", domain.ErrInvalidInput)
	}
	
	if in.TenantID != nil && !actor.IsSuperadmin() && (*actor.TenantID != *in.TenantID) {
		return nil, domain.ErrForbidden
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
	if f.TenantID != nil && !actor.IsSuperadmin() && (*actor.TenantID != *f.TenantID) {
		return domain.ErrForbidden
	}
	
	if err := s.files.Delete(ctx, id); err != nil {
		return err
	}

	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "DELETE_FILE", EntityType: "file", EntityID: &id,
	})
	return nil
}
