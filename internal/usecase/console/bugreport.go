package console

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg" // pendaftar decoder untuk image.DecodeConfig (validasi tangkapan layar)
	_ "image/png"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/notify"
	"acs/pkg/mailer"
)

// Batas & kebijakan laporan bug.
const (
	maxBugReportsPerHour = 10
	bugTitleMin          = 5
	bugTitleMax          = 120
	bugDescMin           = 10
	bugDescMax           = 4000
	bugExpectedMin       = 5
	bugExpectedMax       = 2000
	// Tangkapan layar: PNG/JPEG, <= 2 MiB, sisi 16..6000 px.
	maxScreenshotBytes = 2 << 20
	minScreenshotSide  = 16
	maxScreenshotSide  = 6000
	bugNoteMax           = 500
)

var (
	pagePathRe   = regexp.MustCompile(`^/[A-Za-z0-9._~!$&'()*+,;=:@%/\-]{0,254}$`)
	appVersionRe = regexp.MustCompile(`^[0-9A-Za-z._+\-]{1,32}$`)
)

var severityLabels = map[string]string{
	domain.BugSeverityMinor:   "RINGAN",
	domain.BugSeverityMajor:   "SEDANG",
	domain.BugSeverityBlocker: "BERAT",
}

// UserReader / TenantReader = bagian sempit repository yang dipakai untuk melengkapi email.
type UserReader interface {
	GetByID(ctx context.Context, id uint64) (*domain.User, error)
}

type TenantReader interface {
	GetByID(ctx context.Context, id uint64) (*domain.Tenant, error)
}

type BugService struct {
	repo     domain.BugReportRepository
	activity domain.ActivityLogRepository
	users    UserReader
	tenants  TenantReader
	alerts   domain.Notifier
	mail     mailer.Notifier
	builder  *notify.Builder
	mailOn   bool
	to       string // alamat operator penerima laporan
	log      *slog.Logger
	now      func() time.Time
}

func NewBugService(repo domain.BugReportRepository, activity domain.ActivityLogRepository, users UserReader, tenants TenantReader, recipient string, log *slog.Logger) *BugService {
	if log == nil {
		log = slog.Default()
	}
	return &BugService{repo: repo, activity: activity, users: users, tenants: tenants, to: strings.TrimSpace(recipient), log: log, now: time.Now}
}

// WithNotifier memasang notifikasi in-app untuk operator (nil-safe).
func (s *BugService) WithNotifier(n domain.Notifier) *BugService { s.alerts = n; return s }

// WithEmail mengaktifkan email ke operator. enabled=false (SMTP mati) = laporan tetap tersimpan di database.
func (s *BugService) WithEmail(m mailer.Notifier, b *notify.Builder, enabled bool) *BugService {
	s.mail, s.builder, s.mailOn = m, b, enabled
	return s
}

// CreateBugInput = isi form + konteks yang dikumpulkan Console. Semua dianggap tidak tepercaya.
type CreateBugInput struct {
	Title       string
	Description string // apa yang terjadi
	Expected    string // apa yang diharapkan (kolom terpisah)
	Severity    string
	PagePath    string
	UserAgent   string
	AppVersion  string
	// Screenshot = isi berkas gambar mentah (opsional). Jenis ditentukan SERVER dari isinya, bukan dari klien.
	Screenshot []byte
}

// validateScreenshot memastikan isi benar-benar PNG/JPEG yang dapat didecode (bukan sekadar nama/ekstensi), ukuran dan
// dimensi wajar, lalu menghitung sha256. Jenis konten dari klien tidak dipercaya.
func validateScreenshot(b []byte) (*domain.BugScreenshot, error) {
	if len(b) == 0 || len(b) > maxScreenshotBytes {
		return nil, invalid("tangkapan layar maksimal %d MB", maxScreenshotBytes>>20)
	}
	var ctype string
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		ctype = "image/png"
	case bytes.HasPrefix(b, []byte("\xff\xd8\xff")):
		ctype = "image/jpeg"
	default:
		return nil, invalid("tangkapan layar harus berupa gambar PNG atau JPEG")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || "image/"+format != ctype {
		return nil, invalid("berkas tangkapan layar rusak atau bukan gambar yang valid")
	}
	if cfg.Width < minScreenshotSide || cfg.Height < minScreenshotSide || cfg.Width > maxScreenshotSide || cfg.Height > maxScreenshotSide {
		return nil, invalid("dimensi tangkapan layar harus %d-%d piksel per sisi", minScreenshotSide, maxScreenshotSide)
	}
	sum := sha256.Sum256(b)
	return &domain.BugScreenshot{ContentType: ctype, Width: cfg.Width, Height: cfg.Height, SHA256: hex.EncodeToString(sum[:]), Data: b}, nil
}

