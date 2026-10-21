package task

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

type retryRepo struct {
	domain.TaskRepository
	task     *domain.Task
	requeued bool
}

func (r *retryRepo) GetByID(context.Context, uint64) (*domain.Task, error) { return r.task, nil }
func (r *retryRepo) Requeue(context.Context, uint64, *uint64) error {
	r.requeued = true
	return nil
}

// retryRefs: status (id 1..6 seperti statusRefs) dan jenis task (id 10..13).
type retryRefs struct{ domain.RefRepository }

func (retryRefs) GetByID(_ context.Context, table string, id uint64) (domain.RefLookup, error) {
	if table == domain.RefTableTaskTypes {
		codes := map[uint64]string{10: domain.TaskTypeGetParameterValues, 11: domain.TaskTypeSetParameterValues, 12: domain.TaskTypeReboot, 13: domain.TaskTypeFactoryReset}
		return domain.RefLookup{ID: id, Code: codes[id]}, nil
	}
	return statusRefs{}.GetByID(context.Background(), table, id)
}

const (
	stPend, stSent, stDone, stFail, stCancel, stTimeout = 1, 2, 3, 4, 5, 6
	tyGet, tySet, tyReboot, tyReset                     = 10, 11, 12, 13
)

func retrySvc(task *domain.Task, devTenant uint64) (*Service, *retryRepo) {
	repo := &retryRepo{task: task}
	return &Service{
		tasks:    repo,
		refs:     retryRefs{},
		devices:  &fakeDeviceRepo{dev: &domain.Device{ID: task.DeviceID, TenantID: &devTenant}},
		activity: &fakeActivityRepo{},
	}, repo
}

// Hanya task gagal/dibatalkan/timeout/menunggu yang dinyalakan ulang; yang sedang berjalan atau selesai ditolak.
func TestRetry_HanyaStatusYangBolehDiulang(t *testing.T) {
	super := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	for status, wantOK := range map[uint64]bool{stPend: true, stSent: false, stDone: false, stFail: true, stCancel: true, stTimeout: true} {
		s, repo := retrySvc(&domain.Task{ID: 9, DeviceID: 1, TaskStatusID: status, TaskTypeID: tySet}, 7)
		_, err := s.Retry(context.Background(), super, 9)
		if wantOK && (err != nil || !repo.requeued) {
			t.Errorf("status %d harus bisa diulang: err=%v requeued=%v", status, err, repo.requeued)
		}
		if !wantOK && (!errors.Is(err, domain.ErrConflict) || repo.requeued) {
			t.Errorf("status %d tidak boleh diulang: err=%v requeued=%v", status, err, repo.requeued)
		}
	}
}

// NOC hanya boleh mengulang baca & reboot; task yang menulis/mereset butuh ADMIN. Lintas tenant ditolak.
func TestRetry_RBACDanTenant(t *testing.T) {
	tenant7, tenant8 := uint64(7), uint64(8)
	noc := domain.Actor{UserID: 2, TenantID: &tenant7, Roles: []string{domain.RoleNOC}}
	admin := domain.Actor{UserID: 3, TenantID: &tenant7, Roles: []string{domain.RoleAdmin}}
	other := domain.Actor{UserID: 4, TenantID: &tenant8, Roles: []string{domain.RoleAdmin}}

	cases := []struct {
		name    string
		actor   domain.Actor
		typ     uint64
		wantErr error
	}{
		{"NOC ulang baca", noc, tyGet, nil},
		{"NOC ulang reboot", noc, tyReboot, nil},
		{"NOC ulang SET → ditolak", noc, tySet, domain.ErrForbidden},
		{"NOC ulang factory reset → ditolak", noc, tyReset, domain.ErrForbidden},
		{"ADMIN ulang SET", admin, tySet, nil},
		{"ADMIN tenant lain → ditolak", other, tySet, domain.ErrForbidden},
	}
	for _, c := range cases {
		s, repo := retrySvc(&domain.Task{ID: 9, DeviceID: 1, TaskStatusID: stFail, TaskTypeID: c.typ}, 7)
		_, err := s.Retry(context.Background(), c.actor, 9)
		if !errors.Is(err, c.wantErr) && !(err == nil && c.wantErr == nil) {
			t.Errorf("%s: err=%v, mau %v", c.name, err, c.wantErr)
		}
		if (c.wantErr == nil) != repo.requeued {
			t.Errorf("%s: requeued=%v", c.name, repo.requeued)
		}
	}
}
