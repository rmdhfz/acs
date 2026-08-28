package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"acs/internal/domain"
)

// fakeTaskRepo implementasi in-memory minimal domain.TaskRepository, hanya
// mencatat apa yang dipanggil failOrRetry (IncrementRetry/SetErrorMessage/
// UpdateStatus) — cukup untuk menguji logika retry tanpa database nyata.
type fakeTaskRepo struct {
	retryCount       int
	statusHistory    []uint64
	lastErrorMessage string
	// pendingCountForTenant — nilai yang dikembalikan CountPendingForTenant,
	// dikonfigurasi test yang menguji enforceTenantTaskQuota (default 0).
	pendingCountForTenant int
	// created — task yang "disimpan" lewat Create, dipakai test proactive
	// chunking (TestCreateTaskProactiveChunking) & split-on-fault
	// (TestSplitGetParameterValuesOnFault) untuk memverifikasi jumlah &
	// konten task hasil pecahan.
	created []domain.Task
	// markFailedCalls — mencatat pemanggilan MarkFailed (dipakai
	// FailPermanently), terpisah dari statusHistory/lastErrorMessage yang
	// mencatat jalur failOrRetry (SetErrorMessage+UpdateStatus terpisah).
	markFailedCalls []markFailedCall
}

type markFailedCall struct {
	TaskID   uint64
	StatusID uint64
	ErrMsg   string
}

func (f *fakeTaskRepo) Create(_ context.Context, t *domain.Task) error {
	t.ID = uint64(len(f.created) + 1)
	f.created = append(f.created, *t)
	return nil
}
func (f *fakeTaskRepo) GetByID(context.Context, uint64) (*domain.Task, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeTaskRepo) GetByUUID(context.Context, string) (*domain.Task, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeTaskRepo) NextForDevice(context.Context, uint64) (*domain.Task, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeTaskRepo) HasPendingForDevice(context.Context, uint64) (bool, error) { return false, nil }
func (f *fakeTaskRepo) CountPendingForTenant(context.Context, uint64) (int, error) {
	return f.pendingCountForTenant, nil
}
func (f *fakeTaskRepo) GetSentForDevice(context.Context, uint64) (*domain.Task, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeTaskRepo) ListStaleSent(context.Context, time.Time) ([]domain.Task, error) {
	return nil, nil
}
func (f *fakeTaskRepo) List(context.Context, domain.TaskFilter, domain.Pagination) ([]domain.Task, int, error) {
	return nil, 0, nil
}
func (f *fakeTaskRepo) CountByStatus(context.Context, *uint64) ([]domain.TaskStatusCount, error) {
	return nil, nil
}
func (f *fakeTaskRepo) AvgCompletionSeconds(context.Context, *uint64, time.Time) (*float64, error) {
	return nil, nil
}
func (f *fakeTaskRepo) CountFailedByVendor(context.Context, *uint64) ([]domain.TaskVendorErrorCount, error) {
	return nil, nil
}
func (f *fakeTaskRepo) UpdateStatus(_ context.Context, _ uint64, statusID uint64, _ *uint64) error {
	f.statusHistory = append(f.statusHistory, statusID)
	return nil
}
func (f *fakeTaskRepo) MarkSent(context.Context, uint64, time.Time) error { return nil }
func (f *fakeTaskRepo) MarkCompleted(context.Context, uint64, domain.JSONRawMessage, time.Time) error {
	return nil
}
func (f *fakeTaskRepo) MarkFailed(_ context.Context, taskID, statusID uint64, errMsg string) error {
	f.markFailedCalls = append(f.markFailedCalls, markFailedCall{TaskID: taskID, StatusID: statusID, ErrMsg: errMsg})
	return nil
}
func (f *fakeTaskRepo) SetErrorMessage(_ context.Context, _ uint64, msg string) error {
	f.lastErrorMessage = msg
	return nil
}
func (f *fakeTaskRepo) IncrementRetry(context.Context, uint64) error {
	f.retryCount++
	return nil
}
func (f *fakeTaskRepo) Cancel(context.Context, uint64, *uint64) error { return nil }

