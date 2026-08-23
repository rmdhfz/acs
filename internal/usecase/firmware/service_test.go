package firmware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"acs/internal/domain"
)

// ---- fakes (pola sama seperti internal/usecase/task/service_test.go) ----

type fakeFirmwareFileRepo struct {
	created *domain.FirmwareFile
	getErr  error
	byID    map[uint64]*domain.FirmwareFile
}

func (f *fakeFirmwareFileRepo) Create(_ context.Context, file *domain.FirmwareFile) error {
	file.ID = 1
	f.created = file
	return nil
}
func (f *fakeFirmwareFileRepo) GetByID(_ context.Context, id uint64) (*domain.FirmwareFile, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.byID != nil {
		if v, ok := f.byID[id]; ok {
			return v, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (f *fakeFirmwareFileRepo) GetByUUID(context.Context, string) (*domain.FirmwareFile, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeFirmwareFileRepo) ListByVendor(context.Context, uint64, domain.Pagination) ([]domain.FirmwareFile, int, error) {
	return nil, 0, nil
}
func (f *fakeFirmwareFileRepo) SoftDelete(context.Context, uint64, uint64) error { return nil }

// createErrFirmwareFileRepo — variant yang selalu gagal saat Create, untuk
// menguji jalur cleanup (storage.Delete dipanggil saat insert metadata gagal).
type createErrFirmwareFileRepo struct{ fakeFirmwareFileRepo }

func (f *createErrFirmwareFileRepo) Create(context.Context, *domain.FirmwareFile) error {
	return errors.New("db: insert gagal")
}

type fakeFirmwareJobRepo struct {
	created *domain.FirmwareUpgradeJob
}

func (f *fakeFirmwareJobRepo) Create(_ context.Context, j *domain.FirmwareUpgradeJob) error {
	j.ID = 1
	f.created = j
	return nil
}
func (f *fakeFirmwareJobRepo) GetByID(context.Context, uint64) (*domain.FirmwareUpgradeJob, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeFirmwareJobRepo) GetByTaskID(context.Context, uint64) (*domain.FirmwareUpgradeJob, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeFirmwareJobRepo) ListByDevice(context.Context, uint64, domain.Pagination) ([]domain.FirmwareUpgradeJob, int, error) {
	return nil, 0, nil
}
func (f *fakeFirmwareJobRepo) UpdateStatus(context.Context, uint64, uint64, *string) error { return nil }
func (f *fakeFirmwareJobRepo) MarkStarted(context.Context, uint64, time.Time) error        { return nil }
func (f *fakeFirmwareJobRepo) MarkCompleted(context.Context, uint64, string, time.Time) error {
	return nil
}

type fakeDeviceRepoFW struct {
	dev *domain.Device
	err error
}

func (f *fakeDeviceRepoFW) Create(context.Context, *domain.Device) error { return nil }
func (f *fakeDeviceRepoFW) GetByID(context.Context, uint64) (*domain.Device, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.dev, nil
}
func (f *fakeDeviceRepoFW) GetByUUID(context.Context, string) (*domain.Device, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeDeviceRepoFW) GetByOUISerial(context.Context, string, string) (*domain.Device, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeDeviceRepoFW) List(context.Context, domain.DeviceFilter, domain.Pagination) ([]domain.Device, int, error) {
	return nil, 0, nil
}
func (f *fakeDeviceRepoFW) CountByStatus(context.Context, *uint64) ([]domain.DeviceStatusCount, error) {
	return nil, nil
}
func (f *fakeDeviceRepoFW) CountByVendor(context.Context, *uint64) ([]domain.DeviceVendorCount, error) {
	return nil, nil
}
func (f *fakeDeviceRepoFW) Update(context.Context, *domain.Device) error { return nil }
func (f *fakeDeviceRepoFW) UpdateStatus(context.Context, uint64, uint64, *uint64) error {
	return nil
}
func (f *fakeDeviceRepoFW) MarkStaleOffline(context.Context, uint64, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeDeviceRepoFW) SoftDelete(context.Context, uint64, uint64) error { return nil }

type fakeTaskCreatorFW struct {
	lastInput domain.CreateTaskInput
	err       error
}

func (f *fakeTaskCreatorFW) CreateTask(_ context.Context, _ domain.Actor, in domain.CreateTaskInput) (*domain.Task, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.lastInput = in
	return &domain.Task{ID: 1}, nil
}

type fakeRefRepoFW struct{ ids map[string]uint64 }

func (f *fakeRefRepoFW) GetByCode(_ context.Context, _ string, code string) (domain.RefLookup, error) {
	id, ok := f.ids[code]
	if !ok {
		return domain.RefLookup{}, domain.ErrNotFound
	}
	return domain.RefLookup{ID: id, Code: code}, nil
}
func (f *fakeRefRepoFW) GetByID(context.Context, string, uint64) (domain.RefLookup, error) {
	return domain.RefLookup{}, domain.ErrNotFound
}
func (f *fakeRefRepoFW) List(context.Context, string) ([]domain.RefLookup, error) { return nil, nil }

type fakeActivityLogFW struct{}

func (f *fakeActivityLogFW) Record(context.Context, *domain.ActivityLog) error { return nil }
func (f *fakeActivityLogFW) ListByEntity(context.Context, string, uint64, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}

// fakeObjectStorage — implementasi in-memory domain.ObjectStorage, cukup
// untuk menguji orkestrasi upload/checksum/cleanup tanpa MinIO sungguhan.
type fakeObjectStorage struct {
	objects     map[string][]byte
	uploadErr   error
	presignErr  error
	deletedKeys []string
}

func newFakeObjectStorage() *fakeObjectStorage {
	return &fakeObjectStorage{objects: map[string][]byte{}}
}

func (f *fakeObjectStorage) Upload(_ context.Context, objectKey string, reader io.Reader, size int64, _ string) error {
	if f.uploadErr != nil {
		return f.uploadErr
	}
	buf, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if int64(len(buf)) != size {
		return errors.New("fakeObjectStorage: size mismatch")
	}
	f.objects[objectKey] = buf
	return nil
}

func (f *fakeObjectStorage) PresignedGetURL(_ context.Context, objectKey string, _ time.Duration) (string, error) {
	if f.presignErr != nil {
		return "", f.presignErr
	}
	if _, ok := f.objects[objectKey]; !ok {
		return "", domain.ErrNotFound
	}
	return "https://minio.local/" + objectKey + "?presigned=1", nil
}

func (f *fakeObjectStorage) Delete(_ context.Context, objectKey string) error {
	f.deletedKeys = append(f.deletedKeys, objectKey)
	delete(f.objects, objectKey)
	return nil
}

// ---- tests ----

func newTestService(files domain.FirmwareFileRepository, storage domain.ObjectStorage) (*Service, *fakeFirmwareJobRepo, *fakeDeviceRepoFW, *fakeTaskCreatorFW, *fakeRefRepoFW) {
	jobs := &fakeFirmwareJobRepo{}
	devices := &fakeDeviceRepoFW{}
	tasks := &fakeTaskCreatorFW{}
	refs := &fakeRefRepoFW{ids: map[string]uint64{domain.TaskStatusPending: 1, domain.TaskStatusCompleted: 2, domain.TaskStatusFailed: 3}}
	svc := NewService(files, jobs, devices, tasks, refs, &fakeActivityLogFW{}, storage)
	return svc, jobs, devices, tasks, refs
}

func TestUploadFirmware_ComputesChecksumServerSide(t *testing.T) {
	content := []byte("firmware-binary-content-not-real")
	wantSum := sha256.Sum256(content)
	wantChecksum := hex.EncodeToString(wantSum[:])

	files := &fakeFirmwareFileRepo{}
	storage := newFakeObjectStorage()
	svc, _, _, _, _ := newTestService(files, storage)

	f, err := svc.UploadFirmware(context.Background(), domain.Actor{UserID: 1}, UploadFirmwareInput{
		VendorID: 10,
		Version:  "1.0.0",
		FileName: "image.bin",
		File:     bytes.NewReader(content),
		FileSize: int64(len(content)),
	})
	if err != nil {
		t.Fatalf("UploadFirmware() error = %v", err)
	}
	if f.ChecksumSHA256 == nil || *f.ChecksumSHA256 != wantChecksum {
		t.Fatalf("checksum = %v, want %s (harus dihitung server, bukan dipercaya dari client)", f.ChecksumSHA256, wantChecksum)
	}
	if f.StorageKey == "" || !strings.Contains(f.StorageKey, "firmware/10/") || !strings.HasSuffix(f.StorageKey, "_image.bin") {
		t.Fatalf("storage key tidak sesuai pola yang diharapkan: %q", f.StorageKey)
	}
	if _, ok := storage.objects[f.StorageKey]; !ok {
		t.Fatalf("object %q tidak ditemukan di object storage setelah upload", f.StorageKey)
	}
	if !bytes.Equal(storage.objects[f.StorageKey], content) {
		t.Fatalf("isi object di storage tidak sama dengan file yang diupload")
	}
}

func TestUploadFirmware_ObjectKeyRejectsPathTraversal(t *testing.T) {
	files := &fakeFirmwareFileRepo{}
	storage := newFakeObjectStorage()
	svc, _, _, _, _ := newTestService(files, storage)

	content := []byte("x")
	f, err := svc.UploadFirmware(context.Background(), domain.Actor{UserID: 1}, UploadFirmwareInput{
		VendorID: 1,
		Version:  "1.0.0",
		FileName: "../../etc/passwd",
		File:     bytes.NewReader(content),
		FileSize: int64(len(content)),
	})
	if err != nil {
		t.Fatalf("UploadFirmware() error = %v", err)
	}
	if strings.Contains(f.StorageKey, "..") || strings.Contains(f.StorageKey, "/etc/") {
		t.Fatalf("object key masih mengandung komponen path traversal: %q", f.StorageKey)
	}
}

func TestUploadFirmware_RejectsOversizedFile(t *testing.T) {
	files := &fakeFirmwareFileRepo{}
	storage := newFakeObjectStorage()
	svc, _, _, _, _ := newTestService(files, storage)

	_, err := svc.UploadFirmware(context.Background(), domain.Actor{UserID: 1}, UploadFirmwareInput{
		VendorID: 1,
		Version:  "1.0.0",
		FileName: "big.bin",
		File:     bytes.NewReader([]byte("x")),
		FileSize: domain.MaxFirmwareFileSizeBytes + 1,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput (batas ukuran file)", err)
	}
	if len(storage.objects) != 0 {
		t.Fatalf("upload seharusnya ditolak sebelum menyentuh object storage")
	}
}

func TestUploadFirmware_CleansUpObjectWhenMetadataInsertFails(t *testing.T) {
	files := &createErrFirmwareFileRepo{}
	storage := newFakeObjectStorage()
	svc, _, _, _, _ := newTestService(files, storage)

	content := []byte("data")
	_, err := svc.UploadFirmware(context.Background(), domain.Actor{UserID: 1}, UploadFirmwareInput{
		VendorID: 1,
		Version:  "1.0.0",
		FileName: "image.bin",
		File:     bytes.NewReader(content),
		FileSize: int64(len(content)),
	})
	if err == nil {
		t.Fatal("UploadFirmware() error = nil, want error dari repository")
	}
	if len(storage.objects) != 0 {
		t.Fatalf("objek yatim tidak dibersihkan setelah insert metadata gagal: %v", storage.objects)
	}
	if len(storage.deletedKeys) != 1 {
		t.Fatalf("storage.Delete dipanggil %d kali, want 1", len(storage.deletedKeys))
	}
}

func TestUploadFirmware_PropagatesStorageUploadError(t *testing.T) {
	files := &fakeFirmwareFileRepo{}
	storage := newFakeObjectStorage()
	storage.uploadErr = errors.New("minio: connection refused")
	svc, _, _, _, _ := newTestService(files, storage)

	content := []byte("data")
	_, err := svc.UploadFirmware(context.Background(), domain.Actor{UserID: 1}, UploadFirmwareInput{
		VendorID: 1,
		Version:  "1.0.0",
		FileName: "image.bin",
		File:     bytes.NewReader(content),
		FileSize: int64(len(content)),
	})
	if err == nil {
		t.Fatal("UploadFirmware() error = nil, want error dari object storage")
	}
	if files.created != nil {
		t.Fatalf("metadata firmware_files tidak seharusnya tersimpan kalau upload ke object storage gagal, got %+v", files.created)
	}
	if len(storage.deletedKeys) != 0 {
		t.Fatalf("storage.Delete tidak seharusnya dipanggil untuk objek yang gagal ter-upload (tidak pernah benar-benar ada): %v", storage.deletedKeys)
	}
}

func TestScheduleUpgrade_PropagatesPresignError(t *testing.T) {
	tenantID := uint64(5)
	firmware := &domain.FirmwareFile{ID: 2, StorageKey: "firmware/1/abc_image.bin", Version: "2.0.0", FileName: "image.bin"}
	files := &fakeFirmwareFileRepo{byID: map[uint64]*domain.FirmwareFile{2: firmware}}
	storage := newFakeObjectStorage()
	storage.objects[firmware.StorageKey] = []byte("content")
	storage.presignErr = errors.New("minio: object tidak ditemukan di bucket")

	svc, jobs, devices, tasks, _ := newTestService(files, storage)
	devices.dev = &domain.Device{ID: 7, TenantID: &tenantID}

	actor := domain.Actor{UserID: 1, TenantID: &tenantID, Roles: []string{domain.RoleAdmin}}
	_, err := svc.ScheduleUpgrade(context.Background(), actor, 7, 2, nil)
	if err == nil {
		t.Fatal("ScheduleUpgrade() error = nil, want error dari PresignedGetURL")
	}
	if jobs.created != nil {
		t.Fatalf("job upgrade tidak seharusnya dibuat kalau presigned URL gagal digenerate (state setengah jadi tanpa URL valid): %+v", jobs.created)
	}
	if tasks.lastInput.DeviceID != 0 {
		t.Fatalf("task Download tidak seharusnya dibuat kalau presigned URL gagal digenerate")
	}
}

func TestScheduleUpgrade_UsesPresignedURLFromObjectStorage(t *testing.T) {
	tenantID := uint64(5)
	firmware := &domain.FirmwareFile{ID: 2, StorageKey: "firmware/1/abc_image.bin", Version: "2.0.0", FileName: "image.bin"}
	files := &fakeFirmwareFileRepo{byID: map[uint64]*domain.FirmwareFile{2: firmware}}
	storage := newFakeObjectStorage()
	storage.objects[firmware.StorageKey] = []byte("content")

	svc, jobs, devices, tasks, _ := newTestService(files, storage)
	devices.dev = &domain.Device{ID: 7, TenantID: &tenantID}

	actor := domain.Actor{UserID: 1, TenantID: &tenantID, Roles: []string{domain.RoleAdmin}}
	job, err := svc.ScheduleUpgrade(context.Background(), actor, 7, 2, nil)
	if err != nil {
		t.Fatalf("ScheduleUpgrade() error = %v", err)
	}
	if job == nil || jobs.created == nil {
		t.Fatal("job tidak dibuat")
	}
	gotURL, _ := tasks.lastInput.Parameters["url"].(string)
	if !strings.HasPrefix(gotURL, "https://minio.local/"+firmware.StorageKey) {
		t.Fatalf("url task Download = %q, want presigned URL dari object storage", gotURL)
	}
}

func TestScheduleUpgrade_TenantMismatchForbidden(t *testing.T) {
	tenantA := uint64(1)
	tenantB := uint64(2)
	firmware := &domain.FirmwareFile{ID: 2, StorageKey: "firmware/1/abc_image.bin", Version: "2.0.0"}
	files := &fakeFirmwareFileRepo{byID: map[uint64]*domain.FirmwareFile{2: firmware}}
	storage := newFakeObjectStorage()
	storage.objects[firmware.StorageKey] = []byte("content")

	svc, _, devices, _, _ := newTestService(files, storage)
	devices.dev = &domain.Device{ID: 7, TenantID: &tenantA}

	actor := domain.Actor{UserID: 1, TenantID: &tenantB, Roles: []string{domain.RoleAdmin}}
	_, err := svc.ScheduleUpgrade(context.Background(), actor, 7, 2, nil)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden (tenant mismatch)", err)
	}
}
