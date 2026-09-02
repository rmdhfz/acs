package preset

import (
	"context"
	"fmt"

	"acs/internal/domain"
)

type Service struct {
	presets  domain.PresetRepository
	activity domain.ActivityLogRepository
}

func NewService(presets domain.PresetRepository, activity domain.ActivityLogRepository) *Service {
	return &Service{presets: presets, activity: activity}
}

func (s *Service) Create(ctx context.Context, actor domain.Actor, in domain.Preset) (*domain.Preset, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("%w: nama preset wajib diisi", domain.ErrInvalidInput)
	}
	if err := validatePresetJSON(&in); err != nil {
		return nil, err
	}

	in.TenantID = actor.TenantID
	in.IsActive = true

	if err := s.presets.Create(ctx, &in); err != nil {
		return nil, err
	}

	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "CREATE_PRESET", EntityType: "preset", EntityID: &in.ID,
	})
	return &in, nil
}

func (s *Service) List(ctx context.Context, actor domain.Actor, p domain.Pagination) ([]domain.Preset, int, error) {
	return s.presets.List(ctx, actor.TenantID, p)
}

func (s *Service) Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.Preset, error) {
	preset, err := s.presets.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if preset.TenantID != nil && !actor.IsSuperadmin() && (*actor.TenantID != *preset.TenantID) {
		return nil, domain.ErrForbidden
	}
	return preset, nil
}

func (s *Service) Update(ctx context.Context, actor domain.Actor, id uint64, in domain.Preset) error {
	preset, err := s.Get(ctx, actor, id)
	if err != nil {
		return err
	}
	preset.Name = in.Name
	preset.Weight = in.Weight
	preset.Precondition = in.Precondition
	preset.Configurations = in.Configurations
	preset.IsActive = in.IsActive
	preset.Enforce = in.Enforce
	preset.Channel = in.Channel
	if err := validatePresetJSON(preset); err != nil {
		return err
	}

	if err := s.presets.Update(ctx, preset); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "UPDATE_PRESET", EntityType: "preset", EntityID: &id,
	})
	return nil
}

func (s *Service) Delete(ctx context.Context, actor domain.Actor, id uint64) error {
	preset, err := s.Get(ctx, actor, id)
	if err != nil {
		return err
	}
	if err := s.presets.Delete(ctx, preset.ID); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "DELETE_PRESET", EntityType: "preset", EntityID: &id,
	})
	return nil
}

// validatePresetJSON menormalkan & memvalidasi kolom JSON preset SEBELUM
// disimpan — mencegah garbage masuk ke jalur enforcement (EvaluatePresets).
// Precondition/configurations kosong dinormalkan ke "{}" / "[]".
func validatePresetJSON(p *domain.Preset) error {
	if p.Precondition == "" {
		p.Precondition = "{}"
	}
	if p.Configurations == "" {
		p.Configurations = "[]"
	}
	if _, err := p.ParsedPrecondition(); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
	}
	ops, err := p.ParsedConfigurations()
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
	}
	for i, op := range ops {
		if op.Op != domain.PresetOpSetParameter {
			return fmt.Errorf("%w: configurations[%d].op %q belum didukung (v1 hanya %q)", domain.ErrInvalidInput, i, op.Op, domain.PresetOpSetParameter)
		}
		if op.Key == "" {
			return fmt.Errorf("%w: configurations[%d] (set_parameter) wajib punya key", domain.ErrInvalidInput, i)
		}
	}
	return nil
}