// fakeRefRepo memetakan kode status ke ID tetap, cukup untuk resolusi
// domain.RefTableTaskStatus yang dipakai failOrRetry.
type fakeRefRepo struct{ ids map[string]uint64 }

func (f *fakeRefRepo) GetByCode(_ context.Context, _ string, code string) (domain.RefLookup, error) {
	id, ok := f.ids[code]
	if !ok {
		return domain.RefLookup{}, domain.ErrNotFound
	}
	return domain.RefLookup{ID: id, Code: code}, nil
}
func (f *fakeRefRepo) GetByID(context.Context, string, uint64) (domain.RefLookup, error) {
	return domain.RefLookup{}, domain.ErrNotFound
}
func (f *fakeRefRepo) List(context.Context, string) ([]domain.RefLookup, error) { return nil, nil }

// fakeDeviceRepo implementasi in-memory minimal domain.DeviceRepository —
// hanya GetByID yang dipakai CreateTask (resolve tenant pemilik device untuk
// enforceTenantTaskQuota + cek RBAC scope tenant). Method lain tidak dipanggil
// dari jalur yang diuji di file ini.
type fakeDeviceRepo struct {
	dev *domain.Device
	err error
}

func (f *fakeDeviceRepo) Create(context.Context, *domain.Device) error { return nil }
func (f *fakeDeviceRepo) GetByID(context.Context, uint64) (*domain.Device, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.dev, nil
}
func (f *fakeDeviceRepo) GetByUUID(context.Context, string) (*domain.Device, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeDeviceRepo) GetByOUISerial(context.Context, string, string) (*domain.Device, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeDeviceRepo) List(context.Context, domain.DeviceFilter, domain.Pagination) ([]domain.Device, int, error) {
	return nil, 0, nil
}
func (f *fakeDeviceRepo) CountByStatus(context.Context, *uint64) ([]domain.DeviceStatusCount, error) {
	return nil, nil
}
func (f *fakeDeviceRepo) CountByVendor(context.Context, *uint64) ([]domain.DeviceVendorCount, error) {
	return nil, nil
}
func (f *fakeDeviceRepo) Update(context.Context, *domain.Device) error { return nil }
func (f *fakeDeviceRepo) UpdateStatus(context.Context, uint64, uint64, *uint64) error {
	return nil
}
func (f *fakeDeviceRepo) MarkStaleOffline(context.Context, uint64, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeDeviceRepo) SoftDelete(context.Context, uint64, uint64) error { return nil }

// fakeTenantRepo implementasi in-memory minimal domain.TenantRepository —
// hanya GetByID yang dipakai enforceTenantTaskQuota (resolve max_pending_tasks
// tenant pemilik device). Method lain tidak dipanggil dari CreateTask.
type fakeTenantRepo struct {
	tenant *domain.Tenant
	err    error
}

func (f *fakeTenantRepo) Create(context.Context, *domain.Tenant) error { return nil }
func (f *fakeTenantRepo) GetByID(context.Context, uint64) (*domain.Tenant, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tenant, nil
}
func (f *fakeTenantRepo) GetByUUID(context.Context, string) (*domain.Tenant, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeTenantRepo) GetByCWMPInformUsername(context.Context, string) (*domain.Tenant, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeTenantRepo) List(context.Context, domain.Pagination) ([]domain.Tenant, int, error) {
	return nil, 0, nil
}
func (f *fakeTenantRepo) Update(context.Context, *domain.Tenant) error { return nil }
func (f *fakeTenantRepo) SetCWMPInformCredentials(context.Context, uint64, string, []byte, *uint64) error {
	return nil
}
func (f *fakeTenantRepo) UpdateBranding(context.Context, uint64, *string, *string, *string, *uint64) error {
	return nil
}
func (f *fakeTenantRepo) SetTaskQuota(context.Context, uint64, *uint32, *uint64) error {
	return nil
}
func (f *fakeTenantRepo) SoftDelete(context.Context, uint64, uint64) error { return nil }

