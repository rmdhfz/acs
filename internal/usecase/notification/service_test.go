package notification

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/notify"
	"acs/pkg/mailer"
)

// ---- fakes ----

type fUser struct {
	rc     domain.NotificationRecipient
	active bool
}

type fakeRepo struct {
	mu      sync.Mutex
	types   map[string]domain.NotificationType
	sev     map[string]uint64
	users   []fUser
	prefs   map[[2]uint64]domain.NotificationPreference
	rows    []*domain.Notification
	nextID  uint64
	health  []domain.TenantDeviceHealth
	failed  []domain.TenantCount
	dedupeQ int
	clock   func() time.Time // jam fake: created_at mengikuti jam uji, bukan jam dinding
}

func newFakeRepo() *fakeRepo {
	r := &fakeRepo{
		types: map[string]domain.NotificationType{}, sev: map[string]uint64{domain.NotificationSeverityInfo: 1, domain.NotificationSeverityWarning: 2, domain.NotificationSeverityCritical: 3},
		prefs: map[[2]uint64]domain.NotificationPreference{},
	}
	add := func(id uint64, code, sev string, inApp, email, mand bool) {
		r.types[code] = domain.NotificationType{ID: id, Code: code, Name: code, Severity: sev, SeverityID: r.sev[sev], DefaultInApp: inApp, DefaultEmail: email, IsMandatory: mand}
	}
	add(1, domain.NotifSecurityAlert, "INFO", true, true, true)
	add(2, domain.NotifDeviceOfflineSpike, "WARNING", true, true, false)
	add(3, domain.NotifTaskFailures, "WARNING", true, false, false)
	add(4, domain.NotifRolloutPaused, "CRITICAL", true, true, false)
	add(5, domain.NotifDeviceLimitReached, "WARNING", true, true, false)
	add(6, domain.NotifNewTenantSignup, "INFO", true, true, false)
	add(7, domain.NotifAnnouncement, "INFO", true, true, false)
	add(8, domain.NotifOpticalAlert, "WARNING", true, false, false)
	return r
}

func u64(v uint64) *uint64 { return &v }

func (r *fakeRepo) addUser(id uint64, tenant *uint64, active bool, roles ...string) {
	r.users = append(r.users, fUser{rc: domain.NotificationRecipient{UserID: id, TenantID: tenant, Email: "u" + string(rune('0'+id)) + "@example.test", FullName: "User", Roles: roles}, active: active})
}

