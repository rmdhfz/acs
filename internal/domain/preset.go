package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

type Preset struct {
	ID             uint64  `db:"id" json:"id"`
	TenantID       *uint64 `db:"tenant_id" json:"tenant_id,omitempty"`
	Name           string  `db:"name" json:"name"`
	Weight         int     `db:"weight" json:"weight"`
	Precondition   string  `db:"precondition" json:"precondition"`
	Configurations string  `db:"configurations" json:"configurations"`
	IsActive       bool    `db:"is_active" json:"is_active"`
	// Enforce (migrations/0021) — bila true, usecase/provisioning.EvaluatePresets
	// menegakkan konfigurasi ini tiap sesi CWMP saat device menyimpang.
	// Default false: preset tersimpan tapi engine mengabaikannya (keputusan 5b
	// di PRESET_ENGINE_DESIGN.md). FR-18 TIDAK berlaku untuk preset (keputusan 5a).
	Enforce   bool      `db:"enforce" json:"enforce"`
	Channel   *string   `db:"channel" json:"channel,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// PresetPrecondition — bentuk terstruktur kolom `presets.precondition` (JSON).
// Kosakatanya SENGAJA identik dengan precondition ZeroTouchRule supaya bisa
// reuse mesin pencocokan (matchSQLLike dll) — TANPA bahasa ekspresi/DSL baru
// (PRESET_ENGINE_DESIGN.md §2). Semua field opsional, digabung dengan AND.
// (`tag_id` disebut di desain tapi DITUNDA ke v1.1 — butuh TagRepository di
// provisioning.Service.)
type PresetPrecondition struct {
	VendorID                   *uint64 `json:"vendor_id,omitempty"`
	DeviceModelID              *uint64 `json:"device_model_id,omitempty"`
	OUI                        *string `json:"oui,omitempty"`
	SerialPattern              *string `json:"serial_pattern,omitempty"`
	SoftwareVersionPattern     *string `json:"software_version_pattern,omitempty"`
	MatchParameterName         *string `json:"match_parameter_name,omitempty"`
	MatchParameterValuePattern *string `json:"match_parameter_value_pattern,omitempty"`
}

// Kode `op` yang dikenali engine preset v1. `apply_profile` & `refresh`
// direncanakan v1.1 (lihat PRESET_ENGINE_DESIGN.md §3).
const PresetOpSetParameter = "set_parameter"

// PresetConfigOp — satu elemen array kolom `presets.configurations` (JSON).
type PresetConfigOp struct {
	Op    string `json:"op"`
	Key   string `json:"key,omitempty"`   // set_parameter: logical key ATAU raw TR-069 path
	Value string `json:"value,omitempty"` // set_parameter: nilai target literal
}

// ParsedPrecondition meng-unmarshal kolom Precondition. Precondition kosong
// (`{}` / `null` / string kosong) valid = cocok semua device tenant.
func (p *Preset) ParsedPrecondition() (PresetPrecondition, error) {
	var pc PresetPrecondition
	s := p.Precondition
	if s == "" || s == "null" {
		return pc, nil
	}
	if err := json.Unmarshal([]byte(s), &pc); err != nil {
		return pc, fmt.Errorf("preset %d: precondition JSON tidak valid: %w", p.ID, err)
	}
	return pc, nil
}

// ParsedConfigurations meng-unmarshal kolom Configurations (array op).
func (p *Preset) ParsedConfigurations() ([]PresetConfigOp, error) {
	s := p.Configurations
	if s == "" || s == "null" {
		return nil, nil
	}
	var ops []PresetConfigOp
	if err := json.Unmarshal([]byte(s), &ops); err != nil {
		return nil, fmt.Errorf("preset %d: configurations JSON tidak valid: %w", p.ID, err)
	}
	return ops, nil
}

// PresetApplication — tracking apply preset per (preset, device):
// idempotensi drift-check, cooldown, hitung kegagalan (migrations/0021).
type PresetApplication struct {
	PresetID            uint64     `db:"preset_id" json:"preset_id"`
	DeviceID            uint64     `db:"device_id" json:"device_id"`
	LastAppliedAt       *time.Time `db:"last_applied_at" json:"last_applied_at"`
	LastDriftHash       *string    `db:"last_drift_hash" json:"last_drift_hash"`
	ConsecutiveFailures int        `db:"consecutive_failures" json:"consecutive_failures"`
	Status              string     `db:"status" json:"status"`
	UpdatedAt           time.Time  `db:"updated_at" json:"updated_at"`
}

const (
	PresetApplicationStatusPending   = "PENDING"
	PresetApplicationStatusConverged = "CONVERGED"
	PresetApplicationStatusFailed    = "FAILED"
)
