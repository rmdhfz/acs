// Package session mengorkestrasi siklus hidup sesi CWMP (TECH.md §3) —
// Inform -> InformResponse, lalu pertukaran RPC/task selama sesi terbuka.
//
// Tipe input/output di sini SENGAJA berupa struct Go polos, bukan tipe
// pkg/cwmpxml — parsing XML/SOAP adalah tanggung jawab
// internal/delivery/cwmp (lihat komentar di handler.go), agar usecase tidak
// bergantung pada detail wire format CWMP (Clean Architecture, CLAUDE.md).
package session

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/device"
	"acs/internal/usecase/diagnostics"
	"acs/internal/usecase/firmware"
	"acs/internal/usecase/provisioning"
	"acs/internal/usecase/task"
	"acs/pkg/cryptoutil"
)

type Service struct {
	// opticalPollEvery — interval polling redaman optik (0 = nonaktif); opticalSkip = cache
	// negatif (vendor/model tanpa mapping optical.*) agar tidak menambah query tiap Inform.
	opticalPollEvery time.Duration
	opticalSkip      sync.Map // "vendor:model" -> time.Time kedaluwarsa cache
	// notifier — notifikasi in-app OPTICAL_ALERT (opsional, WithNotifier); nil = hanya activity log & webhook.
	notifier domain.Notifier
	// faults — resource faults (opsional, WithFaults); diakses lewat guard nil.
	faults FaultRecorder
	// automation — aturan otomasi kondisi-aksi (opsional, WithAutomation); diakses lewat guard nil.
	automation AutomationEvaluator
	// pppoePollEvery — interval penyegaran username PPPoE (0 = nonaktif); pppoeNext = device id -> waktu poll berikutnya
	// (di memori: setelah restart tiap device disegarkan sekali pada Inform pertamanya, lalu berkala).
	pppoePollEvery time.Duration
	pppoeNext      sync.Map
	// typeIDs: cache kode tipe XSD -> id ref_parameter_types (lihat paramTypeID).
	typeIDs         sync.Map
	sessions        domain.DeviceSessionRepository
	events          domain.DeviceEventRepository
	deviceParams    domain.DeviceParameterRepository
	devices         domain.DeviceRepository
	tenants         domain.TenantRepository
	refs            domain.RefRepository
	deviceSvc       *device.Service
	taskSvc         *task.Service
	provisioningSvc *provisioning.Service
	firmwareSvc     *firmware.Service
	diagnosticsSvc  *diagnostics.Service
	enc             *cryptoutil.Encryptor
	// activity — jejak audit anomali yang terdeteksi di jalur Inform
	// (DNS hijack, reboot loop, redaman optik kritis). Boleh nil (test yang
	// membangun Service via literal) — SELALU diakses lewat guard nil.
	activity domain.ActivityLogRepository
	// logger — structured logging (log/slog) sepanjang jalur sesi CWMP
	// (TECH.md §10). Boleh nil (mis. test yang membangun Service via literal
	// struct langsung, lihat service_test.go) -- SELALU diakses lewat method
	// log() di bawah, bukan field ini langsung, supaya nil-safe (fallback
	// slog.Default()).
	logger *slog.Logger
	// webhookEnq — fan-out event webhook keluar (migrations/0013). Boleh nil
	// (test / konfigurasi tanpa webhook) -- SELALU lewat notifyWebhook() yang
	// nil-safe & tidak pernah menggagalkan alur sesi.
	webhookEnq domain.WebhookEnqueuer
	publisher  domain.EventPublisher
}

func NewService(
	sessions domain.DeviceSessionRepository,
	events domain.DeviceEventRepository,
	deviceParams domain.DeviceParameterRepository,
	devices domain.DeviceRepository,
	tenants domain.TenantRepository,
	refs domain.RefRepository,
	deviceSvc *device.Service,
	taskSvc *task.Service,
	provisioningSvc *provisioning.Service,
	firmwareSvc *firmware.Service,
	diagnosticsSvc *diagnostics.Service,
	enc *cryptoutil.Encryptor,
	activity domain.ActivityLogRepository,
	logger *slog.Logger,
	webhookEnq domain.WebhookEnqueuer,
	publisher domain.EventPublisher,
) *Service {
	return &Service{
		sessions: sessions, events: events, deviceParams: deviceParams, devices: devices, tenants: tenants, refs: refs,
		deviceSvc: deviceSvc, taskSvc: taskSvc, provisioningSvc: provisioningSvc,
		firmwareSvc: firmwareSvc, diagnosticsSvc: diagnosticsSvc, enc: enc, activity: activity, logger: logger, webhookEnq: webhookEnq, publisher: publisher,
	}
}

// RedactToken menyingkat token sesi untuk log. Token sesi (cookie acs_session)
// adalah satu-satunya bukti autentikasi untuk POST lanjutan dalam sesi CWMP;
// siapa pun yang bisa membaca log selama sesi terbuka bisa menyuntik respons
// RPC atau menarik task bila token tampil utuh. 8 karakter cukup untuk
// mengorelasikan baris log.
func RedactToken(t string) string {
	if len(t) <= 8 {
		return "***"
	}
	return t[:8] + "…"
}

// notifyWebhook mem-fan-out event webhook TANPA menggagalkan alur sesi CWMP
// (error di-log saja) dan nil-safe bila webhook tidak dikonfigurasi.
func (s *Service) notifyWebhook(ctx context.Context, eventCode string, tenantID *uint64, payload any) {
	if s.webhookEnq == nil {
		return
	}
	if err := s.webhookEnq.Enqueue(ctx, eventCode, tenantID, payload); err != nil {
		s.log().Warn("cwmp: gagal enqueue webhook", "event", eventCode, "error", err)
	}
}

// log mengembalikan logger yang aman dipakai (fallback slog.Default() bila
// s.logger nil — lihat komentar field logger di atas).
func (s *Service) log() *slog.Logger {
	if s.logger == nil {
		return slog.Default()
	}
	return s.logger
}

// CountOpenSessions — jumlah sesi CWMP berstatus OPEN saat ini (metrik
// observability TECH.md §10, dipakai internal/metrics.Collector). Lintas
// seluruh tenant — device_sessions tidak menyimpan tenant_id langsung dan
// metrik ini utk operator platform, bukan dashboard tenant.
func (s *Service) CountOpenSessions(ctx context.Context) (int, error) {
	return s.sessions.CountOpen(ctx)
}

// CountOpenSessionsFor — sesi terbuka sesuai cakupan actor: SUPERADMIN melihat seluruh platform, selain itu
// hanya perangkat tenantnya sendiri (actor tanpa tenant dan bukan superadmin ditolak).
func (s *Service) CountOpenSessionsFor(ctx context.Context, actor domain.Actor) (int, error) {
	if actor.IsSuperadmin() {
		return s.sessions.CountOpen(ctx)
	}
	if actor.TenantID == nil {
		return 0, domain.ErrForbidden
	}
	return s.sessions.CountOpenForTenant(ctx, *actor.TenantID)
}

