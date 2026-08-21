package mysql

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// ---- Vendor ----

type vendorRepository struct{ db *sqlx.DB }

func NewVendorRepository(db *sqlx.DB) domain.VendorRepository { return &vendorRepository{db: db} }

func (r *vendorRepository) Create(ctx context.Context, v *domain.Vendor) error {
	now := time.Now()
	v.CreatedAt, v.UpdatedAt = now, now
	const q = `INSERT INTO ref_vendors (code, name, description, is_active, created_at, updated_at, created_by)
		VALUES (:code, :name, :description, :is_active, :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, v)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	v.ID = uint64(id)
	return nil
}

func (r *vendorRepository) GetByID(ctx context.Context, id uint64) (*domain.Vendor, error) {
	var v domain.Vendor
	if err := r.db.GetContext(ctx, &v, `SELECT * FROM ref_vendors WHERE id = ? AND is_deleted = 0`, id); err != nil {
		return nil, translateErr(err)
	}
	return &v, nil
}

func (r *vendorRepository) GetByCode(ctx context.Context, code string) (*domain.Vendor, error) {
	var v domain.Vendor
	if err := r.db.GetContext(ctx, &v, `SELECT * FROM ref_vendors WHERE code = ? AND is_deleted = 0`, code); err != nil {
		return nil, translateErr(err)
	}
	return &v, nil
}

func (r *vendorRepository) List(ctx context.Context, p domain.Pagination) ([]domain.Vendor, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM ref_vendors WHERE is_deleted = 0`); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.Vendor
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM ref_vendors WHERE is_deleted = 0 ORDER BY id LIMIT ? OFFSET ?`, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *vendorRepository) Update(ctx context.Context, v *domain.Vendor) error {
	const q = `UPDATE ref_vendors SET name = :name, description = :description, is_active = :is_active,
		updated_by = :updated_by WHERE id = :id AND is_deleted = 0`
	_, err := r.db.NamedExecContext(ctx, q, v)
	return translateErr(err)
}

func (r *vendorRepository) SoftDelete(ctx context.Context, id, deletedBy uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE ref_vendors SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now(), deletedBy, id)
	return translateErr(err)
}

// ---- VendorOUI ----

type vendorOUIRepository struct{ db *sqlx.DB }

func NewVendorOUIRepository(db *sqlx.DB) domain.VendorOUIRepository { return &vendorOUIRepository{db: db} }

func (r *vendorOUIRepository) Create(ctx context.Context, o *domain.VendorOUI) error {
	now := time.Now()
	o.CreatedAt, o.UpdatedAt = now, now
	const q = `INSERT INTO vendor_ouis (vendor_id, oui, notes, created_at, updated_at, created_by)
		VALUES (:vendor_id, :oui, :notes, :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, o)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	o.ID = uint64(id)
	return nil
}

func (r *vendorOUIRepository) GetByOUI(ctx context.Context, oui string) (*domain.VendorOUI, error) {
	var o domain.VendorOUI
	if err := r.db.GetContext(ctx, &o, `SELECT * FROM vendor_ouis WHERE oui = ? AND is_deleted = 0`, oui); err != nil {
		return nil, translateErr(err)
	}
	return &o, nil
}

func (r *vendorOUIRepository) ListByVendor(ctx context.Context, vendorID uint64) ([]domain.VendorOUI, error) {
	var rows []domain.VendorOUI
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM vendor_ouis WHERE vendor_id = ? AND is_deleted = 0 ORDER BY id`, vendorID)
	if err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *vendorOUIRepository) Delete(ctx context.Context, id, deletedBy uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE vendor_ouis SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now(), deletedBy, id)
	return translateErr(err)
}

// ---- DeviceModel ----

type deviceModelRepository struct{ db *sqlx.DB }

func NewDeviceModelRepository(db *sqlx.DB) domain.DeviceModelRepository { return &deviceModelRepository{db: db} }

func (r *deviceModelRepository) Create(ctx context.Context, m *domain.DeviceModel) error {
	now := time.Now()
	m.CreatedAt, m.UpdatedAt = now, now
	const q = `INSERT INTO device_models
		(vendor_id, device_type_id, data_model_version_id, product_class, model_name, description, is_active, created_at, updated_at, created_by)
		VALUES (:vendor_id, :device_type_id, :data_model_version_id, :product_class, :model_name, :description, :is_active, :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, m)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	m.ID = uint64(id)
	return nil
}

func (r *deviceModelRepository) GetByID(ctx context.Context, id uint64) (*domain.DeviceModel, error) {
	var m domain.DeviceModel
	if err := r.db.GetContext(ctx, &m, `SELECT * FROM device_models WHERE id = ? AND is_deleted = 0`, id); err != nil {
		return nil, translateErr(err)
	}
	return &m, nil
}

