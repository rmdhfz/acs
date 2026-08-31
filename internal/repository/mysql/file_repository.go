package mysql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"acs/internal/domain"

	"github.com/jmoiron/sqlx"
)

type fileRepository struct {
	db *sqlx.DB
}

func NewFileRepository(db *sqlx.DB) domain.FileRepository {
	return &fileRepository{db: db}
}

func (r *fileRepository) Create(ctx context.Context, f *domain.File) error {
	f.CreatedAt = time.Now().UTC()
	f.UpdatedAt = f.CreatedAt

	query := `INSERT INTO files (
		file_uuid, tenant_id, file_type, vendor_id, device_model_id, 
		version, file_name, storage_key, file_size_bytes, 
		created_at, created_by, updated_at, updated_by
	) VALUES (
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
	)`
	res, err := r.db.ExecContext(ctx, query,
		f.FileUUID, f.TenantID, f.FileType, f.VendorID, f.DeviceModelID,
		f.Version, f.FileName, f.StorageKey, f.FileSizeBytes,
		f.CreatedAt, f.CreatedBy, f.UpdatedAt, f.UpdatedBy,
	)
	if err != nil {
		return translateErr(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return translateErr(err)
	}
	f.ID = uint64(id)
	return nil
}

func (r *fileRepository) GetByID(ctx context.Context, id uint64) (*domain.File, error) {
	var f domain.File
	err := r.db.GetContext(ctx, &f, `SELECT * FROM files WHERE id = ? AND is_deleted = 0`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, translateErr(err)
	}
	return &f, nil
}

func (r *fileRepository) GetByUUID(ctx context.Context, uuid string) (*domain.File, error) {
	var f domain.File
	err := r.db.GetContext(ctx, &f, `SELECT * FROM files WHERE file_uuid = ? AND is_deleted = 0`, uuid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, translateErr(err)
	}
	return &f, nil
}

func (r *fileRepository) List(ctx context.Context, tenantID *uint64, p domain.Pagination) ([]domain.File, int, error) {
	var files []domain.File
	var count int

	baseQuery := ` FROM files WHERE is_deleted = 0`
	var args []any

	if tenantID != nil {
		baseQuery += ` AND (tenant_id = ? OR tenant_id IS NULL)` // Global & Tenant
		args = append(args, *tenantID)
	}

	countQuery := `SELECT COUNT(*)` + baseQuery
	err := r.db.GetContext(ctx, &count, countQuery, args...)
	if err != nil {
		return nil, 0, translateErr(err)
	}

	if count == 0 {
		return []domain.File{}, 0, nil
	}

	selectQuery := `SELECT *` + baseQuery + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, p.Limit(), p.Offset())
	err = r.db.SelectContext(ctx, &files, selectQuery, args...)
	if err != nil {
		return nil, 0, translateErr(err)
	}

	return files, count, nil
}

func (r *fileRepository) Delete(ctx context.Context, id uint64) error {
	query := `UPDATE files SET is_deleted = 1, deleted_at = ? WHERE id = ? AND is_deleted = 0`
	res, err := r.db.ExecContext(ctx, query, time.Now().UTC(), id)
	if err != nil {
		return translateErr(err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return translateErr(err)
	}
	if ra == 0 {
		return domain.ErrNotFound
	}
	return nil
}
