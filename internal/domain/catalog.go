package domain

import "context"

// Vendor, DeviceModel, VendorOUI, VendorParameterMapping — katalog vendor CPE.
// Menambah vendor/model baru adalah operasi data, bukan kode (lihat CLAUDE.md).

type Vendor struct {
	ID          uint64  `db:"id" json:"id"`
	Code        string  `db:"code" json:"code"`
	Name        string  `db:"name" json:"name"`
	Description *string `db:"description" json:"description"`
	IsActive    bool    `db:"is_active" json:"is_active"`
	Audit
}

type VendorRepository interface {
	Create(ctx context.Context, v *Vendor) error
	GetByID(ctx context.Context, id uint64) (*Vendor, error)
	GetByCode(ctx context.Context, code string) (*Vendor, error)
	List(ctx context.Context, p Pagination) ([]Vendor, int, error)
	Update(ctx context.Context, v *Vendor) error
	SoftDelete(ctx context.Context, id, deletedBy uint64) error
}

type VendorOUI struct {
	ID       uint64  `db:"id" json:"id"`
	VendorID uint64  `db:"vendor_id" json:"vendor_id"`
	OUI      string  `db:"oui" json:"oui"`
	Notes    *string `db:"notes" json:"notes"`
	Audit
}

type VendorOUIRepository interface {
	Create(ctx context.Context, o *VendorOUI) error
	GetByOUI(ctx context.Context, oui string) (*VendorOUI, error)
	ListByVendor(ctx context.Context, vendorID uint64) ([]VendorOUI, error)
	Delete(ctx context.Context, id, deletedBy uint64) error
}

type DeviceModel struct {
	ID                  uint64  `db:"id" json:"id"`
	VendorID             uint64  `db:"vendor_id" json:"vendor_id"`
	DeviceTypeID          uint64  `db:"device_type_id" json:"device_type_id"`
	DataModelVersionID     uint64  `db:"data_model_version_id" json:"data_model_version_id"`
	ProductClass            *string `db:"product_class" json:"product_class"`
	ModelName                string  `db:"model_name" json:"model_name"`
	Description               *string `db:"description" json:"description"`
	IsActive                   bool    `db:"is_active" json:"is_active"`
	Audit
}

type DeviceModelRepository interface {
	Create(ctx context.Context, m *DeviceModel) error
	GetByID(ctx context.Context, id uint64) (*DeviceModel, error)
	FindByVendorAndProductClass(ctx context.Context, vendorID uint64, productClass string) (*DeviceModel, error)
	ListByVendor(ctx context.Context, vendorID uint64) ([]DeviceModel, error)
	Update(ctx context.Context, m *DeviceModel) error
	SoftDelete(ctx context.Context, id, deletedBy uint64) error
}

// VendorParameterMapping memetakan logical key -> path TR-069 aktual,
// per vendor + data model version, opsional override per device model.
// Lihat TECH.md §5.
type VendorParameterMapping struct {
	ID                  uint64  `db:"id" json:"id"`
	VendorID             uint64  `db:"vendor_id" json:"vendor_id"`
	DataModelVersionID    uint64  `db:"data_model_version_id" json:"data_model_version_id"`
	DeviceModelID          *uint64 `db:"device_model_id" json:"device_model_id"`
	LogicalKey              string  `db:"logical_key" json:"logical_key"`
	TR069Path                string  `db:"tr069_path" json:"tr069_path"`
	ParameterTypeID           *uint64 `db:"parameter_type_id" json:"parameter_type_id"`
	Description                *string `db:"description" json:"description"`
	Audit
}

type VendorParameterMappingRepository interface {
	Create(ctx context.Context, m *VendorParameterMapping) error
	Upsert(ctx context.Context, m *VendorParameterMapping) error
	// Resolve mencari mapping paling spesifik: device_model_id dulu, fallback vendor+dmv saja.
	Resolve(ctx context.Context, vendorID, dataModelVersionID uint64, deviceModelID *uint64, logicalKey string) (*VendorParameterMapping, error)
	ListByVendor(ctx context.Context, vendorID uint64) ([]VendorParameterMapping, error)
	Delete(ctx context.Context, id, deletedBy uint64) error
}
