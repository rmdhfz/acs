package capture

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"acs/internal/domain"
	"acs/pkg/cwmpxml"
)

type fakeRepo struct {
	mu       sync.Mutex
	until    map[uint64]time.Time
	rows     []domain.SessionCapture
	activeQ  int
	insertEr error
}

func newRepo() *fakeRepo { return &fakeRepo{until: map[uint64]time.Time{}} }

func (r *fakeRepo) EnableUntil(_ context.Context, id uint64, t time.Time, _ *uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.until[id] = t
	return nil
}
func (r *fakeRepo) Disable(_ context.Context, id uint64, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.until[id]; ok {
		r.until[id] = now
	}
	return nil
}
func (r *fakeRepo) Status(_ context.Context, id uint64, now time.Time) (*domain.CaptureStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.until[id]
	if !ok {
		return &domain.CaptureStatus{}, nil
	}
	return &domain.CaptureStatus{Active: t.After(now), Until: &t}, nil
}
func (r *fakeRepo) ActiveDeviceIDs(_ context.Context, now time.Time) ([]uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activeQ++
	var ids []uint64
	for id, t := range r.until {
		if t.After(now) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
func (r *fakeRepo) Insert(_ context.Context, c *domain.SessionCapture, _ int, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.insertEr != nil {
		return r.insertEr
	}
	r.rows = append(r.rows, *c)
	return nil
}
func (r *fakeRepo) List(context.Context, uint64, domain.Pagination) ([]domain.SessionCapture, int, error) {
	return r.rows, len(r.rows), nil
}
func (r *fakeRepo) GetByUUID(context.Context, uint64, string) (*domain.SessionCapture, error) {
	return nil, domain.ErrNotFound
}
func (r *fakeRepo) ListAll(context.Context, uint64, int) ([]domain.SessionCapture, error) {
	return r.rows, nil
}

type fakeDevices struct{ tenant uint64 }

func (f fakeDevices) Get(_ context.Context, a domain.Actor, id uint64) (*domain.Device, error) {
	t := f.tenant
	if !a.IsSuperadmin() && (a.TenantID == nil || *a.TenantID != t) {
		return nil, domain.ErrForbidden
	}
	return &domain.Device{ID: id, TenantID: &t}, nil
}

type fakeActivity struct{ actions []string }

func (f *fakeActivity) Record(_ context.Context, l *domain.ActivityLog) error {
	f.actions = append(f.actions, l.Action)
	return nil
}
func (f *fakeActivity) ListByEntity(context.Context, string, uint64, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}
func (f *fakeActivity) ListByTenant(context.Context, *uint64, domain.ActivityLogFilter, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}

func u64(v uint64) *uint64 { return &v }

func newSvc() (*Service, *fakeRepo, *fakeActivity, *time.Time) {
	repo, act := newRepo(), &fakeActivity{}
	s := NewService(repo, fakeDevices{tenant: 7}, nil, act)
	clock := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	return s, repo, act, &clock
}

var admin = domain.Actor{UserID: 1, TenantID: u64(7), Roles: []string{domain.RoleAdmin}}

func TestEnable_BatasRBACDanScopeTenant(t *testing.T) {
	s, _, act, _ := newSvc()
	ctx := context.Background()
	for _, m := range []int{0, -1, 61, 1000} {
		if _, err := s.Enable(ctx, admin, 5, m); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("minutes=%d harus ErrInvalidInput: %v", m, err)
		}
	}
	noc := domain.Actor{UserID: 2, TenantID: u64(7), Roles: []string{domain.RoleNOC}}
	if _, err := s.Enable(ctx, noc, 5, 10); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("NOC tidak boleh menyalakan capture: %v", err)
	}
	other := domain.Actor{UserID: 3, TenantID: u64(8), Roles: []string{domain.RoleAdmin}}
	if _, err := s.Enable(ctx, other, 5, 10); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("ADMIN tenant lain harus ditolak: %v", err)
	}
	st, err := s.Enable(ctx, admin, 5, 60)
	if err != nil || !st.Active || st.Until == nil {
		t.Fatalf("enable 60 menit: %+v %v", st, err)
	}
	if err := s.Disable(ctx, admin, 5); err != nil {
		t.Fatal(err)
	}
	if len(act.actions) != 2 || act.actions[0] != "CAPTURE_ENABLED" || act.actions[1] != "CAPTURE_DISABLED" {
		t.Errorf("audit salah: %v", act.actions)
	}
}

