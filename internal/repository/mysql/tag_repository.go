package mysql

import (
	"context"

	"acs/internal/domain"

	"github.com/jmoiron/sqlx"
)

type tagRepository struct {
	db *sqlx.DB
}

func NewTagRepository(db *sqlx.DB) domain.TagRepository {
	return &tagRepository{db: db}
}

func (r *tagRepository) Create(ctx context.Context, tag *domain.Tag) error {
	query := `INSERT INTO tags (tenant_id, name, color) VALUES (:tenant_id, :name, :color)`
	res, err := r.db.NamedExecContext(ctx, query, tag)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	tag.ID = uint64(id)
	return nil
}

func (r *tagRepository) GetByID(ctx context.Context, id uint64) (*domain.Tag, error) {
	var tag domain.Tag
	if err := r.db.GetContext(ctx, &tag, `SELECT * FROM tags WHERE id = ?`, id); err != nil {
		return nil, translateErr(err)
	}
	return &tag, nil
}

func (r *tagRepository) List(ctx context.Context, tenantID *uint64, p domain.Pagination) ([]domain.Tag, int, error) {
	var tags []domain.Tag
	var total int

	queryTotal := `SELECT COUNT(*) FROM tags WHERE tenant_id = ? OR tenant_id IS NULL`
	if err := r.db.GetContext(ctx, &total, queryTotal, tenantID); err != nil {
		return nil, 0, translateErr(err)
	}

	query := `SELECT * FROM tags WHERE tenant_id = ? OR tenant_id IS NULL ORDER BY id DESC LIMIT ? OFFSET ?`
	if err := r.db.SelectContext(ctx, &tags, query, tenantID, p.Limit(), p.Offset()); err != nil {
		return nil, 0, translateErr(err)
	}

	return tags, total, nil
}

func (r *tagRepository) Delete(ctx context.Context, id uint64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
	return translateErr(err)
}

func (r *tagRepository) AssignToDevice(ctx context.Context, deviceID, tagID uint64) error {
	_, err := r.db.ExecContext(ctx, `INSERT IGNORE INTO device_tags (device_id, tag_id) VALUES (?, ?)`, deviceID, tagID)
	return translateErr(err)
}

func (r *tagRepository) RemoveFromDevice(ctx context.Context, deviceID, tagID uint64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM device_tags WHERE device_id = ? AND tag_id = ?`, deviceID, tagID)
	return translateErr(err)
}

func (r *tagRepository) ListByDevice(ctx context.Context, deviceID uint64) ([]domain.Tag, error) {
	var tags []domain.Tag
	query := `
		SELECT t.* FROM tags t
		JOIN device_tags dt ON dt.tag_id = t.id
		WHERE dt.device_id = ?
	`
	if err := r.db.SelectContext(ctx, &tags, query, deviceID); err != nil {
		return nil, translateErr(err)
	}
	return tags, nil
}