func isStaffOrEndUser(a domain.Actor) bool {
	for _, r := range []string{domain.RoleSuperadmin, domain.RoleAdmin, domain.RoleNOC, domain.RoleViewer, domain.RoleEndUser} {
		if a.HasRole(r) {
			return true
		}
	}
	return false
}

func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// Create menyimpan laporan atas nama actor (tenant & pelapor dari token, TIDAK dari body), lalu mengirim email ke
// operator dan notifikasi in-app ke SUPERADMIN. Kegagalan email/notifikasi tidak menggagalkan laporan.
func (s *BugService) Create(ctx context.Context, actor domain.Actor, in CreateBugInput) (*domain.BugReport, error) {
	if actor.UserID == 0 || !isStaffOrEndUser(actor) {
		return nil, domain.ErrForbidden
	}
	title, err := domain.CleanText("judul", in.Title, bugTitleMin, bugTitleMax, false)
	if err != nil {
		return nil, err
	}
	desc, err := domain.CleanText("apa yang terjadi", in.Description, bugDescMin, bugDescMax, true)
	if err != nil {
		return nil, err
	}
	expected, err := domain.CleanText("apa yang diharapkan", in.Expected, bugExpectedMin, bugExpectedMax, true)
	if err != nil {
		return nil, err
	}
	var shot *domain.BugScreenshot
	if len(in.Screenshot) > 0 {
		if shot, err = validateScreenshot(in.Screenshot); err != nil {
			return nil, err
		}
	}
	sev := strings.ToUpper(strings.TrimSpace(in.Severity))
	if sev == "" {
		sev = domain.BugSeverityMinor
	}
	if _, ok := severityLabels[sev]; !ok {
		return nil, invalid("dampak harus MINOR, MAJOR, atau BLOCKER")
	}
	// Konteks (path/UA/versi) hanya informasi: yang tidak sah dikosongkan, bukan menolak laporan.
	path := in.PagePath
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	if !pagePathRe.MatchString(path) {
		path = ""
	}
	ua := stripControl(strings.TrimSpace(in.UserAgent))
	if r := []rune(ua); len(r) > 255 {
		ua = string(r[:255])
	}
	ver := strings.TrimSpace(in.AppVersion)
	if !appVersionRe.MatchString(ver) {
		ver = ""
	}

	// Batas laju per pelapor: laporan bukan saluran spam (email ke operator).
	n, err := s.repo.CountByReporterSince(ctx, actor.UserID, s.now().Add(-time.Hour))
	if err != nil {
		return nil, err
	}
	if n >= maxBugReportsPerHour {
		return nil, fmt.Errorf("%w: terlalu banyak laporan dalam satu jam, coba lagi nanti", domain.ErrQuotaExceeded)
	}

	var imp *uint64
	if actor.ImpersonatorID != 0 {
		v := actor.ImpersonatorID
		imp = &v
	}
	rep, err := s.repo.Create(ctx, domain.BugReportCreate{
		UUID: uuid.NewString(), TenantID: actor.TenantID, ReporterID: actor.UserID, ImpersonatorID: imp,
		Title: title, Description: desc, Expected: expected, Screenshot: shot, PagePath: path, UserAgent: ua, AppVersion: ver, SeverityCode: sev,
	})
	if err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "BUG_REPORT_CREATED", EntityType: "bug_report", EntityID: &rep.ID})

	s.emailOperator(ctx, actor, rep, shot)
	if s.alerts != nil {
		_ = s.alerts.Notify(ctx, domain.NotificationEvent{
			TypeCode: domain.NotifBugReport, TenantID: actor.TenantID,
			Audience: domain.NotificationAudience{Kind: domain.AudienceSuperadmins},
			Title:    "Laporan bug baru: " + truncate(title, 100),
			Body:     fmt.Sprintf("Dari %s (%s). Dampak: %s.", rep.ReporterName, truncate(rep.PagePath, 60), severityLabels[sev]),
			Link:     "/bug-reports", EntityType: "bug_report", EntityID: &rep.ID, NoEmail: true,
		})
	}
	rep.ReporterEmail = "" // pelapor tidak perlu mendapat kembali emailnya sendiri
	return rep, nil
}

