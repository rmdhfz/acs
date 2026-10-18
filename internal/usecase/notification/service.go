// Package notification — notifikasi persisten (in-app + email) untuk staf Console (migrations/0030).
//
// Alur satu kejadian (Notify): resolusi penerima menurut audiens -> preferensi per pengguna (bawaan dari jenis bila
// belum pernah disimpan; jenis wajib tidak bisa dimatikan) -> dedupe (jenis, tenant, kunci, jendela) -> simpan ->
// email async (maks 1 per pengguna per jenis per jam) -> dorong realtime ke WebSocket pengguna.
//
// Aturan keamanan: ENDUSER tidak pernah menjadi penerima; isi notifikasi/email tidak boleh memuat kredensial
// (tanggung jawab pemanggil, sama seperti activity_logs); tautan hanya path internal Console; pengguna hanya bisa
// membaca/menandai notifikasi miliknya; pengumuman hanya SUPERADMIN.
//
// Kegagalan notifikasi TIDAK PERNAH boleh menggagalkan alur utama: pemanggil mengabaikan error Notify.
package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/notify"
	"acs/pkg/mailer"
)

// Batas panjang = lebar kolom notifications (schema.sql / migrations/0030) dan validasi handler.
const (
	MaxTitleLen = 160
	MaxBodyLen  = 1000
	MaxLinkLen  = 255
	maxMarkIDs  = 200
	maxPrefs    = 100
)

// Ambang evaluator bawaan (dapat ditimpa lewat ACS_NOTIFY_OFFLINE_RATIO / ACS_NOTIFY_FAILED_TASKS).
const (
	// DefaultOfflineRatio — lonjakan offline bila >= 30% perangkat tenant (yang pernah Inform) berstatus OFFLINE...
	DefaultOfflineRatio = 0.30
	// minDevicesOfflineSpike — ...dan tenant punya >= 10 perangkat (tenant kecil: satu perangkat mati = 100%, bukan lonjakan).
	minDevicesOfflineSpike = 10
	offlineSpikeDedupe     = 6 * time.Hour

	// DefaultFailedTasks — >= 5 task FAILED dalam failedTasksWindow terakhir dianggap kegagalan beruntun.
	DefaultFailedTasks = 5
	failedTasksWindow  = 30 * time.Minute
	failedTasksDedupe  = time.Hour

	// emailThrottle — maks 1 email per (pengguna, jenis) per jam walau notifikasi in-app banyak.
	emailThrottle = time.Hour

	// Rate limit pengumuman per aktor (sederhana, di memori per instance).
	maxAnnouncePerHour = 10
)

// Pusher mendorong event realtime ke koneksi WebSocket satu pengguna (ws.Hub).
type Pusher interface {
	BroadcastToUser(userID uint64, eventType string, payload interface{})
}

// Config — ambang evaluator.
type Config struct {
	OfflineRatio float64 // 0 < r <= 1; di luar itu memakai DefaultOfflineRatio
	FailedTasks  int     // <= 0 memakai DefaultFailedTasks
}

type Service struct {
	repo     domain.NotificationRepository
	tenants  domain.TenantRepository
	activity domain.ActivityLogRepository
	cfg      Config
	log      *slog.Logger
	now      func() time.Time

	mail    mailer.Notifier // nil = email nonaktif (SMTP belum dikonfigurasi)
	builder *notify.Builder
	push    Pusher

	annMu sync.Mutex
	ann   map[uint64][]time.Time
}

func NewService(repo domain.NotificationRepository, tenants domain.TenantRepository, activity domain.ActivityLogRepository, cfg Config, log *slog.Logger) *Service {
	if cfg.OfflineRatio <= 0 || cfg.OfflineRatio > 1 {
		cfg.OfflineRatio = DefaultOfflineRatio
	}
	if cfg.FailedTasks <= 0 {
		cfg.FailedTasks = DefaultFailedTasks
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: repo, tenants: tenants, activity: activity, cfg: cfg, log: log, now: time.Now, ann: map[uint64][]time.Time{}}
}

// WithEmail mengaktifkan kanal email (n nil = tetap nonaktif).
func (s *Service) WithEmail(n mailer.Notifier, b *notify.Builder) *Service {
	s.mail, s.builder = n, b
	return s
}

// WithPusher mengaktifkan push realtime.
func (s *Service) WithPusher(p Pusher) *Service {
	s.push = p
	return s
}

