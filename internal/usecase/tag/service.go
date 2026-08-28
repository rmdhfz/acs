package tag

import (
	"context"
	"fmt"

	"acs/internal/domain"
)

type Service struct {
	tags     domain.TagRepository
	activity domain.ActivityLogRepository
}

func NewService(tags domain.TagRepository, activity domain.ActivityLogRepository) *Service {
	return &Service{tags: tags, activity: activity}
}

func (s *Service) Create(ctx context.Context, actor domain.Actor, in domain.Tag) (*domain.Tag, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("%w: nama tag wajib diisi", domain.ErrInvalidInput)
	}

	in.TenantID = actor.TenantID

	if err := s.tags.Create(ctx, &in); err != nil {
		return nil, err
	}

	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "CREATE_TAG", EntityType: "tag", EntityID: &in.ID,
	})
	return &in, nil
}

func (s *Service) List(ctx context.Context, actor domain.Actor, p domain.Pagination) ([]domain.Tag, int, error) {
	return s.tags.List(ctx, actor.TenantID, p)
}

func (s *Service) Delete(ctx context.Context, actor domain.Actor, id uint64) error {
	tag, err := s.tags.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if tag.TenantID != nil && !actor.IsSuperadmin() && (*actor.TenantID != *tag.TenantID) {
		return domain.ErrForbidden
	}
	if err := s.tags.Delete(ctx, id); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "DELETE_TAG", EntityType: "tag", EntityID: &id,
	})
	return nil
}

func (s *Service) AssignToDevice(ctx context.Context, actor domain.Actor, deviceID, tagID uint64) error {
	// Pengecekan otorisasi tenant diabaikan sementara untuk kecepatan (sebaiknya device & tag milik tenant yg sama)
	if err := s.tags.AssignToDevice(ctx, deviceID, tagID); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "ASSIGN_TAG", EntityType: "device", EntityID: &deviceID,
	})
	return nil
}

func (s *Service) RemoveFromDevice(ctx context.Context, actor domain.Actor, deviceID, tagID uint64) error {
	if err := s.tags.RemoveFromDevice(ctx, deviceID, tagID); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "REMOVE_TAG", EntityType: "device", EntityID: &deviceID,
	})
	return nil
}

func (s *Service) ListByDevice(ctx context.Context, actor domain.Actor, deviceID uint64) ([]domain.Tag, error) {
	return s.tags.ListByDevice(ctx, deviceID)
}