func (r *deviceModelRepository) FindByVendorAndProductClass(ctx context.Context, vendorID uint64, productClass string) (*domain.DeviceModel, error) {
	var m domain.DeviceModel
	err := r.db.GetContext(ctx, &m,
		`SELECT * FROM device_models WHERE vendor_id = ? AND product_class = ? AND is_deleted = 0 LIMIT 1`,
		vendorID, productClass)
	if err != nil {
		return nil, translateErr(err)
	}
	return &m, nil
}

func (r *deviceModelRepository) ListByVendor(ctx context.Context, vendorID uint64) ([]domain.DeviceModel, error) {
	var rows []domain.DeviceModel
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM device_models WHERE vendor_id = ? AND is_deleted = 0 ORDER BY id`, vendorID)
	if err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *deviceModelRepository) Update(ctx context.Context, m *domain.DeviceModel) error {
	const q = `UPDATE device_models SET device_type_id = :device_type_id, data_model_version_id = :data_model_version_id,
		product_class = :product_class, model_name = :model_name, description = :description, is_active = :is_active,
		updated_by = :updated_by WHERE id = :id AND is_deleted = 0`
	_, err := r.db.NamedExecContext(ctx, q, m)
	return translateErr(err)
}

func (r *deviceModelRepository) SoftDelete(ctx context.Context, id, deletedBy uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE device_models SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now(), deletedBy, id)
	return translateErr(err)
}

// ---- VendorParameterMapping ----

type vendorParameterMappingRepository struct{ db *sqlx.DB }

func NewVendorParameterMappingRepository(db *sqlx.DB) domain.VendorParameterMappingRepository {
	return &vendorParameterMappingRepository{db: db}
}

func (r *vendorParameterMappingRepository) Create(ctx context.Context, m *domain.VendorParameterMapping) error {
	now := time.Now()
	m.CreatedAt, m.UpdatedAt = now, now
	const q = `INSERT INTO vendor_parameter_mappings
		(vendor_id, data_model_version_id, device_model_id, logical_key, tr069_path, parameter_type_id, description, created_at, updated_at, created_by)
		VALUES (:vendor_id, :data_model_version_id, :device_model_id, :logical_key, :tr069_path, :parameter_type_id, :description, :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, m)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	m.ID = uint64(id)
	return nil
}

func (r *vendorParameterMappingRepository) Upsert(ctx context.Context, m *domain.VendorParameterMapping) error {
	now := time.Now()
	m.CreatedAt, m.UpdatedAt = now, now
	const q = `INSERT INTO vendor_parameter_mappings
		(vendor_id, data_model_version_id, device_model_id, logical_key, tr069_path, parameter_type_id, description, created_at, updated_at, created_by)
		VALUES (:vendor_id, :data_model_version_id, :device_model_id, :logical_key, :tr069_path, :parameter_type_id, :description, :created_at, :updated_at, :created_by)
		ON DUPLICATE KEY UPDATE tr069_path = VALUES(tr069_path), parameter_type_id = VALUES(parameter_type_id),
			description = VALUES(description), updated_at = VALUES(updated_at), is_deleted = 0, deleted_at = NULL, deleted_by = NULL`
	_, err := r.db.NamedExecContext(ctx, q, m)
	return translateErr(err)
}

// Resolve mencari mapping paling spesifik: device_model_id dulu, fallback vendor+dmv saja (TECH.md §5).
func (r *vendorParameterMappingRepository) Resolve(ctx context.Context, vendorID, dataModelVersionID uint64, deviceModelID *uint64, logicalKey string) (*domain.VendorParameterMapping, error) {
	if deviceModelID != nil {
		var m domain.VendorParameterMapping
		err := r.db.GetContext(ctx, &m,
			`SELECT * FROM vendor_parameter_mappings
			 WHERE vendor_id = ? AND data_model_version_id = ? AND device_model_id = ? AND logical_key = ? AND is_deleted = 0`,
			vendorID, dataModelVersionID, *deviceModelID, logicalKey)
		if err == nil {
			return &m, nil
		}
		if translateErr(err) != domain.ErrNotFound {
			return nil, translateErr(err)
		}
	}
	var m domain.VendorParameterMapping
	err := r.db.GetContext(ctx, &m,
		`SELECT * FROM vendor_parameter_mappings
		 WHERE vendor_id = ? AND data_model_version_id = ? AND device_model_id IS NULL AND logical_key = ? AND is_deleted = 0`,
		vendorID, dataModelVersionID, logicalKey)
	if err != nil {
		return nil, translateErr(err)
	}
	return &m, nil
}

func (r *vendorParameterMappingRepository) ListByVendor(ctx context.Context, vendorID uint64) ([]domain.VendorParameterMapping, error) {
	var rows []domain.VendorParameterMapping
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM vendor_parameter_mappings WHERE vendor_id = ? AND is_deleted = 0 ORDER BY id`, vendorID)
	if err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *vendorParameterMappingRepository) Delete(ctx context.Context, id, deletedBy uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE vendor_parameter_mappings SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now(), deletedBy, id)
	return translateErr(err)
}
