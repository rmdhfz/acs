package domain

import (
	"context"
	"time"
)

// Laporan SaaS untuk SUPERADMIN (operator platform): ukuran pelanggan (tenant), pemakaian, dan kesehatan.
// Semua angka agregat dibaca langsung dari data operasional (tanpa tabel ringkasan) sehingga selalu konsisten.

type PlatformStats struct {
	TenantsTotal   int `db:"tenants_total" json:"tenants_total"`
	TenantsActive  int `db:"tenants_active" json:"tenants_active"`
	TenantsNew30d  int `db:"tenants_new_30d" json:"tenants_new_30d"`
	DevicesTotal   int `db:"devices_total" json:"devices_total"`
	DevicesOnline  int `db:"devices_online" json:"devices_online"`
	DevicesNew7d   int `db:"devices_new_7d" json:"devices_new_7d"`
	DevicesNew30d  int `db:"devices_new_30d" json:"devices_new_30d"`
	UsersTotal     int `db:"users_total" json:"users_total"`
	UsersActive30d int `db:"users_active_30d" json:"users_active_30d"`
	Tasks30d       int `db:"tasks_30d" json:"tasks_30d"`
	TasksFailed30d int `db:"tasks_failed_30d" json:"tasks_failed_30d"`
	Sessions24h    int `db:"sessions_24h" json:"sessions_24h"`
	FailedLogins24 int `db:"failed_logins_24h" json:"failed_logins_24h"`
	WebhooksActive int `db:"webhooks_active" json:"webhooks_active"`
	FirmwareFiles  int `db:"firmware_files" json:"firmware_files"`
}

// TenantReport — satu baris per tenant. Health diputuskan di usecase (bukan di SQL).
type TenantReport struct {
	TenantID       uint64     `json:"tenant_id"`
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	IsActive       bool       `json:"is_active"`
	CreatedAt      time.Time  `json:"created_at"`
	Devices        int        `json:"devices"`
	DevicesOnline  int        `json:"devices_online"`
	Users          int        `json:"users"`
	UsersActive30d int        `json:"users_active_30d"`
	Tasks30d       int        `json:"tasks_30d"`
	TasksFailed30d int        `json:"tasks_failed_30d"`
	PendingTasks   int        `json:"pending_tasks"`
	MaxPending     *uint32    `json:"max_pending_tasks"`
	Sessions30d    int        `json:"sessions_30d"`
	Webhooks       int        `json:"webhooks_active"`
	LastInformAt   *time.Time `json:"last_inform_at"`
	LastLoginAt    *time.Time `json:"last_login_at"`
	Health         string     `json:"health"` // sehat | perhatian | nonaktif
	HealthReasons  []string   `json:"health_reasons"`
}

type VendorStat struct {
	Vendor  string `db:"vendor" json:"vendor"`
	Devices int    `db:"devices" json:"devices"`
}

type DailyPoint struct {
	Date       string `json:"date"` // YYYY-MM-DD
	NewDevices int    `json:"new_devices"`
	Sessions   int    `json:"sessions"`
	Tasks      int    `json:"tasks"`
	TasksFail  int    `json:"tasks_failed"`
}

type SaaSReport struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Platform    PlatformStats  `json:"platform"`
	Tenants     []TenantReport `json:"tenants"`
	Vendors     []VendorStat   `json:"vendors"`
	Daily       []DailyPoint   `json:"daily"`
}

// ReportRepository — hanya query agregat; keputusan (kesehatan, urutan) ada di usecase.
type ReportRepository interface {
	Platform(ctx context.Context, now time.Time) (PlatformStats, error)
	// Tenants mengembalikan baris dasar tenant + metrik; Health belum diisi.
	Tenants(ctx context.Context, now time.Time) ([]TenantReport, error)
	Vendors(ctx context.Context) ([]VendorStat, error)
	// Daily mengembalikan hitungan per hari (hanya hari yang ada datanya), kunci YYYY-MM-DD.
	Daily(ctx context.Context, since time.Time) (map[string]DailyPoint, error)
}