// staffRoles = peran yang boleh menerima notifikasi. ENDUSER (portal pelanggan) sengaja tidak ada.
var staffRoles = []string{domain.RoleSuperadmin, domain.RoleAdmin, domain.RoleNOC, domain.RoleViewer}

func isStaffRole(r string) bool {
	for _, s := range staffRoles {
		if s == r {
			return true
		}
	}
	return false
}

// defaultAudiences = audiens bawaan tiap jenis bila pemanggil tidak menyebut audiens. SECURITY_ALERT sengaja tidak
// punya bawaan (harus AudienceSelf dengan UserID eksplisit).
func defaultAudiences(code string) []domain.NotificationAudience {
	switch code {
	case domain.NotifDeviceOfflineSpike, domain.NotifTaskFailures:
		return []domain.NotificationAudience{{Kind: domain.AudienceTenantRoles, Roles: []string{domain.RoleAdmin, domain.RoleNOC}}}
	case domain.NotifRolloutPaused:
		return []domain.NotificationAudience{{Kind: domain.AudienceTenantRoles, Roles: []string{domain.RoleAdmin}}}
	case domain.NotifDeviceLimitReached:
		return []domain.NotificationAudience{
			{Kind: domain.AudienceTenantRoles, Roles: []string{domain.RoleAdmin}},
			{Kind: domain.AudienceSuperadmins},
		}
	case domain.NotifNewTenantSignup:
		return []domain.NotificationAudience{{Kind: domain.AudienceSuperadmins}}
	case domain.NotifAnnouncement:
		return []domain.NotificationAudience{{Kind: domain.AudienceTenantAllStaff}}
	case domain.NotifBillingPaid:
		// Pembayaran langganan: ADMIN tenant + operator platform.
		return []domain.NotificationAudience{
			{Kind: domain.AudienceTenantRoles, Roles: []string{domain.RoleAdmin}},
			{Kind: domain.AudienceSuperadmins},
		}
	case domain.NotifBillingExpiring, domain.NotifBillingExpired, domain.NotifBillingFailed:
		return []domain.NotificationAudience{{Kind: domain.AudienceTenantRoles, Roles: []string{domain.RoleAdmin}}}
	}
	return nil
}

// ---- sanitasi ----

