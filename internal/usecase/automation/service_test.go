package automation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"acs/internal/domain"
)

// ---- fakes ----

type fakeRules struct {
	rows   []*domain.AutomationRule
	nextID uint64
}

func (r *fakeRules) Create(_ context.Context, x *domain.AutomationRule, _ *uint64) error {
	r.nextID++
	x.ID, x.UUID = r.nextID, "uuid-"+strings.Repeat("x", int(r.nextID))
	c := *x
	r.rows = append(r.rows, &c)
	return nil
}
func (r *fakeRules) GetByUUID(_ context.Context, id string) (*domain.AutomationRule, error) {
	for _, v := range r.rows {
		if v.UUID == id {
			c := *v
			return &c, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *fakeRules) List(_ context.Context, tenantID *uint64, _ domain.Pagination) ([]domain.AutomationRule, int, error) {
	var out []domain.AutomationRule
	for _, v := range r.rows {
		if tenantID == nil || v.TenantID == *tenantID {
			out = append(out, *v)
		}
	}
	return out, len(out), nil
}
func (r *fakeRules) ListActive(_ context.Context, tenantID uint64) ([]domain.AutomationRule, error) {
	var out []domain.AutomationRule
	for _, v := range r.rows {
		if v.TenantID == tenantID && v.IsActive {
			out = append(out, *v)
		}
	}
	return out, nil
}
func (r *fakeRules) FindByName(_ context.Context, tenantID uint64, name string) (*domain.AutomationRule, error) {
	for _, v := range r.rows {
		if v.TenantID == tenantID && v.Name == name {
			c := *v
			return &c, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *fakeRules) CountInTenant(_ context.Context, tenantID uint64) (int, error) {
	n := 0
	for _, v := range r.rows {
		if v.TenantID == tenantID {
			n++
		}
	}
	return n, nil
}
func (r *fakeRules) Update(_ context.Context, x *domain.AutomationRule, _ *uint64) error {
	for i, v := range r.rows {
		if v.ID == x.ID {
			c := *x
			r.rows[i] = &c
		}
	}
	return nil
}
func (r *fakeRules) SoftDelete(_ context.Context, id, _ uint64) error {
	for i, v := range r.rows {
		if v.ID == id {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

type fakeStates struct {
	m      map[[2]uint64]domain.AutomationState
	resets int
}

func (s *fakeStates) ListByDevice(_ context.Context, deviceID uint64) (map[uint64]domain.AutomationState, error) {
	out := map[uint64]domain.AutomationState{}
	for k, v := range s.m {
		if k[1] == deviceID {
			out[k[0]] = v
		}
	}
	return out, nil
}
func (s *fakeStates) Save(_ context.Context, st *domain.AutomationState) error {
	s.m[[2]uint64{st.RuleID, st.DeviceID}] = *st
	return nil
}
func (s *fakeStates) ResetConsecutive(_ context.Context, ruleID, deviceID uint64) error {
	k := [2]uint64{ruleID, deviceID}
	if st, ok := s.m[k]; ok {
		st.ConsecutiveFires = 0
		s.m[k] = st
		s.resets++
	}
	return nil
}
func (s *fakeStates) ClearSuspension(_ context.Context, ruleID uint64, deviceID *uint64) (int64, error) {
	var n int64
	for k, v := range s.m {
		if k[0] == ruleID && v.Suspended && (deviceID == nil || *deviceID == k[1]) {
			v.Suspended, v.ConsecutiveFires = false, 0
			s.m[k] = v
			n++
		}
	}
	return n, nil
}

type fakeRuns struct{ rows []domain.AutomationRun }

func (r *fakeRuns) Insert(_ context.Context, x *domain.AutomationRun) error {
	r.rows = append(r.rows, *x)
	return nil
}
func (r *fakeRuns) List(context.Context, domain.AutomationRunFilter, domain.Pagination) ([]domain.AutomationRun, int, error) {
	return r.rows, len(r.rows), nil
}

type fakeParams struct {
	domain.DeviceParameterRepository
	vals     map[uint64]map[string]string
	valsCall int
}

func (p *fakeParams) ValuesFor(_ context.Context, ids []uint64, names []string) (map[uint64]map[string]string, error) {
	p.valsCall++
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	out := map[uint64]map[string]string{}
	for _, id := range ids {
		m := map[string]string{}
		for k, v := range p.vals[id] {
			if want[k] {
				m[k] = v
			}
		}
		out[id] = m
	}
	return out, nil
}
func (p *fakeParams) Get(_ context.Context, id uint64, name string) (*domain.DeviceParameter, error) {
	if v, ok := p.vals[id][name]; ok {
		return &domain.DeviceParameter{DeviceID: id, ParameterName: name, ParameterValue: &v}, nil
	}
	return nil, domain.ErrNotFound
}

type fakeTasks struct {
	resolves int
	pending  bool
	sets     []map[string]string
	reboots  int
	gets     [][]string
	enqErr   error
	pendingN int
}

func (t *fakeTasks) EnqueueSetParameterValues(_ context.Context, _ domain.Actor, _ uint64, p map[string]string, _ uint8) (*domain.Task, error) {
	if t.enqErr != nil {
		return nil, t.enqErr
	}
	t.sets = append(t.sets, p)
	return &domain.Task{}, nil
}
func (t *fakeTasks) EnqueueGetParameterNames(context.Context, domain.Actor, uint64, string, bool, uint8) (*domain.Task, error) {
	return &domain.Task{}, nil
}
func (t *fakeTasks) EnqueueReboot(context.Context, domain.Actor, uint64, uint8) (*domain.Task, error) {
	if t.enqErr != nil {
		return nil, t.enqErr
	}
	t.reboots++
	return &domain.Task{}, nil
}
func (t *fakeTasks) HasPendingForDevice(context.Context, uint64) (bool, error) {
	t.pendingN++
	return t.pending, nil
}
func (t *fakeTasks) ResolveParameterPath(_ context.Context, _ uint64, key string) (string, error) {
	t.resolves++
	if key == "wifi.ssid" { // logical key -> raw path
		return "InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID", nil
	}
	if key == "tak.dikenal" {
		return "", errors.New("belum dipetakan")
	}
	return key, nil
}
func (t *fakeTasks) CreateTask(_ context.Context, _ domain.Actor, in domain.CreateTaskInput) (*domain.Task, error) {
	if in.TaskType == domain.TaskTypeGetParameterValues {
		t.gets = append(t.gets, in.Parameters["names"].([]string))
	}
	return &domain.Task{}, nil
}

type fakeTags struct {
	domain.TagRepository
	tags     map[uint64]*domain.Tag
	onDevice map[uint64]bool
	assigned []uint64
}

func (t *fakeTags) GetByID(_ context.Context, id uint64) (*domain.Tag, error) {
	if v, ok := t.tags[id]; ok {
		return v, nil
	}
	return nil, domain.ErrNotFound
}
func (t *fakeTags) ListByDevice(context.Context, uint64) ([]domain.Tag, error) {
	var out []domain.Tag
	for id := range t.onDevice {
		out = append(out, domain.Tag{ID: id})
	}
	return out, nil
}
func (t *fakeTags) AssignToDevice(_ context.Context, _ uint64, tagID uint64) error {
	t.assigned = append(t.assigned, tagID)
	t.onDevice[tagID] = true
	return nil
}

type fakeHooks struct{ events []string }

func (h *fakeHooks) Enqueue(_ context.Context, code string, _ *uint64, _ any) error {
	h.events = append(h.events, code)
	return nil
}

type fakeDevices struct{ devs map[uint64]*domain.Device }

func (d *fakeDevices) Get(_ context.Context, actor domain.Actor, id uint64) (*domain.Device, error) {
	dev, ok := d.devs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if !actor.IsSuperadmin() && (actor.TenantID == nil || dev.TenantID == nil || *actor.TenantID != *dev.TenantID) {
		return nil, domain.ErrForbidden
	}
	return dev, nil
}

// ---- rig ----

type rig struct {
	svc    *Service
	rules  *fakeRules
	states *fakeStates
	runs   *fakeRuns
	params *fakeParams
	tasks  *fakeTasks
	tags   *fakeTags
	hooks  *fakeHooks
	clock  time.Time
}

func u64(v uint64) *uint64 { return &v }
func sp(s string) *string  { return &s }

const (
	ssidPath = "InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID"
	rxPath   = "InternetGatewayDevice.X_ZTE.OpticalRx"
)

func newRig() *rig {
	g := &rig{
		rules: &fakeRules{}, states: &fakeStates{m: map[[2]uint64]domain.AutomationState{}}, runs: &fakeRuns{},
		params: &fakeParams{vals: map[uint64]map[string]string{1: {rxPath: "-28000", ssidPath: "Lama"}}},
		tasks:  &fakeTasks{},
		tags:   &fakeTags{tags: map[uint64]*domain.Tag{5: {ID: 5, TenantID: u64(7)}, 6: {ID: 6, TenantID: u64(8)}}, onDevice: map[uint64]bool{}},
		hooks:  &fakeHooks{}, clock: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC),
	}
	devs := &fakeDevices{devs: map[uint64]*domain.Device{
		1: {ID: 1, TenantID: u64(7), SerialNumber: "ZTEG0001", OUI: sp("D0154A")},
		2: {ID: 2, TenantID: u64(8), SerialNumber: "OTHER"},
	}}
	g.svc = NewService(g.rules, g.states, g.runs, devs, g.params, g.tasks, g.tags, g.hooks, nil)
	g.svc.now = func() time.Time { return g.clock }
	return g
}

var (
	adminA = domain.Actor{UserID: 1, TenantID: u64(7), Roles: []string{domain.RoleAdmin}}
	adminB = domain.Actor{UserID: 2, TenantID: u64(8), Roles: []string{domain.RoleAdmin}}
	super  = domain.Actor{UserID: 3, Roles: []string{domain.RoleSuperadmin}}
)

func (g *rig) mk(t *testing.T, mod func(*Input)) *domain.AutomationRule {
	t.Helper()
	in := Input{Name: "r" + string(rune('a'+len(g.rules.rows))), TriggerEvent: domain.ZtpTriggerEventEveryInform,
		Condition: "{" + rxPath + "} < -27000", Actions: []domain.AutomationAction{{Type: domain.AutomationActionReboot}}, CooldownSeconds: 600, MaxConsecutiveFires: 2}
	if mod != nil {
		mod(&in)
	}
	r, err := g.svc.Create(context.Background(), adminA, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return r
}

func dev1() *domain.Device {
	return &domain.Device{ID: 1, TenantID: u64(7), SerialNumber: "ZTEG0001", OUI: sp("D0154A")}
}

// ---- validasi ----

func TestCreate_Validasi(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	base := Input{Name: "n", TriggerEvent: "EVERY_INFORM", Condition: "1", Actions: []domain.AutomationAction{{Type: "reboot"}}}
	bad := map[string]func(*Input){
		"trigger tak dikenal":   func(i *Input) { i.TriggerEvent = "SELALU" },
		"kondisi salah sintaks": func(i *Input) { i.Condition = "1 +" },
		"kondisi rahasia":       func(i *Input) { i.Condition = "{Device.WiFi.AccessPoint.1.Security.KeyPassphrase} == 'x'" },
		"konteks tak dikenal":   func(i *Input) { i.Condition = "{@rahasia} == 1" },
		"tanpa aksi":            func(i *Input) { i.Actions = nil },
		"aksi tak dikenal":      func(i *Input) { i.Actions = []domain.AutomationAction{{Type: "exec"}} },
		"reboot dgn field":      func(i *Input) { i.Actions = []domain.AutomationAction{{Type: "reboot", Name: "x"}} },
		"set tanpa value":       func(i *Input) { i.Actions = []domain.AutomationAction{{Type: "set_parameter", Name: "wifi.ssid"}} },
		"set value salah": func(i *Input) {
			i.Actions = []domain.AutomationAction{{Type: "set_parameter", Name: "wifi.ssid", Value: "1 +"}}
		},
		"set target rahasia": func(i *Input) {
			i.Actions = []domain.AutomationAction{{Type: "set_parameter", Name: "InternetGatewayDevice.X.KeyPassphrase", Value: "'rahasia'"}}
		},
		"set nama buruk": func(i *Input) {
			i.Actions = []domain.AutomationAction{{Type: "set_parameter", Name: "a b", Value: "1"}}
		},
		"get tanpa names":         func(i *Input) { i.Actions = []domain.AutomationAction{{Type: "get_parameter_values"}} },
		"tag tak ada":             func(i *Input) { i.Actions = []domain.AutomationAction{{Type: "add_tag", TagID: u64(99)}} },
		"tag tenant lain":         func(i *Input) { i.Actions = []domain.AutomationAction{{Type: "add_tag", TagID: u64(6)}} },
		"cooldown terlalu pendek": func(i *Input) { i.CooldownSeconds = 10 },
		"max fires kebesaran":     func(i *Input) { i.MaxConsecutiveFires = 200 },
	}
	for name, mod := range bad {
		in := base
		mod(&in)
		if _, err := g.svc.Create(ctx, adminA, in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s harus ErrInvalidInput, got %v", name, err)
		}
	}
	many := make([]domain.AutomationAction, domain.AutomationMaxActions+1)
	for i := range many {
		many[i] = domain.AutomationAction{Type: "reboot"}
	}
	if _, err := g.svc.Create(ctx, adminA, Input{Name: "n", TriggerEvent: "EVERY_INFORM", Actions: many}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("terlalu banyak aksi harus ditolak: %v", err)
	}
	// valid: semua jenis aksi, tag milik tenant sendiri
	ok := base
	ok.Actions = []domain.AutomationAction{
		{Type: "set_parameter", Name: "wifi.ssid", Value: "concat('ACS-', {@serial_number})"},
		{Type: "get_parameter_values", Names: []string{rxPath}}, {Type: "add_tag", TagID: u64(5)}, {Type: "webhook"}, {Type: "reboot"},
	}
	r, err := g.svc.Create(ctx, adminA, ok)
	if err != nil || r.CooldownSeconds != domain.AutomationDefaultCooldown || r.MaxConsecutiveFires != domain.AutomationDefaultMaxFires || !r.IsActive || r.TenantID != 7 {
		t.Fatalf("valid: %+v %v", r, err)
	}
	if _, err := g.svc.Create(ctx, adminA, ok); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("nama ganda harus ErrConflict: %v", err)
	}
	// SUPERADMIN wajib menyebut tenant; ADMIN tanpa tenant ditolak
	if _, err := g.svc.Create(ctx, super, Input{Name: "s", TriggerEvent: "EVERY_INFORM", Actions: []domain.AutomationAction{{Type: "reboot"}}}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("superadmin tanpa tenant_id harus ErrInvalidInput: %v", err)
	}
	if _, err := g.svc.Create(ctx, domain.Actor{UserID: 9, Roles: []string{domain.RoleAdmin}}, ok); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("admin tanpa tenant harus ErrForbidden: %v", err)
	}
	// tenant_id klien diabaikan untuk ADMIN
	other := ok
	other.Name, other.TenantID = "lain", u64(8)
	if r, err := g.svc.Create(ctx, adminA, other); err != nil || r.TenantID != 7 {
		t.Errorf("tenant_id klien harus diabaikan untuk ADMIN: %+v %v", r, err)
	}
}

func TestScope_LintasTenant(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	r := g.mk(t, nil)
	if _, err := g.svc.Get(ctx, adminB, r.UUID); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("Get lintas tenant: %v", err)
	}
	if _, err := g.svc.Update(ctx, adminB, r.UUID, UpdateInput{Name: sp("hack")}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("Update lintas tenant: %v", err)
	}
	if err := g.svc.Delete(ctx, adminB, r.UUID); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("Delete lintas tenant: %v", err)
	}
	if _, err := g.svc.ResetSuspension(ctx, adminB, r.UUID, nil); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("Reset lintas tenant: %v", err)
	}
	rows, _, _ := g.svc.List(ctx, adminB, u64(7), domain.Pagination{})
	if len(rows) != 0 {
		t.Errorf("List bocor tenant lain: %+v", rows)
	}
	if _, err := g.svc.Test(ctx, adminB, TestInput{Actions: []domain.AutomationAction{{Type: "reboot"}}}, 1); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("Test pada perangkat tenant lain: %v", err)
	}
}

func TestUpdate_ParsialDanValidasiUlang(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	r := g.mk(t, nil)
	got, err := g.svc.Update(ctx, adminA, r.UUID, UpdateInput{Condition: sp("{" + rxPath + "} < -20000"), IsActive: new(bool)})
	if err != nil || got.Condition != "{"+rxPath+"} < -20000" || got.IsActive || got.Name != r.Name || len(got.Actions) != 1 {
		t.Fatalf("update parsial: %+v %v", got, err)
	}
	if _, err := g.svc.Update(ctx, adminA, r.UUID, UpdateInput{Condition: sp("1 +")}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("kondisi salah harus ditolak: %v", err)
	}
	bad := []domain.AutomationAction{{Type: "set_parameter", Name: "InternetGatewayDevice.X.Password", Value: "'x'"}}
	if _, err := g.svc.Update(ctx, adminA, r.UUID, UpdateInput{Actions: &bad}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("aksi rahasia harus ditolak: %v", err)
	}
	if _, err := g.svc.Update(ctx, adminA, r.UUID, UpdateInput{CooldownSeconds: new(uint32)}); err != nil {
		t.Errorf("cooldown 0 = default, bukan galat: %v", err) // 0 -> bawaan 3600
	}
}

// ---- evaluasi + pagar ----

func TestEvaluate_MemicuRebootLaluCooldown(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	g.mk(t, nil) // kondisi benar (rx -28000 < -27000)
	if err := g.svc.Evaluate(ctx, dev1(), []string{"2 PERIODIC"}); err != nil {
		t.Fatal(err)
	}
	if g.tasks.reboots != 1 || len(g.runs.rows) != 1 || !g.runs.rows[0].Fired {
		t.Fatalf("harus reboot 1x + 1 catatan: reboots=%d runs=%+v", g.tasks.reboots, g.runs.rows)
	}
	// Inform berikutnya di dalam cooldown: tidak memicu lagi
	g.clock = g.clock.Add(5 * time.Minute)
	_ = g.svc.Evaluate(ctx, dev1(), []string{"2 PERIODIC"})
	if g.tasks.reboots != 1 {
		t.Fatalf("cooldown 600s dilanggar: reboots=%d", g.tasks.reboots)
	}
	// setelah cooldown: memicu lagi (pemicuan ke-2)
	g.clock = g.clock.Add(10 * time.Minute)
	_ = g.svc.Evaluate(ctx, dev1(), []string{"2 PERIODIC"})
	if g.tasks.reboots != 2 {
		t.Fatalf("setelah cooldown harus memicu: reboots=%d", g.tasks.reboots)
	}
}

func TestEvaluate_SuspendSetelahBerturutTanpaKonvergen(t *testing.T) {
	g := newRig() // max_consecutive_fires = 2
	ctx := context.Background()
	rule := g.mk(t, nil)
	for i := 0; i < 5; i++ { // kondisi tak pernah salah: perangkat tak pernah membaik
		_ = g.svc.Evaluate(ctx, dev1(), []string{"2 PERIODIC"})
		g.clock = g.clock.Add(11 * time.Minute)
	}
	if g.tasks.reboots != 2 {
		t.Fatalf("hanya 2 pemicuan sebelum suspend, got %d", g.tasks.reboots)
	}
	st := g.states.m[[2]uint64{rule.ID, 1}]
	if !st.Suspended {
		t.Fatalf("aturan harus di-suspend: %+v", st)
	}
	var sus int
	for _, r := range g.runs.rows {
		if r.Suspended {
			sus++
			if r.Error == nil || !strings.Contains(*r.Error, "di-suspend") {
				t.Errorf("catatan suspend harus menjelaskan: %+v", r)
			}
		}
	}
	if sus != 1 {
		t.Errorf("tepat 1 catatan suspend, got %d (%+v)", sus, g.runs.rows)
	}
	// operator mereset -> memicu lagi
	if n, err := g.svc.ResetSuspension(ctx, adminA, rule.UUID, nil); err != nil || n != 1 {
		t.Fatalf("reset: %d %v", n, err)
	}
	_ = g.svc.Evaluate(ctx, dev1(), []string{"2 PERIODIC"})
	if g.tasks.reboots != 3 {
		t.Errorf("setelah reset harus memicu lagi: %d", g.tasks.reboots)
	}
}

func TestEvaluate_KondisiSalahMenolkanHitungan(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	rule := g.mk(t, nil)
	_ = g.svc.Evaluate(ctx, dev1(), nil) // memicu (1)
	if g.states.m[[2]uint64{rule.ID, 1}].ConsecutiveFires != 1 {
		t.Fatalf("hitungan 1 diharapkan: %+v", g.states.m)
	}
	// perangkat membaik: kondisi salah -> hitungan direset
	g.params.vals[1][rxPath] = "-20000"
	g.clock = g.clock.Add(time.Hour)
	_ = g.svc.Evaluate(ctx, dev1(), nil)
	if g.states.m[[2]uint64{rule.ID, 1}].ConsecutiveFires != 0 {
		t.Errorf("kondisi salah harus menolkan hitungan: %+v", g.states.m)
	}
}

func TestEvaluate_PagarTaskTertunda(t *testing.T) {
	g := newRig()
	g.mk(t, nil)
	g.tasks.pending = true
	_ = g.svc.Evaluate(context.Background(), dev1(), nil)
	if g.tasks.reboots != 0 || len(g.runs.rows) != 0 {
		t.Fatalf("perangkat dengan task tertunda tidak boleh dipicu: %d %+v", g.tasks.reboots, g.runs.rows)
	}
	if len(g.states.m) != 0 {
		t.Errorf("tidak boleh menyentuh state: %+v", g.states.m)
	}
}

func TestEvaluate_TanpaAturanNolQuery(t *testing.T) {
	g := newRig()
	_ = g.svc.Evaluate(context.Background(), dev1(), nil)
	if g.tasks.pendingN != 0 || g.params.valsCall != 0 {
		t.Errorf("tanpa aturan tidak boleh query apa pun: pending=%d vals=%d", g.tasks.pendingN, g.params.valsCall)
	}
	// perangkat tanpa tenant: tidak berlaku
	g.mk(t, nil)
	if err := g.svc.Evaluate(context.Background(), &domain.Device{ID: 9}, nil); err != nil || g.tasks.reboots != 0 {
		t.Errorf("perangkat tanpa tenant: %v %d", err, g.tasks.reboots)
	}
}

func TestEvaluate_Trigger(t *testing.T) {
	g := newRig()
	g.mk(t, func(i *Input) { i.TriggerEvent = domain.ZtpTriggerEventBootstrapOnly; i.Name = "boot-only" })
	_ = g.svc.Evaluate(context.Background(), dev1(), []string{"2 PERIODIC"})
	if g.tasks.reboots != 0 {
		t.Fatal("BOOTSTRAP_ONLY tidak boleh memicu pada PERIODIC")
	}
	_ = g.svc.Evaluate(context.Background(), dev1(), []string{"0 BOOTSTRAP", "1 BOOT"})
	if g.tasks.reboots != 1 {
		t.Fatalf("BOOTSTRAP_ONLY harus memicu pada 0 BOOTSTRAP: %d", g.tasks.reboots)
	}
	g2 := newRig()
	g2.mk(t, func(i *Input) { i.TriggerEvent = domain.ZtpTriggerEventBootstrapOrBoot })
	_ = g2.svc.Evaluate(context.Background(), dev1(), []string{"1 BOOT"})
	if g2.tasks.reboots != 1 {
		t.Fatalf("BOOTSTRAP_OR_BOOT harus memicu pada 1 BOOT: %d", g2.tasks.reboots)
	}
}

func TestEvaluate_SetParameterKonvergenDanHitungan(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	rule := g.mk(t, func(i *Input) {
		i.Name = "ssid"
		i.Condition = "" // selalu benar
		i.Actions = []domain.AutomationAction{{Type: "set_parameter", Name: "wifi.ssid", Value: "concat('ACS-', {@serial_number})"}}
	})
	_ = g.svc.Evaluate(ctx, dev1(), nil)
	if len(g.tasks.sets) != 1 || g.tasks.sets[0][ssidPath] != "ACS-ZTEG0001" {
		t.Fatalf("nilai dari ekspresi + logical key -> raw path: %+v", g.tasks.sets)
	}
	// perangkat menerapkan nilai -> konvergen: tidak ada pemicuan baru, hitungan direset, tidak ada catatan baru
	g.params.vals[1][ssidPath] = "ACS-ZTEG0001"
	g.clock = g.clock.Add(time.Hour)
	runsBefore := len(g.runs.rows)
	_ = g.svc.Evaluate(ctx, dev1(), nil)
	if len(g.tasks.sets) != 1 || len(g.runs.rows) != runsBefore {
		t.Fatalf("sudah konvergen tidak boleh memicu: sets=%d runs=%d", len(g.tasks.sets), len(g.runs.rows))
	}
	if g.states.m[[2]uint64{rule.ID, 1}].ConsecutiveFires != 0 {
		t.Errorf("konvergen harus menolkan hitungan: %+v", g.states.m)
	}
}

func TestEvaluate_AddTagIdempotenDanWebhook(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	g.mk(t, func(i *Input) {
		i.Name = "tag"
		i.Condition = ""
		i.Actions = []domain.AutomationAction{{Type: "add_tag", TagID: u64(5)}, {Type: "webhook"}}
	})
	_ = g.svc.Evaluate(ctx, dev1(), nil)
	if len(g.tags.assigned) != 1 || len(g.hooks.events) != 1 || g.hooks.events[0] != domain.WebhookEventAutomationTriggered {
		t.Fatalf("tag + webhook: %+v %+v", g.tags.assigned, g.hooks.events)
	}
	// tag sudah terpasang: add_tag tidak berubah; webhook tetap dianggap perubahan -> tetap memicu tiap cooldown
	g.clock = g.clock.Add(time.Hour)
	_ = g.svc.Evaluate(ctx, dev1(), nil)
	if len(g.tags.assigned) != 1 {
		t.Errorf("tag sudah ada tidak boleh dipasang ulang: %+v", g.tags.assigned)
	}
}

func TestEvaluate_GagalMengantreTetapMemulaiCooldown(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	g.mk(t, nil)
	g.tasks.enqErr = errors.New("kuota task penuh")
	_ = g.svc.Evaluate(ctx, dev1(), nil)
	if len(g.runs.rows) != 1 || g.runs.rows[0].Fired || g.runs.rows[0].Error == nil || !strings.Contains(*g.runs.rows[0].Error, "kuota") {
		t.Fatalf("kegagalan harus tercatat: %+v", g.runs.rows)
	}
	g.tasks.enqErr = nil
	g.clock = g.clock.Add(time.Minute)
	_ = g.svc.Evaluate(ctx, dev1(), nil)
	if g.tasks.reboots != 0 {
		t.Errorf("kegagalan juga memulai cooldown (tidak diulang tiap Inform): %d", g.tasks.reboots)
	}
}

func TestEvaluate_KondisiKonteksEvents(t *testing.T) {
	g := newRig()
	g.mk(t, func(i *Input) { i.Condition = "contains({@events}, '1 BOOT') and startswith({@serial_number}, 'zteg')" })
	_ = g.svc.Evaluate(context.Background(), dev1(), []string{"2 PERIODIC"})
	if g.tasks.reboots != 0 {
		t.Fatal("tanpa event BOOT tidak boleh memicu")
	}
	_ = g.svc.Evaluate(context.Background(), dev1(), []string{"1 BOOT", "2 PERIODIC"})
	if g.tasks.reboots != 1 {
		t.Fatalf("dengan event BOOT + serial cocok harus memicu: %d", g.tasks.reboots)
	}
}

// ---- uji (dry-run) ----

func TestTest_DryRunTanpaEfekSamping(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	res, err := g.svc.Test(ctx, adminA, TestInput{
		Condition: "{" + rxPath + "} < -27000",
		Actions: []domain.AutomationAction{
			{Type: "set_parameter", Name: "wifi.ssid", Value: "'Baru'"}, {Type: "reboot"}, {Type: "add_tag", TagID: u64(5)},
		},
	}, 1)
	if err != nil || !res.OK || !res.Matched || !res.WouldFire || len(res.Steps) != 3 {
		t.Fatalf("hasil: %+v %v", res, err)
	}
	set := res.Steps[0]
	if set.Path != ssidPath || set.Value == nil || *set.Value != "Baru" || set.Current == nil || *set.Current != "Lama" || !set.Change {
		t.Errorf("langkah set_parameter: %+v", set)
	}
	if len(res.Refs) != 1 || res.Refs[0].Name != rxPath || res.Refs[0].Value == nil || *res.Refs[0].Value != "-28000" {
		t.Errorf("refs: %+v", res.Refs)
	}
	if g.tasks.reboots != 0 || len(g.tasks.sets) != 0 || len(g.tags.assigned) != 0 || len(g.runs.rows) != 0 || len(g.states.m) != 0 {
		t.Fatal("dry-run tidak boleh mengeksekusi/menyimpan apa pun")
	}
	// kesalahan definisi = hasil, bukan error HTTP
	res, err = g.svc.Test(ctx, adminA, TestInput{Condition: "1 +", Actions: []domain.AutomationAction{{Type: "reboot"}}}, 1)
	if err != nil || res.OK || res.Error == "" {
		t.Errorf("kondisi salah harus hasil OK=false: %+v %v", res, err)
	}
	// target tak dapat di-resolve: dilewati dengan alasan, bukan menggagalkan
	res, _ = g.svc.Test(ctx, adminA, TestInput{Actions: []domain.AutomationAction{{Type: "set_parameter", Name: "tak.dikenal", Value: "1"}}}, 1)
	if len(res.Steps) != 1 || res.Steps[0].Skip == "" || res.Steps[0].Change || res.WouldFire {
		t.Errorf("langkah tak ter-resolve: %+v", res)
	}
	// event simulasi
	res, _ = g.svc.Test(ctx, adminA, TestInput{Condition: "contains({@events}, '1 BOOT')", Actions: []domain.AutomationAction{{Type: "reboot"}}, Events: []string{"1 BOOT"}}, 1)
	if !res.Matched {
		t.Errorf("events simulasi harus dipakai: %+v", res)
	}
}

// ---- regresi audit keamanan independen (22 Okt 2026) ----

// Ref {param} yang HANYA ada di nilai aksi harus ikut dimuat; dulu hanya ref kondisi yang dimuat sehingga aksinya diam-diam tidak pernah jalan.
func TestEvaluate_RefHanyaDiNilaiAksiIkutDimuat(t *testing.T) {
	g := newRig()
	const src = "InternetGatewayDevice.X_Vendor.Label"
	g.params.vals[1][src] = "Kantor-A"
	g.mk(t, func(i *Input) {
		i.Name = "dari-param"
		i.Condition = "" // kondisi tanpa ref sama sekali
		i.Actions = []domain.AutomationAction{{Type: "set_parameter", Name: "wifi.ssid", Value: "concat('ACS-', {" + src + "})"}}
	})
	_ = g.svc.Evaluate(context.Background(), dev1(), nil)
	if len(g.tasks.sets) != 1 || g.tasks.sets[0][ssidPath] != "ACS-Kantor-A" {
		t.Fatalf("nilai aksi harus memakai parameter perangkat: %+v (runs=%+v)", g.tasks.sets, g.runs.rows)
	}
}

// Target ManagementServer.* ditolak saat simpan DAN saat rencana (logical key yang di-resolve ke path itu).
func TestManagementServerTidakBolehDiaturOtomasi(t *testing.T) {
	g := newRig()
	for _, name := range []string{
		"InternetGatewayDevice.ManagementServer.URL", "Device.ManagementServer.PeriodicInformInterval",
		"InternetGatewayDevice.ManagementServer.ConnectionRequestUsername", "mgmt.periodic_inform_interval",
	} {
		_, err := g.svc.Create(context.Background(), adminA, Input{Name: "ms" + name[len(name)-5:], TriggerEvent: "EVERY_INFORM",
			Actions: []domain.AutomationAction{{Type: "set_parameter", Name: name, Value: "'x'"}}})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s harus ditolak: %v", name, err)
		}
	}
	// logical key biasa yang di-resolve ke path ManagementServer: dilewati saat rencana
	pc := newPlanCtx()
	pc.resolved["alias.aman"] = resolvedPath{path: "InternetGatewayDevice.ManagementServer.URL"}
	steps, err := g.svc.plan(context.Background(), dev1(), []domain.AutomationAction{{Type: "set_parameter", Name: "alias.aman", Value: "'http://x'"}},
		func(string) (string, bool) { return "", false }, pc)
	if err != nil || len(steps) != 1 || steps[0].Change || steps[0].Skip == "" {
		t.Fatalf("path ManagementServer hasil resolusi harus dilewati: %+v %v", steps, err)
	}
}