// fakeActivityRepo — no-op, hanya supaya CreateTask (yang mencatat activity
// log CREATE_TASK setelah task berhasil dibuat) tidak nil-panic di test.
type fakeActivityRepo struct{}

func (f *fakeActivityRepo) Record(context.Context, *domain.ActivityLog) error { return nil }
func (f *fakeActivityRepo) ListByEntity(context.Context, string, uint64, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}

func superadminActor() domain.Actor {
	return domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
}

func u64(v uint64) *uint64 { return &v }
func u32(v uint32) *uint32 { return &v }

// TestCreateTaskTenantQuota menguji enforceTenantTaskQuota lewat CreateTask —
// lihat CLAUDE.md: test task queue wajib mencakup skenario kuota/max tercapai,
// bukan cuma happy path. Analog dengan TestFailOrRetry tapi untuk
// max_pending_tasks, bukan max_retries.
func TestCreateTaskTenantQuota(t *testing.T) {
	const (
		taskTypeID      = 10
		pendingStatusID = 11
	)
	refs := &fakeRefRepo{ids: map[string]uint64{
		domain.TaskTypeReboot:    taskTypeID,
		domain.TaskStatusPending: pendingStatusID,
	}}
	actor := superadminActor()
	in := domain.CreateTaskInput{DeviceID: 1, TaskType: domain.TaskTypeReboot}

	t.Run("tenant tanpa kuota (MaxPendingTasks nil) -> tidak dibatasi walau pending banyak", func(t *testing.T) {
		devices := &fakeDeviceRepo{dev: &domain.Device{ID: 1, TenantID: u64(100)}}
		tenants := &fakeTenantRepo{tenant: &domain.Tenant{ID: 100, MaxPendingTasks: nil}}
		tasks := &fakeTaskRepo{pendingCountForTenant: 999999}
		svc := NewService(tasks, devices, nil, nil, nil, refs, &fakeActivityRepo{}, tenants, nil)

		got, err := svc.CreateTask(context.Background(), actor, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("task tidak dibuat")
		}
	})

	t.Run("tenant dengan kuota, pending belum mencapai batas -> berhasil dibuat", func(t *testing.T) {
		devices := &fakeDeviceRepo{dev: &domain.Device{ID: 1, TenantID: u64(100)}}
		tenants := &fakeTenantRepo{tenant: &domain.Tenant{ID: 100, MaxPendingTasks: u32(5)}}
		tasks := &fakeTaskRepo{pendingCountForTenant: 4}
		svc := NewService(tasks, devices, nil, nil, nil, refs, &fakeActivityRepo{}, tenants, nil)

		got, err := svc.CreateTask(context.Background(), actor, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("task tidak dibuat")
		}
	})

	t.Run("tenant dengan kuota, pending tepat mencapai batas -> ditolak ErrQuotaExceeded", func(t *testing.T) {
		devices := &fakeDeviceRepo{dev: &domain.Device{ID: 1, TenantID: u64(100)}}
		tenants := &fakeTenantRepo{tenant: &domain.Tenant{ID: 100, MaxPendingTasks: u32(5)}}
		tasks := &fakeTaskRepo{pendingCountForTenant: 5}
		svc := NewService(tasks, devices, nil, nil, nil, refs, &fakeActivityRepo{}, tenants, nil)

		got, err := svc.CreateTask(context.Background(), actor, in)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, domain.ErrQuotaExceeded) {
			t.Errorf("error = %v, want wrapping domain.ErrQuotaExceeded", err)
		}
		if got != nil {
			t.Errorf("task seharusnya tidak dibuat, got %+v", got)
		}
	})

	t.Run("device tanpa tenant (orphan) -> tidak dibatasi kuota apapun", func(t *testing.T) {
		devices := &fakeDeviceRepo{dev: &domain.Device{ID: 1, TenantID: nil}}
		// tenants sengaja tidak dikonfigurasi (GetByID tidak boleh dipanggil
		// sama sekali untuk device orphan — guard dev.TenantID == nil harus
		// short-circuit sebelum resolve tenant).
		tenants := &fakeTenantRepo{err: errors.New("GetByID seharusnya tidak dipanggil untuk device orphan")}
		tasks := &fakeTaskRepo{pendingCountForTenant: 999999}
		svc := NewService(tasks, devices, nil, nil, nil, refs, &fakeActivityRepo{}, tenants, nil)

		got, err := svc.CreateTask(context.Background(), actor, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("task tidak dibuat")
		}
	})

	t.Run("TenantRepository.GetByID error -> di-propagate dari CreateTask, tidak ditelan", func(t *testing.T) {
		devices := &fakeDeviceRepo{dev: &domain.Device{ID: 1, TenantID: u64(100)}}
		wantErr := errors.New("tenant tidak ketemu")
		tenants := &fakeTenantRepo{err: wantErr}
		tasks := &fakeTaskRepo{}
		svc := NewService(tasks, devices, nil, nil, nil, refs, &fakeActivityRepo{}, tenants, nil)

		got, err := svc.CreateTask(context.Background(), actor, in)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, wantErr) {
			t.Errorf("error = %v, want wrapping %v", err, wantErr)
		}
		if got != nil {
			t.Errorf("task seharusnya tidak dibuat, got %+v", got)
		}
	})
}