func hasControl(s string, allowNewline bool) bool {
	for _, r := range s {
		if r == '\n' && allowNewline {
			continue
		}
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

// ValidLink: path internal Console saja -- diawali "/", tanpa "//" (protocol-relative), tanpa backslash (browser
// menormalkan "/\" jadi "//"), tanpa skema/spasi/karakter kontrol.
func ValidLink(link string) bool {
	if link == "" || len(link) > MaxLinkLen || link[0] != '/' {
		return false
	}
	if strings.Contains(link, "//") || strings.Contains(link, "\\") || strings.ContainsAny(link, " \t") || hasControl(link, false) {
		return false
	}
	return true
}

// cleanText memangkas spasi tepi dan membuang karakter kontrol (kecuali newline pada body) lalu memotong ke batas.
func cleanText(s string, max int, allowNewline bool) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if r == '\r' {
			continue
		}
		if r == '\n' && allowNewline {
			b.WriteRune(r)
			continue
		}
		if r < 0x20 || r == 0x7f {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	return truncateRunes(strings.TrimSpace(b.String()), max)
}

// ---- Notify ----

// Notify memenuhi domain.Notifier.
func (s *Service) Notify(ctx context.Context, ev domain.NotificationEvent) error {
	_, err := s.dispatch(ctx, ev)
	return err
}

func (s *Service) resolveAudience(ctx context.Context, ev domain.NotificationEvent, a domain.NotificationAudience) ([]domain.NotificationRecipient, error) {
	switch a.Kind {
	case domain.AudienceSelf:
		rc, err := s.repo.GetRecipient(ctx, a.UserID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil, nil // pengguna nonaktif/terhapus: tidak ada yang diberi tahu
			}
			return nil, err
		}
		for _, r := range rc.Roles {
			if isStaffRole(r) {
				return []domain.NotificationRecipient{*rc}, nil
			}
		}
		return nil, nil // ENDUSER murni tidak menerima notifikasi
	case domain.AudienceSuperadmins:
		return s.repo.ListRecipients(ctx, nil, []string{domain.RoleSuperadmin})
	case domain.AudienceTenantRoles:
		if ev.TenantID == nil {
			// Kejadian tingkat platform: jangan pernah menyebar ke admin SEMUA tenant.
			return s.repo.ListRecipients(ctx, nil, []string{domain.RoleSuperadmin})
		}
		var roles []string
		for _, r := range a.Roles {
			if isStaffRole(r) {
				roles = append(roles, r)
			}
		}
		return s.repo.ListRecipients(ctx, ev.TenantID, roles)
	case domain.AudienceTenantAllStaff:
		return s.repo.ListRecipients(ctx, ev.TenantID, staffRoles)
	}
	return nil, fmt.Errorf("%w: audiens notifikasi tidak dikenal", domain.ErrInvalidInput)
}

func (s *Service) recipients(ctx context.Context, ev domain.NotificationEvent) ([]domain.NotificationRecipient, error) {
	auds := defaultAudiences(ev.TypeCode)
	if ev.Audience.Kind != domain.AudienceDefault {
		auds = []domain.NotificationAudience{ev.Audience}
	}
	if len(auds) == 0 {
		return nil, fmt.Errorf("%w: jenis %s membutuhkan audiens eksplisit", domain.ErrInvalidInput, ev.TypeCode)
	}
	seen := map[uint64]bool{}
	var out []domain.NotificationRecipient
	for _, a := range auds {
		rcs, err := s.resolveAudience(ctx, ev, a)
		if err != nil {
			return nil, err
		}
		for _, r := range rcs {
			if !seen[r.UserID] {
				seen[r.UserID] = true
				out = append(out, r)
			}
		}
	}
	return out, nil
}

// dispatch menjalankan seluruh alur dan mengembalikan jumlah penerima yang dibuatkan notifikasi.
func (s *Service) dispatch(ctx context.Context, ev domain.NotificationEvent) (int, error) {
	typ, err := s.repo.GetTypeByCode(ctx, ev.TypeCode)
	if err != nil {
		return 0, fmt.Errorf("notifikasi: jenis %q: %w", ev.TypeCode, err)
	}
	title := cleanText(ev.Title, MaxTitleLen, false)
	body := cleanText(ev.Body, MaxBodyLen, true)
	if title == "" || body == "" {
		return 0, fmt.Errorf("%w: judul dan isi notifikasi wajib diisi", domain.ErrInvalidInput)
	}
	link := ev.Link
	if link != "" && !ValidLink(link) {
		link = "" // tautan bukan path internal: dibuang, notifikasi tetap dibuat
	}

	severityCode, severityID := typ.Severity, typ.SeverityID
	if ev.SeverityOverride != "" && ev.SeverityOverride != typ.Severity {
		id, err := s.repo.SeverityIDByCode(ctx, ev.SeverityOverride)
		if err != nil {
			return 0, fmt.Errorf("%w: keparahan %q tidak dikenal", domain.ErrInvalidInput, ev.SeverityOverride)
		}
		severityCode, severityID = ev.SeverityOverride, id
	}

	now := s.now()
	if ev.DedupeKey != "" && ev.DedupeWindow > 0 {
		dup, err := s.repo.ExistsDedupe(ctx, typ.ID, ev.TenantID, truncateRunes(ev.DedupeKey, 128), now.Add(-ev.DedupeWindow))
		if err != nil {
			return 0, err
		}
		if dup {
			return 0, nil
		}
	}

	rcs, err := s.recipients(ctx, ev)
	if err != nil {
		return 0, err
	}
	if len(rcs) == 0 {
		return 0, nil
	}
	ids := make([]uint64, len(rcs))
	for i, r := range rcs {
		ids[i] = r.UserID
	}
	prefList, err := s.repo.ListPreferencesForUsers(ctx, typ.ID, ids)
	if err != nil {
		return 0, err
	}
	prefs := make(map[uint64]domain.NotificationPreference, len(prefList))
	for _, p := range prefList {
		prefs[p.UserID] = p
	}

	// Throttle email: hanya relevan bila kanal email memang aktif.
	emailOn := s.mail != nil && s.builder != nil && !ev.NoEmail
	var recentlyEmailed map[uint64]bool
	if emailOn {
		recentlyEmailed, err = s.repo.EmailedUsersSince(ctx, typ.ID, ids, now.Add(-emailThrottle))
		if err != nil {
			return 0, err
		}
	}

	var dedupeKey, entityType *string
	if ev.DedupeKey != "" {
		k := truncateRunes(ev.DedupeKey, 128)
		dedupeKey = &k
	}
	if ev.EntityType != "" {
		e := truncateRunes(ev.EntityType, 32)
		entityType = &e
	}
	var linkPtr *string
	if link != "" {
		linkPtr = &link
	}

	type pending struct {
		n  *domain.Notification
		rc domain.NotificationRecipient
	}
	var rows []*domain.Notification
	var emails []pending
	var pushes []*domain.Notification
	for _, rc := range rcs {
		inApp, email := typ.DefaultInApp, typ.DefaultEmail
		if p, ok := prefs[rc.UserID]; ok {
			inApp, email = p.InApp, p.Email
		}
		if typ.IsMandatory {
			inApp, email = true, true
		}
		wantEmail := email && emailOn && rc.Email != "" && !recentlyEmailed[rc.UserID]
		if !inApp && !wantEmail {
			continue
		}
		n := &domain.Notification{
			NotificationUUID: uuid.NewString(), TenantID: ev.TenantID, UserID: rc.UserID, TypeID: typ.ID, SeverityID: severityID,
			Title: title, Body: body, Link: linkPtr, EntityType: entityType, EntityID: ev.EntityID, DedupeKey: dedupeKey,
		}
		if !inApp {
			// Hanya email (in-app dimatikan pengguna): baris tetap dicatat agar dedupe & throttle bekerja,
			// tetapi sudah berstatus dibaca sehingga tidak menaikkan hitungan belum-dibaca.
			t := now
			n.ReadAt = &t
		}
		if wantEmail {
			t := now
			n.EmailedAt = &t
		}
		rows = append(rows, n)
		if wantEmail {
			emails = append(emails, pending{n, rc})
		}
		if inApp {
			pushes = append(pushes, n)
		}
	}
	if len(rows) == 0 {
		return 0, nil
	}
	if err := s.repo.InsertBatch(ctx, rows); err != nil {
		return 0, err
	}

	// Email async lewat mailer.Notifier (antrean; kegagalan kirim hanya di-log oleh mailer).
	paragraphs := splitParagraphs(body)
	for _, e := range emails {
		s.mail.Notify(s.builder.Notification(e.rc.Email, e.rc.FullName, title, paragraphs, link, "Buka di Console", severityCode))
	}
	if s.push != nil {
		for _, n := range pushes {
			s.push.BroadcastToUser(n.UserID, "NOTIFICATION", map[string]any{
				"id": n.NotificationUUID, "type": typ.Code, "severity": severityCode, "title": title, "link": link,
			})
		}
	}
	return len(rows), nil
}

func splitParagraphs(body string) []string {
	var out []string
	for _, p := range strings.Split(body, "\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---- Evaluator periodik ----

// Evaluate dipanggil sweeper (5 menit, di bawah kunci leader). Mengembalikan jumlah notifikasi yang dibuat.
func (s *Service) Evaluate(ctx context.Context) (int, error) {
	created := 0
	health, err := s.repo.TenantDeviceHealth(ctx)
	if err != nil {
		return 0, err
	}
	for _, h := range health {
		// Toleransi 1e-9: 3/10 harus tepat memenuhi ambang 0,30 (galat pembulatan float).
		if h.Total < minDevicesOfflineSpike || float64(h.Offline)/float64(h.Total) < s.cfg.OfflineRatio-1e-9 {
			continue
		}
		tid := h.TenantID
		n, err := s.dispatch(ctx, domain.NotificationEvent{
			TypeCode: domain.NotifDeviceOfflineSpike, TenantID: &tid,
			Title:        "Banyak perangkat offline",
			Body:         fmt.Sprintf("%d dari %d perangkat (%d%%) sedang offline. Periksa jaringan, listrik, atau perangkat di lokasi terdampak.", h.Offline, h.Total, h.Offline*100/h.Total),
			Link:         "/devices",
			DedupeKey:    "offline-spike",
			DedupeWindow: offlineSpikeDedupe,
		})
		if err != nil {
			s.log.Warn("notifikasi: evaluasi lonjakan offline gagal", "tenant_id", tid, "error", err)
			continue
		}
		created += n
	}

	failed, err := s.repo.FailedTasksSince(ctx, s.now().Add(-failedTasksWindow))
	if err != nil {
		return created, err
	}
	for _, f := range failed {
		if f.Count < s.cfg.FailedTasks {
			continue
		}
		tid := f.TenantID
		n, err := s.dispatch(ctx, domain.NotificationEvent{
			TypeCode: domain.NotifTaskFailures, TenantID: &tid,
			Title:        "Banyak task gagal",
			Body:         fmt.Sprintf("%d task gagal dalam %d menit terakhir. Lihat daftar task untuk penyebabnya.", f.Count, int(failedTasksWindow.Minutes())),
			Link:         "/tasks",
			DedupeKey:    "task-failures",
			DedupeWindow: failedTasksDedupe,
		})
		if err != nil {
			s.log.Warn("notifikasi: evaluasi kegagalan task gagal", "tenant_id", tid, "error", err)
			continue
		}
		created += n
	}
	return created, nil
}

// ---- API pengguna (dipanggil handler) ----

func requireUser(actor domain.Actor) error {
	if actor.UserID == 0 {
		return domain.ErrForbidden
	}
	return nil
}

// List — notifikasi milik actor sendiri.
func (s *Service) List(ctx context.Context, actor domain.Actor, unreadOnly bool, p domain.Pagination) ([]domain.NotificationItem, int, int, error) {
	if err := requireUser(actor); err != nil {
		return nil, 0, 0, err
	}
	return s.repo.List(ctx, actor.UserID, unreadOnly, p)
}

func (s *Service) UnreadCount(ctx context.Context, actor domain.Actor) (int, error) {
	if err := requireUser(actor); err != nil {
		return 0, err
	}
	return s.repo.CountUnread(ctx, actor.UserID)
}

// MarkRead menandai baca: all=true semua milik actor, atau daftar id (uuid). id milik orang lain diabaikan diam-diam.
func (s *Service) MarkRead(ctx context.Context, actor domain.Actor, ids []string, all bool) error {
	if err := requireUser(actor); err != nil {
		return err
	}
	now := s.now()
	if all {
		_, err := s.repo.MarkAllRead(ctx, actor.UserID, now)
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("%w: isi ids atau all=true", domain.ErrInvalidInput)
	}
	if len(ids) > maxMarkIDs {
		return fmt.Errorf("%w: maksimal %d id per permintaan", domain.ErrInvalidInput, maxMarkIDs)
	}
	clean := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return fmt.Errorf("%w: id notifikasi bukan UUID", domain.ErrInvalidInput)
		}
		clean = append(clean, strings.ToLower(id))
	}
	_, err := s.repo.MarkRead(ctx, actor.UserID, clean, now)
	return err
}

// PreferenceView = satu baris pengaturan notifikasi efektif untuk pengguna.
type PreferenceView struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	InApp       bool   `json:"in_app"`
	Email       bool   `json:"email"`
	Mandatory   bool   `json:"mandatory"`
}

