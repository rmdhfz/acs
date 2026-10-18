package console

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/notify"
	"acs/pkg/mailer"
)

const (
	maxTransferRows       = 5_000_000
	maxTransferEmailsHour = 20 // batas email notifikasi per pengguna per jam (proses tetap tercatat)
)

var (
	transferModuleRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,47}$`)
	transferFormats  = map[string]bool{"csv": true, "xlsx": true, "json": true, "pdf": true, "txt": true}
)

var transferKindLabels = map[string]string{domain.TransferKindImport: "Impor", domain.TransferKindExport: "Ekspor"}
var transferStatusLabels = map[string]string{
	domain.TransferStatusCompleted: "Selesai",
	domain.TransferStatusPartial:   "Sebagian berhasil",
	domain.TransferStatusFailed:    "Gagal",
}

// moduleLabels = nama tampilan modul untuk email (modul tak dikenal tampil apa adanya).
var moduleLabels = map[string]string{
	"devices": "Perangkat", "tenants": "Tenant", "users": "Pengguna", "tags": "Tag", "presets": "Preset",
	"provisioning_profiles": "Profil provisioning", "zero_touch_rules": "Aturan ZTP", "vendors": "Vendor",
	"device_models": "Model perangkat", "vendor_ouis": "OUI vendor", "vendor_mappings": "Mapping parameter",
	"webhooks": "Webhook", "firmware": "Firmware", "files": "File", "tasks": "Task", "audit": "Audit trail",
	"saas_report": "Laporan SaaS", "device_parameters": "Parameter perangkat", "notifications": "Notifikasi",
}

type TransferService struct {
	repo     domain.DataTransferRepository
	activity domain.ActivityLogRepository
	users    UserReader
	mail     mailer.Notifier
	builder  *notify.Builder
	mailOn   bool
	log      *slog.Logger
	now      func() time.Time
}

func NewTransferService(repo domain.DataTransferRepository, activity domain.ActivityLogRepository, users UserReader, log *slog.Logger) *TransferService {
	if log == nil {
		log = slog.Default()
	}
	return &TransferService{repo: repo, activity: activity, users: users, log: log, now: time.Now}
}

// WithEmail mengaktifkan email ke pelaku. enabled=false (SMTP mati) = hanya riwayat di database.
func (s *TransferService) WithEmail(m mailer.Notifier, b *notify.Builder, enabled bool) *TransferService {
	s.mail, s.builder, s.mailOn = m, b, enabled
	return s
}

// RecordInput = ringkasan satu proses transfer. Isi baris TIDAK pernah dikirim/disimpan.
type RecordInput struct {
	Kind        string
	Status      string
	Module      string
	Format      string
	FileName    string
	TotalRows   int64
	SuccessRows int64
	FailedRows  int64
	Detail      string
}

func isStaff(a domain.Actor) bool {
	for _, r := range []string{domain.RoleSuperadmin, domain.RoleAdmin, domain.RoleNOC, domain.RoleViewer} {
		if a.HasRole(r) {
			return true
		}
	}
	return false
}

// RecordClient mencatat proses yang dijalankan di Console (impor per-baris, ekspor di browser). Angka berasal dari
// klien dan hanya divalidasi kewajarannya; tenant & pelaku SELALU dari token.
func (s *TransferService) RecordClient(ctx context.Context, actor domain.Actor, in RecordInput) (*domain.DataTransferLog, error) {
	return s.record(ctx, actor, in, domain.TransferSourceClient)
}

// RecordServer mencatat proses yang diproses server sendiri (mis. impor perangkat massal, ekspor laporan).
func (s *TransferService) RecordServer(ctx context.Context, actor domain.Actor, in RecordInput) (*domain.DataTransferLog, error) {
	return s.record(ctx, actor, in, domain.TransferSourceServer)
}

func (s *TransferService) record(ctx context.Context, actor domain.Actor, in RecordInput, source string) (*domain.DataTransferLog, error) {
	if actor.UserID == 0 || !isStaff(actor) {
		return nil, domain.ErrForbidden
	}
	if _, ok := transferKindLabels[in.Kind]; !ok {
		return nil, invalid("kind harus IMPORT atau EXPORT")
	}
	if _, ok := transferStatusLabels[in.Status]; !ok {
		return nil, invalid("status harus COMPLETED, PARTIAL, atau FAILED")
	}
	if !transferModuleRe.MatchString(in.Module) {
		return nil, invalid("module tidak valid")
	}
	format := strings.ToLower(strings.TrimSpace(in.Format))
	if format == "" {
		format = "csv"
	}
	if !transferFormats[format] {
		return nil, invalid("format harus salah satu dari csv, xlsx, json, pdf, txt")
	}
	for name, v := range map[string]int64{"total_rows": in.TotalRows, "success_rows": in.SuccessRows, "failed_rows": in.FailedRows} {
		if v < 0 || v > maxTransferRows {
			return nil, invalid("%s harus 0-%d", name, maxTransferRows)
		}
	}
	if in.SuccessRows+in.FailedRows > in.TotalRows {
		return nil, invalid("success_rows + failed_rows tidak boleh melebihi total_rows")
	}
	switch in.Status {
	case domain.TransferStatusCompleted:
		if in.FailedRows != 0 {
			return nil, invalid("status COMPLETED tidak boleh memiliki baris gagal")
		}
	case domain.TransferStatusPartial:
		if in.SuccessRows == 0 || in.FailedRows == 0 {
			return nil, invalid("status PARTIAL butuh baris berhasil dan gagal")
		}
	case domain.TransferStatusFailed:
		if in.SuccessRows != 0 {
			return nil, invalid("status FAILED tidak boleh memiliki baris berhasil")
		}
	}
	var fileName *string
	if in.FileName != "" {
		base := path.Base(strings.ReplaceAll(in.FileName, `\`, "/"))
		fn, err := domain.CleanText("file_name", base, 1, 255, false)
		if err != nil {
			return nil, err
		}
		fileName = &fn
	}
	var detail *string
	if d, err := domain.CleanText("detail", in.Detail, 0, 500, false); err != nil {
		return nil, err
	} else if d != "" {
		detail = &d
	}
	var imp *uint64
	if actor.ImpersonatorID != 0 {
		v := actor.ImpersonatorID
		imp = &v
	}
	rec, err := s.repo.Create(ctx, domain.DataTransferCreate{
		UUID: uuid.NewString(), TenantID: actor.TenantID, UserID: actor.UserID, ImpersonatorID: imp,
		KindCode: in.Kind, StatusCode: in.Status, Module: in.Module, Format: format, FileName: fileName,
		TotalRows: uint32(in.TotalRows), SuccessRows: uint32(in.SuccessRows), FailedRows: uint32(in.FailedRows),
		Detail: detail, Source: source,
	})
	if err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "DATA_" + in.Kind, EntityType: "data_transfer", EntityID: &rec.ID})
	s.emailActor(ctx, actor, rec)
	return rec, nil
}

// emailActor mengirim ringkasan ke email pelaku. Dilewati: SMTP mati, sesi impersonate (jangan mengirim email
// atas nama tenant untuk tindakan dukungan), atau batas email per jam tercapai (proses tetap tercatat).
func (s *TransferService) emailActor(ctx context.Context, actor domain.Actor, rec *domain.DataTransferLog) {
	if !s.mailOn || s.mail == nil || s.builder == nil || actor.ImpersonatorID != 0 {
		return
	}
	if n, err := s.repo.CountEmailedByUserSince(ctx, actor.UserID, s.now().Add(-time.Hour)); err != nil || n >= maxTransferEmailsHour {
		return
	}
	u, err := s.users.GetByID(ctx, actor.UserID)
	if err != nil || u == nil || u.Email == "" {
		return
	}
	file, detail := "", ""
	if rec.FileName != nil {
		file = *rec.FileName
	}
	if rec.Detail != nil {
		detail = *rec.Detail
	}
	mod := moduleLabels[rec.Module]
	if mod == "" {
		mod = rec.Module
	}
	s.mail.Notify(s.builder.DataTransfer(u.Email, u.FullName, transferKindLabels[rec.Kind], mod, transferStatusLabels[rec.Status],
		rec.TotalRows, rec.SuccessRows, rec.FailedRows, file, detail, rec.CreatedAt))
	if err := s.repo.MarkEmailed(ctx, rec.ID); err != nil {
		s.log.Warn("transfer: gagal menandai emailed_at", "error", err)
	}
}

// scope menentukan filter tenant yang BOLEH dipakai actor: non-SUPERADMIN selalu dikunci ke tenant sendiri (nilai
// dari klien diabaikan); SUPERADMIN bebas (nil = semua tenant).
func scope(actor domain.Actor, requested *uint64) (*uint64, error) {
	if !isStaff(actor) {
		return nil, domain.ErrForbidden
	}
	if actor.IsSuperadmin() {
		return requested, nil
	}
	if actor.TenantID == nil {
		return nil, domain.ErrForbidden
	}
	t := *actor.TenantID
	return &t, nil
}

func (s *TransferService) List(ctx context.Context, actor domain.Actor, tenantID *uint64, kind, module string, p domain.Pagination) ([]domain.DataTransferLog, int, error) {
	tid, err := scope(actor, tenantID)
	if err != nil {
		return nil, 0, err
	}
	f := domain.DataTransferFilter{TenantID: tid}
	if kind != "" {
		if _, ok := transferKindLabels[kind]; !ok {
			return nil, 0, invalid("kind harus IMPORT atau EXPORT")
		}
		f.Kind = kind
	}
	if module != "" {
		if !transferModuleRe.MatchString(module) {
			return nil, 0, invalid("module tidak valid")
		}
		f.Module = module
	}
	return s.repo.List(ctx, f, p)
}

// Get: baris tenant lain dijawab ErrNotFound (bukan Forbidden) — keberadaan uuid tidak bocor lintas tenant.
func (s *TransferService) Get(ctx context.Context, actor domain.Actor, id string) (*domain.DataTransferLog, error) {
	tid, err := scope(actor, nil)
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, domain.ErrNotFound
	}
	rec, err := s.repo.GetByUUID(ctx, id)
	if err != nil {
		return nil, err
	}
	if tid != nil && (rec.TenantID == nil || *rec.TenantID != *tid) {
		return nil, fmt.Errorf("%w", domain.ErrNotFound)
	}
	return rec, nil
}
