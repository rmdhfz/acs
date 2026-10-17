// Package report menyusun laporan SaaS untuk SUPERADMIN (operator platform).
package report

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"acs/internal/domain"
)

type Service struct {
	repo domain.ReportRepository
	now  func() time.Time
}

func NewService(repo domain.ReportRepository) *Service { return &Service{repo: repo, now: time.Now} }

const dailyWindowDays = 30

func requireSuperadmin(actor domain.Actor) error {
	if actor.IsSuperadmin() && actor.ImpersonatorID == 0 {
		return nil
	}
	return domain.ErrForbidden
}

// Build menyusun laporan lengkap. Kesehatan tenant diputuskan di sini (aturan transparan, dijelaskan di
// HealthReasons) — bukan di SQL — agar bisa diuji dan diubah tanpa menyentuh query.
func (s *Service) Build(ctx context.Context, actor domain.Actor) (*domain.SaaSReport, error) {
	if err := requireSuperadmin(actor); err != nil {
		return nil, err
	}
	now := s.now()
	plat, err := s.repo.Platform(ctx, now)
	if err != nil {
		return nil, err
	}
	tenants, err := s.repo.Tenants(ctx, now)
	if err != nil {
		return nil, err
	}
	for i := range tenants {
		assessHealth(&tenants[i], now)
	}
	// perhatian dulu, lalu perangkat terbanyak: yang perlu ditangani operator ada di atas.
	sort.SliceStable(tenants, func(i, j int) bool {
		ri, rj := healthRank(tenants[i].Health), healthRank(tenants[j].Health)
		if ri != rj {
			return ri < rj
		}
		return tenants[i].Devices > tenants[j].Devices
	})
	vendors, err := s.repo.Vendors(ctx)
	if err != nil {
		return nil, err
	}
	since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(dailyWindowDays - 1))
	byDay, err := s.repo.Daily(ctx, since)
	if err != nil {
		return nil, err
	}
	daily := make([]domain.DailyPoint, 0, dailyWindowDays)
	for i := 0; i < dailyWindowDays; i++ {
		d := since.AddDate(0, 0, i).Format("2006-01-02")
		p := byDay[d]
		p.Date = d
		daily = append(daily, p)
	}
	return &domain.SaaSReport{GeneratedAt: now, Platform: plat, Tenants: tenants, Vendors: vendors, Daily: daily}, nil
}

func healthRank(h string) int {
	switch h {
	case "perhatian":
		return 0
	case "sehat":
		return 1
	default:
		return 2
	}
}

// assessHealth: nonaktif | perhatian (ada alasan konkret) | sehat.
func assessHealth(t *domain.TenantReport, now time.Time) {
	t.HealthReasons = []string{}
	if !t.IsActive {
		t.Health = "nonaktif"
		return
	}
	if t.Devices > 0 {
		if t.LastInformAt == nil || now.Sub(*t.LastInformAt) > 24*time.Hour {
			t.HealthReasons = append(t.HealthReasons, "tidak ada Inform dalam 24 jam terakhir")
		}
		if float64(t.DevicesOnline)/float64(t.Devices) < 0.5 {
			t.HealthReasons = append(t.HealthReasons, fmt.Sprintf("hanya %d dari %d perangkat online", t.DevicesOnline, t.Devices))
		}
	}
	if t.Tasks30d >= 10 && float64(t.TasksFailed30d)/float64(t.Tasks30d) > 0.2 {
		t.HealthReasons = append(t.HealthReasons, fmt.Sprintf("%d%% task gagal (30 hari)", t.TasksFailed30d*100/t.Tasks30d))
	}
	if t.MaxPending != nil && *t.MaxPending > 0 && float64(t.PendingTasks) >= 0.9*float64(*t.MaxPending) {
		t.HealthReasons = append(t.HealthReasons, "antrean task mendekati kuota")
	}
	if len(t.HealthReasons) > 0 {
		t.Health = "perhatian"
	} else {
		t.Health = "sehat"
	}
}

// CSVSafe menetralkan sel yang diawali = + - @ (tab/CR) agar tidak dijalankan sebagai rumus oleh Excel/Sheets
// (CSV injection). Nama tenant diisi pengguna, jadi harus selalu dinetralkan.
func CSVSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// TenantsCSV — baris CSV (header + data) tabel tenant untuk dibagikan ke manajemen.
func TenantsCSV(rep *domain.SaaSReport) [][]string {
	rows := [][]string{{
		"kode", "nama", "status", "kesehatan", "alasan", "dibuat", "perangkat", "perangkat_online", "pengguna", "pengguna_aktif_30h",
		"task_30h", "task_gagal_30h", "task_antre", "kuota_antre", "sesi_30h", "webhook_aktif", "inform_terakhir", "login_terakhir",
	}}
	ts := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.UTC().Format(time.RFC3339)
	}
	for _, t := range rep.Tenants {
		status := "aktif"
		if !t.IsActive {
			status = "nonaktif"
		}
		quota := ""
		if t.MaxPending != nil {
			quota = strconv.FormatUint(uint64(*t.MaxPending), 10)
		}
		rows = append(rows, []string{
			CSVSafe(t.Code), CSVSafe(t.Name), status, t.Health, CSVSafe(strings.Join(t.HealthReasons, "; ")), t.CreatedAt.UTC().Format(time.RFC3339),
			strconv.Itoa(t.Devices), strconv.Itoa(t.DevicesOnline), strconv.Itoa(t.Users), strconv.Itoa(t.UsersActive30d),
			strconv.Itoa(t.Tasks30d), strconv.Itoa(t.TasksFailed30d), strconv.Itoa(t.PendingTasks), quota,
			strconv.Itoa(t.Sessions30d), strconv.Itoa(t.Webhooks), ts(t.LastInformAt), ts(t.LastLoginAt),
		})
	}
	return rows
}