func TestRuntime_AnyActiveActiveDanKedaluwarsa(t *testing.T) {
	s, repo, _, clock := newSvc()
	ctx := context.Background()
	if s.AnyActive(ctx) || s.Active(ctx, 5) {
		t.Fatal("awalnya tidak ada capture aktif")
	}
	if _, err := s.Enable(ctx, admin, 5, 10); err != nil {
		t.Fatal(err)
	}
	if !s.AnyActive(ctx) || !s.Active(ctx, 5) || s.Active(ctx, 6) {
		t.Fatal("perangkat 5 aktif, 6 tidak")
	}
	*clock = clock.Add(11 * time.Minute) // lewat kedaluwarsa
	if s.Active(ctx, 5) || s.AnyActive(ctx) {
		t.Fatal("capture harus berhenti sendiri setelah kedaluwarsa")
	}
	// jalur cepat: banyak panggilan dalam satu jendela cache tidak membanjiri query
	before := repo.activeQ
	for i := 0; i < 50; i++ {
		s.AnyActive(ctx)
	}
	if repo.activeQ-before > 1 {
		t.Fatalf("refresh harus dibatasi cache: %d query", repo.activeQ-before)
	}
	// dimatikan eksplisit
	_, _ = s.Enable(ctx, admin, 5, 10)
	_ = s.Disable(ctx, admin, 5)
	if s.Active(ctx, 5) {
		t.Fatal("Disable harus mematikan seketika")
	}
}

func TestRecord_RahasiaDisamarkanSebelumDisimpan(t *testing.T) {
	s, repo, _, _ := newSvc()
	raw := `<soap:Envelope><soap:Body><cwmp:SetParameterValues><ParameterList>` +
		`<ParameterValueStruct><Name>Device.WiFi.AccessPoint.1.Security.KeyPassphrase</Name><Value xsi:type="xsd:string">rahasia-wifi</Value></ParameterValueStruct>` +
		`<ParameterValueStruct><Name>Device.WiFi.SSID.1.SSID</Name><Value xsi:type="xsd:string">Rumah</Value></ParameterValueStruct>` +
		`</ParameterList></cwmp:SetParameterValues></soap:Body></soap:Envelope>`
	if err := s.Record(context.Background(), 5, "token-sesi-rahasia", false, []byte(raw), 200); err != nil {
		t.Fatal(err)
	}
	if len(repo.rows) != 1 {
		t.Fatal("harus tersimpan satu baris")
	}
	c := repo.rows[0]
	if strings.Contains(c.Body, "rahasia-wifi") || !strings.Contains(c.Body, cwmpxml.RedactedValue) || !strings.Contains(c.Body, ">Rumah<") {
		t.Fatalf("redaksi salah: %s", c.Body)
	}
	if c.RPCName != "SetParameterValues" || c.FromCPE || c.HTTPStatus == nil || *c.HTTPStatus != 200 {
		t.Fatalf("metadata salah: %+v", c)
	}
	if c.SessionRef == nil || len(*c.SessionRef) != 8 || strings.Contains(*c.SessionRef, "token") {
		t.Fatalf("session_ref harus hash pendek, bukan token: %v", c.SessionRef)
	}
	// baris dari CPE tidak membawa status HTTP; body kosong diberi nama "(kosong)"
	_ = s.Record(context.Background(), 5, "", true, nil, 204)
	if e := repo.rows[1]; e.RPCName != "(kosong)" || e.HTTPStatus != nil || e.SessionRef != nil || !e.FromCPE {
		t.Fatalf("baris kosong dari CPE salah: %+v", e)
	}
}

func TestRecord_DipotongDiBatasTanpaMerusakKarakter(t *testing.T) {
	s, repo, _, _ := newSvc()
	big := "<a>" + strings.Repeat("é", domain.CaptureMaxBodyBytes) + "</a>" // 2 byte per karakter
	if err := s.Record(context.Background(), 5, "t", true, []byte(big), 0); err != nil {
		t.Fatal(err)
	}
	c := repo.rows[0]
	if !c.BodyTruncated || len(c.Body) > domain.CaptureMaxBodyBytes || c.BodyBytes != uint32(len(big)) {
		t.Fatalf("pemotongan salah: truncated=%v len=%d bytes=%d", c.BodyTruncated, len(c.Body), c.BodyBytes)
	}
	for _, r := range c.Body {
		if r == '�' {
			t.Fatal("karakter multibyte terpotong di tengah")
		}
	}
}

func TestRecord_GagalSimpanDikembalikanTanpaPanik(t *testing.T) {
	s, repo, _, _ := newSvc()
	repo.insertEr = errors.New("db down")
	if err := s.Record(context.Background(), 5, "t", true, []byte("<a/>"), 0); err == nil {
		t.Fatal("kegagalan harus dikembalikan agar pemanggil bisa mencatatnya")
	}
}