func (s *BugService) emailOperator(ctx context.Context, actor domain.Actor, rep *domain.BugReport, shot *domain.BugScreenshot) {
	if !s.mailOn || s.mail == nil || s.builder == nil || s.to == "" {
		return
	}
	tenant := "(platform)"
	if rep.TenantName != nil {
		tenant = *rep.TenantName
	}
	role := "-"
	if len(actor.Roles) > 0 {
		role = actor.Roles[0]
	}
	msg := s.builder.BugReport(s.to, severityLabels[rep.Severity], rep.Title, rep.Description, rep.Expected, rep.ReporterName,
		rep.ReporterEmail, tenant, role, rep.PagePath, rep.AppVersion, rep.UserAgent, rep.UUID, rep.Impersonated, shot != nil)
	if shot != nil {
		ext := ".png"
		if shot.ContentType == "image/jpeg" {
			ext = ".jpg"
		}
		msg.Attachments = []mailer.Attachment{{Filename: "bug-" + rep.UUID[:8] + ext, ContentType: shot.ContentType, Data: shot.Data}}
	}
	s.mail.Notify(msg)
	if err := s.repo.MarkEmailed(ctx, rep.ID); err != nil {
		s.log.Warn("bugreport: gagal menandai emailed_at", "error", err)
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// List: SUPERADMIN melihat semua (filter tenant/status opsional); selain itu HANYA laporan milik sendiri — filter
// tenant dari klien diabaikan sepenuhnya.
func (s *BugService) List(ctx context.Context, actor domain.Actor, tenantID *uint64, status string, p domain.Pagination) ([]domain.BugReport, int, error) {
	if actor.UserID == 0 || !isStaffOrEndUser(actor) {
		return nil, 0, domain.ErrForbidden
	}
	f := domain.BugReportFilter{}
	if status != "" {
		switch status {
		case domain.BugStatusOpen, domain.BugStatusInProgress, domain.BugStatusResolved, domain.BugStatusWontFix:
			f.Status = status
		default:
			return nil, 0, invalid("status tidak dikenal")
		}
	}
	if actor.IsSuperadmin() {
		f.TenantID = tenantID
	} else {
		own := actor.UserID
		f.ReporterID = &own
	}
	rows, total, err := s.repo.List(ctx, f, p)
	if err != nil {
		return nil, 0, err
	}
	if !actor.IsSuperadmin() {
		for i := range rows {
			rows[i].ReporterEmail = ""
		}
	}
	return rows, total, nil
}

// Get: SUPERADMIN atau pelapor sendiri; laporan orang lain dijawab ErrNotFound (bukan Forbidden) agar keberadaan
// uuid tidak bisa diprobing.
func (s *BugService) Get(ctx context.Context, actor domain.Actor, id string) (*domain.BugReport, error) {
	if actor.UserID == 0 || !isStaffOrEndUser(actor) {
		return nil, domain.ErrForbidden
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, domain.ErrNotFound
	}
	rep, err := s.repo.GetByUUID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !actor.IsSuperadmin() {
		if rep.ReporterID != actor.UserID {
			return nil, domain.ErrNotFound
		}
		rep.ReporterEmail = ""
	}
	return rep, nil
}

// UpdateStatus: hanya SUPERADMIN (sesi impersonate = ADMIN, jadi otomatis ditolak).
func (s *BugService) UpdateStatus(ctx context.Context, actor domain.Actor, id, status string, note *string) (*domain.BugReport, error) {
	if !actor.IsSuperadmin() {
		return nil, domain.ErrForbidden
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, domain.ErrNotFound
	}
	switch status {
	case domain.BugStatusOpen, domain.BugStatusInProgress, domain.BugStatusResolved, domain.BugStatusWontFix:
	default:
		return nil, invalid("status harus OPEN, IN_PROGRESS, RESOLVED, atau WONTFIX")
	}
	var cleaned *string
	if note != nil {
		v, err := domain.CleanText("catatan", *note, 0, bugNoteMax, true)
		if err != nil {
			return nil, err
		}
		if v != "" {
			cleaned = &v
		}
	}
	before, err := s.repo.GetByUUID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateStatus(ctx, id, status, cleaned, actor.UserID); err != nil {
		return nil, err
	}
	rep, err := s.repo.GetByUUID(ctx, id)
	if err != nil {
		return nil, err
	}
	if before.Status != rep.Status {
		s.notifyReporter(ctx, rep)
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), TenantID: rep.TenantID, Action: "BUG_REPORT_STATUS_" + status, EntityType: "bug_report", EntityID: &rep.ID})
	return rep, nil
}

