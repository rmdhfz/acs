package tag

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"acs/internal/domain"
)

var tagColorFormat = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

type Service struct {
	tags     domain.TagRepository
	activity domain.ActivityLogRepository
}

func NewService(tags domain.TagRepository, activity domain.ActivityLogRepository) *Service {
	return &Service{tags: tags, activity: activity}
}

func (s *Service) Create(ctx context.Context, actor domain.Actor, in domain.Tag) (*domain.Tag, error) {
	name, err := domain.CleanText("nama tag", in.Name, 1, 255, false)
	if err != nil {
		return nil, err
	}
	in.Name = name
	if in.Color != nil && strings.TrimSpace(*in.Color) == "" {
		in.Color = nil
	}
	if in.Color != nil && !tagColorFormat.MatchString(*in.Color) {
		return nil, fmt.Errorf("%w: color harus heksadesimal (#RGB atau #RRGGBB)", domain.ErrInvalidInput)
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

// requireTagMutationScope — scope untuk MENGUBAH/MENGHAPUS tag (lebih ketat
// dari requireTagInTenantScope yang untuk memakai tag): tag milik tenant hanya
// boleh dimutasi actor tenant itu; tag GLOBAL (tenant_id NULL, dipakai semua
// tenant) hanya superadmin — kalau tidak, ADMIN satu tenant bisa mengubah atau
// menghapus tag yang dipakai tenant lain. Actor non-superadmin tanpa tenant
// ditolak (bukan nil-deref).
func (s *Service) requireTagMutationScope(ctx context.Context, actor domain.Actor, id uint64) (*domain.Tag, error) {
	tag, err := s.tags.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if actor.IsSuperadmin() {
		return tag, nil
	}
	if tag.TenantID == nil || actor.TenantID == nil || *tag.TenantID != *actor.TenantID {
		return nil, domain.ErrForbidden
	}
	return tag, nil
}

// UpdateInput — partial update; field nil = tidak diubah. Color "" = kosongkan.
type UpdateInput struct {
	Name  *string
	Color *string
}

func (s *Service) Update(ctx context.Context, actor domain.Actor, id uint64, in UpdateInput) (*domain.Tag, error) {
	tag, err := s.requireTagMutationScope(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		name, verr := domain.CleanText("nama tag", *in.Name, 1, 255, false)
		if verr != nil {
			return nil, verr
		}
		tag.Name = name
	}
	if in.Color != nil {
		if strings.TrimSpace(*in.Color) == "" {
			tag.Color = nil
		} else {
			if !tagColorFormat.MatchString(*in.Color) {
				return nil, fmt.Errorf("%w: color harus heksadesimal (#RGB atau #RRGGBB)", domain.ErrInvalidInput)
			}
			tag.Color = in.Color
		}
	}
	if err := s.tags.Update(ctx, tag); err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "UPDATE_TAG", EntityType: "tag", EntityID: &id,
	})
	return tag, nil
}

func (s *Service) Delete(ctx context.Context, actor domain.Actor, id uint64) error {
	if _, err := s.requireTagMutationScope(ctx, actor, id); err != nil {
		return err
	}
	if err := s.tags.Delete(ctx, id); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "DELETE_TAG", EntityType: "tag", EntityID: &id,
	})
	return nil
}

// requireTagInTenantScope memastikan tag ada & (kalau bukan tag global
// tenant_id=NULL) milik tenant si actor. Kepemilikan device dicek terpisah di
// handler lewat device.Service.Get (yang sudah enforce RequireTenantScope) —
// di sini kita jaga sisi tag-nya supaya operator tenant A tidak bisa
// menempelkan/melepas tag milik tenant B ke device-nya (temuan: cek ini
// sebelumnya "diabaikan sementara").
func (s *Service) requireTagInTenantScope(ctx context.Context, actor domain.Actor, tagID uint64) error {
	tag, err := s.tags.GetByID(ctx, tagID)
	if err != nil {
		return err
	}
	if tag.TenantID != nil && !actor.IsSuperadmin() &&
		(actor.TenantID == nil || *actor.TenantID != *tag.TenantID) {
		return domain.ErrForbidden
	}
	return nil
}

func (s *Service) AssignToDevice(ctx context.Context, actor domain.Actor, deviceID, tagID uint64) error {
	if err := s.requireTagInTenantScope(ctx, actor, tagID); err != nil {
		return err
	}
	if err := s.tags.AssignToDevice(ctx, deviceID, tagID); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "ASSIGN_TAG", EntityType: "device", EntityID: &deviceID,
	})
	return nil
}

func (s *Service) RemoveFromDevice(ctx context.Context, actor domain.Actor, deviceID, tagID uint64) error {
	if err := s.requireTagInTenantScope(ctx, actor, tagID); err != nil {
		return err
	}
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