func (s *Service) ListPreferences(ctx context.Context, actor domain.Actor) ([]PreferenceView, error) {
	if err := requireUser(actor); err != nil {
		return nil, err
	}
	types, err := s.repo.ListTypes(ctx)
	if err != nil {
		return nil, err
	}
	saved, err := s.repo.ListPreferences(ctx, actor.UserID)
	if err != nil {
		return nil, err
	}
	byType := make(map[uint64]domain.NotificationPreference, len(saved))
	for _, p := range saved {
		byType[p.TypeID] = p
	}
	out := make([]PreferenceView, 0, len(types))
	for _, t := range types {
		inApp, email := t.DefaultInApp, t.DefaultEmail
		if p, ok := byType[t.ID]; ok {
			inApp, email = p.InApp, p.Email
		}
		if t.IsMandatory {
			inApp, email = true, true
		}
		out = append(out, PreferenceView{Type: t.Code, Name: t.Name, Description: t.Description, Severity: t.Severity,
			InApp: inApp, Email: email, Mandatory: t.IsMandatory})
	}
	return out, nil
}

type PreferenceInput struct {
	Type  string
	InApp bool
	Email bool
}

// UpdatePreferences menyimpan pilihan kanal. Seluruh permintaan ditolak (400) bila ada jenis tak dikenal atau
// ada upaya mematikan jenis wajib -- tidak ada penyimpanan sebagian.
func (s *Service) UpdatePreferences(ctx context.Context, actor domain.Actor, items []PreferenceInput) error {
	if err := requireUser(actor); err != nil {
		return err
	}
	if len(items) == 0 || len(items) > maxPrefs {
		return fmt.Errorf("%w: items 1..%d", domain.ErrInvalidInput, maxPrefs)
	}
	types, err := s.repo.ListTypes(ctx)
	if err != nil {
		return err
	}
	byCode := make(map[string]domain.NotificationType, len(types))
	for _, t := range types {
		byCode[t.Code] = t
	}
	out := make([]domain.NotificationPreference, 0, len(items))
	for _, it := range items {
		t, ok := byCode[it.Type]
		if !ok {
			return fmt.Errorf("%w: jenis notifikasi %q tidak dikenal", domain.ErrInvalidInput, truncateRunes(it.Type, 48))
		}
		if t.IsMandatory && (!it.InApp || !it.Email) {
			return fmt.Errorf("%w: jenis %s wajib dan tidak dapat dimatikan", domain.ErrInvalidInput, t.Code)
		}
		out = append(out, domain.NotificationPreference{UserID: actor.UserID, TypeID: t.ID, InApp: it.InApp, Email: it.Email})
	}
	return s.repo.UpsertPreferences(ctx, actor.UserID, out)
}