func TestFailOrRetry(t *testing.T) {
	const (
		pendingStatusID = 1
		failedStatusID  = 2
		timeoutStatusID = 3
	)
	refs := &fakeRefRepo{ids: map[string]uint64{
		domain.TaskStatusPending: pendingStatusID,
		domain.TaskStatusFailed:  failedStatusID,
		domain.TaskStatusTimeout: timeoutStatusID,
	}}

	cases := []struct {
		name            string
		retryCount      uint32
		maxRetries      uint32
		useTimeout      bool // false = Fail (cwmp:Fault), true = Timeout
		wantFinalStatus uint64
	}{
		{"fail di bawah max_retries -> kembali PENDING untuk retry", 0, 3, false, pendingStatusID},
		{"fail tepat mencapai max_retries -> FAILED permanen", 2, 3, false, failedStatusID},
		{"timeout di bawah max_retries -> kembali PENDING untuk retry", 0, 3, true, pendingStatusID},
		{"timeout tepat mencapai max_retries -> TIMEOUT permanen", 2, 3, true, timeoutStatusID},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeTaskRepo{}
			svc := NewService(repo, nil, nil, nil, nil, refs, nil, nil, nil)
			tsk := &domain.Task{ID: 1, RetryCount: tc.retryCount, MaxRetries: tc.maxRetries}

			var err error
			if tc.useTimeout {
				err = svc.Timeout(context.Background(), tsk)
			} else {
				err = svc.Fail(context.Background(), tsk, "cwmp fault")
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if repo.retryCount != 1 {
				t.Errorf("retry_count bertambah %d kali, want 1", repo.retryCount)
			}
			if len(repo.statusHistory) != 1 || repo.statusHistory[0] != tc.wantFinalStatus {
				t.Errorf("status history = %v, want [%d]", repo.statusHistory, tc.wantFinalStatus)
			}
			if repo.lastErrorMessage == "" {
				t.Errorf("error_message tidak tersimpan")
			}
		})
	}
}

