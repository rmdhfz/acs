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
	Events         []InformEvent
	Parameters     []InformParameter
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
		OUI:             in.DeviceOUI,
		SerialNumber:    in.SerialNumber,
		ProductClass:    in.ProductClass,
		SoftwareVersion: in.SoftwareVersion,
		HardwareVersion: in.HardwareVersion,
		RemoteIP:        in.RemoteIP,
		TenantID:        auth.TenantID,
		ExistingDevice:  auth.Device,
	})
	if err != nil {
		return nil, fmt.Errorf("session: gagal upsert device dari Inform: %w", err)
	}

	sess, err := s.resolveSession(ctx, in.SessionToken, dev.ID, in.RemoteIP)
	if err != nil {
		return nil, err
	}
	// Simpan namespace CWMP sesi ini SEKALI (idempotent dipanggil ulang kalau
	// Inform lanjutan di sesi yang sama, mis. sesudah value-change -- costnya
	// murah, satu UPDATE) supaya NextRequest bisa memakainya utk RPC proaktif
	// berikutnya (lihat migrations/0012).
	if in.Namespace != "" {
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
			params = append(params, domain.DeviceParameter{DeviceID: dev.ID, ParameterName: p.Name, ParameterValue: &val})
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

	// Dynamic Parameter Auto-Discovery (ROADMAP.md Fase 5)
	// Jika ZTP tidak menemukan rule (atau rule tidak ada) dan vendor tidak dikenal (VendorID nil),
	// antrekan task GetParameterNames di root (path "") untuk menemukan parameter tree.
	if dev.VendorID == nil && hasBootstrap {
		// EnqueueGetParameterNames: path="" dan nextLevel=true untuk mengambil struktur hierarki teratas
		if _, err := s.taskSvc.EnqueueGetParameterNames(ctx, systemActor, dev.ID, "", true, 5); err != nil {
			s.log().Warn("cwmp: gagal enqueue auto-discovery GetParameterNames", "device_id", dev.ID, "error", err)
		}
	}

	_ = s.deviceSvc.MarkOnline(ctx, dev.ID)

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

func (s *Service) resolveSession(ctx context.Context, token string, deviceID uint64, remoteIP string) (*domain.DeviceSession, error) {
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
				"device_id", sess.DeviceID, "session_token", token)
			return nil, true, nil
		}
		return nil, true, err
	}

	typeRef, err := s.refs.GetByID(ctx, domain.RefTableTaskTypes, t.TaskTypeID)
	if err != nil {
		return nil, true, err
	}
	if err := s.taskSvc.MarkSent(ctx, t.ID); err != nil {
		return nil, true, err
	}
	s.log().Debug("cwmp: task ditandai SENT, siap dikirim ke CPE",
		"device_id", sess.DeviceID, "session_token", token, "task_id", t.ID, "task_type", typeRef.Code)

	var ns string
	if sess.CWMPNamespace != nil {
		ns = *sess.CWMPNamespace
	}
	return &OutboundRPC{TaskID: t.ID, TaskUUID: t.TaskUUID, TaskType: typeRef.Code, Parameters: []byte(t.Parameters), Namespace: ns}, false, nil
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
	SessionToken     string
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
		return s.handleFault(ctx, sess.DeviceID, resp.Fault)
	}

	t, err := s.taskSvc.GetSentForDevice(ctx, sess.DeviceID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil // respons tanpa task yang cocok (mis. GetRPCMethods) - abaikan
		}
		return err
	}
	if err := s.taskSvc.Complete(ctx, t.ID, domain.JSONRawMessage(resp.RawResponse)); err != nil {
		return err
	}
	s.log().Debug("cwmp: task COMPLETED dari respons RPC CPE", "device_id", sess.DeviceID, "task_id", t.ID)

	if len(resp.ParameterValues) > 0 {
		params := make([]domain.DeviceParameter, 0, len(resp.ParameterValues))
		for _, p := range resp.ParameterValues {
			val := p.Value
			params = append(params, domain.DeviceParameter{DeviceID: sess.DeviceID, ParameterName: p.Name, ParameterValue: &val, Writable: true})
		}
		_ = s.deviceParams.UpsertBatch(ctx, params)

		// Coba korelasikan ke DeviceDiagnostic (bila task ini memang task
		// pengambilan hasil diagnostic) — abaikan bila tidak ada korelasi.
		if err := s.diagnosticsSvc.HandleResult(ctx, t.ID, domain.JSONRawMessage(resp.RawResponse), true); err != nil && !errors.Is(err, domain.ErrNotFound) {
			_ = err
		}
	}
	return nil
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
func (s *Service) handleFault(ctx context.Context, deviceID uint64, f *FaultInfo) error {
	t, err := s.taskSvc.GetSentForDevice(ctx, deviceID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}
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
	case domain.FaultCodeInvalidParameterName:
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
		if err := s.taskSvc.Complete(ctx, t.ID, domain.JSONRawMessage(payload)); err != nil {
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
		rx := *rxPower
		isAnomaly := false
		// Jika formatnya ribuan (e.g., -2800 = -28.0 dBm)
		if rx < -2700 || (rx < -27.0 && rx > -100.0) {
			isAnomaly = true
		}

		if isAnomaly {
			desc := fmt.Sprintf("Redaman optik kritis (RxPower: %v) terdeteksi pada parameter %s", rx, paramName)
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
				"message":      desc,
			})
		}
	}
}