// TimeoutStaleSessions men-reap sesi CWMP berstatus OPEN yang usianya (dari
// started_at) sudah melewati threshold — yaitu sesi yang CPE-nya tidak pernah
// mengirim POST-kosong penutup (koneksi putus, CPE reboot mendadak, dsb).
// Tanpa ini baris device_sessions.status='OPEN' menumpuk permanen dan metrik
// acs_cwmp_sessions_open (dashboard "sesi aktif") jadi tidak berarti.
// Dipanggil periodik dari runSweepers di cmd/acsd — aman dari instance
// manapun (app server stateless, TECH.md §9). Mengembalikan jumlah sesi yang
// di-reap. Threshold harus jauh di atas durasi sesi CWMP normal (detik s/d
// beberapa menit) supaya tidak pernah menutup sesi yang masih hidup.
func (s *Service) TimeoutStaleSessions(ctx context.Context, threshold time.Duration) (int, error) {
	n, err := s.sessions.TimeoutStaleOpen(ctx, time.Now().Add(-threshold))
	if err != nil {
		return 0, err
	}
	if n > 0 {
		s.log().Info("reap sesi CWMP basi", "count", n, "threshold", threshold.String())
	}
	return int(n), nil
}

// ---- Inform ----

type InformEvent struct {
	EventCode  string
	CommandKey string
}

type InformParameter struct {
	Name  string
	Value string
	Type  string // xsi:type dari CPE (mis. "xsd:unsignedInt"); dipakai agar Set memakai tipe benar
}

type InformInput struct {
	SessionToken    string // dari cookie sesi, kosong bila belum ada sesi
	RemoteIP        string
	DeviceOUI       string
	SerialNumber    string
	ProductClass    string
	SoftwareVersion string
	HardwareVersion string
	// InformUsername/Password dari header Authorization (Basic Auth) request
	// CWMP — divalidasi authenticateInform sebelum device diproses sama
	// sekali (CLAUDE.md: kredensial CWMP wajib divalidasi, TECH.md §3/§8).
	InformUsername string
	InformPassword string
	// ConnectionRequestURL dari parameter Inform standar
	// ManagementServer.ConnectionRequestURL — di-capture ke devices supaya
	// operator bisa memicu Connection Request tanpa konfigurasi manual
	// (penting untuk skala ribuan CPE). Kosong = tidak dilaporkan Inform ini.
	ConnectionRequestURL string
	Events               []InformEvent
	Parameters           []InformParameter
	// Namespace -- namespace CWMP yang dideklarasikan CPE pada Inform ini
	// (mis. "urn:dslforum-org:cwmp-1-0"/"cwmp-1-2"), disimpan ke sesi
	// (migrations/0012) supaya RPC proaktif berikutnya dalam sesi yang sama
	// memakai namespace yang sama, bukan default hardcode -- lihat komentar
	// lengkap di migrations/0012.
	Namespace string
}

type InformResult struct {
	SessionToken string
	DeviceID     uint64
}

