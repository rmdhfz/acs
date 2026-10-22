package mysql

import (
	"context"
	"time"

	"acs/internal/domain"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type virtualParameterRepository struct{ db *sqlx.DB }

func NewVirtualParameterRepository(db *sqlx.DB) domain.VirtualParameterRepository {
	return &virtualParameterRepository{db: db}
}

const vpCols = `id, vp_uuid, tenant_id, name, label, description, expression, unit, is_active, created_at, updated_at`

func (r *virtualParameterRepository) Create(ctx context.Context, v *domain.VirtualParameter, by *uint64) error {
	v.UUID = uuid.NewString()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO virtual_parameters (vp_uuid, tenant_id, name, label, description, expression, unit, is_active, created_by, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.UUID, v.TenantID, v.Name, v.Label, v.Description, v.Expression, v.Unit, v.IsActive, by, by)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	v.ID = uint64(id)
	return r.db.GetContext(ctx, v, `SELECT `+vpCols+` FROM virtual_parameters WHERE id = ?`, v.ID)
}

func (r *virtualParameterRepository) GetByUUID(ctx context.Context, uuid string) (*domain.VirtualParameter, error) {
	var v domain.VirtualParameter
	if err := r.db.GetContext(ctx, &v, `SELECT `+vpCols+` FROM virtual_parameters WHERE vp_uuid = ? AND is_deleted = 0`, uuid); err != nil {
		return nil, translateErr(err)
	}
	return &v, nil
}

func (r *virtualParameterRepository) List(ctx context.Context, tenantID *uint64, activeOnly bool, p domain.Pagination) ([]domain.VirtualParameter, int, error) {
	where := "is_deleted = 0"
	args := []interface{}{}
	if tenantID != nil {
		where += " AND (tenant_id = ? OR tenant_id IS NULL)"
		args = append(args, *tenantID)
	}
	if activeOnly {
		where += " AND is_active = 1"
	}
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM virtual_parameters WHERE `+where, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	rows := []domain.VirtualParameter{}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT `+vpCols+` FROM virtual_parameters WHERE `+where+` ORDER BY name ASC, id ASC LIMIT ? OFFSET ?`,
		append(args, p.Limit(), p.Offset())...); err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *virtualParameterRepository) ListForTenant(ctx context.Context, tenantID *uint64) ([]domain.VirtualParameter, error) {
	rows := []domain.VirtualParameter{}
	var err error
	if tenantID == nil {
		err = r.db.SelectContext(ctx, &rows,
			`SELECT `+vpCols+` FROM virtual_parameters WHERE is_deleted = 0 AND is_active = 1 AND tenant_id IS NULL ORDER BY name`)
	} else {
		err = r.db.SelectContext(ctx, &rows,
			`SELECT `+vpCols+` FROM virtual_parameters WHERE is_deleted = 0 AND is_active = 1 AND (tenant_id = ? OR tenant_id IS NULL)
			 ORDER BY name, tenant_id IS NULL, id`, *tenantID) // definisi tenant (tenant_id tidak NULL) lebih dulu daripada global
	}
	if err != nil {
		return nil, translateErr(err)
	}
	// Definisi tenant menimpa global bernama sama: pertahankan kemunculan pertama tiap nama.
	out := rows[:0]
	seen := map[string]bool{}
	for _, v := range rows {
		if !seen[v.Name] {
			seen[v.Name] = true
			out = append(out, v)
		}
	}
	return out, nil
}

func (r *virtualParameterRepository) FindByName(ctx context.Context, tenantID *uint64, name string) (*domain.VirtualParameter, error) {
	var v domain.VirtualParameter
	err := r.db.GetContext(ctx, &v,
		`SELECT `+vpCols+` FROM virtual_parameters WHERE is_deleted = 0 AND name = ? AND tenant_id <=> ? ORDER BY id LIMIT 1`, name, tenantID)
	if err != nil {
		return nil, translateErr(err)
	}
	return &v, nil
}

func (r *virtualParameterRepository) CountInScope(ctx context.Context, tenantID *uint64) (int, error) {
	var n int
	if err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM virtual_parameters WHERE is_deleted = 0 AND tenant_id <=> ?`, tenantID); err != nil {
		return 0, translateErr(err)
	}
	return n, nil
}

func (r *virtualParameterRepository) Update(ctx context.Context, v *domain.VirtualParameter, by *uint64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE virtual_parameters SET name = ?, label = ?, description = ?, expression = ?, unit = ?, is_active = ?, updated_by = ?
		 WHERE id = ? AND is_deleted = 0`,
		v.Name, v.Label, v.Description, v.Expression, v.Unit, v.IsActive, by, v.ID)
	if err != nil {
		return translateErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// RowsAffected = 0 juga bila tak ada yang berubah; bedakan dari "tidak ada" dengan membaca ulang.
		if _, gerr := r.GetByUUID(ctx, v.UUID); gerr != nil {
			return gerr
		}
	}
	return nil
}

func (r *virtualParameterRepository) SoftDelete(ctx context.Context, id, by uint64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE virtual_parameters SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ? AND is_deleted = 0`, time.Now(), by, id)
	if err != nil {
		return translateErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
