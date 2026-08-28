package mysql

import (
	"context"

	"acs/internal/domain"

	"github.com/jmoiron/sqlx"
)

type presetRepository struct {
	db *sqlx.DB
}

func NewPresetRepository(db *sqlx.DB) domain.PresetRepository {
	return &presetRepository{db: db}
}

func (r *presetRepository) Create(ctx context.Context, preset *domain.Preset) error {
	query := `INSERT INTO presets (tenant_id, name, weight, precondition, configurations, is_active) 
			  VALUES (:tenant_id, :name, :weight, :precondition, :configurations, :is_active)`
	res, err := r.db.NamedExecContext(ctx, query, preset)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	preset.ID = uint64(id)
	return nil
}

func (r *presetRepository) GetByID(ctx context.Context, id uint64) (*domain.Preset, error) {
	var preset domain.Preset
	if err := r.db.GetContext(ctx, &preset, `SELECT * FROM presets WHERE id = ?`, id); err != nil {
		return nil, translateErr(err)
	}
	return &preset, nil
}

func (r *presetRepository) List(ctx context.Context, tenantID *uint64, p domain.Pagination) ([]domain.Preset, int, error) {
	var presets []domain.Preset
	var total int

	queryTotal := `SELECT COUNT(*) FROM presets WHERE tenant_id = ? OR tenant_id IS NULL`
	if err := r.db.GetContext(ctx, &total, queryTotal, tenantID); err != nil {
		return nil, 0, translateErr(err)
	}

	query := `SELECT * FROM presets WHERE tenant_id = ? OR tenant_id IS NULL ORDER BY weight DESC, id DESC LIMIT ? OFFSET ?`
	if err := r.db.SelectContext(ctx, &presets, query, tenantID, p.Limit(), p.Offset()); err != nil {
		return nil, 0, translateErr(err)
	}

	return presets, total, nil
}

func (r *presetRepository) Update(ctx context.Context, preset *domain.Preset) error {
	query := `UPDATE presets SET 
				name = :name, 
				weight = :weight, 
				precondition = :precondition, 
				configurations = :configurations, 
				is_active = :is_active 
			  WHERE id = :id`
	_, err := r.db.NamedExecContext(ctx, query, preset)
	return translateErr(err)
}

func (r *presetRepository) Delete(ctx context.Context, id uint64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM presets WHERE id = ?`, id)
	return translateErr(err)
}