func (s *Service) HandleInform(ctx context.Context, in InformInput) (*InformResult, error) {
	auth, err := s.authenticateInform(ctx, in.DeviceOUI, in.SerialNumber, in.InformUsername, in.InformPassword)
	if err != nil {
		return nil, err
	}

	dev, _, err := s.deviceSvc.FindOrCreateFromInform(ctx, device.InformDeviceInfo{
		OUI:                  in.DeviceOUI,
		SerialNumber:         in.SerialNumber,
		ProductClass:         in.ProductClass,
		SoftwareVersion:      in.SoftwareVersion,
		HardwareVersion:      in.HardwareVersion,
		RemoteIP:             in.RemoteIP,
		ConnectionRequestURL: in.ConnectionRequestURL,
		TenantID:             auth.TenantID,
		ExistingDevice:       auth.Device,
	})
	if err != nil {
		return nil, fmt.Errorf("session: gagal upsert device dari Inform: %w", err)
	}

	sess, err := s.resolveSession(ctx, in.SessionToken, dev.ID, in.RemoteIP, in.Namespace)
	if err != nil {
		return nil, err
	}
	// Simpan namespace CWMP sesi ini supaya NextRequest bisa memakainya utk RPC
	// proaktif berikutnya (lihat migrations/0012). Sesi baru sudah membawa
	// namespace-nya sejak INSERT (resolveSession); UPDATE (+ invalidasi cache
	// Redis) hanya terjadi bila Inform lanjutan di sesi yang sama mengubahnya.
	if in.Namespace != "" && (sess.CWMPNamespace == nil || *sess.CWMPNamespace != in.Namespace) {
		if err := s.sessions.SetCWMPNamespace(ctx, sess.ID, in.Namespace); err != nil {
			return nil, fmt.Errorf("session: gagal menyimpan namespace CWMP: %w", err)
		}
		sess.CWMPNamespace = &in.Namespace
	}

	// Aktor sistem (bukan user login) untuk aksi otomatis seperti evaluasi ZTP.
	systemActor := domain.Actor{TenantID: dev.TenantID}

	var hasBootstrap, hasBoot, hasValueChange bool
	for _, ev := range in.Events {
		code, err := s.refs.GetByCode(ctx, domain.RefTableEventCodes, ev.EventCode)
		if err != nil {
			// event code tidak dikenal (kuirk vendor) - jangan gagalkan seluruh
			// Inform, tapi tetap dicatat (Debug) supaya bisa dilacak kalau
			// device tertentu ternyata sering mengirim event code non-standar.
			s.log().Debug("cwmp: event code tidak dikenal, diabaikan", "device_id", dev.ID, "event_code", ev.EventCode)
			continue
		}
		occurredAt := time.Now()
		var cmdKey *string
		if ev.CommandKey != "" {
			cmdKey = &ev.CommandKey
		}
		_ = s.events.Create(ctx, &domain.DeviceEvent{
			DeviceID: dev.ID, SessionID: &sess.ID, EventCodeID: code.ID, CommandKey: cmdKey, OccurredAt: occurredAt,
		})

		switch ev.EventCode {
		case domain.EventCodeBootstrap:
			hasBootstrap = true
		case domain.EventCodeBoot:
			hasBoot = true
			prevBootAt := dev.LastBootEventAt
			_ = s.deviceSvc.TouchLastBootEvent(ctx, dev, occurredAt)
			s.detectAnomalyBoot(ctx, dev, prevBootAt, occurredAt)
		case domain.EventCodeDiagnosticsComplete:
			// CPE selesai menjalankan diagnostik (Ping/Traceroute): ambil hasilnya.
			if s.diagnosticsSvc != nil {
				if err := s.diagnosticsSvc.OnDiagnosticsComplete(ctx, dev.ID); err != nil {
					s.log().Warn("cwmp: gagal mengambil hasil diagnostik", "device_id", dev.ID, "error", err)
				}
			}
		case domain.EventCodeValueChange:
			hasValueChange = true
		}
	}

	if len(in.Parameters) > 0 {
		// Anomaly Detection (ROADMAP.md Fase 5): Cek perubahan DNS sebelum di-upsert
		s.detectAnomalyDNS(ctx, dev, in.Parameters)
		s.detectAnomalyOptical(ctx, dev, in.Parameters)

		params := make([]domain.DeviceParameter, 0, len(in.Parameters))
		for _, p := range in.Parameters {
			val := p.Value
			params = append(params, domain.DeviceParameter{DeviceID: dev.ID, ParameterName: p.Name, ParameterValue: &val, ParameterTypeID: s.paramTypeID(ctx, p.Type)})
		}
		_ = s.deviceParams.UpsertBatch(ctx, params)
	}

	// Webhook PARAMETER_VALUE_CHANGE (migrations/0013) — CPE mengirim event
	// "4 VALUE CHANGE" berarti satu/lebih parameter berubah di sisi CPE.
	// Payload memuat parameter yang ikut dilaporkan pada Inform ini (bila ada).
	if hasValueChange {
		changed := make(map[string]string, len(in.Parameters))
		for _, p := range in.Parameters {
			changed[p.Name] = p.Value
		}
		s.notifyWebhook(ctx, domain.WebhookEventParameterValueChange, dev.TenantID, map[string]any{
			"device_id":     dev.ID,
			"serial_number": dev.SerialNumber,
			"parameters":    changed,
		})
	}

	// Evaluasi ZTP SETELAH device_parameters di-upsert (bukan di dalam loop
	// event di atas) -- supaya precondition MatchParameterName/
	// MatchParameterValuePattern (migrations/0009) melihat nilai parameter
	// TERBARU dari Inform ini, bukan snapshot lama sebelum Inform ini
	// diproses. triggerCodes menentukan rule ber-trigger apa yang berlaku
	// utk Inform ini (lihat domain.ZtpTriggerEvent* & komentar lengkap di
	// provisioning.Service.EvaluateZeroTouch soal urutan evaluasi/guard-nya).
	// ZTP gagal/tidak match TIDAK boleh menggagalkan seluruh Inform — device
	// tetap tercatat, menunggu tindakan manual (FR-15).
	triggerCodes := []string{domain.ZtpTriggerEventEveryInform}
	if hasBootstrap {
		triggerCodes = append(triggerCodes, domain.ZtpTriggerEventBootstrapOnly, domain.ZtpTriggerEventBootstrapOrBoot)
	} else if hasBoot {
		triggerCodes = append(triggerCodes, domain.ZtpTriggerEventBootstrapOrBoot)
	}
	if _, err := s.provisioningSvc.EvaluateZeroTouch(ctx, systemActor, dev, triggerCodes); err != nil &&
		!errors.Is(err, domain.ErrNoMatchingRule) {
		s.log().Warn("cwmp: evaluasi zero-touch provisioning gagal", "device_id", dev.ID, "error", err)
	}

	// Engine preset (migrations/0021, PRESET_ENGINE_DESIGN.md) — SETELAH ZTP &
	// SETELAH device_parameters ter-update, supaya drift-check melihat nilai
	// terbaru. Preset ber-enforce=1 dijaga sesuai konfigurasi tiap sesi; bila
	// device menyimpang, satu SetParameterValues gabungan diantre. Gagal TIDAK
	// menggagalkan Inform (FR-15, pola sama ZTP).
	if err := s.provisioningSvc.EvaluatePresets(ctx, systemActor, dev); err != nil {
		s.log().Warn("cwmp: evaluasi preset gagal", "device_id", dev.ID, "error", err)
	}

	// Aturan otomasi kondisi-aksi (migrations/0040) — SETELAH ZTP & preset. Gagal TIDAK menggagalkan Inform (FR-15).
	if s.automation != nil {
		codes := make([]string, 0, len(in.Events))
		for _, ev := range in.Events {
			codes = append(codes, ev.EventCode)
		}
		if err := s.automation.Evaluate(ctx, dev, codes); err != nil {
			s.log().Warn("cwmp: evaluasi aturan otomasi gagal", "device_id", dev.ID, "error", err)
		}
	}

	// Dynamic Parameter Auto-Discovery (ROADMAP.md Fase 5)
	// Jika ZTP tidak menemukan rule (atau rule tidak ada) dan vendor tidak dikenal (VendorID nil),
	// antrekan task GetParameterNames di root (path "") untuk menemukan parameter tree.
	if dev.VendorID == nil && hasBootstrap {
		// EnqueueGetParameterNames: path="" dan nextLevel=true untuk mengambil struktur hierarki teratas
		if _, err := s.taskSvc.EnqueueGetParameterNames(ctx, systemActor, dev.ID, "", true, 5); err != nil {
			s.log().Warn("cwmp: gagal enqueue auto-discovery GetParameterNames", "device_id", dev.ID, "error", err)
		}
	}

	s.maybePollOptical(ctx, dev)
	s.maybePollPPPoE(ctx, dev)

	_ = s.deviceSvc.MarkOnline(ctx, dev)

	if s.publisher != nil && dev.TenantID != nil {
		s.publisher.BroadcastToTenant(*dev.TenantID, "DEVICE_ONLINE", map[string]interface{}{
			"device_id": dev.ID,
		})
	}

	return &InformResult{SessionToken: sess.SessionToken, DeviceID: dev.ID}, nil
}

// informAuth adalah hasil authenticateInform — Device diteruskan ke
// FindOrCreateFromInform supaya tidak query devices dua kali per Inform
// (hot path, TECH.md §9; lihat temuan review internal).
type informAuth struct {
	TenantID *uint64
	Device   *domain.Device // nil bila device belum pernah dikenal
}

