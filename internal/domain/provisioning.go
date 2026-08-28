package domain

import (
	"context"
	"time"
)

// ProvisioningProfile — kumpulan parameter default per vendor/model (lihat TECH.md §6).
type ProvisioningProfile struct {
	ID            uint64  `db:"id" json:"id"`
	ProfileUUID   string  `db:"profile_uuid" json:"profile_uuid"`
	TenantID      *uint64 `db:"tenant_id" json:"tenant_id"`
	VendorID      *uint64 `db:"vendor_id" json:"vendor_id"`
	DeviceModelID *uint64 `db:"device_model_id" json:"device_model_id"`
	Name          string  `db:"name" json:"name"`
	Description   *string `db:"description" json:"description"`
	IsDefault     bool    `db:"is_default" json:"is_default"`
	IsActive      bool    `db:"is_active" json:"is_active"`
	Audit
}

type ProvisioningProfileRepository interface {
	Create(ctx context.Context, p *ProvisioningProfile) error
	GetByID(ctx context.Context, id uint64) (*ProvisioningProfile, error)
	GetByUUID(ctx context.Context, uuid string) (*ProvisioningProfile, error)
	List(ctx context.Context, tenantID *uint64, p Pagination) ([]ProvisioningProfile, int, error)
	Update(ctx context.Context, p *ProvisioningProfile) error
	SoftDelete(ctx context.Context, id, deletedBy uint64) error
}

// ProvisioningProfileParameter.ParameterName boleh logical key ATAU raw TR-069
// path (lihat schema.sql). Resolusi logical key dilakukan saat apply, bukan
// saat disimpan, agar tetap portable lintas vendor bila profile bersifat umum.
type ProvisioningProfileParameter struct {
	ID              uint64    `db:"id" json:"id"`
	ProfileID       uint64    `db:"profile_id" json:"profile_id"`
	ParameterName   string    `db:"parameter_name" json:"parameter_name"`
	ParameterValue  *string   `db:"parameter_value" json:"parameter_value"`
	ParameterTypeID *uint64   `db:"parameter_type_id" json:"parameter_type_id"`
	ApplyOrder      uint32    `db:"apply_order" json:"apply_order"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time `db:"updated_at" json:"updated_at"`
}

type ProvisioningProfileParameterRepository interface {
	Replace(ctx context.Context, profileID uint64, params []ProvisioningProfileParameter) error
	ListByProfile(ctx context.Context, profileID uint64) ([]ProvisioningProfileParameter, error)
}

// ZeroTouchRule — aturan pencocokan device baru ke provisioning profile
// dan/atau aksi reboot/firmware push (TECH.md §6; matching & aksi diperluas
// migrations/0009 mengikuti kapabilitas GenieACS presets sbg pembanding).
//
// Kapan rule ini dievaluasi (dulu hardcoded "hanya saat 0 BOOTSTRAP") kini
// ditentukan TriggerEventID (FK ref_ztp_trigger_event) -- lihat konstanta
// ZtpTriggerEvent* di domain/common.go. ProvisioningProfileID sekarang
// OPSIONAL: rule boleh hanya memicu PostApplyReboot dan/atau FirmwareFileID
// tanpa menerapkan profile parameter apa pun. MatchParameterName/
// MatchParameterValuePattern adalah precondition BERPASANGAN opsional --
// keduanya harus nil atau keduanya terisi (ditegakkan CHECK constraint DB
// chk_ztr_match_parameter_pair; evaluasi pencocokan sesungguhnya thd
// device_parameters adalah logic usecase, TIDAK diimplementasikan di sini).
type ZeroTouchRule struct {
	ID            uint64  `db:"id" json:"id"`
	TenantID      *uint64 `db:"tenant_id" json:"tenant_id"`
	VendorID      *uint64 `db:"vendor_id" json:"vendor_id"`
	DeviceModelID *uint64 `db:"device_model_id" json:"device_model_id"`
	OUI           *string `db:"oui" json:"oui"`
	SerialPattern *string `db:"serial_pattern" json:"serial_pattern"`
	// SoftwareVersionPattern — pola SQL LIKE thd devices.software_version,
	// dievaluasi sama persis dgn SerialPattern (lihat matchSQLLike di
	// usecase/provisioning/service.go).
	SoftwareVersionPattern *string `db:"software_version_pattern" json:"software_version_pattern"`
	// MatchParameterName/MatchParameterValuePattern — precondition tambahan
	// opsional berpasangan: kalau MatchParameterName diisi, rule juga
	// mensyaratkan device punya baris device_parameters dgn nama tsb yang
	// nilainya cocok pola MatchParameterValuePattern (SQL LIKE).
	MatchParameterName         *string `db:"match_parameter_name" json:"match_parameter_name"`
	MatchParameterValuePattern *string `db:"match_parameter_value_pattern" json:"match_parameter_value_pattern"`
	// ProvisioningProfileID — NULLABLE sejak migrations/0009 (rule boleh
	// hanya memicu PostApplyReboot dan/atau FirmwareFileID).
	ProvisioningProfileID *uint64 `db:"provisioning_profile_id" json:"provisioning_profile_id"`
	// PostApplyReboot — jika true, enqueue task REBOOT ke device setelah
	// aksi lain rule ini diterapkan.
	PostApplyReboot bool `db:"post_apply_reboot" json:"post_apply_reboot"`
	// FirmwareFileID — jika diisi, enqueue task Download firmware ini ke
	// device yang cocok (FK firmware_files).
	FirmwareFileID *uint64 `db:"firmware_file_id" json:"firmware_file_id"`
	// TriggerEventID — FK ref_ztp_trigger_event, kapan rule ini dievaluasi
	// relatif thd event CWMP Inform (lihat komentar tipe di atas).
	TriggerEventID uint64 `db:"trigger_event_id" json:"trigger_event_id"`
	Priority       uint32 `db:"priority" json:"priority"` // angka lebih kecil dievaluasi lebih dulu
	IsActive       bool   `db:"is_active" json:"is_active"`
	Audit
}

type ZeroTouchRuleRepository interface {
	Create(ctx context.Context, r *ZeroTouchRule) error
	GetByID(ctx context.Context, id uint64) (*ZeroTouchRule, error)
	// ListActiveOrdered mengembalikan rule aktif milik tenant (dan rule global
	// tenant_id NULL), terurut priority ASC, untuk dievaluasi berurutan.
	ListActiveOrdered(ctx context.Context, tenantID *uint64) ([]ZeroTouchRule, error)
	Update(ctx context.Context, r *ZeroTouchRule) error
	SoftDelete(ctx context.Context, id, deletedBy uint64) error
}
