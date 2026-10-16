// Package catalog mengelola data referensi vendor CPE (vendor, OUI, device
// model, vendor parameter mapping) — operasi data, bukan kode (CLAUDE.md,
// TECH.md §5). Semua mutasi superadmin-only karena data ini GLOBAL lintas
// tenant. Hapus = soft delete, dan DITOLAK (409) selama masih ada data aktif
// yang merujuk; jalan keluarnya menonaktifkan (is_active = false), bukan
// menghapus, supaya device/profil/firmware yang ada tidak jadi yatim.
package catalog

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
)

type Service struct {
	vendors  domain.VendorRepository
	ouis     domain.VendorOUIRepository
	models   domain.DeviceModelRepository
	mappings domain.VendorParameterMappingRepository
	usage    domain.UsageRepository
	activity domain.ActivityLogRepository
}

func NewService(
	vendors domain.VendorRepository,
	ouis domain.VendorOUIRepository,
	models domain.DeviceModelRepository,
	mappings domain.VendorParameterMappingRepository,
	usage domain.UsageRepository,
	activity domain.ActivityLogRepository,
) *Service {
	return &Service{vendors: vendors, ouis: ouis, models: models, mappings: mappings, usage: usage, activity: activity}
}

// usageConflict menyusun 409 yang menuntun: apa yang masih memakai + jalan keluar.
func usageConflict(what string, usage []domain.UsageCount) error {
	return fmt.Errorf("%w — nonaktifkan (is_active=false) alih-alih menghapus", domain.NewUsageConflict(what, usage))
}

func (s *Service) record(ctx context.Context, actor domain.Actor, action, entityType string, id uint64) {
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), Action: action, EntityType: entityType, EntityID: &id,
	})
}

// requireActiveVendor memastikan vendor ada dan BELUM dihapus. FK ke
// ref_vendors tidak melihat is_deleted, jadi tanpa cek ini OUI/model/mapping
// bisa dibuat untuk vendor yang sudah di-soft-delete (device lalu ter-resolve
// ke vendor "hantu"); vendor yang tidak ada akan jadi 500 dari FK, bukan 404.
func (s *Service) requireActiveVendor(ctx context.Context, vendorID uint64) (*domain.Vendor, error) {
	if vendorID == 0 {
		return nil, fmt.Errorf("%w: vendor_id wajib diisi", domain.ErrInvalidInput)
	}
	return s.vendors.GetByID(ctx, vendorID)
}

var (
	ouiFormat       = regexp.MustCompile(`^[0-9A-Z]{6}$`)
	tr069PathFormat = regexp.MustCompile(`^(InternetGatewayDevice|Device)\.[A-Za-z0-9_.{}\-]*$`)
)

// AddVendorOUI mendaftarkan OUI untuk vendor. OUI yang pernah dihapus
// dihidupkan kembali oleh repository (lihat vendorOUIRepository.Create).
func (s *Service) AddVendorOUI(ctx context.Context, actor domain.Actor, vendorID uint64, oui string, notes *string) (*domain.VendorOUI, error) {
	if err := auth.RequireRole(actor, domain.RoleSuperadmin); err != nil {
		return nil, err
	}
	oui = strings.ToUpper(strings.TrimSpace(oui))
	if !ouiFormat.MatchString(oui) {
		return nil, fmt.Errorf("%w: oui harus tepat 6 karakter huruf/angka (umumnya heksadesimal, mis. 00D0D0)", domain.ErrInvalidInput)
	}
	if _, err := s.requireActiveVendor(ctx, vendorID); err != nil {
		return nil, err
	}
	o := &domain.VendorOUI{VendorID: vendorID, OUI: oui, Notes: notes, Audit: domain.Audit{CreatedBy: actor.UserIDPtr()}}
	if err := s.ouis.Create(ctx, o); err != nil {
		return nil, err
	}
	s.record(ctx, actor, "ADD_VENDOR_OUI", "vendor", vendorID)
	return o, nil
}

