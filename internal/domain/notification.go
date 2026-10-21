package domain

import (
	"context"
	"time"
)

// ---- Notifikasi persisten (in-app + email), migrations/0030 ----

// Kode ref_notification_severity.
const (
	NotificationSeverityInfo     = "INFO"
	NotificationSeverityWarning  = "WARNING"
	NotificationSeverityCritical = "CRITICAL"
)

// Kode ref_notification_types. Menambah jenis baru = INSERT ke ref_notification_types (+ audiens bawaan di
// usecase/notification), bukan perubahan skema.
const (
	NotifSecurityAlert      = "SECURITY_ALERT"
	NotifDeviceOfflineSpike = "DEVICE_OFFLINE_SPIKE"
	NotifTaskFailures       = "TASK_FAILURES"
	NotifRolloutPaused      = "ROLLOUT_PAUSED"
	NotifDeviceLimitReached = "DEVICE_LIMIT_REACHED"
	NotifNewTenantSignup    = "NEW_TENANT_SIGNUP"
	NotifAnnouncement       = "ANNOUNCEMENT"
	// Redaman optik perangkat melewati ambang waspada/kritis (migrations/0036).
	NotifOpticalAlert = "OPTICAL_ALERT"
	// Langganan SaaS (migrations/0032).
	NotifBillingPaid     = "BILLING_PAID"
	NotifBillingExpiring = "BILLING_EXPIRING"
	NotifBillingExpired  = "BILLING_EXPIRED"
	NotifBillingFailed   = "BILLING_FAILED"
)

// NotificationAudienceKind menentukan siapa penerima sebuah kejadian.
type NotificationAudienceKind string

const (
	// AudienceDefault (nilai kosong): pakai audiens bawaan jenis notifikasi (lihat usecase/notification).
	AudienceDefault NotificationAudienceKind = ""
	// AudienceSelf: satu pengguna (NotificationAudience.UserID) -- mis. peringatan keamanan akun sendiri.
	AudienceSelf NotificationAudienceKind = "SELF"
	// AudienceTenantRoles: pengguna aktif tenant event dengan salah satu NotificationAudience.Roles.
	// Bila event tidak punya tenant, jatuh ke SUPERADMIN saja (tidak pernah menyebar ke semua tenant).
	AudienceTenantRoles NotificationAudienceKind = "TENANT_ROLES"
	// AudienceSuperadmins: semua SUPERADMIN aktif.
	AudienceSuperadmins NotificationAudienceKind = "SUPERADMINS"
	// AudienceTenantAllStaff: semua staf aktif (SUPERADMIN/ADMIN/NOC/VIEWER, bukan ENDUSER) tenant event;
	// event tanpa tenant = seluruh platform.
	AudienceTenantAllStaff NotificationAudienceKind = "TENANT_ALL_STAFF"
)

type NotificationAudience struct {
	Kind   NotificationAudienceKind
	UserID uint64   // AudienceSelf
	Roles  []string // AudienceTenantRoles
}

// NotificationEvent = satu kejadian yang ingin diberitahukan. Diisi pemanggil (service lain), diurus penuh oleh
// usecase/notification (penerima, preferensi, dedupe, email, push WebSocket).
type NotificationEvent struct {
	TypeCode string
	TenantID *uint64
	Audience NotificationAudience // kosong = audiens bawaan jenis
	Title    string               // maks 160 karakter (dipotong)
	Body     string               // maks 1000 karakter (dipotong)
	// Link = path internal Console (diawali "/", tanpa "//" atau skema); yang tidak valid dibuang.
	Link       string
	EntityType string
	EntityID   *uint64
	// SeverityOverride (opsional) menimpa keparahan bawaan jenis (kode INFO/WARNING/CRITICAL).
	SeverityOverride string
	// DedupeKey + DedupeWindow: jangan buat ulang bila (jenis, tenant, kunci) sudah pernah dibuat dalam jendela.
	DedupeKey    string
	DedupeWindow time.Duration
	// NoEmail: jangan kirim email (mis. email transaksional untuk kejadian yang sama sudah ada).
	NoEmail bool
}

// Notifier = antarmuka sempit yang dipasang di service lain. Kegagalan TIDAK boleh menggagalkan alur utama:
// pemanggil mengabaikan error-nya, dan pemanggil boleh memegang nil.
type Notifier interface {
	Notify(ctx context.Context, ev NotificationEvent) error
}

// SecurityAlertEvent membangun peringatan keamanan untuk pemilik akun sendiri (SECURITY_ALERT, in-app + email
// karena jenis wajib). Isi hanya jenis kejadian + waktu; JANGAN pernah memasukkan password/token/rahasia. noEmail=true
// bila email transaksional untuk kejadian yang sama sudah dikirim jalur lain (hindari email ganda).
func SecurityAlertEvent(userID uint64, tenantID *uint64, title, body string, noEmail bool) NotificationEvent {
	uid := userID
	return NotificationEvent{
		TypeCode: NotifSecurityAlert, TenantID: tenantID,
		Audience: NotificationAudience{Kind: AudienceSelf, UserID: userID},
		Title:    title,
		Body:     body + " Waktu: " + time.Now().Format("02-01-2006 15:04 MST") + ".",
		Link:     "/profile", EntityType: "user", EntityID: &uid, NoEmail: noEmail,
	}
}

// NotificationType = baris ref_notification_types (keparahan sudah di-resolve ke kode).
type NotificationType struct {
	ID           uint64 `db:"id"`
	Code         string `db:"code"`
	Name         string `db:"name"`
	Description  string `db:"description"`
	SeverityID   uint64 `db:"severity_id"`
	Severity     string `db:"severity"` // kode keparahan bawaan
	DefaultInApp bool   `db:"default_in_app"`
	DefaultEmail bool   `db:"default_email"`
	IsMandatory  bool   `db:"is_mandatory"`
}