// authenticateInform memvalidasi kredensial Basic Auth Inform CWMP (TECH.md
// §3/§8, PRD.md FR-1) dan meng-resolve tenant_id yang harus di-assign bila
// device belum pernah tercatat. Dua jalur:
//  1. Device sudah dikenal (OUI+Serial) dan punya kredensial sendiri
//     (devices.inform_username) -> override, divalidasi ke device tsb. Bila
//     device ini sudah py tenant_id, tenant tsb juga harus masih aktif —
//     menonaktifkan tenant (offboarding) harus benar-benar memutus akses
//     Inform, termasuk device yang pakai kredensial override sendiri.
//  2. Selain itu -> dicocokkan ke shared secret salah satu tenant
//     (tenants.cwmp_inform_username) — ini yang menangani device BENAR-BENAR
//     baru (FR-13/FR-15 zero-touch) yang belum bisa punya kredensial sendiri.
//     Bila device sudah dikenal dan sudah py tenant_id, secret HARUS milik
//     tenant yang sama (mencegah spoofing lintas-tenant pakai secret tenant
//     lain). Device lama yang tenant_id-nya masih nil (mis. dibuat sebelum
//     kredensial Inform wajib) akan "sembuh" — tenant_id di-assign ke tenant
//     pemilik secret yang pertama kali berhasil Inform setelahnya, lalu
//     terkunci (Inform berikutnya dari tenant lain akan ditolak oleh
//     pengecekan mismatch di atas). Untuk instalasi dgn device lama yang
//     tenant kepemilikannya sudah diketahui, sebaiknya di-backfill manual
//     SEBELUM mengaktifkan banyak shared secret tenant — lihat ROADMAP.md.
//
// Tidak ada kredensial yang cocok -> domain.ErrUnauthorized, device TIDAK
// dibuat/diupdate sama sekali.
func (s *Service) authenticateInform(ctx context.Context, oui, serial, username, password string) (*informAuth, error) {
	if username == "" {
		return nil, domain.ErrUnauthorized
	}

	existing, err := s.devices.GetByOUISerial(ctx, oui, serial)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	deviceExists := existing != nil

	if deviceExists && existing.InformUsername != nil {
		if username != *existing.InformUsername || !s.passwordMatches(existing.InformPasswordEnc, password) {
			return nil, domain.ErrUnauthorized
		}
		if existing.TenantID != nil {
			tenant, err := s.tenants.GetByID(ctx, *existing.TenantID)
			if err != nil || !tenant.IsActive {
				return nil, domain.ErrUnauthorized
			}
		}
		return &informAuth{TenantID: existing.TenantID, Device: existing}, nil
	}

	tenant, err := s.tenants.GetByCWMPInformUsername(ctx, username)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}
	if !s.passwordMatches(tenant.CWMPInformPasswordEnc, password) {
		return nil, domain.ErrUnauthorized
	}
	if deviceExists && existing.TenantID != nil && *existing.TenantID != tenant.ID {
		return nil, domain.ErrUnauthorized
	}

	tenantID := tenant.ID
	return &informAuth{TenantID: &tenantID, Device: existing}, nil
}

// passwordMatches: enc kosong berarti belum ada secret dikonfigurasi ->
// selalu tolak (bukan "cocok dengan password kosong"). Constant-time compare
// supaya durasi respons tidak membocorkan seberapa banyak karakter yang cocok.
func (s *Service) passwordMatches(enc []byte, provided string) bool {
	if len(enc) == 0 {
		return false
	}
	decrypted, err := s.enc.Decrypt(enc)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(decrypted), []byte(provided)) == 1
}

func (s *Service) resolveSession(ctx context.Context, token string, deviceID uint64, remoteIP, namespace string) (*domain.DeviceSession, error) {
	if token != "" {
		if existing, err := s.sessions.GetByToken(ctx, token); err == nil &&
			existing.Status == domain.SessionStatusOpen && existing.DeviceID == deviceID {
			return existing, nil
		}
	}
	sess := &domain.DeviceSession{
		DeviceID:     deviceID,
		SessionToken: uuid.NewString(),
		Status:       domain.SessionStatusOpen,
		RemoteIP:     &remoteIP,
		StartedAt:    time.Now(),
	}
	if namespace != "" {
		sess.CWMPNamespace = &namespace
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// ---- Pengiriman task selama sesi berlangsung (TECH.md §3/§4) ----

// OutboundRPC adalah task yang siap dikirim sebagai RPC CWMP. Delivery/cwmp
// yang menerjemahkannya ke Body cwmpxml — usecase tidak boleh bergantung ke
// package delivery (lihat komentar file).
type OutboundRPC struct {
	TaskID     uint64
	TaskUUID   string
	TaskType   string // kode ref_task_types
	Parameters []byte // JSON task.Parameters, kontrak internal usecase/task <-> delivery/cwmp/builder.go
	// Namespace -- namespace CWMP yang dipakai sepanjang sesi ini (dari
	// device_sessions.cwmp_namespace, diisi saat Inform -- migrations/0012),
	// dipakai delivery/cwmp membangun envelope RPC ini. Boleh kosong (mis.
	// data sesi lama sebelum migrations/0012 ada) -- delivery/cwmp yang
	// menentukan default wire-format saat kosong (paket ini sengaja tidak
	// bergantung pkg/cwmpxml, lihat komentar package di atas).
	Namespace string
}

// NextRequest dipanggil delivery/cwmp saat CPE mengirim POST kosong (minta
// RPC berikutnya). closeSession=true berarti tidak ada task tersisa — ACS
// membalas HTTP 204 dan sesi ditutup (TECH.md §3).
func (s *Service) NextRequest(ctx context.Context, token string) (*OutboundRPC, bool, error) {
	sess, err := s.sessions.GetByToken(ctx, token)
	if err != nil {
		return nil, true, err
	}
	if sess.Status != domain.SessionStatusOpen {
		return nil, true, nil
	}

	t, err := s.taskSvc.NextForDevice(ctx, sess.DeviceID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			now := time.Now()
			_ = s.sessions.UpdateStatus(ctx, sess.ID, domain.SessionStatusClosed, &now)
			// Titik otoritatif "sesi ditutup" (state transition-nya SENDIRI
			// terjadi persis di baris di atas) — dicatat di sini, BUKAN
			// duplikat lagi di delivery/cwmp/handler.go, supaya satu event
			// cuma menghasilkan satu baris log (CLAUDE.md: jaga volume log).
			s.log().Info("cwmp: sesi ditutup (tidak ada task tersisa)",
				"device_id", sess.DeviceID, "session_token", RedactToken(token))
			return nil, true, nil
		}
		return nil, true, err
	}

	typeRef, err := s.refs.GetByID(ctx, domain.RefTableTaskTypes, t.TaskTypeID)
	if err != nil {
		return nil, true, err
	}
	// Nilai rahasia task (password WiFi/PPPoE, ...) tersimpan terenkripsi; dibuka tepat sebelum dikirim ke CPE.
	params, oerr := task.OpenSecrets(s.enc, []byte(t.Parameters))
	if oerr != nil {
		s.log().Error("cwmp: nilai rahasia task tidak dapat dibuka, task digagalkan", "task_id", t.ID, "device_id", sess.DeviceID, "error", oerr)
		if ferr := s.taskSvc.FailPermanently(ctx, t, "nilai rahasia task tidak dapat dibuka (kunci enkripsi berubah?)"); ferr != nil {
			return nil, true, ferr
		}
		return s.NextRequest(ctx, token) // lanjut ke task berikutnya (task ini sudah FAILED)
	}
	if err := s.taskSvc.MarkSent(ctx, t); err != nil {
		return nil, true, err
	}
	s.log().Debug("cwmp: task ditandai SENT, siap dikirim ke CPE",
		"device_id", sess.DeviceID, "session_token", RedactToken(token), "task_id", t.ID, "task_type", typeRef.Code)

	var ns string
	if sess.CWMPNamespace != nil {
		ns = *sess.CWMPNamespace
	}
	return &OutboundRPC{TaskID: t.ID, TaskUUID: t.TaskUUID, TaskType: typeRef.Code, Parameters: params, Namespace: ns}, false, nil
}

// ---- Respons RPC dari CPE (TECH.md §3/§4) ----

type FaultInfo struct {
	Code    string
	Message string
}

type TransferCompleteInfo struct {
	CommandKey   string
	Success      bool
	ErrorMessage string
	StartTime    string
	CompleteTime string
}

// RPCResponse membawa tepat satu dari Fault/TransferComplete/ParameterValues
// terisi, sesuai method yang diterima delivery/cwmp dari CPE.
type RPCResponse struct {
	SessionToken string
	// CWMPID — nilai header cwmp:ID pada respons CPE; ACS memakai task_uuid sebagai ID
	// RPC yang dikirim, jadi dipakai mengorelasikan respons ke task yang BENAR
	// (bukan sekadar "task SENT terakhir"). Kosong bila CPE tidak mengembalikannya.
	CWMPID           string
	Fault            *FaultInfo
	TransferComplete *TransferCompleteInfo
	ParameterValues  []InformParameter // dari GetParameterValuesResponse (FR-30)
	RawResponse      []byte            // representasi JSON umum, disimpan di tasks.response
}

func (s *Service) HandleRPCResponse(ctx context.Context, resp RPCResponse) error {
	sess, err := s.sessions.GetByToken(ctx, resp.SessionToken)
	if err != nil {
		return err
	}

	// TransferComplete bisa datang sebagai RPC yang diinisiasi CPE sendiri
	// (bukan respons langsung atas request ACS) — ditangani terpisah.
	if resp.TransferComplete != nil {
		return s.handleTransferComplete(ctx, sess.DeviceID, resp.TransferComplete)
	}
	if resp.Fault != nil {
		return s.handleFault(ctx, sess.DeviceID, resp.CWMPID, resp.Fault)
	}

	t, err := s.sentTaskFor(ctx, sess.DeviceID, resp.CWMPID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil // respons tanpa task yang cocok - abaikan
		}
		return err
	}
	if err := s.taskSvc.Complete(ctx, t, domain.JSONRawMessage(resp.RawResponse)); err != nil {
		return err
	}
	s.log().Debug("cwmp: task COMPLETED dari respons RPC CPE", "device_id", sess.DeviceID, "task_id", t.ID)

	// SetParameterValues sukses -> catat nilai yang baru diterapkan ke
	// device_parameters. Tanpa ini drift-check preset membandingkan dengan nilai
	// LAMA (atau menganggap parameter "belum pernah terlihat") dan menegakkan
	// ulang tiap cooldown sampai preset ditandai FAILED.
	if typeRef, terr := s.refs.GetByID(ctx, domain.RefTableTaskTypes, t.TaskTypeID); terr == nil {
		if typeRef.Code == domain.TaskTypeSetParameterValues {
			s.recordAppliedValues(ctx, sess.DeviceID, t)
		}
		// RPC yang sama kini berhasil -> fault aktifnya untuk perangkat ini sudah tidak relevan.
		if s.faults != nil {
			if err := s.faults.Resolve(ctx, sess.DeviceID, typeRef.Code); err != nil {
				s.log().Warn("cwmp: gagal menutup fault", "device_id", sess.DeviceID, "error", err)
			}
		}
	}

	if len(resp.ParameterValues) > 0 {
		params := make([]domain.DeviceParameter, 0, len(resp.ParameterValues))
		for _, p := range resp.ParameterValues {
			val := p.Value
			params = append(params, domain.DeviceParameter{DeviceID: sess.DeviceID, ParameterName: p.Name, ParameterValue: &val, ParameterTypeID: s.paramTypeID(ctx, p.Type), Writable: true})
		}
		_ = s.deviceParams.UpsertBatch(ctx, params)

		// Korelasikan ke DeviceDiagnostic (bila task ini task pengambilan hasil diagnostik).
		vals := make(map[string]string, len(resp.ParameterValues))
		for _, p := range resp.ParameterValues {
			vals[p.Name] = p.Value
		}
		if err := s.diagnosticsSvc.HandleParameterValues(ctx, t.ID, vals); err != nil && !errors.Is(err, domain.ErrNotFound) {
			s.log().Warn("cwmp: gagal memproses hasil diagnostik", "task_id", t.ID, "error", err)
		}
		s.handleOpticalResult(ctx, sess.DeviceID, t, vals)
	} else if s.diagnosticsSvc != nil {
		// Respons non-GPV (mis. SetParameterValues pemicu): maju ke fase menunggu hasil.
		if err := s.diagnosticsSvc.HandleResult(ctx, t.ID, nil, true); err != nil && !errors.Is(err, domain.ErrNotFound) {
			s.log().Warn("cwmp: gagal memajukan diagnostik", "task_id", t.ID, "error", err)
		}
	}
	return nil
}

