package fault

import (
	"context"
	"errors"
	"strings"
	"testing"

	"acs/internal/domain"
)

type fakeRepo struct {
	upserted  []domain.DeviceFault
	resolved  [][2]any
	byUUID    map[string]*domain.DeviceFault
	lastList  domain.FaultFilter
	dismissed []uint64
}

func (r *fakeRepo) Upsert(_ context.Context, f *domain.DeviceFault) error {
	r.upserted = append(r.upserted, *f)
	return nil
}
func (r *fakeRepo) ResolveActive(_ context.Context, deviceID uint64, rpc string) (int64, error) {
	r.resolved = append(r.resolved, [2]any{deviceID, rpc})
	return 1, nil
}
func (r *fakeRepo) List(_ context.Context, f domain.FaultFilter, _ domain.Pagination) ([]domain.DeviceFault, int, error) {
	r.lastList = f
	return nil, 0, nil
}
func (r *fakeRepo) GetByUUID(_ context.Context, uuid string) (*domain.DeviceFault, error) {
	if f, ok := r.byUUID[uuid]; ok {
		c := *f
		return &c, nil
	}
	return nil, domain.ErrNotFound
}
func (r *fakeRepo) Dismiss(_ context.Context, id, _ uint64) error {
	r.dismissed = append(r.dismissed, id)
	return nil
}

func u64(v uint64) *uint64 { return &v }

func TestRecord_NormalizesAndTruncates(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, nil)

	long := strings.Repeat("é", domain.FaultStringMaxLen+50) // multibyte: dipangkas per rune, bukan per byte
	if err := s.Record(context.Background(), 7, u64(3), "SET_PARAMETER_VALUES", "  ", long); err != nil {
		t.Fatal(err)
	}
	got := repo.upserted[0]
	if got.FaultCode != "unknown" {
		t.Errorf("kode kosong harus jadi 'unknown', got %q", got.FaultCode)
	}
	if n := len([]rune(got.FaultString)); n != domain.FaultStringMaxLen {
		t.Errorf("fault_string harus dipangkas ke %d rune, got %d", domain.FaultStringMaxLen, n)
	}
	if got.DeviceID != 7 || got.TaskID == nil || *got.TaskID != 3 || got.RPCMethod != "SET_PARAMETER_VALUES" {
		t.Errorf("field tidak terbawa: %+v", got)
	}

	// kode 9005 diterima apa adanya (string standar spec, tidak diubah penulisannya)
	_ = s.Record(context.Background(), 7, nil, "", "9005", "Invalid parameter name")
	if repo.upserted[1].FaultCode != "9005" || repo.upserted[1].TaskID != nil {
		t.Errorf("9005 tanpa task: %+v", repo.upserted[1])
	}
}

func TestResolve_ForwardsDeviceAndRPC(t *testing.T) {
	repo := &fakeRepo{}
	if err := NewService(repo, nil).Resolve(context.Background(), 9, "REBOOT"); err != nil {
		t.Fatal(err)
	}
	if len(repo.resolved) != 1 || repo.resolved[0] != [2]any{uint64(9), "REBOOT"} {
		t.Errorf("resolved = %v", repo.resolved)
	}
}

func TestList_TenantScope(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo, nil)
	ctx := context.Background()

	// ADMIN tenant 5 mencoba melihat tenant 9 lewat query: harus ditimpa ke tenant sendiri
	if _, _, err := s.List(ctx, domain.Actor{TenantID: u64(5), Roles: []string{domain.RoleAdmin}}, domain.FaultFilter{TenantID: u64(9)}, domain.Pagination{}); err != nil {
		t.Fatal(err)
	}
	if repo.lastList.TenantID == nil || *repo.lastList.TenantID != 5 {
		t.Errorf("tenant_id klien harus diabaikan untuk non-superadmin, got %v", repo.lastList.TenantID)
	}

	// non-superadmin tanpa tenant ditolak (bukan 'tanpa filter')
	if _, _, err := s.List(ctx, domain.Actor{Roles: []string{domain.RoleAdmin}}, domain.FaultFilter{}, domain.Pagination{}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("tanpa tenant harus ErrForbidden, got %v", err)
	}

	// superadmin boleh memilih tenant
	if _, _, err := s.List(ctx, domain.Actor{Roles: []string{domain.RoleSuperadmin}}, domain.FaultFilter{TenantID: u64(9)}, domain.Pagination{}); err != nil {
		t.Fatal(err)
	}
	if repo.lastList.TenantID == nil || *repo.lastList.TenantID != 9 {
		t.Errorf("superadmin harus bisa memfilter tenant, got %v", repo.lastList.TenantID)
	}
}

func TestGetAndDismiss_CrossTenantForbidden(t *testing.T) {
	repo := &fakeRepo{byUUID: map[string]*domain.DeviceFault{
		"f-1": {ID: 11, UUID: "f-1", DeviceID: 2, DeviceTenantID: u64(5)},
	}}
	s := NewService(repo, nil)
	ctx := context.Background()
	other := domain.Actor{UserID: 1, TenantID: u64(6), Roles: []string{domain.RoleAdmin}}
	owner := domain.Actor{UserID: 2, TenantID: u64(5), Roles: []string{domain.RoleAdmin}}

	if _, err := s.Get(ctx, other, "f-1"); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("Get lintas tenant harus ErrForbidden, got %v", err)
	}
	if err := s.Dismiss(ctx, other, "f-1"); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("Dismiss lintas tenant harus ErrForbidden, got %v", err)
	}
	if len(repo.dismissed) != 0 {
		t.Fatal("fault tenant lain tidak boleh tertutup")
	}
	if err := s.Dismiss(ctx, owner, "f-1"); err != nil {
		t.Fatal(err)
	}
	if len(repo.dismissed) != 1 || repo.dismissed[0] != 11 {
		t.Errorf("dismissed = %v", repo.dismissed)
	}
	if _, err := s.Get(ctx, owner, "tidak-ada"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("uuid tak dikenal harus ErrNotFound, got %v", err)
	}
}