// Kondisi yang berosilasi akibat aksinya sendiri tidak boleh lolos dari pagar selamanya: batas keras 12 pemicuan per 24 jam.
func TestEvaluate_BatasKerasPerHariWalauKondisiBerosilasi(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	rule := g.mk(t, func(i *Input) {
		i.Condition = "contains({@events}, '2 PERIODIC')"
		i.CooldownSeconds, i.MaxConsecutiveFires = 300, 20 // pagar "berturut-turut" sengaja longgar
	})
	fires := 0
	for i := 0; i < 40; i++ {
		before := g.tasks.reboots
		_ = g.svc.Evaluate(ctx, dev1(), []string{"2 PERIODIC"})
		fires += g.tasks.reboots - before
		g.clock = g.clock.Add(6 * time.Minute)
		_ = g.svc.Evaluate(ctx, dev1(), []string{"1 BOOT"}) // setelah reboot: kondisi salah -> hitungan berturut direset
		g.clock = g.clock.Add(6 * time.Minute)
	}
	if fires != domain.AutomationMaxFiresPerDay {
		t.Fatalf("harus berhenti tepat di %d pemicuan/24 jam, dapat %d", domain.AutomationMaxFiresPerDay, fires)
	}
	if !g.states.m[[2]uint64{rule.ID, 1}].Suspended {
		t.Fatalf("aturan harus di-suspend: %+v", g.states.m)
	}
	var found bool
	for _, r := range g.runs.rows {
		if r.Suspended && r.Error != nil && strings.Contains(*r.Error, "24 jam") {
			found = true
		}
	}
	if !found {
		t.Errorf("catatan suspend harus menyebut batas 24 jam: %+v", g.runs.rows)
	}
}