// paramTypeID me-resolve atribut xsi:type CPE ke id ref_parameter_types
// (di-cache per proses; nil bila tipe kosong/tak dikenal, sehingga nilai tipe yang
// sudah tersimpan tidak tertimpa — lihat COALESCE pada upsert).
func (s *Service) paramTypeID(ctx context.Context, xsd string) *uint64 {
	code := domain.NormalizeXSDType(xsd)
	if code == "" || s.refs == nil {
		return nil
	}
	if v, ok := s.typeIDs.Load(code); ok {
		id := v.(uint64)
		return &id
	}
	ref, err := s.refs.GetByCode(ctx, domain.RefTableParameterTypes, code)
	if err != nil {
		return nil
	}
	s.typeIDs.Store(code, ref.ID)
	id := ref.ID
	return &id
}

// WithOpticalPoll mengaktifkan polling redaman optik berkala (interval 0 = mati).
func (s *Service) WithOpticalPoll(every time.Duration) *Service {
	s.opticalPollEvery = every
	return s
}

// WithNotifier memasang notifikasi persisten untuk redaman optik waspada/kritis.
func (s *Service) WithNotifier(n domain.Notifier) *Service {
	s.notifier = n
	return s
}

// FaultRecorder mencatat/menutup cwmp:Fault aktif per perangkat (usecase/fault.Service; migrations/0038).
type FaultRecorder interface {
	Record(ctx context.Context, deviceID uint64, taskID *uint64, rpcMethod, code, message string) error
	Resolve(ctx context.Context, deviceID uint64, rpcMethod string) error
}

// AutomationEvaluator mengevaluasi aturan otomasi tenant pada tiap Inform (usecase/automation.Service; migrations/0040).
type AutomationEvaluator interface {
	Evaluate(ctx context.Context, dev *domain.Device, events []string) error
}

// WithAutomation memasang engine aturan otomasi; nil = tidak ada otomasi.
func (s *Service) WithAutomation(a AutomationEvaluator) *Service {
	s.automation = a
	return s
}

// WithFaults memasang pencatatan resource faults; nil = fault hanya menggagalkan task & memicu webhook seperti sebelumnya.
func (s *Service) WithFaults(f FaultRecorder) *Service {
	s.faults = f
	return s
}

// WithPPPoEPoll mengaktifkan penyegaran berkala username PPPoE (interval 0 = mati).
func (s *Service) WithPPPoEPoll(every time.Duration) *Service {
	s.pppoePollEvery = every
	return s
}

