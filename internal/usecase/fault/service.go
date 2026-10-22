// Package fault = catatan cwmp:Fault aktif per perangkat (migrations/0038): "perangkat mana yang menolak RPC apa, dan kenapa".
// Dicatat oleh usecase/session saat CPE menjawab RPC dengan Fault, dibersihkan otomatis saat RPC yang sama berhasil, atau
// ditutup manual oleh operator.
package fault

import (
	"context"
	"strings"
	"unicode/utf8"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
)

type Service struct {
	repo     domain.FaultRepository
	activity domain.ActivityLogRepository
}

func NewService(repo domain.FaultRepository, activity domain.ActivityLogRepository) *Service {
	return &Service{repo: repo, activity: activity}
}

// Record mencatat satu kejadian fault dari CPE (dipanggil sistem, bukan pengguna). Teks dari CPE tak tepercaya: dipangkas ke
// batas kolom (per rune), dan fault tanpa kode diberi kode "unknown" supaya tetap terlihat.
func (s *Service) Record(ctx context.Context, deviceID uint64, taskID *uint64, rpcMethod, code, message string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		code = "unknown"
	}
	f := &domain.DeviceFault{
		DeviceID:    deviceID,
		TaskID:      taskID,
		RPCMethod:   truncateRunes(rpcMethod, 64),
		FaultCode:   truncateRunes(code, domain.FaultCodeMaxLen),
		FaultString: truncateRunes(strings.TrimSpace(message), domain.FaultStringMaxLen),
	}
	return s.repo.Upsert(ctx, f)
}

// Resolve menutup fault aktif perangkat untuk RPC itu (RPC yang sama kini berhasil). Tidak ada fault = tidak apa-apa.
func (s *Service) Resolve(ctx context.Context, deviceID uint64, rpcMethod string) error {
	_, err := s.repo.ResolveActive(ctx, deviceID, rpcMethod)
	return err
}

// List — non-superadmin otomatis dibatasi ke tenant-nya (tenant_id dari klien hanya efektif untuk superadmin).
func (s *Service) List(ctx context.Context, actor domain.Actor, f domain.FaultFilter, p domain.Pagination) ([]domain.DeviceFault, int, error) {
	if !actor.IsSuperadmin() {
		tid, err := auth.ScopedTenantFilter(actor)
		if err != nil {
			return nil, 0, err
		}
		f.TenantID = tid
	}
	return s.repo.List(ctx, f, p)
}

// Get: fault milik tenant lain dijawab ErrForbidden (pola yang sama dengan task).
func (s *Service) Get(ctx context.Context, actor domain.Actor, uuid string) (*domain.DeviceFault, error) {
	f, err := s.repo.GetByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireTenantScope(actor, f.DeviceTenantID); err != nil {
		return nil, err
	}
	return f, nil
}

// Dismiss menutup satu fault secara manual (operator sudah menanganinya / menganggapnya tak relevan). Muncul lagi bila CPE
// mengulang fault yang sama.
func (s *Service) Dismiss(ctx context.Context, actor domain.Actor, uuid string) error {
	f, err := s.Get(ctx, actor, uuid)
	if err != nil {
		return err
	}
	by := actor.UserIDPtr()
	if by == nil {
		return domain.ErrForbidden
	}
	if err := s.repo.Dismiss(ctx, f.ID, *by); err != nil {
		return err
	}
	if s.activity != nil {
		id := f.DeviceID
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			UserID: by, TenantID: f.DeviceTenantID, Action: "DISMISS_FAULT", EntityType: "device", EntityID: &id,
			Details: map[string]any{"fault_code": f.FaultCode, "rpc_method": f.RPCMethod, "fault_id": f.UUID},
		})
	}
	return nil
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}
