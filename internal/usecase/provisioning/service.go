// Package provisioning menangani provisioning profile dan zero-touch
// provisioning (ZTP) — lihat TECH.md §6, FR-13..FR-18.
package provisioning

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
)

type Service struct {
	profiles      domain.ProvisioningProfileRepository
	profileParams domain.ProvisioningProfileParameterRepository
	rules         domain.ZeroTouchRuleRepository
	devices       domain.DeviceRepository
	enqueuer      domain.TaskEnqueuer
	activity      domain.ActivityLogRepository
}

func NewService(
	profiles domain.ProvisioningProfileRepository,
	profileParams domain.ProvisioningProfileParameterRepository,
	rules domain.ZeroTouchRuleRepository,
	devices domain.DeviceRepository,
	enqueuer domain.TaskEnqueuer,
	activity domain.ActivityLogRepository,
) *Service {
	return &Service{
		profiles: profiles, profileParams: profileParams, rules: rules,
		devices: devices, enqueuer: enqueuer, activity: activity,
	}
}

// requireProfileReadScope mengizinkan baca/terapkan profile milik tenant
// sendiri ATAU profile global (tenant_id NULL, dibuat superadmin sbg default
// lintas tenant — FR-17, konsisten dgn provisioningProfileRepository.List()
// yang juga menampilkan tenant_id NULL ke semua tenant). Beda dari
// auth.RequireTenantScope di bawah (dipakai utk MUTASI profile — Create/
// Update/Delete) yang sengaja menolak tenant_id nil utk non-superadmin:
// profile global cuma boleh DIEDIT/DIHAPUS superadmin (supaya tidak ada satu
// tenant diam-diam mengubah default bersama), tapi boleh DIBACA/DITERAPKAN
// semua tenant.
func requireProfileReadScope(actor domain.Actor, resourceTenantID *uint64) error {
	if actor.IsSuperadmin() || resourceTenantID == nil {
		return nil
	}
	if actor.TenantID == nil || *actor.TenantID != *resourceTenantID {
		return domain.ErrForbidden
	}
	return nil
}

// ---- Provisioning Profile CRUD (FR-16/FR-17) ----

type CreateProfileInput struct {
	TenantID      *uint64
	VendorID      *uint64
	DeviceModelID *uint64
	Name          string
	Description   *string
	IsDefault     bool
	Parameters    []domain.ProvisioningProfileParameter
}

func (s *Service) CreateProfile(ctx context.Context, actor domain.Actor, in CreateProfileInput) (*domain.ProvisioningProfile, error) {
	if err := auth.RequireTenantScope(actor, in.TenantID); err != nil {
		return nil, err
	}
	p := &domain.ProvisioningProfile{
		ProfileUUID:   uuid.NewString(),
		TenantID:      in.TenantID,
		VendorID:      in.VendorID,
		DeviceModelID: in.DeviceModelID,
		Name:          in.Name,
		Description:   in.Description,
		IsDefault:     in.IsDefault,
		IsActive:      true,
		Audit:         domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if err := s.profiles.Create(ctx, p); err != nil {
		return nil, err
	}
	if len(in.Parameters) > 0 {
		if err := s.profileParams.Replace(ctx, p.ID, in.Parameters); err != nil {
			return nil, err
		}
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "CREATE_PROVISIONING_PROFILE", EntityType: "provisioning_profile", EntityID: &p.ID,
	})
	return p, nil
}

func (s *Service) Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.ProvisioningProfile, []domain.ProvisioningProfileParameter, error) {
	p, err := s.profiles.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if err := requireProfileReadScope(actor, p.TenantID); err != nil {
		return nil, nil, err
	}
	params, err := s.profileParams.ListByProfile(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return p, params, nil
}

// List — queryTenantID datang dari query param `tenant_id`, cuma dipakai
// kalau actor superadmin (filter opsional lintas tenant). Actor non-superadmin
// SELALU dipaksa ke tenant-nya sendiri, mengabaikan queryTenantID — mencegah
// spoofing lewat query param, dan menolak (bukan diam-diam pass-through nil)
// kalau actor.TenantID kosong (lihat auth.ScopedTenantFilter).
func (s *Service) List(ctx context.Context, actor domain.Actor, queryTenantID *uint64, p domain.Pagination) ([]domain.ProvisioningProfile, int, error) {
	tenantID := queryTenantID
	if !actor.IsSuperadmin() {
		tid, err := auth.ScopedTenantFilter(actor)
		if err != nil {
			return nil, 0, err
		}
		tenantID = tid
	}
	return s.profiles.List(ctx, tenantID, p)
}

func (s *Service) UpdateProfile(ctx context.Context, actor domain.Actor, p *domain.ProvisioningProfile, params []domain.ProvisioningProfileParameter) error {
	existing, err := s.profiles.GetByID(ctx, p.ID)
	if err != nil {
		return err
	}
	if err := auth.RequireTenantScope(actor, existing.TenantID); err != nil {
		return err
	}
	p.UpdatedBy = actor.UserIDPtr()
	if err := s.profiles.Update(ctx, p); err != nil {
		return err
	}
	if params != nil {
		if err := s.profileParams.Replace(ctx, p.ID, params); err != nil {
			return err
		}
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "UPDATE_PROVISIONING_PROFILE", EntityType: "provisioning_profile", EntityID: &p.ID,
	})
	return nil
}