// maybePollPPPoE mengantre pembacaan koneksi WAN/PPP bila sudah lewat intervalnya, supaya kolom PPPoE di daftar perangkat
// tidak basi (pelanggan ganti paket/akun). Tidak mengantre bila masih ada task terbuka; data model yang belum diketahui
// atau kuota penuh dicoba lagi 1 jam kemudian. Gagal tidak menggagalkan Inform.
func (s *Service) maybePollPPPoE(ctx context.Context, dev *domain.Device) {
	if s.pppoePollEvery <= 0 {
		return
	}
	if v, ok := s.pppoeNext.Load(dev.ID); ok && time.Now().Before(v.(time.Time)) {
		return
	}
	if pending, err := s.taskSvc.HasPendingForDevice(ctx, dev.ID); err != nil || pending {
		return
	}
	queued, err := s.taskSvc.EnqueuePPPoEPoll(ctx, dev.ID)
	switch {
	case err != nil:
		s.log().Debug("cwmp: penyegaran PPPoE ditunda", "device_id", dev.ID, "error", err)
		s.pppoeNext.Store(dev.ID, time.Now().Add(time.Hour))
	case !queued:
		s.pppoeNext.Store(dev.ID, time.Now().Add(time.Hour))
	default:
		s.pppoeNext.Store(dev.ID, time.Now().Add(s.pppoePollEvery))
	}
}

// maybePollOptical mengantre pembacaan optik (GetParameterValues terpetakan) bila sudah lewat
// intervalnya. Hanya berjalan bila vendor/model punya mapping optical.*; hasil negatif
// di-cache 1 jam per vendor/model.
func (s *Service) maybePollOptical(ctx context.Context, dev *domain.Device) {
	if s.opticalPollEvery <= 0 || dev.VendorID == nil {
		return
	}
	cacheKey := fmt.Sprintf("%d:%v", *dev.VendorID, dev.DeviceModelID)
	if v, ok := s.opticalSkip.Load(cacheKey); ok && time.Now().Before(v.(time.Time)) {
		return
	}
	if last, err := s.deviceSvc.LastOpticalAt(ctx, dev.ID); err == nil && last != nil && time.Since(*last) < s.opticalPollEvery {
		return
	}
	if pending, err := s.taskSvc.HasPendingForDevice(ctx, dev.ID); err != nil || pending {
		return
	}
	queued, err := s.taskSvc.EnqueueOpticalPoll(ctx, dev.ID)
	if err != nil {
		s.log().Warn("cwmp: gagal mengantre polling optik", "device_id", dev.ID, "error", err)
		return
	}
	if !queued {
		s.opticalSkip.Store(cacheKey, time.Now().Add(time.Hour))
	}
}

// handleOpticalResult mengubah hasil GetParameterValues polling optik menjadi metrik
// (nilai × skala mapping) dan menjalankan deteksi anomali yang sama dengan jalur Inform.
func (s *Service) handleOpticalResult(ctx context.Context, deviceID uint64, t *domain.Task, values map[string]string) {
	var p struct {
		Optical *struct {
			Fields map[string]string  `json:"fields"`
			Scales map[string]float64 `json:"scales"`
		} `json:"optical"`
	}
	if json.Unmarshal(t.Parameters, &p) != nil || p.Optical == nil {
		return
	}
	get := func(field string) *float64 {
		path, ok := p.Optical.Fields[field]
		if !ok {
			return nil
		}
		raw, ok := values[path]
		if !ok {
			return nil
		}
		var v float64
		if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%f", &v); err != nil {
			return nil
		}
		if sc := p.Optical.Scales[path]; sc != 0 {
			v *= sc
		}
		return &v
	}
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return
	}
	s.recordOptical(ctx, dev, get("rx_dbm"), get("tx_dbm"), get("voltage"), get("bias_ma"), get("temperature_c"), p.Optical.Fields["rx_dbm"])
}

// failDiagnostic menandai diagnostik terkait task ini FAILED (bila ada).
func (s *Service) failDiagnostic(ctx context.Context, taskID uint64) {
	if s.diagnosticsSvc == nil {
		return
	}
	if err := s.diagnosticsSvc.HandleResult(ctx, taskID, nil, false); err != nil && !errors.Is(err, domain.ErrNotFound) {
		s.log().Warn("cwmp: gagal menandai diagnostik gagal", "task_id", taskID, "error", err)
	}
}

// recordAppliedValues menyimpan values task SET_PARAMETER_VALUES (yang sudah
// di-ACK CPE) ke device_parameters. Best-effort: kegagalan hanya dilog.
func (s *Service) recordAppliedValues(ctx context.Context, deviceID uint64, t *domain.Task) {
	plain, oerr := task.OpenSecrets(s.enc, []byte(t.Parameters))
	if oerr != nil {
		s.log().Warn("cwmp: nilai rahasia task tidak dapat dibuka, tidak dicatat ke device_parameters", "device_id", deviceID, "task_id", t.ID, "error", oerr)
		return
	}
	var p struct {
		Values map[string]string `json:"values"`
	}
	if err := json.Unmarshal(plain, &p); err != nil || len(p.Values) == 0 {
		return
	}
	params := make([]domain.DeviceParameter, 0, len(p.Values))
	for name, v := range p.Values {
		val := v
		params = append(params, domain.DeviceParameter{DeviceID: deviceID, ParameterName: name, ParameterValue: &val, Writable: true})
	}
	if err := s.deviceParams.UpsertBatch(ctx, params); err != nil {
		s.log().Warn("cwmp: gagal mencatat nilai SetParameterValues ke device_parameters", "device_id", deviceID, "task_id", t.ID, "error", err)
	}
}

// handleFault menerjemahkan cwmp:Fault dari CPE ke aksi pada task yang
// sedang SENT (TECH.md §3/§4). Sebagian besar fault code memakai alur retry
// generik (task.Service.Fail -> failOrRetry), TAPI dua fault code standar
// TR-069 dapat penanganan khusus krn retry identik pasti percuma:
//   - domain.FaultCodeInvalidParameterName (9005): parameter memang tidak
//     ada di device ini -> FAILED segera (FailPermanently), skip max_retries.
//   - domain.FaultCodeInvalidArguments (9003) KHUSUS pada task
//     GET_PARAMETER_VALUES: biasanya CPE menolak krn parameter list
//     kepanjangan -> daftar nama dipecah dua, jadi dua task baru
//     (SplitGetParameterValuesOnFault), task asli tidak di-retry apa adanya.
//     9003 pada task type LAIN, atau bila daftar sudah tidak bisa dipecah
//     lagi (<=1 parameter), tetap jatuh ke alur Fail/failOrRetry biasa.
//
// Fault code lain di luar dua ini TIDAK berubah perilakunya sama sekali.
// sentTaskFor memilih task yang dijawab respons CPE: bila cwmp:ID cocok dengan task SENT
// milik device ini, itu yang dipakai; selain itu fallback ke task SENT terakhir device.
func (s *Service) sentTaskFor(ctx context.Context, deviceID uint64, cwmpID string) (*domain.Task, error) {
	if cwmpID != "" {
		if t, err := s.taskSvc.GetByUUID(ctx, cwmpID); err == nil && t.DeviceID == deviceID {
			if st, serr := s.refs.GetByID(ctx, domain.RefTableTaskStatus, t.TaskStatusID); serr == nil && st.Code == domain.TaskStatusSent {
				return t, nil
			}
		}
	}
	return s.taskSvc.GetSentForDevice(ctx, deviceID)
}

