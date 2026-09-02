package domain

import "context"

type TagRepository interface {
	Create(ctx context.Context, tag *Tag) error
	GetByID(ctx context.Context, id uint64) (*Tag, error)
	List(ctx context.Context, tenantID *uint64, p Pagination) ([]Tag, int, error)
	Delete(ctx context.Context, id uint64) error
	AssignToDevice(ctx context.Context, deviceID, tagID uint64) error
	RemoveFromDevice(ctx context.Context, deviceID, tagID uint64) error
	ListByDevice(ctx context.Context, deviceID uint64) ([]Tag, error)
}

type PresetRepository interface {
	Create(ctx context.Context, preset *Preset) error
	GetByID(ctx context.Context, id uint64) (*Preset, error)
	List(ctx context.Context, tenantID *uint64, p Pagination) ([]Preset, int, error)
	Update(ctx context.Context, preset *Preset) error
	Delete(ctx context.Context, id uint64) error
	// ListActiveEnforce — preset is_active=1 AND enforce=1 milik tenant device
	// (dan preset global tenant_id NULL bila nanti diizinkan), terurut
	// weight ASC supaya preset weight lebih besar menimpa saat key bentrok
	// (PRESET_ENGINE_DESIGN.md §4). Dipakai usecase/provisioning.EvaluatePresets
	// tiap sesi CWMP — query kecil & flat, tanpa pagination.
	ListActiveEnforce(ctx context.Context, tenantID *uint64) ([]Preset, error)
}

// PresetApplicationRepository — state per (preset, device) untuk engine preset
// (migrations/0021). Get mengembalikan ErrNotFound bila belum pernah ada baris.
type PresetApplicationRepository interface {
	Get(ctx context.Context, presetID, deviceID uint64) (*PresetApplication, error)
	Upsert(ctx context.Context, a *PresetApplication) error
}
