package task

import (
	"context"
	"errors"
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
}

func (f *fakeTaskRepo) Create(context.Context, *domain.Task) error { return nil }
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
func (f *fakeTaskRepo) MarkFailed(context.Context, uint64, uint64, string) error { return nil }
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
		svc := NewService(tasks, devices, nil, nil, refs, &fakeActivityRepo{}, tenants)

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
		svc := NewService(tasks, devices, nil, nil, refs, &fakeActivityRepo{}, tenants)

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
		svc := NewService(tasks, devices, nil, nil, refs, &fakeActivityRepo{}, tenants)

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
		svc := NewService(tasks, devices, nil, nil, refs, &fakeActivityRepo{}, tenants)

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
		svc := NewService(tasks, devices, nil, nil, refs, &fakeActivityRepo{}, tenants)

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
			svc := NewService(repo, nil, nil, nil, refs, nil, nil)
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
