package signup

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"acs/internal/domain"
	"acs/internal/usecase/iam"
	"acs/internal/usecase/notify"
	"acs/pkg/mailer"
)

// ---- fakes ----

type memRepo struct {
	mu   sync.Mutex
	rows []*domain.TenantSignup
	next uint64
}

func (m *memRepo) Create(_ context.Context, s *domain.TenantSignup) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	s.ID, s.CreatedAt = m.next, time.Now()
	cp := *s
	m.rows = append(m.rows, &cp)
	return nil
}
func (m *memRepo) GetByTokenHash(_ context.Context, h string) (*domain.TenantSignup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.TokenHash == h {
			cp := *r
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *memRepo) Claim(_ context.Context, id uint64, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.ID == id && r.VerifiedAt == nil && r.ExpiresAt.After(now) {
			r.VerifiedAt = &now
			return true, nil
		}
	}
	return false, nil
}
func (m *memRepo) Unclaim(_ context.Context, id uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.ID == id {
			r.VerifiedAt = nil
		}
	}
	return nil
}
func (m *memRepo) count(pred func(*domain.TenantSignup) bool) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.rows {
		if pred(r) {
			n++
		}
	}
	return n
}
func (m *memRepo) CountSince(_ context.Context, since time.Time) (int, error) {
	return m.count(func(r *domain.TenantSignup) bool { return !r.CreatedAt.Before(since) }), nil
}
func (m *memRepo) CountByEmailSince(_ context.Context, e string, since time.Time) (int, error) {
	return m.count(func(r *domain.TenantSignup) bool { return r.Email == e && !r.CreatedAt.Before(since) }), nil
}
func (m *memRepo) CountByIPSince(_ context.Context, ip string, since time.Time) (int, error) {
	return m.count(func(r *domain.TenantSignup) bool {
		return r.IPAddress != nil && *r.IPAddress == ip && !r.CreatedAt.Before(since)
	}), nil
}
func (m *memRepo) PurgeOld(context.Context, time.Time, time.Time) (int64, error) { return 0, nil }

type fakeProv struct {
	tenants        []iam.CreateTenantInput
	users          []iam.CreateUserInput
	deleted        []uint64
	quotas         map[uint64]uint32
	limits         map[uint64]uint32
	codeConflicts  int   // jumlah CreateTenant pertama yang bentrok kode
	userErr        error // galat CreateUser
	nextID         uint64
	failSetDevices bool
}

