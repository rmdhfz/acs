package task

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
	"acs/pkg/netguard"
)

func transferSvc(t *testing.T, allow string) (*Service, *fakeTaskRepo) {
	t.Helper()
	const dlID, ulID, pendingID = 50, 51, 52
	refs := &fakeRefRepo{ids: map[string]uint64{
		domain.TaskTypeDownload: dlID, domain.TaskTypeUpload: ulID, domain.TaskTypeReboot: 53, domain.TaskStatusPending: pendingID,
	}}
	repo := &fakeTaskRepo{}
	svc := NewService(repo, &fakeDeviceRepo{dev: &domain.Device{ID: 1, TenantID: u64(100)}}, nil, nil, nil, refs, &fakeActivityRepo{}, nil, nil, nil)
	if allow != "" {
		a, err := netguard.ParseHostAllowlist(allow)
		if err != nil {
			t.Fatal(err)
		}
		svc.WithTransferHostAllowlist(a)
	}
	return svc, repo
}

// Allowlist host transfer: URL task Download/Upload di luar daftar ditolak (400, tidak ada task dibuat); di dalam daftar,
// task bersumber katalog (storage_key, tanpa URL), tipe lain, dan daftar kosong tidak terkena.
func TestCreateTask_TransferHostAllowlist(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		allow   string
		typ     string
		params  map[string]interface{}
		wantErr bool
	}{
		{"download dari host diizinkan", "files.example.com", domain.TaskTypeDownload, map[string]interface{}{"url": "https://files.example.com/fw.bin"}, false},
		{"download dari subdomain diizinkan", "*.example.com", domain.TaskTypeDownload, map[string]interface{}{"url": "https://cdn.example.com/fw.bin"}, false},
		{"download dari host asing ditolak", "files.example.com", domain.TaskTypeDownload, map[string]interface{}{"url": "https://evil.example.net/fw.bin"}, true},
		{"download dari IP di luar CIDR ditolak", "10.0.0.0/24", domain.TaskTypeDownload, map[string]interface{}{"url": "http://10.0.1.5/fw.bin"}, true},
		{"download dari IP di dalam CIDR diizinkan", "10.0.0.0/24", domain.TaskTypeDownload, map[string]interface{}{"url": "http://10.0.0.9/fw.bin"}, false},
		{"upload ke host asing ditolak (eksfiltrasi konfigurasi/log)", "files.example.com", domain.TaskTypeUpload, map[string]interface{}{"url": "https://evil.example.net/log"}, true},
		{"upload ke host diizinkan", "files.example.com", domain.TaskTypeUpload, map[string]interface{}{"url": "https://files.example.com/log"}, false},
		{"daftar kosong = tidak dibatasi", "", domain.TaskTypeDownload, map[string]interface{}{"url": "https://mana-saja.example.net/fw.bin"}, false},
		{"task katalog (storage_key, tanpa url) tidak terkena", "files.example.com", domain.TaskTypeDownload, map[string]interface{}{"storage_key": "files/firmware/x.bin"}, false},
		{"tipe lain tidak terkena", "files.example.com", domain.TaskTypeReboot, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, repo := transferSvc(t, c.allow)
			_, err := svc.CreateTask(ctx, superadminActor(), domain.CreateTaskInput{DeviceID: 1, TaskType: c.typ, Parameters: c.params})
			if c.wantErr {
				if !errors.Is(err, domain.ErrInvalidInput) {
					t.Fatalf("harus ErrInvalidInput: %v", err)
				}
				if len(repo.created) != 0 {
					t.Fatalf("tidak boleh ada task dibuat: %d", len(repo.created))
				}
				return
			}
			if err != nil {
				t.Fatalf("tidak boleh error: %v", err)
			}
			if len(repo.created) != 1 {
				t.Fatalf("task harus dibuat: %d", len(repo.created))
			}
		})
	}
}
