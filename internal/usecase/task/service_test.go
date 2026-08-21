package task

import (
	"context"
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
func (f *fakeTaskRepo) GetSentForDevice(context.Context, uint64) (*domain.Task, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeTaskRepo) ListStaleSent(context.Context, time.Time) ([]domain.Task, error) {
	return nil, nil
}
func (f *fakeTaskRepo) List(context.Context, domain.TaskFilter, domain.Pagination) ([]domain.Task, int, error) {
	return nil, 0, nil
}
func (f *fakeTaskRepo) UpdateStatus(_ context.Context, _ uint64, statusID uint64, _ *uint64) error {
	f.statusHistory = append(f.statusHistory, statusID)
	return nil
}
func (f *fakeTaskRepo) MarkSent(context.Context, uint64, time.Time) error         { return nil }
func (f *fakeTaskRepo) MarkCompleted(context.Context, uint64, []byte, time.Time) error { return nil }
func (f *fakeTaskRepo) MarkFailed(context.Context, uint64, uint64, string) error  { return nil }
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
			svc := NewService(repo, nil, nil, nil, refs, nil)
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