func (r *fakeRepo) InsertBatch(_ context.Context, items []*domain.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range items {
		r.nextID++
		n.ID = r.nextID
		n.CreatedAt = r.clock()
		r.rows = append(r.rows, n)
	}
	return nil
}
func (r *fakeRepo) List(_ context.Context, userID uint64, unreadOnly bool, p domain.Pagination) ([]domain.NotificationItem, int, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var items []domain.NotificationItem
	unread := 0
	for _, n := range r.rows {
		if n.UserID != userID {
			continue
		}
		if n.ReadAt == nil {
			unread++
		}
		if unreadOnly && n.ReadAt != nil {
			continue
		}
		items = append(items, domain.NotificationItem{ID: n.NotificationUUID, Title: n.Title, ReadAt: n.ReadAt})
	}
	return items, len(items), unread, nil
}
func (r *fakeRepo) CountUnread(_ context.Context, userID uint64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := 0
	for _, n := range r.rows {
		if n.UserID == userID && n.ReadAt == nil {
			c++
		}
	}
	return c, nil
}
func (r *fakeRepo) MarkRead(_ context.Context, userID uint64, uuids []string, at time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var c int64
	for _, n := range r.rows {
		for _, u := range uuids {
			if n.UserID == userID && n.NotificationUUID == u && n.ReadAt == nil {
				t := at
				n.ReadAt = &t
				c++
			}
		}
	}
	return c, nil
}
func (r *fakeRepo) MarkAllRead(_ context.Context, userID uint64, at time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var c int64
	for _, n := range r.rows {
		if n.UserID == userID && n.ReadAt == nil {
			t := at
			n.ReadAt = &t
			c++
		}
	}
	return c, nil
}
func (r *fakeRepo) ListTypes(context.Context) ([]domain.NotificationType, error) {
	var out []domain.NotificationType
	for _, t := range r.types {
		out = append(out, t)
	}
	return out, nil
}
func (r *fakeRepo) GetTypeByCode(_ context.Context, code string) (*domain.NotificationType, error) {
	t, ok := r.types[code]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &t, nil
}
func (r *fakeRepo) SeverityIDByCode(_ context.Context, code string) (uint64, error) {
	if id, ok := r.sev[code]; ok {
		return id, nil
	}
	return 0, domain.ErrNotFound
}
func (r *fakeRepo) ListPreferences(_ context.Context, userID uint64) ([]domain.NotificationPreference, error) {
	var out []domain.NotificationPreference
	for k, p := range r.prefs {
		if k[0] == userID {
			out = append(out, p)
		}
	}
	return out, nil
}
func (r *fakeRepo) ListPreferencesForUsers(_ context.Context, typeID uint64, userIDs []uint64) ([]domain.NotificationPreference, error) {
	var out []domain.NotificationPreference
	for _, id := range userIDs {
		if p, ok := r.prefs[[2]uint64{id, typeID}]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}
func (r *fakeRepo) UpsertPreferences(_ context.Context, userID uint64, prefs []domain.NotificationPreference) error {
	for _, p := range prefs {
		p.UserID = userID
		r.prefs[[2]uint64{userID, p.TypeID}] = p
	}
	return nil
}
func (r *fakeRepo) ExistsDedupe(_ context.Context, typeID uint64, tenantID *uint64, key string, since time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dedupeQ++
	for _, n := range r.rows {
		sameTenant := (n.TenantID == nil && tenantID == nil) || (n.TenantID != nil && tenantID != nil && *n.TenantID == *tenantID)
		if n.TypeID == typeID && sameTenant && n.DedupeKey != nil && *n.DedupeKey == key && !n.CreatedAt.Before(since) {
			return true, nil
		}
	}
	return false, nil
}
func (r *fakeRepo) EmailedUsersSince(_ context.Context, typeID uint64, userIDs []uint64, since time.Time) (map[uint64]bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[uint64]bool{}
	for _, n := range r.rows {
		if n.TypeID == typeID && n.EmailedAt != nil && !n.EmailedAt.Before(since) {
			out[n.UserID] = true
		}
	}
	return out, nil
}
func hasAny(have []string, want []string) bool {
	for _, h := range have {
		for _, w := range want {
			if h == w {
				return true
			}
		}
	}
	return false
}
func (r *fakeRepo) ListRecipients(_ context.Context, tenantID *uint64, roles []string) ([]domain.NotificationRecipient, error) {
	var out []domain.NotificationRecipient
	for _, u := range r.users {
		if !u.active || !hasAny(u.rc.Roles, roles) {
			continue
		}
		if tenantID != nil && (u.rc.TenantID == nil || *u.rc.TenantID != *tenantID) {
			continue
		}
		out = append(out, u.rc)
	}
	return out, nil
}
func (r *fakeRepo) GetRecipient(_ context.Context, userID uint64) (*domain.NotificationRecipient, error) {
	for _, u := range r.users {
		if u.rc.UserID == userID && u.active {
			rc := u.rc
			return &rc, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *fakeRepo) TenantDeviceHealth(context.Context) ([]domain.TenantDeviceHealth, error) {
	return r.health, nil
}
func (r *fakeRepo) FailedTasksSince(context.Context, time.Time) ([]domain.TenantCount, error) {
	return r.failed, nil
}

func (r *fakeRepo) forUser(id uint64) []*domain.Notification {
	var out []*domain.Notification
	for _, n := range r.rows {
		if n.UserID == id {
			out = append(out, n)
		}
	}
	return out
}

type fakeMail struct {
	mu   sync.Mutex
	msgs []mailer.Message
}

func (f *fakeMail) Notify(m mailer.Message) {
	f.mu.Lock()
	f.msgs = append(f.msgs, m)
	f.mu.Unlock()
}

type pushRec struct {
	user uint64
	typ  string
	data map[string]any
}
type fakePush struct{ got []pushRec }

func (f *fakePush) BroadcastToUser(userID uint64, eventType string, payload interface{}) {
	m, _ := payload.(map[string]any)
	f.got = append(f.got, pushRec{userID, eventType, m})
}

type fakeTenants struct {
	domain.TenantRepository
	known map[uint64]bool
}

func (f fakeTenants) GetByID(_ context.Context, id uint64) (*domain.Tenant, error) {
	if f.known[id] {
		return &domain.Tenant{ID: id}, nil
	}
	return nil, domain.ErrNotFound
}

type fakeActivity struct {
	domain.ActivityLogRepository
	recorded []*domain.ActivityLog
}

func (f *fakeActivity) Record(_ context.Context, l *domain.ActivityLog) error {
	f.recorded = append(f.recorded, l)
	return nil
}

type env struct {
	repo *fakeRepo
	mail *fakeMail
	push *fakePush
	act  *fakeActivity
	svc  *Service
	now  time.Time
}

// Tenant 1: admin(10), noc(11), viewer(12), enduser(13), admin nonaktif(14). Tenant 2: admin(20). Superadmin(1).
func newEnv(t *testing.T, withEmail bool) *env {
	t.Helper()
	repo := newFakeRepo()
	t1, t2 := u64(1), u64(2)
	repo.addUser(1, nil, true, domain.RoleSuperadmin)
	repo.addUser(10, t1, true, domain.RoleAdmin)
	repo.addUser(11, t1, true, domain.RoleNOC)
	repo.addUser(12, t1, true, domain.RoleViewer)
	repo.addUser(13, t1, true, domain.RoleEndUser)
	repo.addUser(14, t1, false, domain.RoleAdmin)
	repo.addUser(20, t2, true, domain.RoleAdmin)
	e := &env{repo: repo, mail: &fakeMail{}, push: &fakePush{}, act: &fakeActivity{}, now: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
	e.svc = NewService(repo, fakeTenants{known: map[uint64]bool{1: true, 2: true}}, e.act, Config{}, nil).WithPusher(e.push)
	e.svc.now = func() time.Time { return e.now }
	repo.clock = e.svc.now
	if withEmail {
		e.svc.WithEmail(e.mail, notify.NewBuilder("ACS Console", "https://acs.example.test"))
	}
	return e
}

func (e *env) users(t *testing.T) []uint64 {
	var ids []uint64
	for id := uint64(1); id <= 20; id++ {
		if len(e.repo.forUser(id)) > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func eq(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func ev(code string, tenant *uint64) domain.NotificationEvent {
	return domain.NotificationEvent{TypeCode: code, TenantID: tenant, Title: "Judul", Body: "Isi pesan", Link: "/devices"}
}

var ctx = context.Background()

// ---- resolusi penerima per audiens ----

func TestAudiens_PerJenis(t *testing.T) {
	cases := []struct {
		name   string
		code   string
		tenant *uint64
		want   []uint64
	}{
		{"offline spike -> ADMIN+NOC tenant, bukan viewer/enduser/nonaktif/tenant lain", domain.NotifDeviceOfflineSpike, u64(1), []uint64{10, 11}},
		{"task failures -> ADMIN+NOC", domain.NotifTaskFailures, u64(1), []uint64{10, 11}},
		{"redaman optik -> ADMIN+NOC tenant perangkat", domain.NotifOpticalAlert, u64(1), []uint64{10, 11}},
		{"rollout paused -> ADMIN saja", domain.NotifRolloutPaused, u64(1), []uint64{10}},
		{"device limit -> ADMIN tenant + SUPERADMIN", domain.NotifDeviceLimitReached, u64(1), []uint64{1, 10}},
		{"signup -> SUPERADMIN", domain.NotifNewTenantSignup, u64(1), []uint64{1}},
		{"pengumuman tenant -> semua staf tenant (tanpa ENDUSER)", domain.NotifAnnouncement, u64(1), []uint64{10, 11, 12}},
		{"pengumuman platform -> semua staf semua tenant", domain.NotifAnnouncement, nil, []uint64{1, 10, 11, 12, 20}},
		{"rollout tanpa tenant -> hanya SUPERADMIN (tak menyebar ke semua tenant)", domain.NotifRolloutPaused, nil, []uint64{1}},
	}
	for _, c := range cases {
		e := newEnv(t, false)
		if err := e.svc.Notify(ctx, ev(c.code, c.tenant)); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := e.users(t); !eq(got, c.want) {
			t.Errorf("%s: penerima %v, mau %v", c.name, got, c.want)
		}
		if len(e.repo.forUser(13)) != 0 {
			t.Errorf("%s: ENDUSER tidak boleh menerima", c.name)
		}
	}
}

func TestSecurityAlert_SelfDanEnduserTakPernah(t *testing.T) {
	e := newEnv(t, false)
	x := ev(domain.NotifSecurityAlert, u64(1))
	x.Audience = domain.NotificationAudience{Kind: domain.AudienceSelf, UserID: 11}
	if err := e.svc.Notify(ctx, x); err != nil {
		t.Fatal(err)
	}
	if !eq(e.users(t), []uint64{11}) {
		t.Fatalf("SECURITY_ALERT hanya untuk pengguna itu sendiri: %v", e.users(t))
	}
	// ENDUSER dan pengguna nonaktif tidak menerima
	for _, id := range []uint64{13, 14, 999} {
		x.Audience.UserID = id
		_ = e.svc.Notify(ctx, x)
		if len(e.repo.forUser(id)) != 0 {
			t.Errorf("pengguna %d tidak boleh menerima", id)
		}
	}
	// tanpa audiens eksplisit -> ditolak (tak ada bawaan, cegah salah kirim massal)
	if err := e.svc.Notify(ctx, ev(domain.NotifSecurityAlert, u64(1))); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("SECURITY_ALERT tanpa audiens harus ditolak: %v", err)
	}
	// ENDUSER tidak lolos walau diminta lewat AudienceTenantRoles
	y := ev(domain.NotifAnnouncement, u64(1))
	y.Audience = domain.NotificationAudience{Kind: domain.AudienceTenantRoles, Roles: []string{domain.RoleEndUser}}
	e2 := newEnv(t, false)
	_ = e2.svc.Notify(ctx, y)
	if len(e2.repo.rows) != 0 {
		t.Fatal("peran ENDUSER tidak boleh pernah jadi audiens")
	}
}

// ---- preferensi ----

func TestPreferensi_BawaanDanOverride(t *testing.T) {
	e := newEnv(t, true)
	// TASK_FAILURES: bawaan in-app saja, tanpa email
	_ = e.svc.Notify(ctx, ev(domain.NotifTaskFailures, u64(1)))
	if len(e.mail.msgs) != 0 {
		t.Fatalf("bawaan TASK_FAILURES tanpa email, terkirim %d", len(e.mail.msgs))
	}
	if len(e.repo.forUser(10)) != 1 || e.repo.forUser(10)[0].EmailedAt != nil {
		t.Fatal("harus ada notifikasi in-app tanpa emailed_at")
	}
	// admin(10) menyalakan email untuk TASK_FAILURES, noc(11) mematikan in-app
	e.repo.prefs[[2]uint64{10, 3}] = domain.NotificationPreference{UserID: 10, TypeID: 3, InApp: true, Email: true}
	e.repo.prefs[[2]uint64{11, 3}] = domain.NotificationPreference{UserID: 11, TypeID: 3, InApp: false, Email: false}
	e.repo.rows = nil
	_ = e.svc.Notify(ctx, ev(domain.NotifTaskFailures, u64(1)))
	if len(e.repo.forUser(10)) != 1 || e.repo.forUser(10)[0].EmailedAt == nil || len(e.mail.msgs) != 1 {
		t.Fatalf("admin harus dapat in-app + email: rows=%d mails=%d", len(e.repo.forUser(10)), len(e.mail.msgs))
	}
	if len(e.repo.forUser(11)) != 0 {
		t.Fatal("noc mematikan semua kanal: tidak boleh ada notifikasi")
	}
}

func TestPreferensi_InAppMatiEmailNyala_BarisSudahDibaca(t *testing.T) {
	e := newEnv(t, true)
	e.repo.prefs[[2]uint64{10, 2}] = domain.NotificationPreference{UserID: 10, TypeID: 2, InApp: false, Email: true}
	_ = e.svc.Notify(ctx, ev(domain.NotifDeviceOfflineSpike, u64(1)))
	rows := e.repo.forUser(10)
	if len(rows) != 1 || rows[0].ReadAt == nil || rows[0].EmailedAt == nil {
		t.Fatalf("email-saja: baris dicatat sudah dibaca + emailed: %+v", rows)
	}
	for _, p := range e.push.got {
		if p.user == 10 {
			t.Fatal("pengguna yang mematikan in-app tidak boleh dapat push")
		}
	}
	if c, _ := e.repo.CountUnread(ctx, 10); c != 0 {
		t.Fatal("tidak boleh menaikkan unread")
	}
}

func TestMandatory_TakBisaDimatikan(t *testing.T) {
	e := newEnv(t, true)
	// bahkan bila ada preferensi tersimpan yang mematikan, jenis wajib tetap in-app + email
	e.repo.prefs[[2]uint64{11, 1}] = domain.NotificationPreference{UserID: 11, TypeID: 1, InApp: false, Email: false}
	x := ev(domain.NotifSecurityAlert, u64(1))
	x.Audience = domain.NotificationAudience{Kind: domain.AudienceSelf, UserID: 11}
	_ = e.svc.Notify(ctx, x)
	rows := e.repo.forUser(11)
	if len(rows) != 1 || rows[0].ReadAt != nil || rows[0].EmailedAt == nil || len(e.mail.msgs) != 1 {
		t.Fatalf("mandatory: harus in-app + email: %+v mails=%d", rows, len(e.mail.msgs))
	}
	// NoEmail dihormati walau mandatory (email transaksional sudah ada)
	e2 := newEnv(t, true)
	x.NoEmail = true
	_ = e2.svc.Notify(ctx, x)
	if len(e2.mail.msgs) != 0 || len(e2.repo.forUser(11)) != 1 {
		t.Fatal("NoEmail: in-app ada, email tidak")
	}
	// API: mematikan jenis wajib ditolak 400, tidak ada penyimpanan sebagian
	err := e.svc.UpdatePreferences(ctx, domain.Actor{UserID: 11}, []PreferenceInput{
		{Type: domain.NotifTaskFailures, InApp: true, Email: true},
		{Type: domain.NotifSecurityAlert, InApp: true, Email: false},
	})
	if !errors.Is(err, domain.ErrInvalidInput) || !strings.Contains(err.Error(), domain.NotifSecurityAlert) {
		t.Fatalf("mematikan jenis wajib harus ErrInvalidInput yang jelas: %v", err)
	}
	if _, ok := e.repo.prefs[[2]uint64{11, 3}]; ok {
		t.Fatal("permintaan ditolak harus atomik: tidak ada preferensi tersimpan")
	}
}

func TestPreferensi_APIListDanUpdate(t *testing.T) {
	e := newEnv(t, true)
	actor := domain.Actor{UserID: 10}
	list, err := e.svc.ListPreferences(ctx, actor)
	if err != nil || len(list) != 8 {
		t.Fatalf("list: %d %v", len(list), err)
	}
	byType := map[string]PreferenceView{}
	for _, p := range list {
		byType[p.Type] = p
	}
	if p := byType[domain.NotifTaskFailures]; !p.InApp || p.Email || p.Mandatory {
		t.Fatalf("bawaan TASK_FAILURES salah: %+v", p)
	}
	if p := byType[domain.NotifSecurityAlert]; !p.Mandatory || !p.InApp || !p.Email {
		t.Fatalf("SECURITY_ALERT harus mandatory in-app+email: %+v", p)
	}
	if err := e.svc.UpdatePreferences(ctx, actor, []PreferenceInput{{Type: domain.NotifTaskFailures, InApp: false, Email: true}}); err != nil {
		t.Fatal(err)
	}
	list, _ = e.svc.ListPreferences(ctx, actor)
	for _, p := range list {
		if p.Type == domain.NotifTaskFailures && (p.InApp || !p.Email) {
			t.Fatalf("preferensi tidak tersimpan: %+v", p)
		}
	}
	// pengguna lain tidak terpengaruh
	other, _ := e.svc.ListPreferences(ctx, domain.Actor{UserID: 11})
	for _, p := range other {
		if p.Type == domain.NotifTaskFailures && (!p.InApp || p.Email) {
			t.Fatal("preferensi pengguna lain tidak boleh berubah")
		}
	}
	for _, bad := range [][]PreferenceInput{nil, {{Type: "NGAWUR", InApp: true}}} {
		if err := e.svc.UpdatePreferences(ctx, actor, bad); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("input tidak sah harus 400: %v", err)
		}
	}
	if err := e.svc.UpdatePreferences(ctx, domain.Actor{}, []PreferenceInput{{Type: domain.NotifTaskFailures}}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("tanpa identitas pengguna harus ditolak: %v", err)
	}
}

// ---- dedupe & throttle email ----

func TestDedupe_DalamJendela(t *testing.T) {
	e := newEnv(t, false)
	x := ev(domain.NotifDeviceOfflineSpike, u64(1))
	x.DedupeKey, x.DedupeWindow = "offline-spike", 6*time.Hour
	_ = e.svc.Notify(ctx, x)
	first := len(e.repo.rows)
	if first != 2 {
		t.Fatalf("pertama: 2 penerima, dapat %d", first)
	}
	e.now = e.now.Add(5 * time.Hour)
	_ = e.svc.Notify(ctx, x)
	if len(e.repo.rows) != first {
		t.Fatal("dalam jendela 6 jam: tidak boleh dibuat ulang")
	}
	// tenant lain dengan kunci sama tetap dibuat
	y := x
	y.TenantID = u64(2)
	_ = e.svc.Notify(ctx, y)
	if len(e.repo.forUser(20)) == 0 { // 20 = ADMIN tenant 2
		t.Fatal("dedupe per tenant: tenant lain harus tetap menerima")
	}
	// lewat jendela -> dibuat lagi
	e.now = e.now.Add(2 * time.Hour)
	_ = e.svc.Notify(ctx, x)
	if len(e.repo.forUser(10)) != 2 {
		t.Fatalf("lewat jendela harus dibuat lagi: %d", len(e.repo.forUser(10)))
	}
	// tanpa window = tanpa dedupe
	z := ev(domain.NotifDeviceOfflineSpike, u64(1))
	z.DedupeKey = "k"
	_ = e.svc.Notify(ctx, z)
	_ = e.svc.Notify(ctx, z)
	if len(e.repo.forUser(10)) != 4 {
		t.Fatal("dedupe butuh DedupeWindow > 0")
	}
}

func TestEmail_ThrottleSatuPerJamPerJenis(t *testing.T) {
	e := newEnv(t, true)
	x := ev(domain.NotifDeviceOfflineSpike, u64(1))
	_ = e.svc.Notify(ctx, x)
	if len(e.mail.msgs) != 2 {
		t.Fatalf("ADMIN+NOC: 2 email, dapat %d", len(e.mail.msgs))
	}
	e.now = e.now.Add(30 * time.Minute)
	_ = e.svc.Notify(ctx, x)
	if len(e.mail.msgs) != 2 {
		t.Fatalf("dalam 1 jam: tidak ada email baru, dapat %d", len(e.mail.msgs))
	}
	if len(e.repo.forUser(10)) != 2 {
		t.Fatal("in-app tetap dibuat walau email di-throttle")
	}
	if e.repo.forUser(10)[1].EmailedAt != nil {
		t.Fatal("notifikasi yang email-nya di-throttle tidak boleh bertanda emailed")
	}
	// jenis lain tidak ikut di-throttle
	_ = e.svc.Notify(ctx, ev(domain.NotifRolloutPaused, u64(1)))
	if len(e.mail.msgs) != 3 {
		t.Fatalf("jenis lain tidak di-throttle: %d", len(e.mail.msgs))
	}
	// lewat 1 jam -> email lagi
	e.now = e.now.Add(time.Hour)
	_ = e.svc.Notify(ctx, x)
	if len(e.mail.msgs) != 5 {
		t.Fatalf("setelah 1 jam email lagi: %d", len(e.mail.msgs))
	}
}

func TestEmail_NonaktifBilaSMTPTidakAda(t *testing.T) {
	e := newEnv(t, false)
	_ = e.svc.Notify(ctx, ev(domain.NotifDeviceOfflineSpike, u64(1)))
	for _, n := range e.repo.rows {
		if n.EmailedAt != nil {
			t.Fatal("tanpa mailer, emailed_at tidak boleh terisi")
		}
	}
	if len(e.repo.rows) != 2 {
		t.Fatal("in-app tetap jalan")
	}
}

func TestEmail_IsiAmanDanTautanAbsolut(t *testing.T) {
	e := newEnv(t, true)
	x := ev(domain.NotifRolloutPaused, u64(1))
	x.Title = "Rollout <b>dijeda</b>"
	x.Body = "Batch #5 <script>alert(1)</script>\nBaris kedua"
	x.Link = "/firmware"
	_ = e.svc.Notify(ctx, x)
	if len(e.mail.msgs) != 1 {
		t.Fatalf("1 email untuk ADMIN: %d", len(e.mail.msgs))
	}
	m := e.mail.msgs[0]
	if strings.Contains(m.HTML, "<script>") || strings.Contains(m.HTML, "<b>dijeda") {
		t.Error("isi harus di-escape di HTML")
	}
	if !strings.Contains(m.HTML, "https://acs.example.test/firmware") || !strings.Contains(m.Text, "Baris kedua") || !strings.Contains(m.HTML, "Kritis") {
		t.Errorf("email tidak lengkap: %s", m.Text)
	}
}

// ---- sanitasi: link berbahaya, panjang ----

func TestLinkBerbahayaDibuangPadaNotify(t *testing.T) {
	for _, l := range []string{"https://evil.test", "//evil.test/x", "javascript:alert(1)", `/\evil`, "/a//b", "/a b"} {
		e := newEnv(t, true)
		x := ev(domain.NotifRolloutPaused, u64(1))
		x.Link = l
		_ = e.svc.Notify(ctx, x)
		if len(e.repo.rows) != 1 {
			t.Fatalf("notifikasi tetap dibuat untuk link %q", l)
		}
		if e.repo.rows[0].Link != nil {
			t.Errorf("link %q harus dibuang, tersimpan %q", l, *e.repo.rows[0].Link)
		}
		if strings.Contains(e.mail.msgs[0].HTML, "evil") || strings.Contains(e.mail.msgs[0].HTML, "javascript") {
			t.Errorf("link %q bocor ke email", l)
		}
	}
}

func TestPanjangDipotongDanKontrolDibuang(t *testing.T) {
	e := newEnv(t, false)
	x := ev(domain.NotifRolloutPaused, u64(1))
	x.Title = strings.Repeat("é", 400) + "\r\nBcc: x"
	x.Body = strings.Repeat("a", 5000)
	_ = e.svc.Notify(ctx, x)
	n := e.repo.rows[0]
	if len([]rune(n.Title)) > MaxTitleLen || len([]rune(n.Body)) > MaxBodyLen {
		t.Fatalf("harus dipotong: %d/%d", len([]rune(n.Title)), len([]rune(n.Body)))
	}
	if strings.ContainsAny(n.Title, "\r\n") {
		t.Fatal("judul tidak boleh memuat CR/LF")
	}
	if err := e.svc.Notify(ctx, domain.NotificationEvent{TypeCode: domain.NotifRolloutPaused, TenantID: u64(1)}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("judul/isi kosong harus ditolak: %v", err)
	}
	if err := e.svc.Notify(ctx, domain.NotificationEvent{TypeCode: "TIDAK_ADA", Title: "a", Body: "b"}); err == nil {
		t.Fatal("jenis tak dikenal harus error")
	}
}

func TestSeverityOverride(t *testing.T) {
	e := newEnv(t, false)
	x := ev(domain.NotifTaskFailures, u64(1))
	x.SeverityOverride = domain.NotificationSeverityCritical
	_ = e.svc.Notify(ctx, x)
	if e.repo.rows[0].SeverityID != 3 {
		t.Fatalf("override CRITICAL (id 3), dapat %d", e.repo.rows[0].SeverityID)
	}
	x.SeverityOverride = "NGAWUR"
	if err := e.svc.Notify(ctx, x); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("keparahan tak dikenal: %v", err)
	}
}

// ---- push realtime ----

func TestPushRealtimeKePenerima(t *testing.T) {
	e := newEnv(t, false)
	_ = e.svc.Notify(ctx, ev(domain.NotifRolloutPaused, u64(1)))
	if len(e.push.got) != 1 || e.push.got[0].user != 10 || e.push.got[0].typ != "NOTIFICATION" {
		t.Fatalf("push: %+v", e.push.got)
	}
	d := e.push.got[0].data
	if d["id"] != e.repo.rows[0].NotificationUUID || d["type"] != domain.NotifRolloutPaused || d["severity"] != "CRITICAL" || d["title"] != "Judul" || d["link"] != "/devices" {
		t.Fatalf("payload push: %+v", d)
	}
	if _, err := uuid.Parse(d["id"].(string)); err != nil {
		t.Fatal("id push harus uuid")
	}
}

// ---- evaluator ----

func TestEvaluate_OfflineSpikeDanTaskFailures(t *testing.T) {
	e := newEnv(t, false)
	e.repo.health = []domain.TenantDeviceHealth{
		{TenantID: 1, Total: 10, Offline: 3},   // tepat 30% & 10 perangkat -> memicu
		{TenantID: 2, Total: 9, Offline: 9},    // < 10 perangkat -> tidak
		{TenantID: 3, Total: 100, Offline: 29}, // < 30% -> tidak
	}
	e.repo.failed = []domain.TenantCount{{TenantID: 1, Count: 5}, {TenantID: 2, Count: 4}}
	n, err := e.svc.Evaluate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// tenant 1: offline spike (admin10+noc11) + task failures (10+11) = 4 baris
	if n != 4 || len(e.repo.rows) != 4 {
		t.Fatalf("harus 4 notifikasi, dapat %d", n)
	}
	for _, r := range e.repo.rows {
		if r.TenantID == nil || *r.TenantID != 1 {
			t.Fatalf("hanya tenant 1 yang memicu: %+v", r)
		}
	}
	// evaluasi berikutnya (5 menit kemudian) tidak menggandakan (dedupe 6 jam / 1 jam)
	e.now = e.now.Add(5 * time.Minute)
	if n, _ := e.svc.Evaluate(ctx); n != 0 {
		t.Fatalf("dedupe: tidak ada yang baru, dapat %d", n)
	}
	// task failures dibuat lagi setelah 1 jam; offline spike belum (6 jam)
	e.now = e.now.Add(70 * time.Minute)
	if n, _ := e.svc.Evaluate(ctx); n != 2 {
		t.Fatalf("setelah 1 jam hanya task failures yang kembali: %d", n)
	}
	e.now = e.now.Add(6 * time.Hour)
	if n, _ := e.svc.Evaluate(ctx); n != 4 {
		t.Fatalf("setelah 6 jam keduanya kembali: %d", n)
	}
}

func TestEvaluate_AmbangDariConfig(t *testing.T) {
	e := newEnv(t, false)
	e.svc.cfg = Config{OfflineRatio: 0.5, FailedTasks: 8}
	e.repo.health = []domain.TenantDeviceHealth{{TenantID: 1, Total: 20, Offline: 9}}
	e.repo.failed = []domain.TenantCount{{TenantID: 1, Count: 7}}
	if n, _ := e.svc.Evaluate(ctx); n != 0 {
		t.Fatalf("di bawah ambang kustom: %d", n)
	}
	e.repo.health[0].Offline = 10
	e.repo.failed[0].Count = 8
	if n, _ := e.svc.Evaluate(ctx); n != 4 {
		t.Fatalf("mencapai ambang kustom: %d", n)
	}
	// nilai tak sah jatuh ke bawaan
	s := NewService(newFakeRepo(), nil, nil, Config{OfflineRatio: 7, FailedTasks: -1}, nil)
	if s.cfg.OfflineRatio != DefaultOfflineRatio || s.cfg.FailedTasks != DefaultFailedTasks {
		t.Fatalf("config tak sah harus jatuh ke bawaan: %+v", s.cfg)
	}
}

// ---- API: list / baca / isolasi ----

func TestMarkRead_HanyaMilikSendiri(t *testing.T) {
	e := newEnv(t, false)
	_ = e.svc.Notify(ctx, ev(domain.NotifDeviceOfflineSpike, u64(1)))
	adminRow, nocRow := e.repo.forUser(10)[0], e.repo.forUser(11)[0]

	// admin mencoba menandai notifikasi milik NOC: diam-diam tidak berefek
	if err := e.svc.MarkRead(ctx, domain.Actor{UserID: 10}, []string{nocRow.NotificationUUID}, false); err != nil {
		t.Fatal(err)
	}
	if nocRow.ReadAt != nil {
		t.Fatal("notifikasi pengguna lain tidak boleh berubah")
	}
	if c, _ := e.svc.UnreadCount(ctx, domain.Actor{UserID: 10}); c != 1 {
		t.Fatalf("unread admin 1, dapat %d", c)
	}
	if err := e.svc.MarkRead(ctx, domain.Actor{UserID: 10}, []string{strings.ToUpper(adminRow.NotificationUUID)}, false); err != nil {
		t.Fatal(err)
	}
	if c, _ := e.svc.UnreadCount(ctx, domain.Actor{UserID: 10}); c != 0 {
		t.Fatalf("setelah mark-read unread 0, dapat %d", c)
	}
	if err := e.svc.MarkRead(ctx, domain.Actor{UserID: 11}, nil, true); err != nil || nocRow.ReadAt == nil {
		t.Fatalf("all=true menandai semua milik sendiri: %v", err)
	}
	for _, bad := range [][]string{nil, {"bukan-uuid"}, make([]string, maxMarkIDs+1)} {
		if err := e.svc.MarkRead(ctx, domain.Actor{UserID: 10}, bad, false); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("ids %v harus ditolak 400: %v", len(bad), err)
		}
	}
	if err := e.svc.MarkRead(ctx, domain.Actor{}, nil, true); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("tanpa identitas ditolak")
	}
}

func TestList_HanyaMilikSendiri(t *testing.T) {
	e := newEnv(t, false)
	_ = e.svc.Notify(ctx, ev(domain.NotifRolloutPaused, u64(1)))
	items, total, unread, err := e.svc.List(ctx, domain.Actor{UserID: 10}, false, domain.Pagination{})
	if err != nil || total != 1 || unread != 1 || len(items) != 1 {
		t.Fatalf("admin: %d %d %v", total, unread, err)
	}
	if items, total, _, _ = e.svc.List(ctx, domain.Actor{UserID: 20}, false, domain.Pagination{}); total != 0 || len(items) != 0 {
		t.Fatal("admin tenant lain tidak boleh melihat")
	}
}

// ---- pengumuman ----

func TestAnnounce_HanyaSuperadmin(t *testing.T) {
	e := newEnv(t, true)
	in := AnnounceInput{Title: "Pemeliharaan", Body: "Sabtu malam", Link: "/dashboard", TenantID: u64(1), Email: true}
	for _, a := range []domain.Actor{
		{UserID: 10, TenantID: u64(1), Roles: []string{domain.RoleAdmin}},
		{UserID: 11, TenantID: u64(1), Roles: []string{domain.RoleNOC}},
		{UserID: 1, Roles: []string{domain.RoleSuperadmin}, ImpersonatorID: 5}, // sesi impersonate
		{Roles: []string{domain.RoleSuperadmin}},                               // tanpa user
	} {
		if _, err := e.svc.Announce(ctx, a, in); !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("aktor %+v harus ditolak: %v", a.Roles, err)
		}
	}
	if len(e.repo.rows) != 0 {
		t.Fatal("tidak ada yang boleh terkirim")
	}
	n, err := e.svc.Announce(ctx, domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}, in)
	if err != nil || n != 3 {
		t.Fatalf("superadmin ke tenant 1: 3 staf (admin, noc, viewer), dapat %d %v", n, err)
	}
	if len(e.repo.forUser(20)) != 0 || len(e.repo.forUser(13)) != 0 || len(e.repo.forUser(1)) != 0 {
		t.Fatal("tenant lain, ENDUSER, dan pengirim tidak boleh menerima pengumuman tenant")
	}
	if len(e.mail.msgs) != 3 {
		t.Fatalf("email=true: 3 email, dapat %d", len(e.mail.msgs))
	}
	if len(e.act.recorded) != 1 || e.act.recorded[0].Action != "ANNOUNCE_NOTIFICATION" {
		t.Fatal("pengumuman harus tercatat di audit trail")
	}
	// email=false -> tanpa email; tenant kosong = seluruh platform
	e.mail.msgs = nil
	n, err = e.svc.Announce(ctx, domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}, AnnounceInput{Title: "Semua", Body: "Halo", Email: false})
	if err != nil || n != 5 || len(e.mail.msgs) != 0 {
		t.Fatalf("platform tanpa email: n=%d mails=%d %v", n, len(e.mail.msgs), err)
	}
}