// ---- Pengumuman (SUPERADMIN) ----

type AnnounceInput struct {
	Title    string
	Body     string
	Link     string
	TenantID *uint64 // nil = seluruh platform
	Email    bool
}

func (s *Service) allowAnnounce(actorID uint64) bool {
	s.annMu.Lock()
	defer s.annMu.Unlock()
	cutoff := s.now().Add(-time.Hour)
	kept := s.ann[actorID][:0]
	for _, t := range s.ann[actorID] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= maxAnnouncePerHour {
		s.ann[actorID] = kept
		return false
	}
	s.ann[actorID] = append(kept, s.now())
	return true
}

// Announce menyiarkan pengumuman ke seluruh staf aktif (tenant tertentu atau seluruh platform) dan
// mengembalikan jumlah penerima.
func (s *Service) Announce(ctx context.Context, actor domain.Actor, in AnnounceInput) (int, error) {
	if !actor.IsSuperadmin() || actor.ImpersonatorID != 0 || actor.UserID == 0 {
		return 0, domain.ErrForbidden
	}
	in.Title, in.Body, in.Link = strings.TrimSpace(in.Title), strings.TrimSpace(in.Body), strings.TrimSpace(in.Link)
	if in.Title == "" || in.Body == "" {
		return 0, fmt.Errorf("%w: judul dan isi wajib diisi", domain.ErrInvalidInput)
	}
	if utf8.RuneCountInString(in.Title) > MaxTitleLen {
		return 0, fmt.Errorf("%w: judul maksimal %d karakter", domain.ErrInvalidInput, MaxTitleLen)
	}
	if utf8.RuneCountInString(in.Body) > MaxBodyLen {
		return 0, fmt.Errorf("%w: isi maksimal %d karakter", domain.ErrInvalidInput, MaxBodyLen)
	}
	if hasControl(in.Title, false) || hasControl(in.Body, true) {
		return 0, fmt.Errorf("%w: judul/isi tidak boleh memuat karakter kontrol", domain.ErrInvalidInput)
	}
	if in.Link != "" && !ValidLink(in.Link) {
		return 0, fmt.Errorf("%w: link harus berupa path internal diawali \"/\" (tanpa \"//\", skema, atau spasi), maksimal %d karakter", domain.ErrInvalidInput, MaxLinkLen)
	}
	if in.TenantID != nil {
		if _, err := s.tenants.GetByID(ctx, *in.TenantID); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return 0, fmt.Errorf("%w: tenant tidak ditemukan", domain.ErrInvalidInput)
			}
			return 0, err
		}
	}
	if !s.allowAnnounce(actor.UserID) {
		return 0, fmt.Errorf("%w: maksimal %d pengumuman per jam", domain.ErrQuotaExceeded, maxAnnouncePerHour)
	}
	n, err := s.dispatch(ctx, domain.NotificationEvent{
		TypeCode: domain.NotifAnnouncement, TenantID: in.TenantID,
		Audience: domain.NotificationAudience{Kind: domain.AudienceTenantAllStaff},
		Title:    in.Title, Body: in.Body, Link: in.Link, NoEmail: !in.Email,
	})
	if err != nil {
		return 0, err
	}
	if s.activity != nil {
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			UserID: actor.UserIDPtr(), TenantID: in.TenantID, Action: "ANNOUNCE_NOTIFICATION", EntityType: "notification",
			Details: map[string]any{"recipients": n, "email": in.Email, "title": truncateRunes(in.Title, 80)},
		})
	}
	return n, nil
}