// Jendela 24 jam bergulir: setelah lewat 24 jam hitungan mulai dari nol (aturan yang sehat tidak terkunci selamanya).
func TestEvaluate_JendelaHarianBergulir(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	g.mk(t, func(i *Input) {
		i.CooldownSeconds, i.MaxConsecutiveFires = 300, 20
		i.Condition = "contains({@events}, '2 PERIODIC')"
	})
	for i := 0; i < 6; i++ {
		_ = g.svc.Evaluate(ctx, dev1(), []string{"2 PERIODIC"})
		g.clock = g.clock.Add(6 * time.Minute)
		_ = g.svc.Evaluate(ctx, dev1(), []string{"1 BOOT"})
		g.clock = g.clock.Add(6 * time.Minute)
	}
	g.clock = g.clock.Add(25 * time.Hour)
	before := g.tasks.reboots
	_ = g.svc.Evaluate(ctx, dev1(), []string{"2 PERIODIC"})
	if g.tasks.reboots != before+1 {
		t.Fatalf("setelah jendela 24 jam lewat harus bisa memicu lagi: %d -> %d", before, g.tasks.reboots)
	}
}

// Antrean tenant penuh (ErrQuotaExceeded) bukan kesalahan aturan: tidak dihitung berturut-turut, tidak meng-suspend aturan yang benar.
func TestEvaluate_KuotaPenuhTidakMenyuspend(t *testing.T) {
	g := newRig() // max_consecutive_fires = 2
	ctx := context.Background()
	rule := g.mk(t, nil)
	g.tasks.enqErr = domain.ErrQuotaExceeded
	for i := 0; i < 6; i++ {
		_ = g.svc.Evaluate(ctx, dev1(), nil)
		g.clock = g.clock.Add(11 * time.Minute)
	}
	st := g.states.m[[2]uint64{rule.ID, 1}]
	if st.Suspended || st.ConsecutiveFires != 0 {
		t.Fatalf("kuota penuh tidak boleh dihitung/di-suspend: %+v", st)
	}
	g.tasks.enqErr = nil
	_ = g.svc.Evaluate(ctx, dev1(), nil)
	if g.tasks.reboots != 1 {
		t.Fatalf("begitu antrean lega aturan harus memicu: %d", g.tasks.reboots)
	}
}

