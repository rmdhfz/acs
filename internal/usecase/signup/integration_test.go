package signup

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/repository/mysql"
	"acs/internal/usecase/iam"
	"acs/internal/usecase/notify"
	"acs/pkg/cryptoutil"
	"acs/pkg/passwordhash"
)

// Integrasi MariaDB NYATA: alur pendaftaran tenant mandiri end-to-end memakai repository & iam.Service sungguhan.
// Dilewati tanpa ACS_TEST_DSN (migrations/0028 harus sudah diterapkan).
func TestSignupIntegration_AlurPenuh(t *testing.T) {
	dsn := os.Getenv("ACS_TEST_DSN")
	if dsn == "" {
		t.Skip("ACS_TEST_DSN tidak diisi — test integrasi MariaDB dilewati")
	}
	db, err := mysql.Connect(dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	enc, err := cryptoutil.NewEncryptor([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	activity := mysql.NewActivityLogRepository(db)
	users := mysql.NewUserRepository(db)
	iamSvc := iam.NewService(mysql.NewTenantRepository(db), users, mysql.NewRefRepository(db), activity, enc, mysql.NewUsageRepository(db)).
		WithQuota(mysql.NewTenantQuotaRepository(db))
	notifier := &capNotifier{}
	svc := NewService(mysql.NewSignupRepository(db), iamSvc, users, activity, notifier, notify.NewBuilder("ACS", "https://acs.test"),
		Config{Enabled: true, DailyCap: 100000, MaxDevices: 7, MaxPendingTasks: 11})

	suffix := uuid.NewString()[:8]
	email := "it-signup-" + suffix + "@example.test"
	company := "IT Signup " + suffix
	var tenantID, userID uint64
	t.Cleanup(func() {
		if userID != 0 {
			_, _ = db.Exec(`DELETE FROM user_roles WHERE user_id = ?`, userID)
			_, _ = db.Exec(`DELETE FROM activity_logs WHERE user_id = ? OR entity_id = ?`, userID, userID)
			_, _ = db.Exec(`DELETE FROM users WHERE id = ?`, userID)
		}
		if tenantID != 0 {
			_, _ = db.Exec(`DELETE FROM activity_logs WHERE tenant_id = ?`, tenantID)
			_, _ = db.Exec(`DELETE FROM tenants WHERE id = ?`, tenantID)
		}
		_, _ = db.Exec(`DELETE FROM tenant_signups WHERE email = ?`, email)
	})

	if err := svc.Request(ctx, RequestInput{CompanyName: company, FullName: "Tester IT", Email: email, Password: "Sandi-Kuat-Sekali-99!", AcceptTerms: true, IP: "203.0.113.77"}); err != nil {
		t.Fatalf("Request: %v", err)
	}
	// belum ada tenant/user sebelum verifikasi
	if _, err := users.GetByEmail(ctx, email); err == nil {
		t.Fatal("user tidak boleh ada sebelum verifikasi")
	}
	var pending int
	_ = db.Get(&pending, `SELECT COUNT(*) FROM tenant_signups WHERE email = ? AND verified_at IS NULL`, email)
	if pending != 1 {
		t.Fatalf("pendaftaran tertunda = %d, harus 1", pending)
	}
	m := tokenRe.FindStringSubmatch(notifier.msgs[0].HTML + notifier.msgs[0].Text)
	if m == nil {
		t.Fatal("tautan verifikasi tidak ada di email")
	}
	var raw struct {
		TokenHash string `db:"token_hash"`
		PassHash  string `db:"password_hash"`
	}
	_ = db.Get(&raw, `SELECT token_hash, password_hash FROM tenant_signups WHERE email = ?`, email)
	if raw.TokenHash == m[1] || strings.Contains(raw.PassHash, "Sandi-Kuat") {
		t.Fatal("token/password tidak boleh tersimpan polos")
	}

	res, err := svc.Verify(ctx, m[1])
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if _, err := svc.Verify(ctx, m[1]); err == nil {
		t.Fatal("pemakaian kedua harus gagal")
	}

	var tn struct {
		ID         uint64  `db:"id"`
		Name       string  `db:"name"`
		MaxDevices *uint32 `db:"max_devices"`
		MaxPending *uint32 `db:"max_pending_tasks"`
		IsActive   bool    `db:"is_active"`
	}
	if err := db.Get(&tn, `SELECT id, name, max_devices, max_pending_tasks, is_active FROM tenants WHERE code = ?`, res.TenantCode); err != nil {
		t.Fatalf("tenant tidak ada: %v", err)
	}
	tenantID = tn.ID
	if tn.Name != company || !tn.IsActive || tn.MaxDevices == nil || *tn.MaxDevices != 7 || tn.MaxPending == nil || *tn.MaxPending != 11 {
		t.Fatalf("tenant/paket awal salah: %+v", tn)
	}
	u, err := users.GetByEmail(ctx, email)
	if err != nil {
		t.Fatalf("admin tidak ada: %v", err)
	}
	userID = u.ID
	if u.TenantID == nil || *u.TenantID != tenantID || len(u.Roles) != 1 || u.Roles[0] != domain.RoleAdmin {
		t.Fatalf("admin salah: tenant=%v roles=%v", u.TenantID, u.Roles)
	}
	// password yang didaftarkan benar-benar bisa dipakai login (hash dipindahkan utuh)
	if ok, _, err := passwordhash.Verify("Sandi-Kuat-Sekali-99!", u.PasswordHash); err != nil || !ok {
		t.Fatalf("password pendaftaran harus lolos verifikasi hash: ok=%v err=%v", ok, err)
	}
	if ok, _, _ := passwordhash.Verify("salah", u.PasswordHash); ok {
		t.Fatal("password salah tidak boleh lolos")
	}
	// kuota perangkat nyata: DeviceCapacity membaca batas & jumlah
	max, cur, err := mysql.NewTenantQuotaRepository(db).DeviceCapacity(ctx, tenantID)
	if err != nil || max == nil || *max != 7 || cur != 0 {
		t.Fatalf("DeviceCapacity: max=%v cur=%d err=%v", max, cur, err)
	}
	// pangkas: baris terverifikasi lama dihapus, baris baru tetap
	if n, err := mysql.NewSignupRepository(db).PurgeOld(ctx, time.Now().AddDate(0, 0, -7), time.Now().Add(time.Hour)); err != nil || n < 1 {
		t.Fatalf("PurgeOld: n=%d err=%v", n, err)
	}
}