// statusLabels = teks status untuk pelapor (RESOLVED = "sudah ditangani").
var statusLabels = map[string]string{
	domain.BugStatusOpen:       "Baru diterima",
	domain.BugStatusInProgress: "Sedang ditangani",
	domain.BugStatusResolved:   "Sudah ditangani",
	domain.BugStatusWontFix:    "Tidak akan diperbaiki",
}

// notifyReporter memberi tahu PELAPOR (in-app + email) bahwa status laporannya berubah. Dilewati bila laporan dibuat
// dalam sesi impersonate (pelapor sebenarnya SUPERADMIN, bukan pengguna tenant). Kegagalan tidak menggagalkan update.
func (s *BugService) notifyReporter(ctx context.Context, rep *domain.BugReport) {
	if rep.Impersonated {
		return
	}
	label := statusLabels[rep.Status]
	note := ""
	if rep.AdminNote != nil {
		note = *rep.AdminNote
	}
	title := "Status laporan bug Anda: " + label
	if rep.Status == domain.BugStatusResolved {
		title = "Laporan bug Anda sudah ditangani"
	}
	if s.alerts != nil {
		body := fmt.Sprintf("\"%s\" — %s.", truncate(rep.Title, 100), label)
		if note != "" {
			body += " Catatan: " + truncate(note, 300)
		}
		_ = s.alerts.Notify(ctx, domain.NotificationEvent{
			TypeCode: domain.NotifBugReportUpdate, TenantID: rep.TenantID,
			Audience: domain.NotificationAudience{Kind: domain.AudienceSelf, UserID: rep.ReporterID},
			Title:    title, Body: body, Link: "/bug-reports", EntityType: "bug_report", EntityID: &rep.ID, NoEmail: true,
		})
	}
	if !s.mailOn || s.mail == nil || s.builder == nil {
		return
	}
	u, err := s.users.GetByID(ctx, rep.ReporterID)
	if err != nil || u == nil || u.Email == "" {
		return
	}
	s.mail.Notify(s.builder.BugReportUpdated(u.Email, u.FullName, rep.Title, label, note, rep.UUID))
}

// GetScreenshot mengembalikan tangkapan layar laporan. Akses = akses ke laporannya (pelapor sendiri atau SUPERADMIN);
// laporan orang lain atau yang tanpa gambar dijawab ErrNotFound.
func (s *BugService) GetScreenshot(ctx context.Context, actor domain.Actor, id string) (*domain.BugScreenshot, error) {
	rep, err := s.Get(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if !rep.HasScreenshot {
		return nil, domain.ErrNotFound
	}
	return s.repo.GetScreenshot(ctx, rep.ID)
}