func TestAnnounce_Validasi(t *testing.T) {
	e := newEnv(t, false)
	sa := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	ok := AnnounceInput{Title: "J", Body: "B"}
	cases := map[string]AnnounceInput{
		"judul kosong":         {Title: " ", Body: "B"},
		"isi kosong":           {Title: "J", Body: ""},
		"judul 161":            {Title: strings.Repeat("a", 161), Body: "B"},
		"isi 1001":             {Title: "J", Body: strings.Repeat("a", 1001)},
		"kontrol di judul":     {Title: "J\r\nX", Body: "B"},
		"link absolut":         {Title: "J", Body: "B", Link: "https://evil.test"},
		"link protokol-rel":    {Title: "J", Body: "B", Link: "//evil.test"},
		"link skema":           {Title: "J", Body: "B", Link: "javascript:alert(1)"},
		"link tanpa slash":     {Title: "J", Body: "B", Link: "devices"},
		"link backslash":       {Title: "J", Body: "B", Link: `/\evil`},
		"link terlalu panjang": {Title: "J", Body: "B", Link: "/" + strings.Repeat("a", 255)},
		"tenant tak ada":       {Title: "J", Body: "B", TenantID: u64(99)},
	}
	for name, in := range cases {
		if _, err := e.svc.Announce(ctx, sa, in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: harus ErrInvalidInput, dapat %v", name, err)
		}
	}
	if len(e.repo.rows) != 0 {
		t.Fatal("input tidak sah tidak boleh menghasilkan notifikasi")
	}
	// batas tepat lolos
	edge := AnnounceInput{Title: strings.Repeat("é", 160), Body: strings.Repeat("b", 1000), Link: "/" + strings.Repeat("a", 254)}
	if _, err := e.svc.Announce(ctx, sa, edge); err != nil {
		t.Fatalf("batas tepat harus lolos: %v", err)
	}
	// body multi-baris boleh
	ok.Body = "baris 1\nbaris 2"
	if _, err := e.svc.Announce(ctx, sa, ok); err != nil {
		t.Fatalf("newline pada isi harus boleh: %v", err)
	}
}