// Satu pembacaan batch untuk semua target set_parameter, dan resolusi path di-memo: biaya per Inform tidak berlipat dengan jumlah aturan.
func TestEvaluate_TargetDiBatchDanResolusiDiMemo(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		i := i
		g.mk(t, func(in *Input) {
			in.Name = fmt.Sprintf("tgt-%d", i)
			in.Condition = ""
			in.Actions = []domain.AutomationAction{
				{Type: "set_parameter", Name: "wifi.ssid", Value: "'Sama'"}, // semua aturan menarget path yang sama
				{Type: "set_parameter", Name: fmt.Sprintf("InternetGatewayDevice.X.P%d", i), Value: "'v'"},
			}
		})
	}
	g.params.valsCall, g.tasks.resolves = 0, 0
	if err := g.svc.Evaluate(ctx, dev1(), nil); err != nil {
		t.Fatal(err)
	}
	// 1x ValuesFor untuk ref kondisi/nilai (kosong -> dilewati) + 1x untuk seluruh target; bukan satu per aksi
	if g.params.valsCall > 2 {
		t.Errorf("ValuesFor dipanggil %d kali; harus <= 2 (batch)", g.params.valsCall)
	}
	// wifi.ssid dipakai 5 aturan -> di-resolve sekali; 5 path unik lain -> total 6 (bukan 10 per lintasan x 2 lintasan)
	if g.tasks.resolves > 6 {
		t.Errorf("resolusi path %d kali; harus <= 6 (memo)", g.tasks.resolves)
	}
}

