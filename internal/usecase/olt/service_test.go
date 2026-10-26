package olt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"acs/internal/domain"
	"acs/internal/oltdriver"
	"acs/internal/oltdriver/sim"
	"acs/pkg/cryptoutil"
)

// ---- fakes ----

var (
	pollIDs = map[string]uint64{domain.OLTPollUnknown: 1, domain.OLTPollOK: 2, domain.OLTPollUnreachable: 3, domain.OLTPollAuthFailed: 4, domain.OLTPollError: 5}
	onuIDs  = map[string]uint64{domain.ONUStatusPending: 11, domain.ONUStatusIgnored: 12, domain.ONUStatusResolved: 13}
)

type fakeRefs struct{}

func (fakeRefs) GetByCode(_ context.Context, table, code string) (domain.RefLookup, error) {
	var m map[string]uint64
	switch table {
	case domain.RefTableOLTPollStatus:
		m = pollIDs
	case domain.RefTableONUDiscoveryStatus:
		m = onuIDs
	}
	if id, ok := m[code]; ok {
		return domain.RefLookup{ID: id, Code: code}, nil
	}
	return domain.RefLookup{}, domain.ErrNotFound
}
func (fakeRefs) GetByID(context.Context, string, uint64) (domain.RefLookup, error) {
	return domain.RefLookup{}, domain.ErrNotFound
}
func (fakeRefs) List(context.Context, string) ([]domain.RefLookup, error) { return nil, nil }

func codeOf(m map[string]uint64, id uint64) string {
	for c, i := range m {
		if i == id {
			return c
		}
	}
	return ""
}

type fakeActivity struct {
	mu      sync.Mutex
	actions []string
	details []string
}

func (f *fakeActivity) Record(_ context.Context, a *domain.ActivityLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, a.Action)
	b, _ := json.Marshal(a.Details)
	f.details = append(f.details, string(b))
	return nil
}
func (f *fakeActivity) ListByEntity(context.Context, string, uint64, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}
func (f *fakeActivity) ListByTenant(context.Context, *uint64, domain.ActivityLogFilter, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}

// fakeRepo meniru semantik SQL: unik per (olt, serial), join status/olt, prune, dan scope tenant pada daftar.
type fakeRepo struct {
	mu        sync.Mutex
	olts      map[uint64]*domain.OLT
	onus      map[uint64]*domain.DiscoveredONU
	nextOLT   uint64
	nextONU   uint64
	polls     []domain.OLTPollResult
	pruneKeep []int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{olts: map[uint64]*domain.OLT{}, onus: map[uint64]*domain.DiscoveredONU{}}
}