// recordFault mencatat fault ke resource faults (best-effort: gagal tidak boleh mengubah penanganan task).
func (s *Service) recordFault(ctx context.Context, deviceID uint64, t *domain.Task, f *FaultInfo) {
	if s.faults == nil {
		return
	}
	var taskID *uint64
	rpc := ""
	if t != nil {
		taskID = &t.ID
		if typeRef, err := s.refs.GetByID(ctx, domain.RefTableTaskTypes, t.TaskTypeID); err == nil {
			rpc = typeRef.Code
		}
	}
	if err := s.faults.Record(ctx, deviceID, taskID, rpc, f.Code, f.Message); err != nil {
		s.log().Warn("cwmp: gagal mencatat fault", "device_id", deviceID, "error", err)
	}
}

func (s *Service) handleFault(ctx context.Context, deviceID uint64, cwmpID string, f *FaultInfo) error {
	t, err := s.sentTaskFor(ctx, deviceID, cwmpID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Fault tanpa task yang cocok tetap dicatat: operator perlu tahu CPE menolak sesuatu.
			s.recordFault(ctx, deviceID, nil, f)
			return nil
		}
		return err
	}
	s.recordFault(ctx, deviceID, t, f)
	msg := f.Message
	if f.Code != "" {
		msg = fmt.Sprintf("[%s] %s", f.Code, f.Message)
	}
	s.log().Error("cwmp: menangani cwmp:Fault dari CPE", "device_id", deviceID, "task_id", t.ID, "fault_code", f.Code, "fault_string", f.Message)

	// Webhook DEVICE_FAULT (migrations/0013) — CPE menolak RPC dgn cwmp:Fault.
	// tenant_id di-resolve dari device (fault jarang, bukan hot path).
	if s.webhookEnq != nil && s.devices != nil {
		var tenantID *uint64
		if dev, derr := s.devices.GetByID(ctx, deviceID); derr == nil {
			tenantID = dev.TenantID
		}
		s.notifyWebhook(ctx, domain.WebhookEventDeviceFault, tenantID, map[string]any{
			"device_id":    deviceID,
			"task_id":      t.ID,
			"fault_code":   f.Code,
			"fault_string": f.Message,
		})
	}

	switch f.Code {
	case domain.FaultCodeACSBuild:
		s.failDiagnostic(ctx, t.ID)
		return s.taskSvc.FailPermanently(ctx, t, msg)

	case domain.FaultCodeInvalidParameterName:
		s.failDiagnostic(ctx, t.ID)
		s.log().Warn("cwmp: fault 9005 Invalid Parameter Name -> task FAILED permanen (parameter tidak didukung device ini)",
			"device_id", deviceID, "task_id", t.ID)
		return s.taskSvc.FailPermanently(ctx, t, msg+" (parameter tidak didukung/tidak dikenal device ini)")

	case domain.FaultCodeInvalidArguments:
		typeRef, errRef := s.refs.GetByID(ctx, domain.RefTableTaskTypes, t.TaskTypeID)
		if errRef == nil && typeRef.Code == domain.TaskTypeGetParameterValues {
			handled, splitErr := s.taskSvc.SplitGetParameterValuesOnFault(ctx, t, msg)
			if splitErr != nil {
				return splitErr
			}
			if handled {
				s.log().Warn("cwmp: fault 9003 Invalid Arguments pada GET_PARAMETER_VALUES -> dipecah jadi 2 task baru",
					"device_id", deviceID, "task_id", t.ID)
				return nil
			}
			// handled=false (daftar sudah <=1 parameter, tidak bisa dipecah
			// lagi) -> jatuh ke Fail/failOrRetry generik di bawah.
		}
	}
	return s.taskSvc.Fail(ctx, t, msg)
}

func (s *Service) handleTransferComplete(ctx context.Context, deviceID uint64, tc *TransferCompleteInfo) error {
	var t *domain.Task
	var err error
	// CommandKey diisi ACS dengan task_uuid saat mengirim Download (lihat
	// delivery/cwmp/builder.go), sehingga TransferComplete bisa dikorelasikan
	// balik ke task yang tepat meski datang di luar urutan strict SENT.
	if tc.CommandKey != "" {
		t, err = s.taskSvc.GetByUUID(ctx, tc.CommandKey)
		// CommandKey berasal dari CPE (tidak tepercaya): task harus milik device
		// pengirim. Tanpa ini device tenant A bisa menyelesaikan/menggagalkan task
		// device/tenant lain hanya dengan menebak UUID-nya.
		if t != nil && t.DeviceID != deviceID {
			s.log().Warn("cwmp: TransferComplete dengan CommandKey milik device lain diabaikan", "device_id", deviceID, "task_device_id", t.DeviceID)
			return nil
		}
	}
	if t == nil {
		t, err = s.taskSvc.GetSentForDevice(ctx, deviceID)
	}
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}

	if tc.Success {
		payload, _ := json.Marshal(map[string]string{"start_time": tc.StartTime, "complete_time": tc.CompleteTime})
		if err := s.taskSvc.Complete(ctx, t, domain.JSONRawMessage(payload)); err != nil {
			return err
		}
	} else if err := s.taskSvc.Fail(ctx, t, tc.ErrorMessage); err != nil {
		return err
	}

	if err := s.firmwareSvc.HandleTransferComplete(ctx, t.ID, tc.Success, tc.ErrorMessage); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return nil
}

func (s *Service) detectAnomalyDNS(ctx context.Context, dev *domain.Device, params []InformParameter) {
	// DNS Servers parameter key
	var dnsNewVal string
	var dnsParamName string
	for _, p := range params {
		// TR-098: InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.DNSServers
		// TR-181: Device.DNS.Client.Server.1.DNSServer
		if strings.Contains(p.Name, "DNSServer") {
			dnsNewVal = p.Value
			dnsParamName = p.Name
			break
		}
	}

	if dnsNewVal != "" {
		// Dapatkan nilai lama
		oldParam, err := s.deviceParams.Get(ctx, dev.ID, dnsParamName)
		if err == nil && oldParam.ParameterValue != nil && *oldParam.ParameterValue != dnsNewVal {
			// Anomaly: DNS berubah
			desc := fmt.Sprintf("DNS berubah mencurigakan dari %s menjadi %s pada parameter %s", *oldParam.ParameterValue, dnsNewVal, dnsParamName)
			s.recordActivity(ctx, &domain.ActivityLog{
				TenantID:    dev.TenantID,
				Action:      "ANOMALY_DETECTED",
				EntityType:  "device",
				EntityID:    &dev.ID,
				Description: &desc,
			})
			s.log().Warn("cwmp: anomaly DNS change terdeteksi", "device_id", dev.ID, "old_dns", *oldParam.ParameterValue, "new_dns", dnsNewVal)
			s.notifyWebhook(ctx, domain.WebhookEventDeviceFault, dev.TenantID, map[string]any{
				"device_id":    dev.ID,
				"anomaly_type": "DNS_HIJACKING",
				"old_dns":      *oldParam.ParameterValue,
				"new_dns":      dnsNewVal,
				"message":      desc,
			})
		}
	}
}