func TestSetParamBudgetPerTenant(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	mkRule := func(name string, n int, active bool) error {
		var acts []domain.AutomationAction
		for i := 0; i < n; i++ {
			acts = append(acts, domain.AutomationAction{Type: "set_parameter", Name: fmt.Sprintf("InternetGatewayDevice.X.%s%d", name, i), Value: "'v'"})
		}
		_, err := g.svc.Create(ctx, adminA, Input{Name: name, TriggerEvent: "EVERY_INFORM", Actions: acts, IsActive: &active})
		return err
	}
	for _, n := range []string{"a", "b", "c"} {
		if err := mkRule(n, 10, true); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
	if err := mkRule("d", 1, true); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("aksi ke-%d harus ditolak: %v", domain.AutomationMaxSetParamsPerTenant+1, err)
	}
	if err := mkRule("nonaktif", 5, false); err != nil {
		t.Fatalf("aturan nonaktif tidak dihitung: %v", err)
	}
	// mengaktifkan aturan nonaktif tidak boleh melewati batas
	rules, _, _ := g.svc.List(ctx, adminA, nil, domain.Pagination{Page: 1, PageSize: 50})
	var off *domain.AutomationRule
	for i := range rules {
		if rules[i].Name == "nonaktif" {
			off = &rules[i]
		}
	}
	on := true
	if _, err := g.svc.Update(ctx, adminA, off.UUID, UpdateInput{IsActive: &on}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("mengaktifkan harus ditolak bila melewati batas: %v", err)
	}
}