func TestAnnounce_RateLimitPerAktor(t *testing.T) {
	e := newEnv(t, false)
	sa := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	other := domain.Actor{UserID: 2, Roles: []string{domain.RoleSuperadmin}}
	in := AnnounceInput{Title: "J", Body: "B"}
	for i := 0; i < maxAnnouncePerHour; i++ {
		if _, err := e.svc.Announce(ctx, sa, in); err != nil {
			t.Fatalf("pengumuman ke-%d: %v", i+1, err)
		}
	}
	if _, err := e.svc.Announce(ctx, sa, in); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("ke-%d harus 429: %v", maxAnnouncePerHour+1, err)
	}
	if _, err := e.svc.Announce(ctx, other, in); err != nil {
		t.Fatalf("aktor lain tidak terpengaruh: %v", err)
	}
	e.now = e.now.Add(61 * time.Minute)
	if _, err := e.svc.Announce(ctx, sa, in); err != nil {
		t.Fatalf("setelah 1 jam boleh lagi: %v", err)
	}
}

func TestAnnounce_InputTidakSahTidakMenghabiskanKuota(t *testing.T) {
	e := newEnv(t, false)
	sa := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	for i := 0; i < 3*maxAnnouncePerHour; i++ {
		_, _ = e.svc.Announce(ctx, sa, AnnounceInput{Title: "", Body: ""})
	}
	if _, err := e.svc.Announce(ctx, sa, AnnounceInput{Title: "J", Body: "B"}); err != nil {
		t.Fatalf("permintaan gagal validasi tidak boleh memakai kuota: %v", err)
	}
}