// Notification = baris yang disisipkan. ID/CreatedAt diisi repository setelah insert.
type Notification struct {
	ID               uint64
	NotificationUUID string
	TenantID         *uint64
	UserID           uint64
	TypeID           uint64
	SeverityID       uint64
	Title            string
	Body             string
	Link             *string
	EntityType       *string
	EntityID         *uint64
	DedupeKey        *string
	ReadAt           *time.Time
	EmailedAt        *time.Time
	CreatedAt        time.Time
}

// NotificationItem = bentuk baca untuk API. "id" = notification_uuid; PK numerik TIDAK PERNAH diekspos.
type NotificationItem struct {
	ID         string     `db:"notification_uuid" json:"id"`
	Type       string     `db:"type_code" json:"type"`
	TypeName   string     `db:"type_name" json:"type_name"`
	Severity   string     `db:"severity" json:"severity"`
	Title      string     `db:"title" json:"title"`
	Body       string     `db:"body" json:"body"`
	Link       *string    `db:"link" json:"link"`
	EntityType *string    `db:"entity_type" json:"entity_type"`
	EntityID   *uint64    `db:"entity_id" json:"entity_id"`
	ReadAt     *time.Time `db:"read_at" json:"read_at"`
	CreatedAt  time.Time  `db:"created_at" json:"created_at"`
}

// NotificationPreference = pilihan kanal pengguna untuk satu jenis (baris tidak ada = pakai bawaan jenis).
type NotificationPreference struct {
	UserID uint64 `db:"user_id"`
	TypeID uint64 `db:"type_id"`
	InApp  bool   `db:"in_app"`
	Email  bool   `db:"email"`
}

// NotificationRecipient = pengguna aktif calon penerima (hasil resolusi audiens).
type NotificationRecipient struct {
	UserID   uint64   `db:"id"`
	TenantID *uint64  `db:"tenant_id"`
	Email    string   `db:"email"`
	FullName string   `db:"full_name"`
	Roles    []string `db:"-"`
}

// TenantDeviceHealth = agregat perangkat (yang pernah Inform) per tenant aktif -- bahan evaluator lonjakan offline.
type TenantDeviceHealth struct {
	TenantID uint64 `db:"tenant_id"`
	Total    int    `db:"total"`
	Offline  int    `db:"offline"`
}

// TenantCount = jumlah per tenant (mis. task gagal).
type TenantCount struct {
	TenantID uint64 `db:"tenant_id"`
	Count    int    `db:"n"`
}

// NotificationRepository -- hanya query, tanpa logika keputusan (semua keputusan di usecase/notification).
type NotificationRepository interface {
	// InsertBatch menyisipkan notifikasi dalam satu transaksi dan mengisi ID tiap elemen.
	InsertBatch(ctx context.Context, items []*Notification) error
	// List: milik userID saja. total = jumlah sesuai filter, unread = jumlah belum dibaca (tanpa filter).
	List(ctx context.Context, userID uint64, unreadOnly bool, p Pagination) (items []NotificationItem, total int, unread int, err error)
	CountUnread(ctx context.Context, userID uint64) (int, error)
	// MarkRead menandai baca notifikasi (uuid) MILIK userID; uuid milik orang lain diabaikan diam-diam.
	MarkRead(ctx context.Context, userID uint64, uuids []string, at time.Time) (int64, error)
	MarkAllRead(ctx context.Context, userID uint64, at time.Time) (int64, error)

	ListTypes(ctx context.Context) ([]NotificationType, error)
	GetTypeByCode(ctx context.Context, code string) (*NotificationType, error)
	// SeverityIDByCode: id ref_notification_severity untuk kode INFO/WARNING/CRITICAL (ErrNotFound bila tak dikenal).
	SeverityIDByCode(ctx context.Context, code string) (uint64, error)
	ListPreferences(ctx context.Context, userID uint64) ([]NotificationPreference, error)
	// ListPreferencesForUsers: preferensi satu jenis untuk banyak pengguna (resolusi penerima).
	ListPreferencesForUsers(ctx context.Context, typeID uint64, userIDs []uint64) ([]NotificationPreference, error)
	UpsertPreferences(ctx context.Context, userID uint64, prefs []NotificationPreference) error

	// ExistsDedupe: sudah ada notifikasi (jenis, tenant, kunci) sejak waktu `since`?
	ExistsDedupe(ctx context.Context, typeID uint64, tenantID *uint64, key string, since time.Time) (bool, error)
	// EmailedUsersSince: dari userIDs, siapa yang sudah diemail jenis ini sejak `since` (throttle email).
	EmailedUsersSince(ctx context.Context, typeID uint64, userIDs []uint64, since time.Time) (map[uint64]bool, error)

	// ListRecipients: pengguna aktif (tenant aktif / SUPERADMIN) dengan salah satu roles. tenantID nil = semua tenant.
	ListRecipients(ctx context.Context, tenantID *uint64, roles []string) ([]NotificationRecipient, error)
	// GetRecipient: satu pengguna aktif + perannya; ErrNotFound bila tidak ada/nonaktif.
	GetRecipient(ctx context.Context, userID uint64) (*NotificationRecipient, error)

	// Pemangkasan notifikasi lama (retensi) ada di RetentionRepository.PruneLogs, bersama tabel log lain.

	// Agregat evaluator (tenant aktif saja).
	TenantDeviceHealth(ctx context.Context) ([]TenantDeviceHealth, error)
	FailedTasksSince(ctx context.Context, since time.Time) ([]TenantCount, error)
}
