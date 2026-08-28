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
}