func (s *Service) DeleteProfile(ctx context.Context, actor domain.Actor, id uint64) error {
	p, err := s.profiles.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := auth.RequireTenantScope(actor, p.TenantID); err != nil {
		return err
	}
	if err := s.profiles.SoftDelete(ctx, id, actor.UserID); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "DELETE_PROVISIONING_PROFILE", EntityType: "provisioning_profile", EntityID: &id,
	})
	return nil
}

// ApplyProfile mengantre task SetParameterValues berisi seluruh parameter
// profile ke device tertentu. Pemanggilan SELALU eksplisit (manual atau dari
// EvaluateZeroTouch) — perubahan pada profile TIDAK otomatis mendorong ulang
// ke device yang sudah terprovisioning (FR-18; keputusan produk yang
// disengaja per CLAUDE.md, jangan diotomatisasi tanpa konfirmasi eksplisit).
func (s *Service) ApplyProfile(ctx context.Context, actor domain.Actor, deviceID, profileID uint64) (*domain.Task, error) {
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireTenantScope(actor, dev.TenantID); err != nil {
		return nil, err
	}
	profile, err := s.profiles.GetByID(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if err := requireProfileReadScope(actor, profile.TenantID); err != nil {
		return nil, err
	}
	params, err := s.profileParams.ListByProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if len(params) == 0 {
		return nil, fmt.Errorf("provisioning: profile %d tidak punya parameter untuk diterapkan", profileID)
	}
	values := make(map[string]string, len(params))
	for _, pp := range params {
		val := ""
		if pp.ParameterValue != nil {
			val = *pp.ParameterValue
		}
		values[pp.ParameterName] = val
	}
	t, err := s.enqueuer.EnqueueSetParameterValues(ctx, actor, deviceID, values, 3)
	if err != nil {
		return nil, err
	}

	dev.ProvisioningProfileID = &profileID
	dev.UpdatedBy = actor.UserIDPtr()
	if err := s.devices.Update(ctx, dev); err != nil {
		return nil, err
	}

	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "APPLY_PROVISIONING_PROFILE", EntityType: "device", EntityID: &deviceID,
	})
	return t, nil
}

// ---- Zero-Touch Provisioning (TECH.md §6, FR-13/14/15) ----

// EvaluateZeroTouch dipanggil usecase/session saat event 0 BOOTSTRAP diterima
// dari device yang belum punya provisioning_profile_id. Rule pertama yang
// cocok (priority ASC) langsung diterapkan. Bila tidak ada yang cocok,
// mengembalikan domain.ErrNoMatchingRule — device TETAP di inventory
// menunggu tindakan manual, tidak silently diabaikan (FR-15).
func (s *Service) EvaluateZeroTouch(ctx context.Context, actor domain.Actor, dev *domain.Device) (*domain.ZeroTouchRule, error) {
	if dev.ProvisioningProfileID != nil {
		return nil, nil
	}
	rules, err := s.rules.ListActiveOrdered(ctx, dev.TenantID)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		r := &rules[i]
		if !matchZeroTouchRule(r, dev) {
			continue
		}
		if _, err := s.ApplyProfile(ctx, actor, dev.ID, r.ProvisioningProfileID); err != nil {
			// Rule COCOK tapi apply GAGAL (mis. kuota task queue tenant
			// penuh, ROADMAP.md Fase 2) HARUS tetap kelihatan operator/NOC --
			// FR-15 eksplisit bilang device tidak boleh silently diabaikan.
			// Sebelum ada kuota task queue, error di titik ini nyaris tidak
			// pernah terjadi; sekarang jalur ini nyata bisa kena, jadi wajib
			// dicatat, bukan cuma dikembalikan sbg error yang lalu ditelan
			// caller (usecase/session sengaja tidak menggagalkan Inform demi
			// event lain, lihat komentar di sana).
			desc := err.Error()
			_ = s.activity.Record(ctx, &domain.ActivityLog{
				TenantID: dev.TenantID, Action: "ZERO_TOUCH_APPLY_FAILED", EntityType: "device", EntityID: &dev.ID,
				Description: &desc,
			})
			return nil, err
		}
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			TenantID: dev.TenantID, Action: "ZERO_TOUCH_MATCH", EntityType: "device", EntityID: &dev.ID,
		})
		return r, nil
	}
	return nil, domain.ErrNoMatchingRule
}