// CreateDeviceModel membuat model untuk vendor aktif. device_type_id dan
// data_model_version_id wajib (0 = tidak diisi) — tanpa itu FK gagal sebagai 500.
func (s *Service) CreateDeviceModel(ctx context.Context, actor domain.Actor, m *domain.DeviceModel) error {
	if err := auth.RequireRole(actor, domain.RoleSuperadmin); err != nil {
		return err
	}
	if strings.TrimSpace(m.ModelName) == "" {
		return fmt.Errorf("%w: model_name wajib diisi", domain.ErrInvalidInput)
	}
	if m.DeviceTypeID == 0 || m.DataModelVersionID == 0 {
		return fmt.Errorf("%w: device_type_id dan data_model_version_id wajib diisi", domain.ErrInvalidInput)
	}
	if _, err := s.requireActiveVendor(ctx, m.VendorID); err != nil {
		return err
	}
	m.IsActive = true
	m.CreatedBy = actor.UserIDPtr()
	if err := s.models.Create(ctx, m); err != nil {
		return err
	}
	s.record(ctx, actor, "CREATE_DEVICE_MODEL", "device_model", m.ID)
	return nil
}

// UpsertParameterMapping membuat/mengganti mapping untuk vendor aktif. Bila
// device_model_id diisi, model itu harus ada dan milik vendor yang sama.
func (s *Service) UpsertParameterMapping(ctx context.Context, actor domain.Actor, m *domain.VendorParameterMapping) error {
	if err := auth.RequireRole(actor, domain.RoleSuperadmin); err != nil {
		return err
	}
	m.LogicalKey = strings.TrimSpace(m.LogicalKey)
	m.TR069Path = strings.TrimSpace(m.TR069Path)
	if m.LogicalKey == "" || m.TR069Path == "" {
		return fmt.Errorf("%w: logical_key dan tr069_path wajib diisi", domain.ErrInvalidInput)
	}
	// Data referensi GLOBAL: path salah merusak provisioning semua tenant (CPE membalas
	// fault 9005). Root data model hanya TR-098 atau TR-181 dan tanpa spasi/karakter aneh.
	if !tr069PathFormat.MatchString(m.TR069Path) {
		return fmt.Errorf("%w: tr069_path harus berawalan InternetGatewayDevice. atau Device. dan hanya berisi huruf, angka, titik, _ , -", domain.ErrInvalidInput)
	}
	if _, err := s.requireActiveVendor(ctx, m.VendorID); err != nil {
		return err
	}
	if m.DeviceModelID != nil {
		dm, err := s.models.GetByID(ctx, *m.DeviceModelID)
		if err != nil {
			return err
		}
		if dm.VendorID != m.VendorID {
			return fmt.Errorf("%w: device_model_id bukan milik vendor_id tsb", domain.ErrInvalidInput)
		}
	}
	m.CreatedBy = actor.UserIDPtr()
	if err := s.mappings.Upsert(ctx, m); err != nil {
		return err
	}
	s.record(ctx, actor, "UPSERT_VENDOR_PARAMETER_MAPPING", "vendor_parameter_mapping", m.ID)
	return nil
}

// ---- Vendor ----

func (s *Service) GetVendor(ctx context.Context, id uint64) (*domain.Vendor, error) {
	return s.vendors.GetByID(ctx, id)
}

// UpdateVendorInput — partial update; field nil = tidak diubah. Code TIDAK
// bisa diubah (dipakai sebagai identitas stabil oleh seed/migrasi/skrip).
type UpdateVendorInput struct {
	Name        *string
	Description *string // "" = kosongkan
	IsActive    *bool
}