func (f *fakeRepo) Create(_ context.Context, o *domain.OLT) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextOLT++
	o.ID = f.nextOLT
	cp := *o
	f.olts[o.ID] = &cp
	return nil
}
func (f *fakeRepo) view(o *domain.OLT) *domain.OLT {
	cp := *o
	cp.PollStatus = codeOf(pollIDs, cp.PollStatusID)
	return &cp
}
func (f *fakeRepo) GetByUUID(_ context.Context, uuid string) (*domain.OLT, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, o := range f.olts {
		if o.UUID == uuid {
			return f.view(o), nil
		}
	}
	return nil, domain.ErrNotFound
}
func (f *fakeRepo) List(_ context.Context, fl domain.OLTFilter, _ domain.Pagination) ([]domain.OLT, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []domain.OLT{}
	for _, o := range f.olts {
		if fl.TenantID == nil || *fl.TenantID == o.TenantID {
			out = append(out, *f.view(o))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, len(out), nil
}
func (f *fakeRepo) Update(_ context.Context, o *domain.OLT) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.olts[o.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *o
	f.olts[o.ID] = &cp
	return nil
}
func (f *fakeRepo) SoftDelete(_ context.Context, id, _ uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.olts[id]; !ok {
		return domain.ErrNotFound
	}
	delete(f.olts, id)
	for nid, n := range f.onus {
		if n.OLTID == id {
			delete(f.onus, nid)
		}
	}
	return nil
}
func (f *fakeRepo) RecordPoll(_ context.Context, id uint64, r domain.OLTPollResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.olts[id]
	if !ok {
		return domain.ErrNotFound
	}
	now := time.Now()
	o.PollStatusID, o.LastPollAt, o.LastPollError = r.StatusID, &now, r.Error
	if r.Info != nil {
		n, v := r.Info.Name, r.Info.Version
		o.ReportedName, o.ReportedVersion = &n, &v
	}
	f.polls = append(f.polls, r)
	return nil
}

func (f *fakeRepo) joinONU(n *domain.DiscoveredONU) {
	o := f.olts[n.OLTID]
	n.OLTUUID, n.OLTName, n.TenantID = o.UUID, o.Name, o.TenantID
	n.Status = codeOf(onuIDs, n.StatusID)
}
func (f *fakeRepo) ListONUsByOLT(_ context.Context, oltID uint64) ([]domain.DiscoveredONU, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []domain.DiscoveredONU{}
	for _, n := range f.onus {
		if n.OLTID == oltID {
			f.joinONU(n)
			out = append(out, *n)
		}
	}
	return out, nil
}
func (f *fakeRepo) UpsertONUs(_ context.Context, oltID uint64, rows []domain.DiscoveredONU) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range rows {
		var found *domain.DiscoveredONU
		for _, n := range f.onus {
			if n.OLTID == oltID && n.SerialNumber == r.SerialNumber {
				found = n
			}
		}
		if found == nil {
			f.nextONU++
			r.ID, r.OLTID = f.nextONU, oltID
			cp := r
			f.onus[r.ID] = &cp
			continue
		}
		found.PonPort, found.ONUType, found.StatusID, found.LastSeenAt, found.ResolvedAt = r.PonPort, r.ONUType, r.StatusID, r.LastSeenAt, r.ResolvedAt
	}
	return nil
}
func (f *fakeRepo) SetONUsStatus(_ context.Context, ids []uint64, statusID uint64, resolvedAt *time.Time, _ *uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range ids {
		if n, ok := f.onus[id]; ok {
			n.StatusID, n.ResolvedAt = statusID, resolvedAt
		}
	}
	return nil
}
func (f *fakeRepo) PruneResolvedONUs(_ context.Context, oltID, resolvedStatusID uint64, keep int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pruneKeep = append(f.pruneKeep, keep)
	var resolved []*domain.DiscoveredONU
	for _, n := range f.onus {
		if n.OLTID == oltID && n.StatusID == resolvedStatusID {
			resolved = append(resolved, n)
		}
	}
	sort.Slice(resolved, func(i, j int) bool { return resolved[i].ID > resolved[j].ID })
	var removed int64
	for i, n := range resolved {
		if i >= keep {
			delete(f.onus, n.ID)
			removed++
		}
	}
	return removed, nil
}
func (f *fakeRepo) ListONUs(_ context.Context, fl domain.ONUFilter, _ domain.Pagination) ([]domain.DiscoveredONU, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []domain.DiscoveredONU{}
	for _, n := range f.onus {
		f.joinONU(n)
		if fl.TenantID != nil && *fl.TenantID != n.TenantID {
			continue
		}
		if fl.OLTID != nil && *fl.OLTID != n.OLTID {
			continue
		}
		if fl.StatusID != nil && *fl.StatusID != n.StatusID {
			continue
		}
		out = append(out, *n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SerialNumber < out[j].SerialNumber })
	return out, len(out), nil
}
func (f *fakeRepo) GetONUByUUID(_ context.Context, uuid string) (*domain.DiscoveredONU, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.onus {
		if n.UUID == uuid {
			f.joinONU(n)
			cp := *n
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

// ---- rig ----

func u64(v uint64) *uint64 { return &v }

var (
	superadmin = domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	adminA     = domain.Actor{UserID: 2, TenantID: u64(7), Roles: []string{domain.RoleAdmin}}
	nocA       = domain.Actor{UserID: 3, TenantID: u64(7), Roles: []string{domain.RoleNOC}}
	viewerA    = domain.Actor{UserID: 4, TenantID: u64(7), Roles: []string{domain.RoleViewer}}
	adminB     = domain.Actor{UserID: 5, TenantID: u64(8), Roles: []string{domain.RoleAdmin}}
	noTenant   = domain.Actor{UserID: 6, Roles: []string{domain.RoleAdmin}}
)

type rig struct {
	svc  *Service
	repo *fakeRepo
	act  *fakeActivity
	reg  *oltdriver.Registry
	enc  *cryptoutil.Encryptor
}

func newRig(t *testing.T, extra ...domain.OLTDriver) *rig {
	t.Helper()
	enc, err := cryptoutil.NewEncryptor([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	reg := oltdriver.NewRegistry()
	if err := reg.Register(sim.New()); err != nil {
		t.Fatal(err)
	}
	for _, d := range extra {
		if err := reg.Register(d); err != nil {
			t.Fatal(err)
		}
	}
	repo, act := newFakeRepo(), &fakeActivity{}
	return &rig{svc: NewService(repo, reg, fakeRefs{}, enc, act), repo: repo, act: act, reg: reg, enc: enc}
}

func (r *rig) mkOLT(t *testing.T, actor domain.Actor, host string) *domain.OLT {
	t.Helper()
	o, err := r.svc.Create(context.Background(), actor, CreateInput{TenantID: u64(7), Name: "OLT-" + host, DriverCode: sim.Code, Host: host})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return o
}

func serialsByStatus(t *testing.T, r *rig, o *domain.OLT) map[string]string {
	t.Helper()
	rows, _, err := r.svc.ListONUs(context.Background(), superadmin, ONUListInput{OLTUUID: o.UUID}, domain.Pagination{})
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, n := range rows {
		m[n.SerialNumber] = n.Status
	}
	return m
}

func wantQueue(t *testing.T, got map[string]string, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("antrean = %v, harapan %v", got, want)
	}
	for s, st := range want {
		if got[s] != st {
			t.Fatalf("antrean = %v, harapan %v", got, want)
		}
	}
}

// ---- inventaris ----

func TestCreate_RBACDanTenant(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	in := CreateInput{Name: "OLT Pusat", DriverCode: sim.Code, Host: "10.1.1.1"}

	for _, a := range []domain.Actor{nocA, viewerA} {
		if _, err := r.svc.Create(ctx, a, in); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("%v harus ditolak: %v", a.Roles, err)
		}
	}
	if _, err := r.svc.Create(ctx, noTenant, in); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("admin tanpa tenant harus ditolak: %v", err)
	}
	// ADMIN selalu menulis ke tenant-nya sendiri; menyebut tenant lain ditolak
	if _, err := r.svc.Create(ctx, adminA, CreateInput{TenantID: u64(8), Name: "x", DriverCode: sim.Code, Host: "10.1.1.1"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("tenant lain harus ditolak: %v", err)
	}
	o, err := r.svc.Create(ctx, adminA, in)
	if err != nil || o.TenantID != 7 || o.SNMPPort != 161 || o.SSHPort != 22 || !o.IsActive || o.PollStatus != domain.OLTPollUnknown {
		t.Fatalf("Create ADMIN: %+v %v", o, err)
	}
	// superadmin wajib menyebut tenant
	if _, err := r.svc.Create(ctx, superadmin, in); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("superadmin tanpa tenant_id harus ErrInvalidInput: %v", err)
	}
	if o, err := r.svc.Create(ctx, superadmin, CreateInput{TenantID: u64(8), Name: "OLT B", DriverCode: sim.Code, Host: "10.1.1.2"}); err != nil || o.TenantID != 8 {
		t.Fatalf("superadmin ke tenant 8: %+v %v", o, err)
	}
	if len(r.act.actions) != 2 || r.act.actions[0] != "CREATE_OLT" {
		t.Fatalf("audit: %v", r.act.actions)
	}
}

func TestCreate_Validasi(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	ok := CreateInput{Name: "OLT", DriverCode: sim.Code, Host: "10.1.1.1"}
	mut := func(f func(*CreateInput)) CreateInput { c := ok; f(&c); return c }
	ip := func(v int) *int { return &v }

	cases := map[string]CreateInput{
		"nama kosong":            mut(func(c *CreateInput) { c.Name = "  " }),
		"nama terlalu panjang":   mut(func(c *CreateInput) { c.Name = strings.Repeat("a", domain.OLTNameMaxLen+1) }),
		"driver tidak dikenal":   mut(func(c *CreateInput) { c.DriverCode = "zte" }),
		"host kosong":            mut(func(c *CreateInput) { c.Host = "" }),
		"host berskema":          mut(func(c *CreateInput) { c.Host = "http://10.1.1.1" }),
		"host berport":           mut(func(c *CreateInput) { c.Host = "10.1.1.1:22" }),
		"host berjalur":          mut(func(c *CreateInput) { c.Host = "10.1.1.1/x" }),
		"host loopback":          mut(func(c *CreateInput) { c.Host = "127.0.0.1" }),
		"host metadata cloud":    mut(func(c *CreateInput) { c.Host = "169.254.169.254" }),
		"host tak dapat resolve": mut(func(c *CreateInput) { c.Host = "tidak-ada.invalid" }),
		"host berspasi":          mut(func(c *CreateInput) { c.Host = "olt pusat" }),
		"port snmp 0":            mut(func(c *CreateInput) { c.SNMPPort = ip(0) }),
		"port ssh 70000":         mut(func(c *CreateInput) { c.SSHPort = ip(70000) }),
		"community berkontrol":   mut(func(c *CreateInput) { c.SNMPCommunity = "ab\ncd" }),
		"password terlalu panjang": mut(func(c *CreateInput) {
			c.SSHPassword = strings.Repeat("p", domain.OLTCredentialMaxLen+1)
		}),
		"username berkontrol": mut(func(c *CreateInput) { c.SSHUsername = "a\x00b" }),
	}
	for name, in := range cases {
		if _, err := r.svc.Create(ctx, adminA, in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: harus ErrInvalidInput, dapat %v", name, err)
		}
	}
	if len(r.repo.olts) != 0 {
		t.Fatalf("masukan tidak valid tidak boleh tersimpan: %d", len(r.repo.olts))
	}
	// batas bawah/atas yang sah diterima
	good := mut(func(c *CreateInput) {
		c.SNMPPort, c.SSHPort = ip(1), ip(65535)
		c.Name = strings.Repeat("a", domain.OLTNameMaxLen)
		c.Host = "10.1.1.9"
	})
	if _, err := r.svc.Create(ctx, adminA, good); err != nil {
		t.Fatalf("nilai batas yang sah harus diterima: %v", err)
	}
}

// Kredensial: tersimpan terenkripsi, bisa dibuka lagi untuk koneksi, tidak pernah muncul di JSON maupun audit trail.
func TestCredentials_TerenkripsiDanTidakBocor(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	o, err := r.svc.Create(ctx, adminA, CreateInput{
		Name: "OLT", DriverCode: sim.Code, Host: "10.1.1.1", SNMPCommunity: "komunitas-rahasia", SSHUsername: "operator", SSHPassword: "  sandi rahasia  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := r.repo.olts[o.ID]
	if len(stored.SNMPCommunityEnc) == 0 || strings.Contains(string(stored.SNMPCommunityEnc), "komunitas-rahasia") ||
		strings.Contains(string(stored.SSHPasswordEnc), "sandi rahasia") {
		t.Fatalf("kredensial harus tersimpan terenkripsi: %q / %q", stored.SNMPCommunityEnc, stored.SSHPasswordEnc)
	}
	if !o.HasSNMPCommunity || !o.HasSSHPassword {
		t.Fatalf("penanda kredensial harus true: %+v", o)
	}
	// password tidak dipangkas (spasi di tepi adalah bagian sah dari password)
	_, conn, err := r.svc.connect(stored)
	if err != nil || conn.SNMPCommunity != "komunitas-rahasia" || conn.SSHPassword != "  sandi rahasia  " || conn.SSHUsername != "operator" || conn.Host != "10.1.1.1" {
		t.Fatalf("connect: %+v %v", conn, err)
	}
	// JSON respons: tanpa nilai maupun ciphertext
	b, _ := json.Marshal(o)
	for _, bad := range []string{"komunitas-rahasia", "sandi rahasia", "snmp_community_enc", "ssh_password_enc"} {
		if strings.Contains(string(b), bad) {
			t.Fatalf("JSON memuat %q: %s", bad, b)
		}
	}
	if !strings.Contains(string(b), `"has_snmp_community":true`) || !strings.Contains(string(b), `"has_ssh_password":true`) {
		t.Fatalf("JSON harus memuat penanda: %s", b)
	}
	// audit trail: tanpa kredensial
	for _, d := range r.act.details {
		if strings.Contains(d, "komunitas-rahasia") || strings.Contains(d, "sandi rahasia") {
			t.Fatalf("audit trail memuat kredensial: %s", d)
		}
	}
}

// Username SSH hanya terlihat oleh ADMIN/SUPERADMIN (hak akses minimal); NOC/VIEWER tetap melihat OLT-nya tanpa username.
func TestSSHUsername_HanyaUntukAdmin(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	o, err := r.svc.Create(ctx, adminA, CreateInput{Name: "OLT", DriverCode: sim.Code, Host: "10.1.1.1", SSHUsername: "operator", SSHPassword: "x"})
	if err != nil || o.SSHUsername == nil || *o.SSHUsername != "operator" {
		t.Fatalf("ADMIN pembuat melihat username: %+v %v", o, err)
	}
	for _, c := range []struct {
		name  string
		actor domain.Actor
		want  bool
	}{{"ADMIN", adminA, true}, {"SUPERADMIN", superadmin, true}, {"NOC", nocA, false}, {"VIEWER", viewerA, false}} {
		g, err := r.svc.Get(ctx, c.actor, o.UUID)
		if err != nil || (g.SSHUsername != nil) != c.want {
			t.Fatalf("Get %s: username terlihat=%v, harapan %v (%v)", c.name, g != nil && g.SSHUsername != nil, c.want, err)
		}
		rows, _, err := r.svc.List(ctx, c.actor, domain.OLTFilter{TenantID: u64(7)}, domain.Pagination{})
		if err != nil || len(rows) != 1 || (rows[0].SSHUsername != nil) != c.want {
			t.Fatalf("List %s: %+v %v", c.name, rows, err)
		}
		if !g.HasSSHPassword {
			t.Fatalf("%s: penanda password tetap harus terlihat", c.name)
		}
	}
	// menyembunyikan di respons tidak boleh mengubah data tersimpan
	if st := r.repo.olts[o.ID]; st.SSHUsername == nil || *st.SSHUsername != "operator" {
		t.Fatalf("data tersimpan berubah: %+v", st.SSHUsername)
	}
}

func TestUpdate(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	o, _ := r.svc.Create(ctx, adminA, CreateInput{Name: "Lama", DriverCode: sim.Code, Host: "10.1.1.1", SNMPCommunity: "c1", SSHPassword: "p1", Notes: "catatan"})

	// parsial: hanya nama berubah; kredensial dan catatan tetap
	name := "Baru"
	u, err := r.svc.Update(ctx, adminA, o.UUID, UpdateInput{Name: &name})
	if err != nil || u.Name != "Baru" || !u.HasSNMPCommunity || !u.HasSSHPassword || u.Notes == nil || *u.Notes != "catatan" || u.Host != "10.1.1.1" {
		t.Fatalf("update parsial: %+v %v", u, err)
	}
	// kredensial/catatan "" mengosongkan; kredensial baru menggantikan
	empty, newPass := "", "p2"
	u, err = r.svc.Update(ctx, adminA, o.UUID, UpdateInput{SNMPCommunity: &empty, SSHPassword: &newPass, Notes: &empty})
	if err != nil || u.HasSNMPCommunity || !u.HasSSHPassword || u.Notes != nil {
		t.Fatalf("pengosongan/penggantian: %+v %v", u, err)
	}
	if _, conn, _ := r.svc.connect(r.repo.olts[o.ID]); conn.SSHPassword != "p2" || conn.SNMPCommunity != "" {
		t.Fatalf("kredensial setelah update: %+v", conn)
	}
	// host baru divalidasi
	bad := "127.0.0.1"
	if _, err := r.svc.Update(ctx, adminA, o.UUID, UpdateInput{Host: &bad}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("host loopback harus ditolak: %v", err)
	}
	drv := "zte"
	if _, err := r.svc.Update(ctx, adminA, o.UUID, UpdateInput{DriverCode: &drv}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("driver tak dikenal harus ditolak: %v", err)
	}
	if r.repo.olts[o.ID].Host != "10.1.1.1" {
		t.Fatal("nilai ditolak tidak boleh tersimpan")
	}
	// RBAC & scope
	if _, err := r.svc.Update(ctx, nocA, o.UUID, UpdateInput{Name: &name}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("NOC: %v", err)
	}
	if _, err := r.svc.Update(ctx, adminB, o.UUID, UpdateInput{Name: &name}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("tenant lain: %v", err)
	}
	if _, err := r.svc.Update(ctx, adminA, "tidak-ada", UpdateInput{Name: &name}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tak ada: %v", err)
	}
	// audit memuat nama bidang, bukan nilainya
	last := r.act.details[len(r.act.details)-1]
	if strings.Contains(last, "p2") || strings.Contains(last, "10.1.1.1") {
		t.Fatalf("audit update tidak boleh memuat nilai: %s", last)
	}
}

func TestDelete_DanScope(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	o := r.mkOLT(t, adminA, "10.1.1.1")
	if _, err := r.svc.Poll(ctx, adminA, o.UUID); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Delete(ctx, nocA, o.UUID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("NOC: %v", err)
	}
	if err := r.svc.Delete(ctx, adminB, o.UUID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("tenant lain: %v", err)
	}
	if err := r.svc.Delete(ctx, adminA, o.UUID); err != nil {
		t.Fatal(err)
	}
	if len(r.repo.olts) != 0 || len(r.repo.onus) != 0 {
		t.Fatalf("OLT dan antreannya harus terhapus: %d/%d", len(r.repo.olts), len(r.repo.onus))
	}
	if err := r.svc.Delete(ctx, adminA, o.UUID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("hapus ulang: %v", err)
	}
}

func TestListGet_ScopeTenant(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	a := r.mkOLT(t, adminA, "10.1.1.1")
	b, err := r.svc.Create(ctx, adminB, CreateInput{Name: "B", DriverCode: sim.Code, Host: "10.1.1.2"})
	if err != nil {
		t.Fatal(err)
	}
	rows, total, err := r.svc.List(ctx, viewerA, domain.OLTFilter{TenantID: u64(8)}, domain.Pagination{}) // tenant_id klien diabaikan
	if err != nil || total != 1 || rows[0].UUID != a.UUID {
		t.Fatalf("viewer hanya melihat tenant-nya: %+v %v", rows, err)
	}
	if rows, total, _ := r.svc.List(ctx, superadmin, domain.OLTFilter{}, domain.Pagination{}); total != 2 || len(rows) != 2 {
		t.Fatalf("superadmin melihat semua: %d", total)
	}
	if rows, _, _ := r.svc.List(ctx, superadmin, domain.OLTFilter{TenantID: u64(8)}, domain.Pagination{}); len(rows) != 1 || rows[0].UUID != b.UUID {
		t.Fatalf("superadmin memfilter tenant: %+v", rows)
	}
	if _, _, err := r.svc.List(ctx, noTenant, domain.OLTFilter{}, domain.Pagination{}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-superadmin tanpa tenant harus ditolak (bukan 'tanpa filter'): %v", err)
	}
	if _, err := r.svc.Get(ctx, viewerA, b.UUID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("Get tenant lain: %v", err)
	}
	if g, err := r.svc.Get(ctx, viewerA, a.UUID); err != nil || g.UUID != a.UUID {
		t.Fatalf("Get sendiri: %v", err)
	}
}

// ---- koneksi ----

func TestTest_Probe(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()

	ok := r.mkOLT(t, adminA, "10.1.1.1")
	got, err := r.svc.Test(ctx, adminA, ok.UUID)
	if err != nil || got.PollStatus != domain.OLTPollOK || got.ReportedName == nil || *got.ReportedName != "SIM-OLT-10.1.1.1" || got.LastPollAt == nil {
		t.Fatalf("Test OK: %+v %v", got, err)
	}
	if _, err := r.svc.Test(ctx, nocA, ok.UUID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("NOC tidak boleh menguji koneksi: %v", err)
	}
	if _, err := r.svc.Test(ctx, adminB, ok.UUID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("tenant lain: %v", err)
	}

	for host, c := range map[string]struct {
		want   error
		status string
	}{
		sim.HostUnreachable: {domain.ErrOLTUnreachable, domain.OLTPollUnreachable},
		sim.HostAuthFailed:  {domain.ErrOLTAuthFailed, domain.OLTPollAuthFailed},
	} {
		o := r.mkOLT(t, adminA, host)
		if _, err := r.svc.Test(ctx, adminA, o.UUID); !errors.Is(err, c.want) || !errors.Is(err, domain.ErrUpstreamUnavailable) {
			t.Fatalf("%s: %v", host, err)
		}
		cur, _ := r.svc.Get(ctx, adminA, o.UUID)
		if cur.PollStatus != c.status || cur.LastPollError == nil || cur.LastPollAt == nil {
			t.Fatalf("%s: status harus tercatat: %+v", host, cur)
		}
	}
}

// errorDriver mengembalikan galat dengan teks "dari perangkat" yang tidak boleh sampai ke klien maupun ke database.
type errorDriver struct{}

func (errorDriver) Code() string { return "errdrv" }
func (errorDriver) Probe(context.Context, domain.OLTConnection) (domain.OLTInfo, error) {
	return domain.OLTInfo{}, errors.New("login: password=hunter2 ditolak oleh 10.9.9.9")
}
func (errorDriver) ListUnregisteredONUs(context.Context, domain.OLTConnection) ([]domain.UnregisteredONU, error) {
	return nil, errors.New("login: password=hunter2 ditolak oleh 10.9.9.9")
}

func TestGalatDriverTakDikenal_TidakBocor(t *testing.T) {
	r := newRig(t, errorDriver{})
	ctx := context.Background()
	o, err := r.svc.Create(ctx, adminA, CreateInput{Name: "x", DriverCode: "errdrv", Host: "10.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func() error{
		"Test": func() error { _, err := r.svc.Test(ctx, adminA, o.UUID); return err },
		"Poll": func() error { _, err := r.svc.Poll(ctx, adminA, o.UUID); return err },
	} {
		err := call()
		if !errors.Is(err, domain.ErrUpstreamUnavailable) || strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "10.9.9.9") {
			t.Fatalf("%s: galat ke klien harus generik: %v", name, err)
		}
		cur, _ := r.svc.Get(ctx, adminA, o.UUID)
		if cur.PollStatus != domain.OLTPollError || cur.LastPollError == nil || strings.Contains(*cur.LastPollError, "hunter2") {
			t.Fatalf("%s: last_poll_error tidak boleh memuat teks perangkat: %+v", name, cur)
		}
	}
}

// ---- polling & rekonsiliasi ----

func TestPoll_Rekonsiliasi(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	o := r.mkOLT(t, adminA, sim.HostRotating)
	A, B, C, D := "SIMONU0001", "SIMONU0002", "SIMONU0003", "SIMONU0004"
	P, I, R := domain.ONUStatusPending, domain.ONUStatusIgnored, domain.ONUStatusResolved

	// polling 1: A,B,C baru
	sum, err := r.svc.Poll(ctx, nocA, o.UUID)
	if err != nil || sum.Seen != 3 || sum.New != 3 || sum.Resolved != 0 || sum.QueueLength != 3 {
		t.Fatalf("polling 1: %+v %v", sum, err)
	}
	wantQueue(t, serialsByStatus(t, r, o), map[string]string{A: P, B: P, C: P})

	// operator mengabaikan B
	rows, _, _ := r.svc.ListONUs(ctx, adminA, ONUListInput{OLTUUID: o.UUID, Search: ""}, domain.Pagination{})
	var bUUID string
	for _, n := range rows {
		if n.SerialNumber == B {
			bUUID = n.UUID
		}
	}
	if n, err := r.svc.SetONUIgnored(ctx, nocA, bUUID, true); err != nil || n.Status != I {
		t.Fatalf("abaikan B: %+v %v", n, err)
	}

	// polling 2: B,C,D → A hilang (RESOLVED), B tetap IGNORED, C tetap PENDING, D baru
	sum, err = r.svc.Poll(ctx, adminA, o.UUID)
	if err != nil || sum.New != 1 || sum.Resolved != 1 || sum.QueueLength != 2 {
		t.Fatalf("polling 2: %+v %v", sum, err)
	}
	wantQueue(t, serialsByStatus(t, r, o), map[string]string{A: R, B: I, C: P, D: P})

	// polling 3: kosong → semua PENDING/IGNORED menjadi RESOLVED
	sum, err = r.svc.Poll(ctx, adminA, o.UUID)
	if err != nil || sum.Seen != 0 || sum.Resolved != 3 || sum.QueueLength != 0 {
		t.Fatalf("polling 3: %+v %v", sum, err)
	}
	wantQueue(t, serialsByStatus(t, r, o), map[string]string{A: R, B: R, C: R, D: R})

	// polling 4: A,B,C muncul lagi → PENDING (ingatan IGNORED hilang, sesuai rancangan); uuid dan first_seen_at tetap
	before := map[string]time.Time{}
	uuids := map[string]string{}
	for _, n := range r.repo.onus {
		before[n.SerialNumber], uuids[n.SerialNumber] = n.FirstSeenAt, n.UUID
	}
	sum, err = r.svc.Poll(ctx, adminA, o.UUID)
	if err != nil || sum.Reappeared != 3 || sum.New != 0 || sum.QueueLength != 3 {
		t.Fatalf("polling 4: %+v %v", sum, err)
	}
	wantQueue(t, serialsByStatus(t, r, o), map[string]string{A: P, B: P, C: P, D: R})
	for _, n := range r.repo.onus {
		if !n.FirstSeenAt.Equal(before[n.SerialNumber]) || n.UUID != uuids[n.SerialNumber] {
			t.Fatalf("%s: first_seen_at/uuid harus dipertahankan", n.SerialNumber)
		}
	}
	// entri yang kembali PENDING tidak lagi punya resolved_at
	for _, n := range r.repo.onus {
		if n.StatusID == onuIDs[P] && n.ResolvedAt != nil {
			t.Fatalf("%s PENDING tidak boleh punya resolved_at", n.SerialNumber)
		}
	}
	// pemangkasan dipanggil dengan batas yang benar pada setiap polling sukses
	if len(r.repo.pruneKeep) != 4 || r.repo.pruneKeep[0] != domain.OLTKeepResolvedONUs {
		t.Fatalf("prune: %v", r.repo.pruneKeep)
	}
	cur, _ := r.svc.Get(ctx, adminA, o.UUID)
	if cur.PollStatus != domain.OLTPollOK || cur.LastPollError != nil {
		t.Fatalf("status OLT setelah polling sukses: %+v", cur)
	}
}

func TestPoll_RBACDanKondisi(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	o := r.mkOLT(t, adminA, "10.1.1.1")

	if _, err := r.svc.Poll(ctx, viewerA, o.UUID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("VIEWER tidak boleh polling: %v", err)
	}
	if _, err := r.svc.Poll(ctx, adminB, o.UUID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("tenant lain: %v", err)
	}
	off := false
	if _, err := r.svc.Update(ctx, adminA, o.UUID, UpdateInput{IsActive: &off}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Poll(ctx, adminA, o.UUID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("OLT nonaktif harus ErrConflict: %v", err)
	}
	if len(r.repo.polls) != 0 {
		t.Fatal("polling yang ditolak tidak boleh tercatat sebagai percobaan koneksi")
	}
}

func TestPoll_GalatTidakMengubahAntrean(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	o := r.mkOLT(t, adminA, "10.1.1.1")
	if _, err := r.svc.Poll(ctx, adminA, o.UUID); err != nil {
		t.Fatal(err)
	}
	// OLT yang sama kini tidak terjangkau: antrean lama harus utuh (tidak ada yang ditandai RESOLVED karena galat)
	host := sim.HostUnreachable
	if _, err := r.svc.Update(ctx, adminA, o.UUID, UpdateInput{Host: &host}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Poll(ctx, adminA, o.UUID); !errors.Is(err, domain.ErrOLTUnreachable) {
		t.Fatalf("harus ErrOLTUnreachable: %v", err)
	}
	wantQueue(t, serialsByStatus(t, r, o), map[string]string{"SIMONU0001": domain.ONUStatusPending, "SIMONU0002": domain.ONUStatusPending, "SIMONU0003": domain.ONUStatusPending})
	cur, _ := r.svc.Get(ctx, adminA, o.UUID)
	if cur.PollStatus != domain.OLTPollUnreachable {
		t.Fatalf("status: %+v", cur)
	}
}

// fixedDriver mengembalikan daftar yang ditentukan test.
type fixedDriver struct {
	code string
	list []domain.UnregisteredONU
}

func (f fixedDriver) Code() string { return f.code }
func (f fixedDriver) Probe(context.Context, domain.OLTConnection) (domain.OLTInfo, error) {
	return domain.OLTInfo{Name: "n\x00ama\n" + strings.Repeat("x", 500), Version: "v1"}, nil
}
func (f fixedDriver) ListUnregisteredONUs(context.Context, domain.OLTConnection) ([]domain.UnregisteredONU, error) {
	return f.list, nil
}

func TestPoll_DataPerangkatDibersihkan(t *testing.T) {
	long := strings.Repeat("s", domain.ONUSerialMaxLen+1)
	r := newRig(t, fixedDriver{code: "fixed", list: []domain.UnregisteredONU{
		{PonPort: "1/1", SerialNumber: "  OKONU0001  ", ONUType: "T"},
		{PonPort: "1/1", SerialNumber: "OKONU0001", ONUType: "duplikat"},           // duplikat serial
		{PonPort: "1/1", SerialNumber: ""},                                         // serial kosong
		{PonPort: "1/1", SerialNumber: long},                                       // serial kepanjangan
		{PonPort: "1/1", SerialNumber: "BAD\x00SERIAL"},                            // karakter kontrol
		{PonPort: strings.Repeat("p", domain.ONUPortMaxLen+1), SerialNumber: "P1"}, // port kepanjangan
		{PonPort: "1/2", SerialNumber: "OKONU0002"},
	}})
	ctx := context.Background()
	o, err := r.svc.Create(ctx, adminA, CreateInput{Name: "x", DriverCode: "fixed", Host: "10.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	sum, err := r.svc.Poll(ctx, adminA, o.UUID)
	if err != nil || sum.Seen != 2 || sum.Skipped != 5 || sum.New != 2 {
		t.Fatalf("ringkasan: %+v %v", sum, err)
	}
	wantQueue(t, serialsByStatus(t, r, o), map[string]string{"OKONU0001": domain.ONUStatusPending, "OKONU0002": domain.ONUStatusPending})

	// identitas yang dilaporkan perangkat dibersihkan dan dipangkas
	got, err := r.svc.Test(ctx, adminA, o.UUID)
	if err != nil || got.ReportedName == nil {
		t.Fatalf("Test: %+v %v", got, err)
	}
	if strings.ContainsAny(*got.ReportedName, "\x00\n") || len([]rune(*got.ReportedName)) > domain.OLTInfoMaxLen {
		t.Fatalf("reported_name tidak dibersihkan: %q", *got.ReportedName)
	}
}

func TestPoll_MelebihiBatasTidakDiterapkan(t *testing.T) {
	big := make([]domain.UnregisteredONU, domain.OLTMaxUnregisteredONUs+1)
	for i := range big {
		big[i] = domain.UnregisteredONU{PonPort: "1/1", SerialNumber: fmt.Sprintf("S%06d", i)}
	}
	r := newRig(t, fixedDriver{code: "big", list: big})
	ctx := context.Background()
	o, _ := r.svc.Create(ctx, adminA, CreateInput{Name: "x", DriverCode: "big", Host: "10.1.1.1"})
	if _, err := r.svc.Poll(ctx, adminA, o.UUID); !errors.Is(err, domain.ErrUpstreamUnavailable) {
		t.Fatalf("harus ditolak: %v", err)
	}
	if len(r.repo.onus) != 0 {
		t.Fatalf("daftar terpotong tidak boleh diterapkan: %d baris", len(r.repo.onus))
	}
	cur, _ := r.svc.Get(ctx, adminA, o.UUID)
	if cur.PollStatus != domain.OLTPollError || cur.LastPollError == nil {
		t.Fatalf("status: %+v", cur)
	}
}

// blockingDriver menahan ListUnregisteredONUs sampai dilepas: untuk menguji satu polling per OLT.
type blockingDriver struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingDriver) Code() string { return "block" }
func (b *blockingDriver) Probe(context.Context, domain.OLTConnection) (domain.OLTInfo, error) {
	return domain.OLTInfo{}, nil
}
func (b *blockingDriver) ListUnregisteredONUs(context.Context, domain.OLTConnection) ([]domain.UnregisteredONU, error) {
	b.once.Do(func() { close(b.started) })
	<-b.release
	return nil, nil
}

func TestPoll_SatuPerOLT(t *testing.T) {
	bd := &blockingDriver{started: make(chan struct{}), release: make(chan struct{})}
	r := newRig(t, bd)
	ctx := context.Background()
	o, _ := r.svc.Create(ctx, adminA, CreateInput{Name: "x", DriverCode: "block", Host: "10.1.1.1"})
	other, _ := r.svc.Create(ctx, adminA, CreateInput{Name: "y", DriverCode: sim.Code, Host: "10.1.1.2"})

	done := make(chan error, 1)
	go func() { _, err := r.svc.Poll(ctx, adminA, o.UUID); done <- err }()
	<-bd.started
	// Dijalankan di goroutine dengan batas waktu: bila penjaga hilang, polling kedua ikut tertahan di driver dan tes ini harus
	// GAGAL CEPAT, bukan menggantung sampai batas waktu go test.
	second := make(chan error, 1)
	go func() { _, err := r.svc.Poll(ctx, adminA, o.UUID); second <- err }()
	select {
	case err := <-second:
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("polling kedua pada OLT yang sama harus ErrConflict: %v", err)
		}
	case <-time.After(2 * time.Second):
		close(bd.release) // lepaskan goroutine yang tertahan agar tes tidak bocor
		t.Fatal("polling kedua pada OLT yang sama tidak ditolak (penjaga satu-polling hilang)")
	}
	// OLT lain tidak terhalang
	if _, err := r.svc.Poll(ctx, adminA, other.UUID); err != nil {
		t.Fatalf("OLT lain tidak boleh terhalang: %v", err)
	}
	close(bd.release)
	if err := <-done; err != nil {
		t.Fatalf("polling pertama: %v", err)
	}
	// penjaga dilepas setelah selesai
	if _, err := r.svc.Poll(ctx, adminA, o.UUID); err != nil {
		t.Fatalf("polling berikutnya harus bisa: %v", err)
	}
}

// ---- antrean ----

func TestSetONUIgnored(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	o := r.mkOLT(t, adminA, "10.1.1.1")
	if _, err := r.svc.Poll(ctx, adminA, o.UUID); err != nil {
		t.Fatal(err)
	}
	rows, _, _ := r.svc.ListONUs(ctx, adminA, ONUListInput{OLTUUID: o.UUID}, domain.Pagination{})
	id := rows[0].UUID

	if _, err := r.svc.SetONUIgnored(ctx, viewerA, id, true); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("VIEWER: %v", err)
	}
	if _, err := r.svc.SetONUIgnored(ctx, adminB, id, true); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("tenant lain: %v", err)
	}
	if _, err := r.svc.SetONUIgnored(ctx, adminA, "tidak-ada", true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tak ada: %v", err)
	}
	// abaikan → idempoten → pulihkan → idempoten
	for i := 0; i < 2; i++ {
		if n, err := r.svc.SetONUIgnored(ctx, nocA, id, true); err != nil || n.Status != domain.ONUStatusIgnored {
			t.Fatalf("abaikan #%d: %+v %v", i, n, err)
		}
	}
	for i := 0; i < 2; i++ {
		if n, err := r.svc.SetONUIgnored(ctx, adminA, id, false); err != nil || n.Status != domain.ONUStatusPending {
			t.Fatalf("pulihkan #%d: %+v %v", i, n, err)
		}
	}
	// entri RESOLVED tidak dapat diubah
	for _, n := range r.repo.onus {
		n.StatusID = onuIDs[domain.ONUStatusResolved]
	}
	if _, err := r.svc.SetONUIgnored(ctx, adminA, id, true); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("RESOLVED harus ErrConflict: %v", err)
	}
	if _, err := r.svc.SetONUIgnored(ctx, adminA, id, false); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("RESOLVED harus ErrConflict (pulihkan): %v", err)
	}
}

func TestListONUs_ScopeDanFilter(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	a := r.mkOLT(t, adminA, "10.1.1.1")
	b, _ := r.svc.Create(ctx, adminB, CreateInput{Name: "B", DriverCode: sim.Code, Host: "10.1.1.2"})
	for _, o := range []*domain.OLT{a, b} {
		a2 := adminA
		if o.UUID == b.UUID {
			a2 = adminB
		}
		if _, err := r.svc.Poll(ctx, a2, o.UUID); err != nil {
			t.Fatal(err)
		}
	}
	if rows, total, err := r.svc.ListONUs(ctx, viewerA, ONUListInput{TenantID: u64(8)}, domain.Pagination{}); err != nil || total != 3 || rows[0].OLTUUID != a.UUID {
		t.Fatalf("viewer hanya tenant-nya (tenant_id klien diabaikan): %d %v", total, err)
	}
	if _, total, _ := r.svc.ListONUs(ctx, superadmin, ONUListInput{}, domain.Pagination{}); total != 6 {
		t.Fatalf("superadmin semua: %d", total)
	}
	if _, total, _ := r.svc.ListONUs(ctx, superadmin, ONUListInput{TenantID: u64(8)}, domain.Pagination{}); total != 3 {
		t.Fatalf("superadmin filter tenant: %d", total)
	}
	// olt_id milik tenant lain ditolak, bukan bocor
	if _, _, err := r.svc.ListONUs(ctx, viewerA, ONUListInput{OLTUUID: b.UUID}, domain.Pagination{}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("olt tenant lain: %v", err)
	}
	if _, _, err := r.svc.ListONUs(ctx, noTenant, ONUListInput{}, domain.Pagination{}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("tanpa tenant: %v", err)
	}
	if _, _, err := r.svc.ListONUs(ctx, viewerA, ONUListInput{Status: "BUKAN_STATUS"}, domain.Pagination{}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("status tak dikenal: %v", err)
	}
	if _, total, err := r.svc.ListONUs(ctx, viewerA, ONUListInput{Status: domain.ONUStatusPending}, domain.Pagination{}); err != nil || total != 3 {
		t.Fatalf("filter status: %d %v", total, err)
	}
	if _, total, _ := r.svc.ListONUs(ctx, viewerA, ONUListInput{Status: domain.ONUStatusResolved}, domain.Pagination{}); total != 0 {
		t.Fatalf("filter RESOLVED: %d", total)
	}
}

func TestDrivers(t *testing.T) {
	r := newRig(t)
	if got := r.svc.Drivers(); len(got) != 1 || got[0] != sim.Code {
		t.Fatalf("Drivers: %v", got)
	}
}