func matchZeroTouchRule(r *domain.ZeroTouchRule, dev *domain.Device) bool {
	if r.VendorID != nil {
		if dev.VendorID == nil || *r.VendorID != *dev.VendorID {
			return false
		}
	}
	if r.DeviceModelID != nil {
		if dev.DeviceModelID == nil || *r.DeviceModelID != *dev.DeviceModelID {
			return false
		}
	}
	if r.OUI != nil {
		if dev.OUI == nil || !strings.EqualFold(*r.OUI, *dev.OUI) {
			return false
		}
	}
	if r.SerialPattern != nil {
		if !matchSQLLike(*r.SerialPattern, dev.SerialNumber) {
			return false
		}
	}
	return true
}

// matchSQLLike meniru semantik SQL LIKE (% dan _) di memori — jumlah rule ZTP
// kecil dan dievaluasi sekali per event BOOTSTRAP, jadi tidak perlu query per rule.
func matchSQLLike(pattern, s string) bool {
	re := "^" + regexp.QuoteMeta(pattern) + "$"
	re = strings.ReplaceAll(re, `\%`, ".*")
	re = strings.ReplaceAll(re, `\_`, ".")
	compiled, err := regexp.Compile("(?is)" + re)
	if err != nil {
		return false
	}
	return compiled.MatchString(s)
}

// ListZeroTouchRules — pola sama seperti List (profile) di atas: queryTenantID
// cuma berlaku utk superadmin, non-superadmin selalu dipaksa ke tenant sendiri
// dan ditolak (bukan fail-open) kalau actor.TenantID kosong.
func (s *Service) ListZeroTouchRules(ctx context.Context, actor domain.Actor, queryTenantID *uint64) ([]domain.ZeroTouchRule, error) {
	tenantID := queryTenantID
	if !actor.IsSuperadmin() {
		tid, err := auth.ScopedTenantFilter(actor)
		if err != nil {
			return nil, err
		}
		tenantID = tid
	}
	return s.rules.ListActiveOrdered(ctx, tenantID)
}

func (s *Service) CreateZeroTouchRule(ctx context.Context, actor domain.Actor, r *domain.ZeroTouchRule) error {
	if err := auth.RequireTenantScope(actor, r.TenantID); err != nil {
		return err
	}
	r.CreatedBy = actor.UserIDPtr()
	if err := s.rules.Create(ctx, r); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "CREATE_ZERO_TOUCH_RULE", EntityType: "zero_touch_rule", EntityID: &r.ID,
	})
	return nil
}

func (s *Service) UpdateZeroTouchRule(ctx context.Context, actor domain.Actor, r *domain.ZeroTouchRule) error {
	existing, err := s.rules.GetByID(ctx, r.ID)
	if err != nil {
		return err
	}
	if err := auth.RequireTenantScope(actor, existing.TenantID); err != nil {
		return err
	}
	r.UpdatedBy = actor.UserIDPtr()
	if err := s.rules.Update(ctx, r); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "UPDATE_ZERO_TOUCH_RULE", EntityType: "zero_touch_rule", EntityID: &r.ID,
	})
	return nil
}

func (s *Service) DeleteZeroTouchRule(ctx context.Context, actor domain.Actor, id uint64) error {
	r, err := s.rules.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := auth.RequireTenantScope(actor, r.TenantID); err != nil {
		return err
	}
	if err := s.rules.SoftDelete(ctx, id, actor.UserID); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "DELETE_ZERO_TOUCH_RULE", EntityType: "zero_touch_rule", EntityID: &id,
	})
	return nil
}
