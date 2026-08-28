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
	ID                 uint64  `db:"id" json:"id"`
	VendorID           uint64  `db:"vendor_id" json:"vendor_id"`
	DeviceTypeID       uint64  `db:"device_type_id" json:"device_type_id"`
	DataModelVersionID uint64  `db:"data_model_version_id" json:"data_model_version_id"`
	ProductClass       *string `db:"product_class" json:"product_class"`
	ModelName          string  `db:"model_name" json:"model_name"`
	Description        *string `db:"description" json:"description"`
	IsActive           bool    `db:"is_active" json:"is_active"`
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
// per vendor + data model version, opsional override per device model
// dan/atau per rentang software_version (migrations/0010 -- vendor
// white-label reference-design spt V-SOL/BDCOM/Dasan Zhone bisa punya
// perbedaan path TR-069 antar batch firmware pada "model" yang nominal sama).
// Lihat TECH.md §5.
type VendorParameterMapping struct {
	ID                 uint64  `db:"id" json:"id"`
	VendorID           uint64  `db:"vendor_id" json:"vendor_id"`
	DataModelVersionID uint64  `db:"data_model_version_id" json:"data_model_version_id"`
	DeviceModelID      *uint64 `db:"device_model_id" json:"device_model_id"`
	// SoftwareVersionPattern — pola SQL LIKE thd devices.software_version,
	// dievaluasi gaya sama seperti zero_touch_rules.serial_pattern (lihat
	// migrations/0010). NULL = mapping generik lintas semua versi software.
	// Baris dgn pattern yang cocok LEBIH SPESIFIK / diutamakan dibanding
	// baris tanpa pattern saat Resolve() -- lihat komentar lengkap di
	// repository/mysql/catalog_repository.go.
	SoftwareVersionPattern *string `db:"software_version_pattern" json:"software_version_pattern"`
	LogicalKey             string  `db:"logical_key" json:"logical_key"`
	TR069Path              string  `db:"tr069_path" json:"tr069_path"`
	ParameterTypeID        *uint64 `db:"parameter_type_id" json:"parameter_type_id"`
	Description            *string `db:"description" json:"description"`
	Audit
}

type VendorParameterMappingRepository interface {
	Create(ctx context.Context, m *VendorParameterMapping) error
	Upsert(ctx context.Context, m *VendorParameterMapping) error
	// Resolve mencari mapping paling spesifik utk (vendor, data model
	// version, device model, logical key), dengan urutan spesifisitas
	// (paling spesifik menang) -- lihat implementasi utk detail lengkap:
	//   1. device_model_id spesifik + software_version_pattern COCOK (LIKE)
	//      dgn softwareVersion.
	//   2. device_model_id spesifik, TANPA software_version_pattern.
	//   3. device_model_id NULL (generik lintas model) + pattern COCOK.
	//   4. device_model_id NULL, TANPA software_version_pattern (paling
	//      generik -- satu-satunya perilaku yang ada sebelum migrations/0010).
	// softwareVersion boleh nil/kosong (mis. device belum pernah Inform
	// lengkap) -- langkah 1 & 3 dilewati pada kasus itu.
	Resolve(ctx context.Context, vendorID, dataModelVersionID uint64, deviceModelID *uint64, logicalKey string, softwareVersion *string) (*VendorParameterMapping, error)
	ListByVendor(ctx context.Context, vendorID uint64) ([]VendorParameterMapping, error)
	Delete(ctx context.Context, id, deletedBy uint64) error
}