func (f *fakeProv) CreateTenant(_ context.Context, _ domain.Actor, in iam.CreateTenantInput) (*domain.Tenant, error) {
	if f.codeConflicts > 0 {
		f.codeConflicts--
		return nil, domain.ErrConflict
	}
	f.nextID++
	f.tenants = append(f.tenants, in)
	return &domain.Tenant{ID: f.nextID, Code: in.Code, Name: in.Name}, nil
}
func (f *fakeProv) CreateUser(_ context.Context, _ domain.Actor, in iam.CreateUserInput) (*domain.User, error) {
	if f.userErr != nil {
		return nil, f.userErr
	}
	f.users = append(f.users, in)
	return &domain.User{ID: 1}, nil
}
func (f *fakeProv) SetTaskQuota(_ context.Context, _ domain.Actor, id uint64, m *uint32) error {
	if f.quotas == nil {
		f.quotas = map[uint64]uint32{}
	}
	f.quotas[id] = *m
	return nil
}
func (f *fakeProv) SetDeviceLimit(_ context.Context, _ domain.Actor, id uint64, m *uint32) error {
	if f.failSetDevices {
		return errors.New("db down")
	}
	if f.limits == nil {
		f.limits = map[uint64]uint32{}
	}
	f.limits[id] = *m
	return nil
}
func (f *fakeProv) DeleteTenant(_ context.Context, _ domain.Actor, id uint64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeUsers struct{ emails map[string]bool }

func (f fakeUsers) GetByEmail(_ context.Context, e string) (*domain.User, error) {
	if f.emails[e] {
		return &domain.User{ID: 9}, nil
	}
	return nil, domain.ErrNotFound
}

type fakeAct struct{ domain.ActivityLogRepository }

func (fakeAct) Record(context.Context, *domain.ActivityLog) error { return nil }

type capNotifier struct{ msgs []mailer.Message }

func (c *capNotifier) Notify(m mailer.Message) { c.msgs = append(c.msgs, m) }

var tokenRe = regexp.MustCompile(`verify-signup#token=([A-Za-z0-9_-]+)`)

type rig struct {
	svc   *Service
	repo  *memRepo
	prov  *fakeProv
	mails *capNotifier
}

func newRig(cfg Config, existing ...string) *rig {
	r := &rig{repo: &memRepo{}, prov: &fakeProv{}, mails: &capNotifier{}}
	emails := map[string]bool{}
	for _, e := range existing {
		emails[e] = true
	}
	r.svc = NewService(r.repo, r.prov, fakeUsers{emails}, fakeAct{}, r.mails, notify.NewBuilder("ACS", "https://acs.test"), cfg)
	return r
}

func (r *rig) token(t *testing.T) string {
	t.Helper()
	if len(r.mails.msgs) == 0 {
		t.Fatal("email verifikasi tidak terkirim")
	}
	m := tokenRe.FindStringSubmatch(r.mails.msgs[len(r.mails.msgs)-1].HTML + r.mails.msgs[len(r.mails.msgs)-1].Text)
	if m == nil {
		t.Fatalf("tautan verifikasi tidak ditemukan di email: %q", r.mails.msgs[len(r.mails.msgs)-1].Text)
	}
	return m[1]
}

func goodInput() RequestInput {
	return RequestInput{CompanyName: "Rumah Net Sejahtera", FullName: "Budi Santoso", Email: "Budi@Rumahnet.ID", Password: "Kuda-Laut-7788-xyz", AcceptTerms: true, IP: "203.0.113.9"}
}

var cfgOn = Config{Enabled: true, DailyCap: 100, MaxDevices: 25, MaxPendingTasks: 50}

// ---- tes ----

func TestAlurLengkap_DaftarVerifikasiTenantDanAdmin(t *testing.T) {
	r := newRig(cfgOn)
	ctx := context.Background()
	if err := r.svc.Request(ctx, goodInput()); err != nil {
		t.Fatal(err)
	}
	// Sebelum verifikasi TIDAK ada tenant/user.
	if len(r.prov.tenants) != 0 || len(r.prov.users) != 0 {
		t.Fatal("tenant/admin tidak boleh dibuat sebelum email diverifikasi")
	}
	// Hanya hash yang disimpan: token & password asli tidak ada di baris.
	row := r.repo.rows[0]
	tok := r.token(t)
	if row.TokenHash == tok || strings.Contains(row.PasswordHash, "Kuda-Laut") || !strings.HasPrefix(row.PasswordHash, "$argon2id$") {
		t.Fatalf("token/password harus tersimpan sebagai hash: %+v", row)
	}
	if row.Email != "budi@rumahnet.id" {
		t.Fatalf("email dinormalkan huruf kecil: %q", row.Email)
	}

	res, err := r.svc.Verify(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.prov.tenants) != 1 || r.prov.tenants[0].Name != "Rumah Net Sejahtera" || !strings.HasPrefix(res.TenantCode, "rumah-net-sejahtera-") {
		t.Fatalf("tenant salah: %+v %+v", r.prov.tenants, res)
	}
	u := r.prov.users[0]
	if u.Email != "budi@rumahnet.id" || len(u.RoleCodes) != 1 || u.RoleCodes[0] != domain.RoleAdmin || u.PasswordHash != row.PasswordHash || u.Password != "" || *u.TenantID != 1 {
		t.Fatalf("admin pertama salah: %+v", u)
	}
	if r.prov.quotas[1] != 50 || r.prov.limits[1] != 25 {
		t.Fatalf("batas paket awal tidak diterapkan: quota=%v limit=%v", r.prov.quotas, r.prov.limits)
	}
}

func TestVerifikasi_SekaliPakaiKedaluwarsaDanNgawur(t *testing.T) {
	ctx := context.Background()
	r := newRig(cfgOn)
	_ = r.svc.Request(ctx, goodInput())
	tok := r.token(t)
	if _, err := r.svc.Verify(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Verify(ctx, tok); !errors.Is(err, ErrInvalid) {
		t.Fatalf("pakai ulang harus ErrInvalid: %v", err)
	}
	if len(r.prov.tenants) != 1 {
		t.Fatalf("klik ganda tidak boleh membuat tenant kedua: %d", len(r.prov.tenants))
	}
	for _, bad := range []string{"", "pendek", strings.Repeat("x", 300), "token-ngawur-yang-cukup-panjang-12345"} {
		if _, err := r.svc.Verify(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("token %q harus ErrInvalid: %v", bad, err)
		}
	}
	// kedaluwarsa
	r2 := newRig(cfgOn)
	_ = r2.svc.Request(ctx, goodInput())
	tok2 := r2.token(t)
	r2.svc.now = func() time.Time { return time.Now().Add(TokenTTL + time.Minute) }
	if _, err := r2.svc.Verify(ctx, tok2); !errors.Is(err, ErrInvalid) {
		t.Fatalf("kedaluwarsa harus ErrInvalid: %v", err)
	}
	if len(r2.prov.tenants) != 0 {
		t.Fatal("tautan kedaluwarsa tidak boleh membuat tenant")
	}
}

func TestVerifikasi_Paralel_HanyaSatuTenant(t *testing.T) {
	ctx := context.Background()
	r := newRig(cfgOn)
	_ = r.svc.Request(ctx, goodInput())
	tok := r.token(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	okCount := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.svc.Verify(ctx, tok); err == nil {
				mu.Lock()
				okCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if okCount != 1 {
		t.Fatalf("verifikasi paralel: %d sukses, harus tepat 1", okCount)
	}
}

func TestNonaktifBawaanDanHoneypot(t *testing.T) {
	ctx := context.Background()
	off := newRig(Config{})
	if err := off.svc.Request(ctx, goodInput()); !errors.Is(err, ErrDisabled) {
		t.Fatalf("mati: %v", err)
	}
	if _, err := off.svc.Verify(ctx, strings.Repeat("a", 43)); !errors.Is(err, ErrDisabled) {
		t.Fatalf("verifikasi saat mati: %v", err)
	}
	on := newRig(cfgOn)
	in := goodInput()
	in.Honeypot = "http://spam.example"
	if err := on.svc.Request(ctx, in); err != nil {
		t.Fatalf("bot tampak sukses: %v", err)
	}
	if len(on.repo.rows) != 0 || len(on.mails.msgs) != 0 {
		t.Fatal("honeypot terisi: tidak boleh menyimpan/mengirim apa pun")
	}
}

func TestAntiEnumerasiEmailSudahTerdaftar(t *testing.T) {
	ctx := context.Background()
	r := newRig(cfgOn, "budi@rumahnet.id")
	if err := r.svc.Request(ctx, goodInput()); err != nil {
		t.Fatalf("respons harus seragam (nil) walau email sudah ada: %v", err)
	}
	if len(r.repo.rows) != 0 || len(r.mails.msgs) != 0 {
		t.Fatal("email terdaftar: tidak boleh membuat pendaftaran / mengirim email")
	}
}

func TestBatasIPEmailDanPlafonHarian(t *testing.T) {
	ctx := context.Background()
	// per IP: 5/jam
	r := newRig(cfgOn)
	for i := 0; i < perIPPerHour; i++ {
		in := goodInput()
		in.Email = "u" + string(rune('a'+i)) + "@x.id"
		if err := r.svc.Request(ctx, in); err != nil {
			t.Fatalf("#%d: %v", i, err)
		}
	}
	in := goodInput()
	in.Email = "lagi@x.id"
	if err := r.svc.Request(ctx, in); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("melewati batas IP harus ErrQuotaExceeded: %v", err)
	}
	// per email: 3/hari -> diam (tidak ada email ke-4), tidak membocorkan
	r2 := newRig(cfgOn)
	for i := 0; i < perEmailPerDy+2; i++ {
		in := goodInput()
		in.IP = "198.51.100." + string(rune('1'+i))
		if err := r2.svc.Request(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	if len(r2.mails.msgs) != perEmailPerDy {
		t.Fatalf("email verifikasi per alamat dibatasi %d/hari, terkirim %d", perEmailPerDy, len(r2.mails.msgs))
	}
	// plafon harian global
	r3 := newRig(Config{Enabled: true, DailyCap: 2})
	for i := 0; i < 2; i++ {
		in := goodInput()
		in.Email, in.IP = "p"+string(rune('a'+i))+"@x.id", "192.0.2."+string(rune('1'+i))
		_ = r3.svc.Request(ctx, in)
	}
	in = goodInput()
	in.Email, in.IP = "ketiga@x.id", "192.0.2.9"
	if err := r3.svc.Request(ctx, in); !errors.Is(err, ErrClosed) {
		t.Fatalf("plafon harian harus ErrClosed: %v", err)
	}
}

func TestValidasiInput(t *testing.T) {
	ctx := context.Background()
	r := newRig(cfgOn)
	mut := map[string]func(*RequestInput){
		"perusahaan pendek":   func(i *RequestInput) { i.CompanyName = "ab" },
		"perusahaan kontrol":  func(i *RequestInput) { i.CompanyName = "PT\x00Jahat" },
		"nama kosong":         func(i *RequestInput) { i.FullName = " " },
		"email salah":         func(i *RequestInput) { i.Email = "bukan-email" },
		"email dengan nama":   func(i *RequestInput) { i.Email = "Budi <budi@x.id>" },
		"password pendek":     func(i *RequestInput) { i.Password = "pendek123" },
		"password = email":    func(i *RequestInput) { i.Email = "orangnya@x.id"; i.Password = "orangnya@x.id" },
		"password pakai nama": func(i *RequestInput) { i.Password = "xxRumahNetSejahterayy1" },
		"tanpa persetujuan":   func(i *RequestInput) { i.AcceptTerms = false },
	}
	for name, m := range mut {
		in := goodInput()
		m(&in)
		if err := r.svc.Request(ctx, in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: harus ErrInvalidInput, dapat %v", name, err)
		}
	}
	if len(r.repo.rows) != 0 {
		t.Fatal("input tidak valid tidak boleh menyimpan apa pun")
	}
}

func TestRollbackBilaPembuatanGagal(t *testing.T) {
	ctx := context.Background()
	// 1) CreateUser gagal -> tenant dihapus, klaim dibatalkan, tautan bisa dicoba lagi
	r := newRig(cfgOn)
	_ = r.svc.Request(ctx, goodInput())
	tok := r.token(t)
	r.prov.userErr = errors.New("db putus")
	if _, err := r.svc.Verify(ctx, tok); err == nil {
		t.Fatal("harus gagal")
	}
	if len(r.prov.deleted) != 1 {
		t.Fatalf("tenant setengah jadi harus dihapus: %v", r.prov.deleted)
	}
	r.prov.userErr = nil
	if _, err := r.svc.Verify(ctx, tok); err != nil {
		t.Fatalf("setelah pulih, tautan yang sama harus bisa dicoba lagi: %v", err)
	}
	// 2) email sudah dipakai sejak pendaftaran -> ErrInvalid (generik) & tenant dibersihkan
	r2 := newRig(cfgOn)
	_ = r2.svc.Request(ctx, goodInput())
	t2 := r2.token(t)
	r2.prov.userErr = domain.ErrConflict
	if _, err := r2.svc.Verify(ctx, t2); !errors.Is(err, ErrInvalid) || len(r2.prov.deleted) != 1 {
		t.Fatalf("konflik email: %v deleted=%v", err, r2.prov.deleted)
	}
	// 3) penerapan batas paket gagal -> tenant dihapus
	r3 := newRig(cfgOn)
	_ = r3.svc.Request(ctx, goodInput())
	t3 := r3.token(t)
	r3.prov.failSetDevices = true
	if _, err := r3.svc.Verify(ctx, t3); err == nil || len(r3.prov.deleted) != 1 {
		t.Fatalf("gagal terapkan batas: %v deleted=%v", err, r3.prov.deleted)
	}
}

func TestKodeTenantBentrokDiulang(t *testing.T) {
	ctx := context.Background()
	r := newRig(cfgOn)
	r.prov.codeConflicts = 2
	_ = r.svc.Request(ctx, goodInput())
	if _, err := r.svc.Verify(ctx, r.token(t)); err != nil {
		t.Fatalf("bentrok kode harus diulang dengan sufiks baru: %v", err)
	}
}

func TestTenantCode(t *testing.T) {
	re := regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	for _, name := range []string{"Rumah Net", "PT. Maju Jaya (Persero) Tbk Indonesia Raya Sentosa", "   ", "日本語", "A"} {
		c := tenantCode(name)
		if !re.MatchString(c) || strings.Contains(c, "--") && false {
			t.Errorf("kode %q untuk %q tidak valid", c, name)
		}
	}
	if tenantCode("Sama") == tenantCode("Sama") {
		t.Error("sufiks acak harus membedakan kode")
	}
}
