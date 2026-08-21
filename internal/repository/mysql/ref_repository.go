package mysql

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// refTableWhitelist mencegah nama tabel ref_* diteruskan mentah ke SQL —
// satu-satunya bagian dari query yang dibangun dari input non-literal.
var refTableWhitelist = map[string]bool{
	domain.RefTableVendors:           true,
	domain.RefTableDeviceTypes:       true,
	domain.RefTableDataModelVersions: true,
	domain.RefTableEventCodes:        true,
	domain.RefTableTaskTypes:         true,
	domain.RefTableTaskStatus:        true,
	domain.RefTableDeviceStatus:      true,
	domain.RefTableParameterTypes:    true,
	domain.RefTableRoles:             true,
}

type refRepository struct {
	db *sqlx.DB
}

func NewRefRepository(db *sqlx.DB) domain.RefRepository {
	return &refRepository{db: db}
}

func (r *refRepository) validate(table string) error {
	if !refTableWhitelist[table] {
		return fmt.Errorf("mysql: tabel ref tidak dikenal: %s", table)
	}
	return nil
}

// nameColumn: ref_parameter_types tidak punya kolom name (hanya code).
func nameColumn(table string) string {
	if table == domain.RefTableParameterTypes {
		return "code AS name"
	}
	return "name"
}

func (r *refRepository) GetByCode(ctx context.Context, table, code string) (domain.RefLookup, error) {
	if err := r.validate(table); err != nil {
		return domain.RefLookup{}, err
	}
	query := fmt.Sprintf("SELECT id, code, %s AS name FROM %s WHERE code = ? AND is_deleted = 0", nameColumn(table), table)
	var row struct {
		ID   uint64 `db:"id"`
		Code string `db:"code"`
		Name string `db:"name"`
	}
	if err := r.db.GetContext(ctx, &row, query, code); err != nil {
		return domain.RefLookup{}, translateErr(err)
	}
	return domain.RefLookup{ID: row.ID, Code: row.Code, Name: row.Name}, nil
}

func (r *refRepository) GetByID(ctx context.Context, table string, id uint64) (domain.RefLookup, error) {
	if err := r.validate(table); err != nil {
		return domain.RefLookup{}, err
	}
	query := fmt.Sprintf("SELECT id, code, %s AS name FROM %s WHERE id = ? AND is_deleted = 0", nameColumn(table), table)
	var row struct {
		ID   uint64 `db:"id"`
		Code string `db:"code"`
		Name string `db:"name"`
	}
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		return domain.RefLookup{}, translateErr(err)
	}
	return domain.RefLookup{ID: row.ID, Code: row.Code, Name: row.Name}, nil
}

func (r *refRepository) List(ctx context.Context, table string) ([]domain.RefLookup, error) {
	if err := r.validate(table); err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT id, code, %s AS name FROM %s WHERE is_deleted = 0 ORDER BY id", nameColumn(table), table)
	var rows []struct {
		ID   uint64 `db:"id"`
		Code string `db:"code"`
		Name string `db:"name"`
	}
	if err := r.db.SelectContext(ctx, &rows, query); err != nil {
		return nil, translateErr(err)
	}
	out := make([]domain.RefLookup, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.RefLookup{ID: row.ID, Code: row.Code, Name: row.Name})
	}
	return out, nil
}
