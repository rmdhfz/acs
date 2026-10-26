package file

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"acs/internal/domain"
)

type fakeStorage struct {
	uploads []string
	deleted []string
}

func (s *fakeStorage) Upload(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	_, _ = io.Copy(io.Discard, r)
	s.uploads = append(s.uploads, key)
	return nil
}
func (s *fakeStorage) PresignedGetURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (s *fakeStorage) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

type createFiles struct {
	fakeFiles
	created []*domain.File
}

func (f *createFiles) Create(_ context.Context, x *domain.File) error {
	f.created = append(f.created, x)
	return nil
}

type fakeStorageQuota struct {
	max  *uint64
	used uint64
	err  error
}

func (q *fakeStorageQuota) SetMaxStorageBytes(context.Context, uint64, *uint64, *uint64) error {
	return nil
}
func (q *fakeStorageQuota) StorageUsage(context.Context, uint64) (*uint64, uint64, error) {
	return q.max, q.used, q.err
}

func u64p(v uint64) *uint64 { return &v }

func uploadInput(tenant *uint64, size int64) UploadFileInput {
	return UploadFileInput{
		TenantID: tenant, FileType: domain.FileTypeVendorConfig, FileName: "cfg.xml",
		File: strings.NewReader(strings.Repeat("x", int(size))), FileSize: size,
	}
}

// Kuota penyimpanan per tenant: unggahan yang membuat total melampaui batas ditolak SEBELUM objek dikirim ke storage;
// batas tepat boleh; tanpa batas / tanpa repo / superadmin tanpa tenant tidak dibatasi; galat pembacaan batas gagal-tertutup.
func TestUploadFile_StorageQuota(t *testing.T) {
	ctx := context.Background()
	tenant := u64p(7)
	admin := domain.Actor{UserID: 2, TenantID: tenant, Roles: []string{domain.RoleAdmin}}
	super := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}

	cases := []struct {
		name      string
		quota     *fakeStorageQuota // nil = tanpa repo kuota
		actor     domain.Actor
		tenantIn  *uint64
		size      int64
		wantErr   error
		wantStore bool
	}{
		{"tanpa repo kuota", nil, admin, tenant, 100, nil, true},
		{"tenant tanpa batas (nil)", &fakeStorageQuota{max: nil, used: 1 << 40}, admin, tenant, 100, nil, true},
		{"masih di bawah batas", &fakeStorageQuota{max: u64p(1000), used: 800}, admin, tenant, 100, nil, true},
		{"tepat mencapai batas diizinkan", &fakeStorageQuota{max: u64p(1000), used: 900}, admin, tenant, 100, nil, true},
		{"melampaui batas satu byte", &fakeStorageQuota{max: u64p(1000), used: 901}, admin, tenant, 100, domain.ErrQuotaExceeded, false},
		{"sudah penuh", &fakeStorageQuota{max: u64p(1000), used: 1000}, admin, tenant, 1, domain.ErrQuotaExceeded, false},
		{"terpakai melebihi batas (batas diturunkan)", &fakeStorageQuota{max: u64p(1000), used: 5000}, admin, tenant, 1, domain.ErrQuotaExceeded, false},
		{"batas nol menolak semua", &fakeStorageQuota{max: u64p(0), used: 0}, admin, tenant, 1, domain.ErrQuotaExceeded, false},
		{"superadmin tanpa tenant tidak dibatasi", &fakeStorageQuota{max: u64p(1), used: 999}, super, nil, 100, nil, true},
		{"galat baca batas gagal-tertutup", &fakeStorageQuota{err: errors.New("db mati")}, admin, tenant, 100, errors.New("db mati"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store, files := &fakeStorage{}, &createFiles{}
			svc := NewService(files, store, fakeAct{})
			if c.quota != nil {
				svc.WithStorageQuota(c.quota)
			}
			_, err := svc.UploadFile(ctx, c.actor, uploadInput(c.tenantIn, c.size))
			switch {
			case c.wantErr == nil && err != nil:
				t.Fatalf("tidak boleh error: %v", err)
			case c.wantErr != nil && err == nil:
				t.Fatal("seharusnya ditolak")
			case c.wantErr == domain.ErrQuotaExceeded && !errors.Is(err, domain.ErrQuotaExceeded):
				t.Fatalf("harus ErrQuotaExceeded: %v", err)
			case c.wantErr != nil && c.wantErr != domain.ErrQuotaExceeded && err.Error() != c.wantErr.Error():
				t.Fatalf("galat harus diteruskan apa adanya: %v", err)
			}
			if (len(store.uploads) == 1) != c.wantStore {
				t.Fatalf("objek terkirim ke storage = %d, harapan terkirim=%v", len(store.uploads), c.wantStore)
			}
			if (len(files.created) == 1) != c.wantStore {
				t.Fatalf("baris metadata dibuat = %d, harapan dibuat=%v", len(files.created), c.wantStore)
			}
		})
	}
}

// Menghapus berkas juga menghapus objeknya: tanpa itu siklus unggah-hapus menumpuk objek yatim di bucket bersama sementara
// kuota (yang hanya menghitung baris aktif) tetap kosong.
func TestDelete_RemovesStorageObject(t *testing.T) {
	repo := &fakeFiles{byID: map[uint64]*domain.File{2: {ID: 2, TenantID: u64p(7), StorageKey: "files/config/abc_cfg.xml"}}}
	store := &fakeStorage{}
	svc := NewService(repo, store, fakeAct{})
	admin := domain.Actor{UserID: 2, TenantID: u64p(7), Roles: []string{domain.RoleAdmin}}

	if err := svc.Delete(context.Background(), admin, 2); err != nil {
		t.Fatal(err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "files/config/abc_cfg.xml" {
		t.Fatalf("objek storage harus ikut dihapus: %v", store.deleted)
	}

	// ditolak karena bukan pemilik: objek TIDAK boleh disentuh
	store2 := &fakeStorage{}
	other := domain.Actor{UserID: 3, TenantID: u64p(8), Roles: []string{domain.RoleAdmin}}
	if err := NewService(repo, store2, fakeAct{}).Delete(context.Background(), other, 2); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("harus ErrForbidden: %v", err)
	}
	if len(store2.deleted) != 0 {
		t.Fatalf("objek tenant lain tidak boleh terhapus: %v", store2.deleted)
	}
}
