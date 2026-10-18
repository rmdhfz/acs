package preset

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"acs/internal/domain"
)

// Batas preset. Preset dievaluasi pada SETIAP Inform tiap device tenant (jalur panas CWMP), jadi jumlah operasi,
// panjang kunci/nilai dan pola dibatasi (temuan audit validasi T2). Angka mengikuti kolom/batas ZTP.
const (
	MaxPresetsPerTenant = 100
	maxPresetOps        = 50
	maxPresetKeyLen     = 512
	maxPresetValueLen   = 8192
	maxPresetPatternLen = 255
	maxPresetJSONLen    = 64 << 10
)

type Service struct {
	presets  domain.PresetRepository
	activity domain.ActivityLogRepository
}

func NewService(presets domain.PresetRepository, activity domain.ActivityLogRepository) *Service {
	return &Service{presets: presets, activity: activity}
}

func (s *Service) Create(ctx context.Context, actor domain.Actor, in domain.Preset) (*domain.Preset, error) {
	name, err := domain.CleanText("nama preset", in.Name, 1, 255, false)
	if err != nil {
		return nil, err
	}
	in.Name = name
	if err := validatePresetJSON(&in); err != nil {
		return nil, err
	}
	if _, total, err := s.presets.List(ctx, actor.TenantID, domain.Pagination{Page: 1, PageSize: 1}); err == nil && total >= MaxPresetsPerTenant {
		return nil, fmt.Errorf("%w: batas %d preset per tenant tercapai", domain.ErrQuotaExceeded, MaxPresetsPerTenant)
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
	presets, total, err := s.presets.List(ctx, actor.TenantID, p)
	if err != nil {
		return nil, 0, err
	}
	if !actor.CanSeeSecrets() {
		for i := range presets {
			presets[i].Configurations = domain.RedactPresetConfigurations(presets[i].Configurations)
		}
	}
	return presets, total, nil
}

func (s *Service) Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.Preset, error) {
	preset, err := s.presets.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// Baca: preset milik tenant sendiri atau preset global (tenant_id NULL).
	// Actor non-superadmin tanpa tenant ditolak (bukan nil-deref).
	if preset.TenantID != nil && !actor.IsSuperadmin() &&
		(actor.TenantID == nil || *actor.TenantID != *preset.TenantID) {
		return nil, domain.ErrForbidden
	}
	// NOC/VIEWER boleh melihat preset tetapi tidak nilai rahasianya (password PPPoE/PSK WiFi default).
	if !actor.CanSeeSecrets() {
		preset.Configurations = domain.RedactPresetConfigurations(preset.Configurations)
	}
	return preset, nil
}

// getForMutation — scope untuk MENGUBAH/MENGHAPUS, lebih ketat dari Get:
// preset GLOBAL (tenant_id NULL) berlaku untuk device semua tenant, jadi
// hanya superadmin yang boleh memutasinya. Tanpa ini ADMIN satu tenant bisa
// mengubah/menghapus konfigurasi yang ditegakkan ke device tenant lain.
func (s *Service) getForMutation(ctx context.Context, actor domain.Actor, id uint64) (*domain.Preset, error) {
	preset, err := s.Get(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if preset.TenantID == nil && !actor.IsSuperadmin() {
		return nil, domain.ErrForbidden
	}
	return preset, nil
}

func (s *Service) Update(ctx context.Context, actor domain.Actor, id uint64, in domain.Preset) error {
	preset, err := s.getForMutation(ctx, actor, id)
	if err != nil {
		return err
	}
	name, err := domain.CleanText("nama preset", in.Name, 1, 255, false)
	if err != nil {
		return err
	}
	preset.Name = name
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
	preset, err := s.getForMutation(ctx, actor, id)
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
	if len(p.Precondition) > maxPresetJSONLen || len(p.Configurations) > maxPresetJSONLen {
		return fmt.Errorf("%w: precondition/configurations maksimal %d KB", domain.ErrInvalidInput, maxPresetJSONLen>>10)
	}
	if p.Precondition == "" {
		p.Precondition = "{}"
	}
	if p.Configurations == "" {
		p.Configurations = "[]"
	}
	pc, err := p.ParsedPrecondition()
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
	}
	for field, v := range map[string]*string{
		"oui": pc.OUI, "serial_pattern": pc.SerialPattern, "software_version_pattern": pc.SoftwareVersionPattern,
		"match_parameter_name": pc.MatchParameterName, "match_parameter_value_pattern": pc.MatchParameterValuePattern,
	} {
		if v != nil && (utf8.RuneCountInString(*v) > maxPresetPatternLen || strings.ContainsFunc(*v, unicode.IsControl)) {
			return fmt.Errorf("%w: precondition.%s maksimal %d karakter tanpa karakter kontrol", domain.ErrInvalidInput, field, maxPresetPatternLen)
		}
	}
	ops, err := p.ParsedConfigurations()
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
	}
	if len(ops) > maxPresetOps {
		return fmt.Errorf("%w: configurations maksimal %d operasi", domain.ErrInvalidInput, maxPresetOps)
	}
	for i, op := range ops {
		if utf8.RuneCountInString(op.Key) > maxPresetKeyLen || utf8.RuneCountInString(op.Value) > maxPresetValueLen ||
			strings.ContainsFunc(op.Key, unicode.IsControl) {
			return fmt.Errorf("%w: configurations[%d] key maks. %d / value maks. %d karakter, key tanpa karakter kontrol", domain.ErrInvalidInput, i, maxPresetKeyLen, maxPresetValueLen)
		}
		if op.Op != domain.PresetOpSetParameter {
			return fmt.Errorf("%w: configurations[%d].op %q belum didukung (v1 hanya %q)", domain.ErrInvalidInput, i, op.Op, domain.PresetOpSetParameter)
		}
		if op.Key == "" {
			return fmt.Errorf("%w: configurations[%d] (set_parameter) wajib punya key", domain.ErrInvalidInput, i)
		}
	}
	return nil
}