func (s *Service) UpdateVendor(ctx context.Context, actor domain.Actor, id uint64, in UpdateVendorInput) (*domain.Vendor, error) {
	if err := auth.RequireRole(actor, domain.RoleSuperadmin); err != nil {
		return nil, err
	}
	v, err := s.vendors.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return nil, fmt.Errorf("%w: name tidak boleh kosong", domain.ErrInvalidInput)
		}
		v.Name = *in.Name
	}
	if in.Description != nil {
		v.Description = nilIfEmpty(*in.Description)
	}
	if in.IsActive != nil {
		v.IsActive = *in.IsActive
	}
	v.UpdatedBy = actor.UserIDPtr()
	if err := s.vendors.Update(ctx, v); err != nil {
		return nil, err
	}
	s.record(ctx, actor, "UPDATE_VENDOR", "vendor", v.ID)
	return v, nil
}

// DeleteVendor — soft delete. Ditolak (409) bila masih dipakai device, model,
// mapping, firmware, profil, rule ZTP, atau rollout aktif. OUI vendor ikut
// di-soft-delete (OUI tanpa vendor tidak bermakna dan akan salah me-resolve).
func (s *Service) DeleteVendor(ctx context.Context, actor domain.Actor, id uint64) error {
	if err := auth.RequireRole(actor, domain.RoleSuperadmin); err != nil {
		return err
	}
	v, err := s.vendors.GetByID(ctx, id)
	if err != nil {
		return err
	}
	usage, err := s.usage.VendorUsage(ctx, id)
	if err != nil {
		return err
	}
	if len(usage) > 0 {
		return usageConflict(fmt.Sprintf("vendor %q", v.Code), usage)
	}
	ouis, err := s.ouis.ListByVendor(ctx, id)
	if err != nil {
		return err
	}
	for _, o := range ouis {
		if err := s.ouis.Delete(ctx, o.ID, actor.UserID); err != nil {
			return err
		}
	}
	if err := s.vendors.SoftDelete(ctx, id, actor.UserID); err != nil {
		return err
	}
	s.record(ctx, actor, "DELETE_VENDOR", "vendor", id)
	return nil
}

// DeleteVendorOUI menghapus (soft) satu OUI milik vendor. OUI milik vendor
// lain atau yang tidak ada -> 404 (jangan bocorkan keberadaan lintas vendor).
func (s *Service) DeleteVendorOUI(ctx context.Context, actor domain.Actor, vendorID, ouiID uint64) error {
	if err := auth.RequireRole(actor, domain.RoleSuperadmin); err != nil {
		return err
	}
	if _, err := s.vendors.GetByID(ctx, vendorID); err != nil {
		return err
	}
	ouis, err := s.ouis.ListByVendor(ctx, vendorID)
	if err != nil {
		return err
	}
	found := false
	for _, o := range ouis {
		if o.ID == ouiID {
			found = true
			break
		}
	}
	if !found {
		return domain.ErrNotFound
	}
	if err := s.ouis.Delete(ctx, ouiID, actor.UserID); err != nil {
		return err
	}
	s.record(ctx, actor, "DELETE_VENDOR_OUI", "vendor", vendorID)
	return nil
}

// ---- Device model ----

func (s *Service) GetDeviceModel(ctx context.Context, id uint64) (*domain.DeviceModel, error) {
	return s.models.GetByID(ctx, id)
}

// UpdateDeviceModelInput — partial update. VendorID TIDAK bisa diubah
// (model milik vendor tertentu; memindahkan = hapus + buat baru).
type UpdateDeviceModelInput struct {
	DeviceTypeID       *uint64
	DataModelVersionID *uint64
	ProductClass       *string // "" = kosongkan
	ModelName          *string
	Description        *string // "" = kosongkan
	IsActive           *bool
}

