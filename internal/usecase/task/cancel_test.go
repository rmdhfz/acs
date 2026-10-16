package task

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

type cancelRepo struct {
	domain.TaskRepository
	task      *domain.Task
	cancelled bool
}

func (r *cancelRepo) GetByID(context.Context, uint64) (*domain.Task, error) { return r.task, nil }
func (r *cancelRepo) Cancel(context.Context, uint64, *uint64) error {
	r.cancelled = true
	return nil
}

type statusRefs struct{ domain.RefRepository }

func (statusRefs) GetByID(_ context.Context, _ string, id uint64) (domain.RefLookup, error) {
	codes := map[uint64]string{1: domain.TaskStatusPending, 2: domain.TaskStatusSent, 3: domain.TaskStatusCompleted, 4: domain.TaskStatusFailed, 5: domain.TaskStatusCancelled, 6: domain.TaskStatusTimeout}
	return domain.RefLookup{ID: id, Code: codes[id]}, nil
}

// Riwayat task yang sudah berakhir tidak boleh tertimpa CANCELLED.
func TestCancel_HanyaTaskYangBelumBerakhir(t *testing.T) {
	super := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	for status, wantOK := range map[uint64]bool{1: true, 2: true, 3: false, 4: false, 5: false, 6: false} {
		repo := &cancelRepo{task: &domain.Task{ID: 9, DeviceID: 1, TaskStatusID: status}}
		s := &Service{tasks: repo, refs: statusRefs{}}
		err := s.Cancel(context.Background(), super, 9)
		if wantOK && (err != nil || !repo.cancelled) {
			t.Errorf("status %d harus bisa dibatalkan: err=%v cancelled=%v", status, err, repo.cancelled)
		}
		if !wantOK && (!errors.Is(err, domain.ErrConflict) || repo.cancelled) {
			t.Errorf("status %d tidak boleh dibatalkan: err=%v cancelled=%v", status, err, repo.cancelled)
		}
	}
}
