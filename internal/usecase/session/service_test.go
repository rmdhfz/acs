package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"acs/internal/domain"
	"acs/pkg/cryptoutil"
)

// fakeDeviceRepoAuth implementasi in-memory minimal domain.DeviceRepository
// — cukup untuk menguji authenticateInform, bukan seluruh perilaku repo.
type fakeDeviceRepoAuth struct {
	byOUISerial map[string]*domain.Device
}

func (f *fakeDeviceRepoAuth) key(oui, serial string) string { return oui + "|" + serial }

func (f *fakeDeviceRepoAuth) put(d *domain.Device) { f.byOUISerial[f.key(*d.OUI, d.SerialNumber)] = d }

func (f *fakeDeviceRepoAuth) Create(context.Context, *domain.Device) error { return nil }
func (f *fakeDeviceRepoAuth) GetByID(context.Context, uint64) (*domain.Device, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeDeviceRepoAuth) GetByUUID(context.Context, string) (*domain.Device, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeDeviceRepoAuth) GetByOUISerial(_ context.Context, oui, serial string) (*domain.Device, error) {
	if d, ok := f.byOUISerial[f.key(oui, serial)]; ok {
		return d, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeDeviceRepoAuth) List(context.Context, domain.DeviceFilter, domain.Pagination) ([]domain.Device, int, error) {
	return nil, 0, nil
}
func (f *fakeDeviceRepoAuth) Update(context.Context, *domain.Device) error { return nil }
func (f *fakeDeviceRepoAuth) UpdateStatus(context.Context, uint64, uint64, *uint64) error {
	return nil
}
func (f *fakeDeviceRepoAuth) MarkStaleOffline(context.Context, uint64, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeDeviceRepoAuth) SoftDelete(context.Context, uint64, uint64) error { return nil }

// fakeTenantRepoAuth implementasi in-memory minimal domain.TenantRepository.
type fakeTenantRepoAuth struct {
	byID       map[uint64]*domain.Tenant
	byUsername map[string]*domain.Tenant
}

func (f *fakeTenantRepoAuth) put(t *domain.Tenant) {
	f.byID[t.ID] = t
	if t.CWMPInformUsername != nil {
		f.byUsername[*t.CWMPInformUsername] = t
	}
}

func (f *fakeTenantRepoAuth) Create(context.Context, *domain.Tenant) error { return nil }
func (f *fakeTenantRepoAuth) GetByID(_ context.Context, id uint64) (*domain.Tenant, error) {
	if t, ok := f.byID[id]; ok {
		return t, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeTenantRepoAuth) GetByUUID(context.Context, string) (*domain.Tenant, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeTenantRepoAuth) GetByCWMPInformUsername(_ context.Context, username string) (*domain.Tenant, error) {
	if t, ok := f.byUsername[username]; ok {
		return t, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeTenantRepoAuth) List(context.Context, domain.Pagination) ([]domain.Tenant, int, error) {
	return nil, 0, nil
}
func (f *fakeTenantRepoAuth) Update(context.Context, *domain.Tenant) error { return nil }
func (f *fakeTenantRepoAuth) SetCWMPInformCredentials(context.Context, uint64, string, []byte, *uint64) error {
	return nil
}
func (f *fakeTenantRepoAuth) SoftDelete(context.Context, uint64, uint64) error { return nil }

// testFixture membangun Service dengan hanya field yang dipakai
// authenticateInform terisi (devices, tenants, enc) — field lain (deviceSvc,
// taskSvc, dst) sengaja dibiarkan nil karena tidak disentuh fungsi ini.
type testFixture struct {
	svc     *Service
	devices *fakeDeviceRepoAuth
	tenants *fakeTenantRepoAuth
	enc     *cryptoutil.Encryptor
}

func newTestFixture(t *testing.T) *testFixture {
	t.Helper()
	enc, err := cryptoutil.NewEncryptor([]byte("01234567890123456789012345678901"[:32]))
	if err != nil {
		t.Fatalf("setup encryptor: %v", err)
	}
	devices := &fakeDeviceRepoAuth{byOUISerial: map[string]*domain.Device{}}
	tenants := &fakeTenantRepoAuth{byID: map[uint64]*domain.Tenant{}, byUsername: map[string]*domain.Tenant{}}
	return &testFixture{
		svc:     &Service{devices: devices, tenants: tenants, enc: enc},
		devices: devices,
		tenants: tenants,
		enc:     enc,
	}
}

func (f *testFixture) encryptOrFatal(t *testing.T, plaintext string) []byte {
	t.Helper()
	enc, err := f.enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return enc
}

func strPtr(s string) *string { return &s }
func u64Ptr(v uint64) *uint64 { return &v }

func TestAuthenticateInform(t *testing.T) {
	const oui = "OUI1"

	t.Run("username kosong -> ErrUnauthorized tanpa perlu lookup apapun", func(t *testing.T) {
		f := newTestFixture(t)
		_, err := f.svc.authenticateInform(context.Background(), oui, "ANY", "", "whatever")
		if !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("want ErrUnauthorized, got %v", err)
		}
	})

	t.Run("username tenant tidak dikenal -> ErrUnauthorized", func(t *testing.T) {
		f := newTestFixture(t)
		_, err := f.svc.authenticateInform(context.Background(), oui, "NEW1", "unknown-user", "whatever")
		if !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("want ErrUnauthorized, got %v", err)
		}
	})

	t.Run("device baru + shared secret tenant benar -> tenant_id ter-resolve", func(t *testing.T) {
		f := newTestFixture(t)
		tenant1 := &domain.Tenant{ID: 1, IsActive: true, CWMPInformUsername: strPtr("tenant1-user"), CWMPInformPasswordEnc: f.encryptOrFatal(t, "tenant1-pass")}
		f.tenants.put(tenant1)

		auth, err := f.svc.authenticateInform(context.Background(), oui, "NEW1", "tenant1-user", "tenant1-pass")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if auth.TenantID == nil || *auth.TenantID != 1 {
			t.Fatalf("want tenant_id=1, got %v", auth.TenantID)
		}
		if auth.Device != nil {
			t.Fatalf("want Device nil untuk device baru, got %+v", auth.Device)
		}
	})

	t.Run("shared secret tenant dgn password salah -> ErrUnauthorized", func(t *testing.T) {
		f := newTestFixture(t)
		tenant1 := &domain.Tenant{ID: 1, IsActive: true, CWMPInformUsername: strPtr("tenant1-user"), CWMPInformPasswordEnc: f.encryptOrFatal(t, "tenant1-pass")}
		f.tenants.put(tenant1)

		_, err := f.svc.authenticateInform(context.Background(), oui, "NEW1", "tenant1-user", "salah")
		if !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("want ErrUnauthorized, got %v", err)
		}
	})

	t.Run("tenant tanpa secret dikonfigurasi (password_enc kosong) -> selalu tolak", func(t *testing.T) {
		f := newTestFixture(t)
		tenantNoSecret := &domain.Tenant{ID: 4, IsActive: true, CWMPInformUsername: strPtr("tenant4-user"), CWMPInformPasswordEnc: nil}
		f.tenants.put(tenantNoSecret)

		_, err := f.svc.authenticateInform(context.Background(), oui, "NEW1", "tenant4-user", "")
		if !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("password kosong: want ErrUnauthorized, got %v", err)
		}
		_, err = f.svc.authenticateInform(context.Background(), oui, "NEW1", "tenant4-user", "apapun")
		if !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("password non-kosong: want ErrUnauthorized, got %v", err)
		}
	})

	t.Run("device sudah py inform_username sendiri -> shared secret tenant TIDAK berlaku, wajib override", func(t *testing.T) {
		f := newTestFixture(t)
		tenant1 := &domain.Tenant{ID: 1, IsActive: true, CWMPInformUsername: strPtr("tenant1-user"), CWMPInformPasswordEnc: f.encryptOrFatal(t, "tenant1-pass")}
		f.tenants.put(tenant1)
		devOverride := &domain.Device{
			OUI: strPtr(oui), SerialNumber: "OVERRIDE1", TenantID: u64Ptr(1),
			InformUsername: strPtr("device-user"), InformPasswordEnc: f.encryptOrFatal(t, "device-pass"),
		}
		f.devices.put(devOverride)

		// Kredensial tenant yang VALID tetap ditolak karena device ini wajib pakai override-nya sendiri.
		_, err := f.svc.authenticateInform(context.Background(), oui, "OVERRIDE1", "tenant1-user", "tenant1-pass")
		if !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("want ErrUnauthorized (override wajib dipakai), got %v", err)
		}
	})

	t.Run("device override dgn kredensial benar -> sukses", func(t *testing.T) {
		f := newTestFixture(t)
		devOverride := &domain.Device{
			OUI: strPtr(oui), SerialNumber: "OVERRIDE1", TenantID: u64Ptr(1),
			InformUsername: strPtr("device-user"), InformPasswordEnc: f.encryptOrFatal(t, "device-pass"),
		}
		f.devices.put(devOverride)
		f.tenants.put(&domain.Tenant{ID: 1, IsActive: true})

		auth, err := f.svc.authenticateInform(context.Background(), oui, "OVERRIDE1", "device-user", "device-pass")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if auth.TenantID == nil || *auth.TenantID != 1 {
			t.Fatalf("want tenant_id=1, got %v", auth.TenantID)
		}
		if auth.Device != devOverride {
			t.Fatalf("want Device diteruskan (hindari query ganda), got %+v", auth.Device)
		}
	})

	t.Run("device override dgn password salah -> ErrUnauthorized", func(t *testing.T) {
		f := newTestFixture(t)
		devOverride := &domain.Device{
			OUI: strPtr(oui), SerialNumber: "OVERRIDE1", TenantID: u64Ptr(1),
			InformUsername: strPtr("device-user"), InformPasswordEnc: f.encryptOrFatal(t, "device-pass"),
		}
		f.devices.put(devOverride)

		_, err := f.svc.authenticateInform(context.Background(), oui, "OVERRIDE1", "device-user", "salah")
		if !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("want ErrUnauthorized, got %v", err)
		}
	})

	t.Run("device override milik tenant yang sudah nonaktif -> ErrUnauthorized (offboarding harus memutus akses)", func(t *testing.T) {
		f := newTestFixture(t)
		devOverride := &domain.Device{
			OUI: strPtr(oui), SerialNumber: "OVERRIDE2", TenantID: u64Ptr(3),
			InformUsername: strPtr("device-user2"), InformPasswordEnc: f.encryptOrFatal(t, "device-pass2"),
		}
		f.devices.put(devOverride)
		f.tenants.put(&domain.Tenant{ID: 3, IsActive: false})

		_, err := f.svc.authenticateInform(context.Background(), oui, "OVERRIDE2", "device-user2", "device-pass2")
		if !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("want ErrUnauthorized (tenant nonaktif), got %v", err)
		}
	})

	t.Run("anti cross-tenant spoofing: device milik tenant A, coba Inform pakai secret tenant B -> ErrUnauthorized", func(t *testing.T) {
		f := newTestFixture(t)
		tenantA := &domain.Tenant{ID: 1, IsActive: true, CWMPInformUsername: strPtr("tenantA-user"), CWMPInformPasswordEnc: f.encryptOrFatal(t, "tenantA-pass")}
		tenantB := &domain.Tenant{ID: 2, IsActive: true, CWMPInformUsername: strPtr("tenantB-user"), CWMPInformPasswordEnc: f.encryptOrFatal(t, "tenantB-pass")}
		f.tenants.put(tenantA)
		f.tenants.put(tenantB)
		devA := &domain.Device{OUI: strPtr(oui), SerialNumber: "KNOWN1", TenantID: u64Ptr(1)}
		f.devices.put(devA)

		_, err := f.svc.authenticateInform(context.Background(), oui, "KNOWN1", "tenantB-user", "tenantB-pass")
		if !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("want ErrUnauthorized (spoofing lintas tenant harus diblok), got %v", err)
		}
	})

	t.Run("device orphan (tenant_id nil) berhasil di-assign tenant yang berhasil Inform", func(t *testing.T) {
		f := newTestFixture(t)
		tenant1 := &domain.Tenant{ID: 1, IsActive: true, CWMPInformUsername: strPtr("tenant1-user"), CWMPInformPasswordEnc: f.encryptOrFatal(t, "tenant1-pass")}
		f.tenants.put(tenant1)
		devOrphan := &domain.Device{OUI: strPtr(oui), SerialNumber: "ORPHAN1", TenantID: nil}
		f.devices.put(devOrphan)

		auth, err := f.svc.authenticateInform(context.Background(), oui, "ORPHAN1", "tenant1-user", "tenant1-pass")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if auth.TenantID == nil || *auth.TenantID != 1 {
			t.Fatalf("want tenant_id=1 utk device orphan yang berhasil auth, got %v", auth.TenantID)
		}
		if auth.Device != devOrphan {
			t.Fatalf("want Device diteruskan, got %+v", auth.Device)
		}
	})
}
