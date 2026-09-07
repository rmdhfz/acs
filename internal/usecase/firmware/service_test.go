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
	// jobs -- seluruh job "tersimpan" (baik pre-seeded test maupun hasil
	// Create), dipakai ListByRolloutBatch/CountByRolloutWave sungguhan alih-
	// alih no-op, supaya TestAdvanceRollout_* menguji logika Service yang
	// sesungguhnya membaca balik state, bukan cuma nilai preset terisolasi.
	jobs []domain.FirmwareUpgradeJob
	// waveStatusCounts -- kalau diisi non-nil, CountByRolloutWave langsung
	// mengembalikan ini (override) tanpa menghitung dari jobs -- dipakai test
	// yang ingin menguji skenario status murni tanpa perlu menyusun jobs
	// detail (mis. reklasifikasi stale->TIMEOUT).
	waveStatusCounts []domain.FirmwareRolloutWaveStatusCount
}

func (f *fakeFirmwareJobRepo) Create(_ context.Context, j *domain.FirmwareUpgradeJob) error {
	j.ID = uint64(len(f.jobs) + 1)
	f.created = j
	f.jobs = append(f.jobs, *j)
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
func (f *fakeFirmwareJobRepo) ListByRolloutBatch(_ context.Context, batchID uint64, waveNumber *uint32, p domain.Pagination) ([]domain.FirmwareUpgradeJob, int, error) {
	var matched []domain.FirmwareUpgradeJob
	for _, j := range f.jobs {
		if j.RolloutBatchID == nil || *j.RolloutBatchID != batchID {
			continue
		}
		if waveNumber != nil && (j.WaveNumber == nil || *j.WaveNumber != *waveNumber) {
			continue
		}
		matched = append(matched, j)
	}
	limit := p.Limit()
	if limit > len(matched) {
		limit = len(matched)
	}
	return matched[:limit], len(matched), nil
}
func (f *fakeFirmwareJobRepo) CountByRolloutWave(_ context.Context, batchID uint64, waveNumber *uint32, _ time.Time, _ uint64) ([]domain.FirmwareRolloutWaveStatusCount, error) {
	if f.waveStatusCounts != nil {
		return f.waveStatusCounts, nil
	}
	counts := map[uint64]int{}
	for _, j := range f.jobs {
		if j.RolloutBatchID == nil || *j.RolloutBatchID != batchID {
			continue
		}
		if waveNumber != nil && (j.WaveNumber == nil || *j.WaveNumber != *waveNumber) {
			continue
		}
		counts[j.TaskStatusID]++
	}
	out := make([]domain.FirmwareRolloutWaveStatusCount, 0, len(counts))
	for statusID, cnt := range counts {
		out = append(out, domain.FirmwareRolloutWaveStatusCount{TaskStatusID: statusID, Count: cnt})
	}
	return out, nil
}
func (f *fakeFirmwareJobRepo) UpdateStatus(context.Context, uint64, uint64, *string) error {
	return nil
}
func (f *fakeFirmwareJobRepo) MarkStarted(context.Context, uint64, time.Time) error { return nil }

// fakeFirmwareRolloutRepo — no-op minimal by default (cukup utk test yang
// tidak menyentuh alur rollout batch, lihat internal/usecase/task/
// service_test.go utk pola sama), TAPI batch/lastUpdated bisa diisi test
// yang MEMANG menguji AdvanceRollout (lihat TestAdvanceRollout_*).
type fakeFirmwareRolloutRepo struct {
	batch       *domain.FirmwareRolloutBatch
	lastUpdated *domain.FirmwareRolloutBatch // salinan batch persis sesaat sebelum Update dipanggil
}

func (f *fakeFirmwareRolloutRepo) Create(context.Context, *domain.FirmwareRolloutBatch) error {
	return nil
}
func (f *fakeFirmwareRolloutRepo) GetByID(_ context.Context, id uint64) (*domain.FirmwareRolloutBatch, error) {
	if f.batch == nil || f.batch.ID != id {
		return nil, domain.ErrNotFound
	}
	cp := *f.batch
	return &cp, nil
}
func (f *fakeFirmwareRolloutRepo) GetByUUID(context.Context, string) (*domain.FirmwareRolloutBatch, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeFirmwareRolloutRepo) List(context.Context, *uint64, domain.Pagination) ([]domain.FirmwareRolloutBatch, int, error) {
	return nil, 0, nil
}
func (f *fakeFirmwareRolloutRepo) Update(_ context.Context, b *domain.FirmwareRolloutBatch) error {
	cp := *b
	f.lastUpdated = &cp
	f.batch = &cp
	return nil
}
func (f *fakeFirmwareRolloutRepo) SoftDelete(context.Context, uint64, uint64) error { return nil }

// RunWithAdvanceLock -- fake tanpa MariaDB tidak bisa mensimulasikan named
// lock sungguhan, jadi cukup jalankan fn langsung (selalu "berhasil dapat
// lock") -- memadai utk test yang tidak menyentuh alur rollout batch.
func (f *fakeFirmwareRolloutRepo) RunWithAdvanceLock(ctx context.Context, _ uint64, fn func(context.Context) error) (bool, error) {
	return true, fn(ctx)
}

func (f *fakeFirmwareJobRepo) MarkCompleted(context.Context, uint64, string, time.Time) error {
	return nil
}

type fakeDeviceRepoFW struct {
	dev *domain.Device
	err error
	// population -- daftar device dikembalikan List (dipakai
	// resolveRolloutWaveTargets), diabaikan filter DeviceFilter-nya (test
	// yang memakai ini sudah menyiapkan populasi yang relevan sendiri).
	population []domain.Device
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
func (f *fakeDeviceRepoFW) List(_ context.Context, _ domain.DeviceFilter, p domain.Pagination) ([]domain.Device, int, error) {
	start := p.Offset()
	if start > len(f.population) {
		start = len(f.population)
	}
	end := start + p.Limit()
	if end > len(f.population) {
		end = len(f.population)
	}
	return f.population[start:end], len(f.population), nil
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

// fakeRefRepoFW -- ids DIKUNCI per (table, code), BUKAN cuma code, karena
// beberapa kode ref_* di proyek ini bertabrakan nilai string-nya antar tabel
// berbeda (mis. domain.TaskStatusPending == domain.FirmwareRolloutStatusPending
// == "PENDING") -- tanpa awareness tabel, test yang menyentuh KEDUA jenis ref
// sekaligus (mis. TestAdvanceRollout_*) akan salah ambil ID.
type fakeRefRepoFW struct{ ids map[string]map[string]uint64 } // table -> code -> id

func (f *fakeRefRepoFW) GetByCode(_ context.Context, table string, code string) (domain.RefLookup, error) {
	id, ok := f.ids[table][code]
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

// ListByTenant tidak dipakai test di paket ini — cukup penuhi kontrak
// domain.ActivityLogRepository.
func (f *fakeActivityLogFW) ListByTenant(context.Context, *uint64, domain.ActivityLogFilter, domain.Pagination) ([]domain.ActivityLog, int, error) {
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
	refs := &fakeRefRepoFW{ids: map[string]map[string]uint64{
		domain.RefTableTaskStatus: {domain.TaskStatusPending: 1, domain.TaskStatusCompleted: 2, domain.TaskStatusFailed: 3},
	}}
	svc := NewService(files, jobs, &fakeFirmwareRolloutRepo{}, devices, tasks, refs, &fakeActivityLogFW{}, storage)
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

func uint64Ptr(v uint64) *uint64 { return &v }

// ---- AdvanceRollout (canary/staged firmware rollout) ----

const (
	rtStatusPending   = 101
	rtStatusInProg    = 102
	rtStatusPaused    = 103
	rtStatusCompleted = 104
	rtStatusCancelled = 105

	taskStatusPending   = 1
	taskStatusQueued    = 2
	taskStatusSent      = 3
	taskStatusCompleted = 4
	taskStatusFailed    = 5
	taskStatusCancelled = 6
	taskStatusTimeout   = 7
)

// newRolloutTestService -- refs lengkap (task status DAN firmware rollout
// status sekaligus, dgn ID terpisah krn beberapa kode bertabrakan string,
// lihat komentar fakeRefRepoFW), dipakai KHUSUS test AdvanceRollout.
func newRolloutTestService() (*Service, *fakeFirmwareRolloutRepo, *fakeFirmwareJobRepo, *fakeDeviceRepoFW) {
	files := &fakeFirmwareFileRepo{byID: map[uint64]*domain.FirmwareFile{1: {ID: 1, Version: "2.0.0", StorageKey: "fw/1.bin"}}}
	storage := newFakeObjectStorage()
	storage.objects["fw/1.bin"] = []byte("content")
	jobs := &fakeFirmwareJobRepo{}
	devices := &fakeDeviceRepoFW{}
	tasks := &fakeTaskCreatorFW{}
	rollouts := &fakeFirmwareRolloutRepo{}
	refs := &fakeRefRepoFW{ids: map[string]map[string]uint64{
		domain.RefTableTaskStatus: {
			domain.TaskStatusPending: taskStatusPending, domain.TaskStatusQueued: taskStatusQueued,
			domain.TaskStatusSent: taskStatusSent, domain.TaskStatusCompleted: taskStatusCompleted,
			domain.TaskStatusFailed: taskStatusFailed, domain.TaskStatusCancelled: taskStatusCancelled,
			domain.TaskStatusTimeout: taskStatusTimeout,
		},
		domain.RefTableFirmwareRolloutStatus: {
			domain.FirmwareRolloutStatusPending: rtStatusPending, domain.FirmwareRolloutStatusInProgress: rtStatusInProg,
			domain.FirmwareRolloutStatusPausedFailureThreshold: rtStatusPaused,
			domain.FirmwareRolloutStatusCompleted:              rtStatusCompleted, domain.FirmwareRolloutStatusCancelled: rtStatusCancelled,
		},
	}}
	svc := NewService(files, jobs, rollouts, devices, tasks, refs, &fakeActivityLogFW{}, storage)
	return svc, rollouts, jobs, devices
}

func TestAdvanceRollout_WaveInFlight_NoOp(t *testing.T) {
	svc, rollouts, jobs, devices := newRolloutTestService()
	rollouts.batch = &domain.FirmwareRolloutBatch{ID: 1, FirmwareFileID: 1, WavePercentage: 50, MaxFailureRatePercent: 30, CurrentWave: 1, StatusID: rtStatusInProg}
	wave := uint32(1)
	jobs.jobs = []domain.FirmwareUpgradeJob{
		{ID: 1, DeviceID: 10, RolloutBatchID: uint64Ptr(1), WaveNumber: &wave, TaskStatusID: taskStatusSent},
	}
	devices.population = []domain.Device{{ID: 10}, {ID: 11}}

	got, err := svc.AdvanceRollout(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.CurrentWave != 1 || got.StatusID != rtStatusInProg {
		t.Fatalf("batch berubah padahal wave masih in-flight: %+v", got)
	}
	if rollouts.lastUpdated != nil {
		t.Fatal("Update TIDAK seharusnya terpanggil selagi wave masih in-flight")
	}
}

func TestAdvanceRollout_WaveComplete_AdvancesToNextWave(t *testing.T) {
	svc, rollouts, jobs, devices := newRolloutTestService()
	rollouts.batch = &domain.FirmwareRolloutBatch{ID: 1, FirmwareFileID: 1, WavePercentage: 50, MaxFailureRatePercent: 30, CurrentWave: 1, StatusID: rtStatusInProg}
	wave := uint32(1)
	jobs.jobs = []domain.FirmwareUpgradeJob{
		{ID: 1, DeviceID: 10, RolloutBatchID: uint64Ptr(1), WaveNumber: &wave, TaskStatusID: taskStatusCompleted},
	}
	// Populasi 4 device, wave 1 (50%) sudah menarget device 10 -- wave 2
	// seharusnya menarget SISA yang belum ditarget (11,12,13), diambil
	// 50% dari TOTAL populasi asli (4*50%=2 device), bukan 50% dari sisa.
	devices.population = []domain.Device{{ID: 10}, {ID: 11}, {ID: 12}, {ID: 13}}
	// fakeDeviceRepoFW.GetByID (dipakai scheduleUpgradeJob) mengembalikan
	// SATU dev statis ini apa pun ID yang diminta -- cukup utk test ini
	// krn tidak menguji per-device tenant-scope, cuma jumlah/wave job baru.
	devices.dev = &domain.Device{ID: 0}

	got, err := svc.AdvanceRollout(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.CurrentWave != 2 {
		t.Fatalf("CurrentWave = %d, want 2", got.CurrentWave)
	}
	if got.StatusID != rtStatusInProg {
		t.Fatalf("StatusID = %d, want IN_PROGRESS (%d)", got.StatusID, rtStatusInProg)
	}
	// 2 job baru (device 11,12) seharusnya ditambahkan utk wave 2, TIDAK
	// menyentuh device 10 (sudah ditarget wave 1) atau 13 (sisa utk wave 3).
	var wave2Devices []uint64
	for _, j := range jobs.jobs {
		if j.WaveNumber != nil && *j.WaveNumber == 2 {
			wave2Devices = append(wave2Devices, j.DeviceID)
		}
	}
	if len(wave2Devices) != 2 {
		t.Fatalf("job wave 2 = %v, want 2 device baru", wave2Devices)
	}
}

func TestAdvanceRollout_FailureRateExceeded_Pauses(t *testing.T) {
	svc, rollouts, jobs, devices := newRolloutTestService()
	rollouts.batch = &domain.FirmwareRolloutBatch{ID: 1, FirmwareFileID: 1, WavePercentage: 50, MaxFailureRatePercent: 30, CurrentWave: 1, StatusID: rtStatusInProg}
	wave := uint32(1)
	// 1 dari 2 job GAGAL -- failure rate 50% > batas 30%.
	jobs.jobs = []domain.FirmwareUpgradeJob{
		{ID: 1, DeviceID: 10, RolloutBatchID: uint64Ptr(1), WaveNumber: &wave, TaskStatusID: taskStatusFailed},
		{ID: 2, DeviceID: 11, RolloutBatchID: uint64Ptr(1), WaveNumber: &wave, TaskStatusID: taskStatusCompleted},
	}
	devices.population = []domain.Device{{ID: 10}, {ID: 11}}

	got, err := svc.AdvanceRollout(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.StatusID != rtStatusPaused {
		t.Fatalf("StatusID = %d, want PAUSED_FAILURE_THRESHOLD (%d)", got.StatusID, rtStatusPaused)
	}
	if got.CurrentWave != 1 {
		t.Fatalf("CurrentWave tidak seharusnya maju saat dipause: got %d", got.CurrentWave)
	}
}

func TestAdvanceRollout_PopulationExhausted_Completes(t *testing.T) {
	svc, rollouts, jobs, devices := newRolloutTestService()
	rollouts.batch = &domain.FirmwareRolloutBatch{ID: 1, FirmwareFileID: 1, WavePercentage: 50, MaxFailureRatePercent: 30, CurrentWave: 1, StatusID: rtStatusInProg}
	wave := uint32(1)
	jobs.jobs = []domain.FirmwareUpgradeJob{
		{ID: 1, DeviceID: 10, RolloutBatchID: uint64Ptr(1), WaveNumber: &wave, TaskStatusID: taskStatusCompleted},
	}
	// Populasi HANYA device 10 -- sudah ditarget wave 1, tidak ada sisa.
	devices.population = []domain.Device{{ID: 10}}

	got, err := svc.AdvanceRollout(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.StatusID != rtStatusCompleted {
		t.Fatalf("StatusID = %d, want COMPLETED (%d)", got.StatusID, rtStatusCompleted)
	}
	if got.CompletedAt == nil {
		t.Fatal("CompletedAt seharusnya terisi saat batch selesai")
	}
}

// TestAdvanceRollout_ZeroScheduled_Pauses menguji fix utk temuan review kode:
// SEMUA percobaan jadwal wave gagal (mis. kuota task queue tenant habis)
// TIDAK boleh diam-diam retry selamanya -- harus PAUSED, bukan tetap
// IN_PROGRESS dgn CurrentWave maju padahal 0 device benar-benar dijadwalkan.
func TestAdvanceRollout_ZeroScheduled_Pauses(t *testing.T) {
	tenantID := uint64(1)
	svc, rollouts, _, devices := newRolloutTestService()
	rollouts.batch = &domain.FirmwareRolloutBatch{ID: 1, TenantID: &tenantID, FirmwareFileID: 1, WavePercentage: 100, MaxFailureRatePercent: 30, CurrentWave: 0, StatusID: rtStatusPending}
	// Device DENGAN tenant BERBEDA dari batch -- scheduleUpgradeJob akan
	// gagal krn requireDeviceTenantScope (mensimulasikan "gagal dijadwalkan"
	// tanpa perlu memalsukan error kuota terpisah).
	otherTenant := uint64(2)
	devices.population = []domain.Device{{ID: 10, TenantID: &otherTenant}}
	devices.dev = &domain.Device{ID: 10, TenantID: &otherTenant}

	got, err := svc.AdvanceRollout(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.StatusID != rtStatusPaused {
		t.Fatalf("StatusID = %d, want PAUSED_FAILURE_THRESHOLD (%d) -- 0 device berhasil dijadwalkan seharusnya pause, bukan diam-diam retry selamanya", got.StatusID, rtStatusPaused)
	}
	if got.CurrentWave != 0 {
		t.Fatalf("CurrentWave = %d, want tetap 0 (tidak ada wave yang benar-benar berjalan)", got.CurrentWave)
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