// detectAnomalyBoot menandai reboot loop: dua event BOOT berturut-turut dengan
// jarak < 15 menit. prevBootAt = nilai LastBootEventAt SEBELUM di-update oleh
// TouchLastBootEvent (kalau dibaca sesudahnya selalu ~0 dan setiap boot keliru
// dianggap anomali).
func (s *Service) detectAnomalyBoot(ctx context.Context, dev *domain.Device, prevBootAt *time.Time, thisBootAt time.Time) {
	if prevBootAt == nil {
		return
	}
	gap := thisBootAt.Sub(*prevBootAt)
	if gap <= 0 || gap >= 15*time.Minute {
		return
	}
	desc := fmt.Sprintf("Reboot berulang terdeteksi (jarak dari boot sebelumnya: %s)", gap.Round(time.Second))
	s.recordActivity(ctx, &domain.ActivityLog{
		TenantID:    dev.TenantID,
		Action:      "ANOMALY_DETECTED",
		EntityType:  "device",
		EntityID:    &dev.ID,
		Description: &desc,
	})
	s.log().Warn("cwmp: anomaly frequent reboot terdeteksi", "device_id", dev.ID, "gap", gap)
	s.notifyWebhook(ctx, domain.WebhookEventDeviceFault, dev.TenantID, map[string]any{
		"device_id":    dev.ID,
		"anomaly_type": "FREQUENT_REBOOT",
		"gap":          gap.String(),
		"message":      desc,
	})
}

// recordActivity nil-safe wrapper (Service dpt dibangun tanpa activity di test).
func (s *Service) recordActivity(ctx context.Context, l *domain.ActivityLog) {
	if s.activity == nil {
		return
	}
	_ = s.activity.Record(ctx, l)
}

func (s *Service) detectAnomalyOptical(ctx context.Context, dev *domain.Device, params []InformParameter) {
	var rxPower, txPower, voltage, bias, temp *float64
	var paramName string

	for _, p := range params {
		if strings.Contains(p.Name, "Optical") {
			var val float64
			if _, err := fmt.Sscanf(p.Value, "%f", &val); err == nil {
				if strings.Contains(p.Name, "RxPower") {
					rxPower = &val
					paramName = p.Name
				} else if strings.Contains(p.Name, "TxPower") {
					txPower = &val
				} else if strings.Contains(p.Name, "Voltage") {
					voltage = &val
				} else if strings.Contains(p.Name, "BiasCurrent") {
					bias = &val
				} else if strings.Contains(p.Name, "Temperature") {
					temp = &val
				}
			}
		}
	}

	s.recordOptical(ctx, dev, rxPower, txPower, voltage, bias, temp, paramName)
}

// recordOptical menyimpan metrik optik & memicu deteksi anomali redaman (dBm kanonik).
func (s *Service) recordOptical(ctx context.Context, dev *domain.Device, rxPower, txPower, voltage, bias, temp *float64, paramName string) {
	if rxPower != nil || txPower != nil || voltage != nil || bias != nil || temp != nil {
		metric := &domain.DeviceOpticalMetric{
			DeviceID:           dev.ID,
			RxPowerDBM:         rxPower,
			TxPowerDBM:         txPower,
			Voltage:            voltage,
			BiasCurrentMA:      bias,
			TemperatureCelsius: temp,
			RecordedAt:         time.Now(),
		}
		_ = s.deviceSvc.SaveOpticalMetric(ctx, metric)
	}

	if rxPower != nil {
		s.alertOptical(ctx, dev, *rxPower, paramName)
	}
}

// alertOptical menilai RX terhadap ambang waspada (-25 dBm) & kritis (-27 dBm atau overload > -8 dBm; domain.OpticalLevelOf).
// Kritis: activity log + webhook DEVICE_FAULT (perilaku lama) + notifikasi CRITICAL. Waspada: hanya notifikasi WARNING.
// Notifikasi ditekan 24 jam per (perangkat, level) sehingga polling berkala tidak membanjiri; naik level tetap dikabarkan.
// Satuan yang tak dapat ditebak (di luar rentang fisik) diabaikan — alarm palsu lebih merugikan daripada terlewat.
func (s *Service) alertOptical(ctx context.Context, dev *domain.Device, rawRX float64, paramName string) {
	rx, ok := domain.NormalizeRXDBM(rawRX)
	if !ok {
		s.log().Debug("cwmp: nilai RX di luar rentang fisik, tidak dinilai", "device_id", dev.ID, "rx_raw", rawRX)
		return
	}
	level := domain.OpticalLevelOf(&rx)
	if level != domain.OpticalBad && level != domain.OpticalWarn {
		return
	}
	if level == domain.OpticalBad {
		desc := fmt.Sprintf("Redaman optik kritis (RxPower: %.2f dBm) terdeteksi pada parameter %s", rx, paramName)
		s.recordActivity(ctx, &domain.ActivityLog{
			TenantID:    dev.TenantID,
			Action:      "ANOMALY_DETECTED",
			EntityType:  "device",
			EntityID:    &dev.ID,
			Description: &desc,
		})
		s.log().Warn("cwmp: anomaly redaman optik terdeteksi", "device_id", dev.ID, "rx_power", rx)
		s.notifyWebhook(ctx, domain.WebhookEventDeviceFault, dev.TenantID, map[string]any{
			"device_id":    dev.ID,
			"anomaly_type": "OPTICAL_POWER",
			"rx_power":     rx,
			"level":        level,
			"message":      desc,
		})
	}
	if s.notifier == nil || dev.TenantID == nil {
		return
	}
	severity, title, threshold := domain.NotificationSeverityWarning, "Redaman optik waspada", "ambang waspada -25 dBm"
	if level == domain.OpticalBad {
		severity, title, threshold = domain.NotificationSeverityCritical, "Redaman optik kritis", "di luar rentang aman -27..-8 dBm"
	}
	_ = s.notifier.Notify(ctx, domain.NotificationEvent{
		TypeCode: domain.NotifOpticalAlert, TenantID: dev.TenantID, SeverityOverride: severity,
		Title:      title,
		Body:       fmt.Sprintf("Perangkat %s: RX %.2f dBm (%s). Periksa konektor, tikungan kabel, dan splitter.", dev.SerialNumber, rx, threshold),
		Link:       fmt.Sprintf("/devices/%d", dev.ID),
		EntityType: "device", EntityID: &dev.ID,
		DedupeKey: fmt.Sprintf("optical:%d:%s", dev.ID, level), DedupeWindow: 24 * time.Hour,
	})
}
