package mysql

import (
	"context"
	"database/sql"
	"errors"
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

func NewVendorOUIRepository(db *sqlx.DB) domain.VendorOUIRepository {
	return &vendorOUIRepository{db: db}
}

func (r *vendorOUIRepository) Create(ctx context.Context, o *domain.VendorOUI) error {
	now := time.Now()
	o.CreatedAt, o.UpdatedAt = now, now

	// Unique key uq_vendor_ouis_oui TIDAK melihat is_deleted, jadi OUI yang
	// pernah dihapus (soft delete) selamanya menolak INSERT ulang. Sebelum
	// INSERT, hidupkan kembali baris terhapus dengan OUI yang sama (id lama
	// dipertahankan; vendor & notes mengikuti permintaan baru). Baris AKTIF
	// tidak tersentuh (is_deleted = 1 di WHERE) -> duplikat aktif tetap 409.
	const revive = `UPDATE vendor_ouis
		SET is_deleted = 0, deleted_at = NULL, deleted_by = NULL,
		    vendor_id = ?, notes = ?, created_by = ?, created_at = ?, updated_at = ?
		WHERE oui = ? AND is_deleted = 1`
	res, err := r.db.ExecContext(ctx, revive, o.VendorID, o.Notes, o.CreatedBy, now, now, o.OUI)
	if err != nil {
		return translateErr(err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		var id uint64
		if err := r.db.GetContext(ctx, &id, `SELECT id FROM vendor_ouis WHERE oui = ? AND is_deleted = 0`, o.OUI); err != nil {
			return translateErr(err)
		}
		o.ID = id
		return nil
	}

	const q = `INSERT INTO vendor_ouis (vendor_id, oui, notes, created_at, updated_at, created_by)
		VALUES (:vendor_id, :oui, :notes, :created_at, :updated_at, :created_by)`
	res, err = r.db.NamedExecContext(ctx, q, o)
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

func NewDeviceModelRepository(db *sqlx.DB) domain.DeviceModelRepository {
	return &deviceModelRepository{db: db}
}

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
		(vendor_id, data_model_version_id, device_model_id, software_version_pattern, logical_key, tr069_path, parameter_type_id, description, created_at, updated_at, created_by)
		VALUES (:vendor_id, :data_model_version_id, :device_model_id, :software_version_pattern, :logical_key, :tr069_path, :parameter_type_id, :description, :created_at, :updated_at, :created_by)`
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
	// ON DUPLICATE KEY berdasar uq_vpm_scope_key yang sekarang MENCAKUP
	// software_version_pattern (migrations/0010) -- dua baris dgn pattern
	// berbeda pada scope vendor/dmv/model/logical_key yang sama BUKAN
	// duplikat, dan sengaja tidak saling menimpa.
	const q = `INSERT INTO vendor_parameter_mappings
		(vendor_id, data_model_version_id, device_model_id, software_version_pattern, logical_key, tr069_path, parameter_type_id, description, created_at, updated_at, created_by)
		VALUES (:vendor_id, :data_model_version_id, :device_model_id, :software_version_pattern, :logical_key, :tr069_path, :parameter_type_id, :description, :created_at, :updated_at, :created_by)
		ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id), tr069_path = VALUES(tr069_path), parameter_type_id = VALUES(parameter_type_id),
			description = VALUES(description), updated_at = VALUES(updated_at), is_deleted = 0, deleted_at = NULL, deleted_by = NULL`
	// `id = LAST_INSERT_ID(id)` membuat LastInsertId() mengembalikan id baris
	// yang BENAR baik saat INSERT baru maupun saat meng-update baris yang sudah
	// ada — tanpanya respons POST /vendor-parameter-mappings membawa id 0 dan
	// klien tak bisa mereferensikan mapping (GET/DELETE by id).
	// UNIQUE KEY MariaDB tidak menganggap NULL sama: untuk scope generik
	// (device_model_id / software_version_pattern NULL — kasus paling umum) ON DUPLICATE
	// KEY TIDAK pernah memicu dan tiap POST membuat baris baru yang bersaing. Cari baris
	// yang sama secara null-safe (<=>) dan perbarui itu.
	var existingID uint64
	if gerr := r.db.GetContext(ctx, &existingID, `SELECT id FROM vendor_parameter_mappings
		WHERE vendor_id = ? AND data_model_version_id = ? AND device_model_id <=> ? AND logical_key = ?
		  AND software_version_pattern <=> ? ORDER BY is_deleted ASC, id DESC LIMIT 1`,
		m.VendorID, m.DataModelVersionID, m.DeviceModelID, m.LogicalKey, m.SoftwareVersionPattern); gerr == nil {
		if _, uerr := r.db.ExecContext(ctx, `UPDATE vendor_parameter_mappings SET tr069_path = ?, parameter_type_id = ?, description = ?,
			updated_at = ?, is_deleted = 0, deleted_at = NULL, deleted_by = NULL WHERE id = ?`,
			m.TR069Path, m.ParameterTypeID, m.Description, m.UpdatedAt, existingID); uerr != nil {
			return translateErr(uerr)
		}
		m.ID = existingID
		return nil
	} else if !errors.Is(gerr, sql.ErrNoRows) {
		return translateErr(gerr)
	}
	res, err := r.db.NamedExecContext(ctx, q, m)
	if err != nil {
		return translateErr(err)
	}
	if id, ierr := res.LastInsertId(); ierr == nil && id > 0 {
		m.ID = uint64(id)
	}
	return nil
}

// Resolve mencari mapping paling spesifik utk (vendor, data model version,
// device model, logical key), dgn urutan spesifisitas (paling spesifik
// menang, lihat komentar di domain.VendorParameterMappingRepository):
//  1. device_model_id spesifik + software_version_pattern COCOK (LIKE) dgn
//     softwareVersion -- kalau >1 baris cocok, pattern TERPANJANG menang
//     (heuristik "string lebih panjang = lebih spesifik").
//  2. device_model_id spesifik, TANPA software_version_pattern.
//  3. device_model_id NULL (generik lintas model) + pattern COCOK.
//  4. device_model_id NULL, TANPA software_version_pattern (paling generik --
//     satu-satunya perilaku Resolve() sebelum migrations/0010).
//
// Query LIKE dibalik sengaja (`? LIKE software_version_pattern`, bukan
// `software_version_pattern LIKE ?`) karena pola wildcard-nya ADA DI KOLOM
// (disimpan admin), sedangkan versi software device adalah literal yang
// dicocokkan terhadap pola tsb -- valid secara sintaks MariaDB (LIKE
// menerima ekspresi kolom di kedua sisi).
func (r *vendorParameterMappingRepository) Resolve(ctx context.Context, vendorID, dataModelVersionID uint64, deviceModelID *uint64, logicalKey string, softwareVersion *string) (*domain.VendorParameterMapping, error) {
	tryPattern := softwareVersion != nil && *softwareVersion != ""

	lookup := func(modelID *uint64, requirePattern bool) (*domain.VendorParameterMapping, error) {
		var m domain.VendorParameterMapping
		var err error
		if requirePattern {
			err = r.db.GetContext(ctx, &m,
				`SELECT * FROM vendor_parameter_mappings
				 WHERE vendor_id = ? AND data_model_version_id = ? AND device_model_id <=> ?
				   AND logical_key = ? AND software_version_pattern IS NOT NULL
				   AND ? LIKE software_version_pattern AND is_deleted = 0
				 ORDER BY LENGTH(software_version_pattern) DESC LIMIT 1`,
				vendorID, dataModelVersionID, modelID, logicalKey, *softwareVersion)
		} else {
			err = r.db.GetContext(ctx, &m,
				`SELECT * FROM vendor_parameter_mappings
				 WHERE vendor_id = ? AND data_model_version_id = ? AND device_model_id <=> ?
				   AND logical_key = ? AND software_version_pattern IS NULL AND is_deleted = 0
				 ORDER BY id DESC LIMIT 1`,
				vendorID, dataModelVersionID, modelID, logicalKey)
		}
		if err != nil {
			return nil, translateErr(err)
		}
		return &m, nil
	}

	if deviceModelID != nil {
		if tryPattern {
			if m, err := lookup(deviceModelID, true); err == nil {
				return m, nil
			} else if !errors.Is(err, domain.ErrNotFound) {
				return nil, err
			}
		}
		if m, err := lookup(deviceModelID, false); err == nil {
			return m, nil
		} else if !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
	}

	if tryPattern {
		if m, err := lookup(nil, true); err == nil {
			return m, nil
		} else if !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
	}
	return lookup(nil, false)
}

func (r *vendorParameterMappingRepository) GetByID(ctx context.Context, id uint64) (*domain.VendorParameterMapping, error) {
	var m domain.VendorParameterMapping
	if err := r.db.GetContext(ctx, &m, `SELECT * FROM vendor_parameter_mappings WHERE id = ? AND is_deleted = 0`, id); err != nil {
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