func (s *Service) UpdateDeviceModel(ctx context.Context, actor domain.Actor, id uint64, in UpdateDeviceModelInput) (*domain.DeviceModel, error) {
	if err := auth.RequireRole(actor, domain.RoleSuperadmin); err != nil {
		return nil, err
	}
	m, err := s.models.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// Mengubah device_type_id / data_model_version_id (mis. TR-098 -> TR-181)
	// pada model yang SUDAH dipakai device membuat resolusi path parameter
	// (Resolve) gagal untuk device-device itu. Ditolak selama masih ada device.
	changesDataModel := (in.DeviceTypeID != nil && *in.DeviceTypeID != m.DeviceTypeID) ||
		(in.DataModelVersionID != nil && *in.DataModelVersionID != m.DataModelVersionID)
	if changesDataModel {
		usage, err := s.usage.DeviceModelUsage(ctx, id)
		if err != nil {
			return nil, err
		}
		for _, u := range usage {
			if u.Kind == "device" {
				return nil, fmt.Errorf("%w — device_type_id/data_model_version_id tidak bisa diubah selama model dipakai device; buat model baru",
					domain.NewUsageConflict(fmt.Sprintf("model %q", m.ModelName), []domain.UsageCount{u}))
			}
		}
	}
	if in.DeviceTypeID != nil {
		if *in.DeviceTypeID == 0 {
			return nil, fmt.Errorf("%w: device_type_id tidak valid", domain.ErrInvalidInput)
		}
		m.DeviceTypeID = *in.DeviceTypeID
	}
	if in.DataModelVersionID != nil {
		if *in.DataModelVersionID == 0 {
			return nil, fmt.Errorf("%w: data_model_version_id tidak valid", domain.ErrInvalidInput)
		}
		m.DataModelVersionID = *in.DataModelVersionID
	}
	if in.ProductClass != nil {
		m.ProductClass = nilIfEmpty(*in.ProductClass)
	}
	if in.ModelName != nil {
		if strings.TrimSpace(*in.ModelName) == "" {
			return nil, fmt.Errorf("%w: model_name tidak boleh kosong", domain.ErrInvalidInput)
		}
		m.ModelName = *in.ModelName
	}
	if in.Description != nil {
		m.Description = nilIfEmpty(*in.Description)
	}
	if in.IsActive != nil {
		m.IsActive = *in.IsActive
	}
	m.UpdatedBy = actor.UserIDPtr()
	if err := s.models.Update(ctx, m); err != nil {
		return nil, err
	}
	s.record(ctx, actor, "UPDATE_DEVICE_MODEL", "device_model", m.ID)
	return m, nil
}

func (s *Service) DeleteDeviceModel(ctx context.Context, actor domain.Actor, id uint64) error {
	if err := auth.RequireRole(actor, domain.RoleSuperadmin); err != nil {
		return err
	}
	m, err := s.models.GetByID(ctx, id)
	if err != nil {
		return err
	}
	usage, err := s.usage.DeviceModelUsage(ctx, id)
	if err != nil {
		return err
	}
	if len(usage) > 0 {
		return usageConflict(fmt.Sprintf("model %q", m.ModelName), usage)
	}
	if err := s.models.SoftDelete(ctx, id, actor.UserID); err != nil {
		return err
	}
	s.record(ctx, actor, "DELETE_DEVICE_MODEL", "device_model", id)
	return nil
}

// ---- Vendor parameter mapping ----

func (s *Service) GetParameterMapping(ctx context.Context, id uint64) (*domain.VendorParameterMapping, error) {
	return s.mappings.GetByID(ctx, id)
}

// DeleteParameterMapping — soft delete satu mapping. Tidak ada guard "dipakai":
// mapping adalah konfigurasi lookup, bukan entitas yang dirujuk FK; setelah
// dihapus, Resolve() jatuh ke mapping yang lebih generik bila ada, atau
// logical key itu tidak ter-resolve (operator masih bisa memakai path mentah).
func (s *Service) DeleteParameterMapping(ctx context.Context, actor domain.Actor, id uint64) error {
	if err := auth.RequireRole(actor, domain.RoleSuperadmin); err != nil {
		return err
	}
	if _, err := s.mappings.GetByID(ctx, id); err != nil {
		return err
	}
	if err := s.mappings.Delete(ctx, id, actor.UserID); err != nil {
		return err
	}
	s.record(ctx, actor, "DELETE_VENDOR_PARAMETER_MAPPING", "vendor_parameter_mapping", id)
	return nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
