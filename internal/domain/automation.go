package domain

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// ---- Aturan otomasi kondisi-aksi (migrations/0040, pkg/vexpr) ----

// Batas aturan otomasi (satu sumber untuk usecase, repository, dan dokumentasi).
const (
	AutomationNameMaxLen        = 128
	AutomationDescriptionMaxLen = 1024
	AutomationConditionMaxLen   = 1000
	AutomationMaxActions        = 10
	AutomationMaxPerTenant      = 50
	AutomationMinCooldownSec    = 300 // jeda terpendek antar-pemicuan; di bawah ini sama saja dengan membanjiri perangkat
	AutomationMaxCooldownSec    = 30 * 24 * 3600
	AutomationDefaultCooldown   = 3600
	AutomationDefaultMaxFires   = 3
	AutomationMaxFiresCeiling   = 20
	AutomationRunRetention      = 30 * 24 * time.Hour
	// AutomationMaxFiresPerDay = batas keras pemicuan per (aturan, perangkat) per 24 jam, tak bergantung kondisi (migrations/0042).
	AutomationMaxFiresPerDay = 12
	// AutomationMaxSetParamsPerTenant = total aksi set_parameter pada aturan AKTIF satu tenant; membatasi biaya evaluasi per Inform.
	AutomationMaxSetParamsPerTenant = 30
)

// Jenis aksi (daftar tertutup; menambah jenis = perubahan kode yang ditinjau, bukan data).
const (
	AutomationActionSetParameter       = "set_parameter"
	AutomationActionReboot             = "reboot"
	AutomationActionGetParameterValues = "get_parameter_values"
	AutomationActionAddTag             = "add_tag"
	AutomationActionWebhook            = "webhook"
)

// AutomationAction = satu aksi aturan. Field yang dipakai bergantung pada Type:
//
//	set_parameter         Name (logical key atau raw path) + Value (EKSPRESI vexpr; teks literal ditulis 'dalam kutip')
//	get_parameter_values  Names (raw path/logical key; maks 20)
//	add_tag               TagID
//	reboot, webhook       tanpa field tambahan
type AutomationAction struct {
	Type  string   `json:"type"`
	Name  string   `json:"name,omitempty"`
	Value string   `json:"value,omitempty"`
	Names []string `json:"names,omitempty"`
	TagID *uint64  `json:"tag_id,omitempty"`
}

// AutomationActions = kolom actions (JSON) yang dibaca/ditulis langsung oleh sqlx.
type AutomationActions []AutomationAction

func (a AutomationActions) Value() (driver.Value, error) {
	if a == nil {
		a = AutomationActions{}
	}
	b, err := json.Marshal(a)
	return string(b), err
}

func (a *AutomationActions) Scan(src any) error {
	var b []byte
	switch v := src.(type) {
	case nil:
		*a = AutomationActions{}
		return nil
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("automation actions: tipe %T tak didukung", src)
	}
	if len(b) == 0 {
		*a = AutomationActions{}
		return nil
	}
	return json.Unmarshal(b, a)
}