// Pesan seragam untuk tag yang tidak ada dan tag milik tenant lain (tidak ada oracle pencarian id).
func TestTagOracleSeragam(t *testing.T) {
	g := newRig()
	mk := func(id uint64) error {
		_, err := g.svc.Create(context.Background(), adminA, Input{Name: fmt.Sprintf("t%d", id), TriggerEvent: "EVERY_INFORM",
			Actions: []domain.AutomationAction{{Type: "add_tag", TagID: &id}}})
		return err
	}
	e1, e2 := mk(99), mk(6) // 99 tidak ada; 6 milik tenant 8
	if e1 == nil || e2 == nil {
		t.Fatalf("keduanya harus ditolak: %v / %v", e1, e2)
	}
	norm := func(e error) string { return strings.NewReplacer("99", "N", "6", "N").Replace(e.Error()) }
	if norm(e1) != norm(e2) {
		t.Errorf("pesan harus seragam: %v | %v", e1, e2)
	}
}

// Parameter sumber yang belum dilaporkan: aksi dilewati, TIDAK menulis nilai parsial seperti "ACS-" ke perangkat.
func TestEvaluate_SumberBelumDilaporkanTidakMenulisNilaiParsial(t *testing.T) {
	g := newRig()
	g.mk(t, func(i *Input) {
		i.Name = "parsial"
		i.Condition = ""
		i.Actions = []domain.AutomationAction{{Type: "set_parameter", Name: "wifi.ssid", Value: "concat('ACS-', {InternetGatewayDevice.X_Vendor.TidakAda})"}}
	})
	_ = g.svc.Evaluate(context.Background(), dev1(), nil)
	if len(g.tasks.sets) != 0 {
		t.Fatalf("tidak boleh menulis nilai parsial: %+v", g.tasks.sets)
	}
	res, _ := g.svc.Test(context.Background(), adminA, TestInput{Actions: []domain.AutomationAction{{Type: "set_parameter", Name: "wifi.ssid", Value: "concat('ACS-', {X.TidakAda})"}}}, 1)
	if len(res.Steps) != 1 || res.Steps[0].Change || !strings.Contains(res.Steps[0].Skip, "X.TidakAda") {
		t.Fatalf("dry-run harus menjelaskan sumber yang hilang: %+v", res.Steps)
	}
}
