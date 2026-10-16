package mysql

import (
	"context"

	"acs/internal/domain"

	"github.com/jmoiron/sqlx"
)

type presetRepository struct {
	db     *sqlx.DB
	sealer ValueSealer // nil = tanpa enkripsi at-rest (lihat sealed_values.go)
}

func NewPresetRepository(db *sqlx.DB, sealer ValueSealer) domain.PresetRepository {
	return &presetRepository{db: db, sealer: sealer}
}

// openAll membuka nilai rahasia pada hasil query (fail closed).
func (r *presetRepository) openAll(presets []domain.Preset) error {
	for i := range presets {
		out, err := openPresetConfigs(r.sealer, presets[i].Configurations)
		if err != nil {
			return err
		}
		presets[i].Configurations = out
	}
	return nil
}

func (r *presetRepository) Create(ctx context.Context, preset *domain.Preset) error {
	sealed, err := sealPresetConfigs(r.sealer, preset.Configurations)
	if err != nil {
		return err
	}
	row := *preset // salinan: struct pemanggil tetap berisi plaintext
	row.Configurations = sealed
	query := `INSERT INTO presets (tenant_id, name, weight, precondition, configurations, is_active, enforce, channel)
			  VALUES (:tenant_id, :name, :weight, :precondition, :configurations, :is_active, :enforce, :channel)`
	res, err := r.db.NamedExecContext(ctx, query, &row)
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
	opened := []domain.Preset{preset}
	if err := r.openAll(opened); err != nil {
		return nil, err
	}
	return &opened[0], nil
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
	if err := r.openAll(presets); err != nil {
		return nil, 0, err
	}

	return presets, total, nil
}

func (r *presetRepository) Update(ctx context.Context, preset *domain.Preset) error {
	sealed, err := sealPresetConfigs(r.sealer, preset.Configurations)
	if err != nil {
		return err
	}
	row := *preset
	row.Configurations = sealed
	query := `UPDATE presets SET
				name = :name,
				weight = :weight,
				precondition = :precondition,
				configurations = :configurations,
				is_active = :is_active,
				enforce = :enforce,
				channel = :channel
			  WHERE id = :id`
	_, err = r.db.NamedExecContext(ctx, query, &row)
	return translateErr(err)
}

func (r *presetRepository) Delete(ctx context.Context, id uint64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM presets WHERE id = ?`, id)
	return translateErr(err)
}

// ListActiveEnforce — preset aktif & enforce untuk tenant device, terurut
// weight ASC (preset weight lebih besar dievaluasi belakangan -> menimpa saat
// key bentrok, PRESET_ENGINE_DESIGN.md §4). tenantID nil -> hanya preset global.
func (r *presetRepository) ListActiveEnforce(ctx context.Context, tenantID *uint64) ([]domain.Preset, error) {
	var presets []domain.Preset
	query := `SELECT * FROM presets
			  WHERE is_active = 1 AND enforce = 1 AND (tenant_id = ? OR tenant_id IS NULL)
			  ORDER BY weight ASC, id ASC`
	if err := r.db.SelectContext(ctx, &presets, query, tenantID); err != nil {
		return nil, translateErr(err)
	}
	if err := r.openAll(presets); err != nil {
		return nil, err
	}
	return presets, nil
}