// AutomationRule = definisi aturan. ID numerik tidak diekspos (UUID publik). TriggerEvent = kode ref_ztp_trigger_event.
type AutomationRule struct {
	ID                  uint64            `db:"id" json:"-"`
	UUID                string            `db:"rule_uuid" json:"id"`
	TenantID            uint64            `db:"tenant_id" json:"tenant_id"`
	Name                string            `db:"name" json:"name"`
	Description         *string           `db:"description" json:"description"`
	TriggerEventID      uint64            `db:"trigger_event_id" json:"-"`
	TriggerEvent        string            `db:"trigger_event" json:"trigger_event"`
	Condition           string            `db:"condition_expr" json:"condition"`
	Actions             AutomationActions `db:"actions" json:"actions"`
	CooldownSeconds     uint32            `db:"cooldown_seconds" json:"cooldown_seconds"`
	MaxConsecutiveFires uint8             `db:"max_consecutive_fires" json:"max_consecutive_fires"`
	Priority            uint32            `db:"priority" json:"priority"`
	IsActive            bool              `db:"is_active" json:"is_active"`
	CreatedAt           time.Time         `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time         `db:"updated_at" json:"updated_at"`
}

// AutomationState = state pagar per (aturan, perangkat).
type AutomationState struct {
	RuleID           uint64     `db:"rule_id"`
	DeviceID         uint64     `db:"device_id"`
	LastFiredAt      *time.Time `db:"last_fired_at"`
	ConsecutiveFires uint32     `db:"consecutive_fires"`
	Suspended        bool       `db:"suspended"`
	// WindowStartedAt/WindowFires = jendela 24 jam penghitung pemicuan (batas keras AutomationMaxFiresPerDay).
	WindowStartedAt *time.Time `db:"window_started_at"`
	WindowFires     uint32     `db:"window_fires"`
}

// AutomationRun = satu catatan pemicuan (riwayat). RuleName/DeviceSerial dari join untuk tampilan.
type AutomationRun struct {
	ID           uint64    `db:"id" json:"-"`
	UUID         string    `db:"run_uuid" json:"id"`
	RuleID       uint64    `db:"rule_id" json:"-"`
	RuleUUID     string    `db:"rule_uuid" json:"rule_id"`
	RuleName     string    `db:"rule_name" json:"rule_name"`
	DeviceID     uint64    `db:"device_id" json:"device_id"`
	DeviceSerial string    `db:"device_serial" json:"device_serial"`
	Fired        bool      `db:"fired" json:"fired"`
	Suspended    bool      `db:"suspended" json:"suspended"`
	Summary      string    `db:"summary" json:"summary"`
	Error        *string   `db:"error" json:"error"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

// AutomationRunFilter — TenantID wajib diisi usecase untuk non-superadmin (tenant dari aturan).
type AutomationRunFilter struct {
	TenantID *uint64
	RuleUUID string
	DeviceID *uint64
}

type AutomationRuleRepository interface {
	Create(ctx context.Context, r *AutomationRule, by *uint64) error
	GetByUUID(ctx context.Context, uuid string) (*AutomationRule, error)
	// List: tenantID nil = semua tenant (superadmin).
	List(ctx context.Context, tenantID *uint64, p Pagination) ([]AutomationRule, int, error)
	// ListActive: aturan aktif milik tenant, urut priority lalu id. Dipanggil pada SETIAP Inform (hot path; di-cache hotcache).
	ListActive(ctx context.Context, tenantID uint64) ([]AutomationRule, error)
	FindByName(ctx context.Context, tenantID uint64, name string) (*AutomationRule, error)
	CountInTenant(ctx context.Context, tenantID uint64) (int, error)
	Update(ctx context.Context, r *AutomationRule, by *uint64) error
	SoftDelete(ctx context.Context, id, by uint64) error
}

type AutomationStateRepository interface {
	// ListByDevice: state perangkat itu untuk semua aturan (satu query per evaluasi).
	ListByDevice(ctx context.Context, deviceID uint64) (map[uint64]AutomationState, error)
	Save(ctx context.Context, s *AutomationState) error
	// ResetConsecutive menolkan hitungan berturut-turut (kondisi salah / sudah konvergen); tidak menyentuh suspended.
	ResetConsecutive(ctx context.Context, ruleID, deviceID uint64) error
	// ClearSuspension: operator mereset aturan untuk perangkat (atau semua perangkat bila deviceID nil).
	ClearSuspension(ctx context.Context, ruleID uint64, deviceID *uint64) (int64, error)
}

type AutomationRunRepository interface {
	// Insert menyimpan catatan lalu (sesekali) memangkas yang lebih tua dari AutomationRunRetention.
	Insert(ctx context.Context, r *AutomationRun) error
	List(ctx context.Context, f AutomationRunFilter, p Pagination) ([]AutomationRun, int, error)
}