// TestFailPermanently menguji FailPermanently (dipakai handleFault utk fault
// code 9005 Invalid Parameter Name) — HARUS langsung FAILED lewat MarkFailed,
// TIDAK LEWAT failOrRetry sama sekali (statusHistory via UpdateStatus generik
// harus tetap kosong), walau retry_count masih jauh di bawah max_retries.
func TestFailPermanently(t *testing.T) {
	const failedStatusID = 42
	refs := &fakeRefRepo{ids: map[string]uint64{domain.TaskStatusFailed: failedStatusID}}
	repo := &fakeTaskRepo{}
	svc := NewService(repo, nil, nil, nil, nil, refs, nil, nil, nil)
	tsk := &domain.Task{ID: 7, RetryCount: 0, MaxRetries: 5}

	if err := svc.FailPermanently(context.Background(), tsk, "parameter tidak didukung"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.retryCount != 1 {
		t.Errorf("retry_count harus bertambah 1 (mencatat 1 percobaan terjadi), got %d", repo.retryCount)
	}
	if len(repo.markFailedCalls) != 1 {
		t.Fatalf("MarkFailed harus dipanggil tepat 1x, got %d", len(repo.markFailedCalls))
	}
	call := repo.markFailedCalls[0]
	if call.TaskID != tsk.ID || call.StatusID != failedStatusID {
		t.Errorf("MarkFailed dipanggil dgn task_id/status_id salah: %+v", call)
	}
	if call.ErrMsg == "" {
		t.Errorf("error_message tidak tersimpan")
	}
	if len(repo.statusHistory) != 0 {
		t.Errorf("failOrRetry generik (UpdateStatus) seharusnya TIDAK dipanggil sama sekali, statusHistory=%v", repo.statusHistory)
	}
}

// TestSplitGetParameterValuesOnFault menguji penanganan REAKTIF fault 9003
// Invalid Arguments pada task GET_PARAMETER_VALUES (CLAUDE.md: test task
// queue wajib mencakup skenario retry/gagal, bukan cuma happy path).
func TestSplitGetParameterValuesOnFault(t *testing.T) {
	const (
		taskTypeID      = 20
		pendingStatusID = 21
		failedStatusID  = 22
	)
	refs := &fakeRefRepo{ids: map[string]uint64{
		domain.TaskTypeGetParameterValues: taskTypeID,
		domain.TaskStatusPending:          pendingStatusID,
		domain.TaskStatusFailed:           failedStatusID,
	}}

	t.Run("names > 1 -> dipecah jadi 2 task baru, task asli FAILED (bukan retry)", func(t *testing.T) {
		devices := &fakeDeviceRepo{dev: &domain.Device{ID: 1, TenantID: u64(100)}}
		repo := &fakeTaskRepo{}
		svc := NewService(repo, devices, nil, nil, nil, refs, &fakeActivityRepo{}, nil, nil)

		orig := &domain.Task{
			ID: 99, DeviceID: 1, Priority: 3, MaxRetries: 3,
			Parameters: domain.JSONRawMessage(`{"names":["A","B","C","D"]}`),
		}
		handled, err := svc.SplitGetParameterValuesOnFault(context.Background(), orig, "[9003] Invalid Arguments")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !handled {
			t.Fatal("want handled=true")
		}
		if len(repo.created) != 2 {
			t.Fatalf("want 2 task baru dibuat, got %d: %+v", len(repo.created), repo.created)
		}

		var total int
		for i, ct := range repo.created {
			if ct.TaskTypeID != taskTypeID {
				t.Errorf("chunk %d: task baru harus GET_PARAMETER_VALUES, got task_type_id=%d", i, ct.TaskTypeID)
			}
			if ct.DeviceID != orig.DeviceID {
				t.Errorf("chunk %d: device_id harus sama dgn task asli, got %d", i, ct.DeviceID)
			}
			var p struct {
				Names []string `json:"names"`
			}
			if err := json.Unmarshal(ct.Parameters, &p); err != nil {
				t.Fatalf("chunk %d: parameters tidak valid: %v", i, err)
			}
			total += len(p.Names)
		}
		if total != 4 {
			t.Errorf("total nama across 2 task baru = %d, want 4 (tidak boleh hilang/duplikat)", total)
		}
		if len(repo.markFailedCalls) != 1 || repo.markFailedCalls[0].TaskID != orig.ID {
			t.Fatalf("task asli harus di-MarkFailed tepat 1x, got %+v", repo.markFailedCalls)
		}
	})

	t.Run("names <= 1 -> tidak bisa dipecah lagi, handled=false, task asli TIDAK disentuh", func(t *testing.T) {
		devices := &fakeDeviceRepo{dev: &domain.Device{ID: 1, TenantID: u64(100)}}
		repo := &fakeTaskRepo{}
		svc := NewService(repo, devices, nil, nil, nil, refs, &fakeActivityRepo{}, nil, nil)

		orig := &domain.Task{ID: 99, DeviceID: 1, Parameters: domain.JSONRawMessage(`{"names":["A"]}`)}
		handled, err := svc.SplitGetParameterValuesOnFault(context.Background(), orig, "[9003] Invalid Arguments")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if handled {
			t.Fatal("want handled=false (tidak bisa dipecah lagi, pemanggil harus fallback ke Fail/failOrRetry biasa)")
		}
		if len(repo.created) != 0 {
			t.Errorf("tidak boleh ada task baru dibuat, got %d", len(repo.created))
		}
		if len(repo.markFailedCalls) != 0 {
			t.Errorf("task asli tidak boleh disentuh sama sekali, got MarkFailed calls: %+v", repo.markFailedCalls)
		}
	})
}

// TestCreateTaskProactiveChunking menguji proactive chunking GET_PARAMETER_VALUES
// (CreateTask, threshold MaxGetParameterValuesNamesPerTask) — dicek SAAT
// task dibuat, terpisah dari penanganan reaktif 9003 di atas.
func TestCreateTaskProactiveChunking(t *testing.T) {
	const (
		taskTypeID      = 30
		pendingStatusID = 31
	)
	refs := &fakeRefRepo{ids: map[string]uint64{
		domain.TaskTypeGetParameterValues: taskTypeID,
		domain.TaskStatusPending:          pendingStatusID,
	}}
	devices := &fakeDeviceRepo{dev: &domain.Device{ID: 1, TenantID: u64(100)}}
	actor := superadminActor()

	names := make([]string, 0, 120)
	for i := 0; i < 120; i++ {
		names = append(names, fmt.Sprintf("Device.Param.%d", i))
	}

	t.Run("melebihi threshold -> dipecah jadi beberapa task berurutan, masing2 <= threshold", func(t *testing.T) {
		repo := &fakeTaskRepo{}
		svc := NewService(repo, devices, nil, nil, nil, refs, &fakeActivityRepo{}, nil, nil)

		got, err := svc.CreateTask(context.Background(), actor, domain.CreateTaskInput{
			DeviceID: 1, TaskType: domain.TaskTypeGetParameterValues,
			Parameters: map[string]interface{}{"names": names},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("task representatif (chunk pertama) tidak dikembalikan")
		}

		const wantChunks = 3 // 50 + 50 + 20 = 120
		if len(repo.created) != wantChunks {
			t.Fatalf("want %d task hasil chunking, got %d", wantChunks, len(repo.created))
		}
		var total int
		for i, ct := range repo.created {
			var p struct {
				Names []string `json:"names"`
			}
			if err := json.Unmarshal(ct.Parameters, &p); err != nil {
				t.Fatalf("chunk %d: parameters tidak valid: %v", i, err)
			}
			if len(p.Names) > MaxGetParameterValuesNamesPerTask {
				t.Errorf("chunk %d melebihi threshold: %d nama", i, len(p.Names))
			}
			if len(p.Names) == 0 {
				t.Errorf("chunk %d kosong", i)
			}
			total += len(p.Names)
		}
		if total != len(names) {
			t.Errorf("total nama across semua chunk = %d, want %d (tidak boleh hilang/duplikat)", total, len(names))
		}
		if got.ID != repo.created[0].ID {
			t.Errorf("task representatif yang dikembalikan harus chunk PERTAMA (created_at paling awal)")
		}
	})

	t.Run("tepat di ambang batas (bukan melebihi) -> TIDAK dipecah, tetap 1 task", func(t *testing.T) {
		repo := &fakeTaskRepo{}
		svc := NewService(repo, devices, nil, nil, nil, refs, &fakeActivityRepo{}, nil, nil)

		exactNames := names[:MaxGetParameterValuesNamesPerTask]
		_, err := svc.CreateTask(context.Background(), actor, domain.CreateTaskInput{
			DeviceID: 1, TaskType: domain.TaskTypeGetParameterValues,
			Parameters: map[string]interface{}{"names": exactNames},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(repo.created) != 1 {
			t.Fatalf("want tepat 1 task (tidak dipecah krn tidak MELEBIHI threshold), got %d", len(repo.created))
		}
	})
}
