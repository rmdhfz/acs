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
) *Service {
	return &Service{
		sessions: sessions, events: events, deviceParams: deviceParams, devices: devices, tenants: tenants, refs: refs,
		deviceSvc: deviceSvc, taskSvc: taskSvc, provisioningSvc: provisioningSvc,
		firmwareSvc: firmwareSvc, diagnosticsSvc: diagnosticsSvc, enc: enc,
	}
}

// CountOpenSessions — jumlah sesi CWMP berstatus OPEN saat ini (metrik
// observability TECH.md §10, dipakai internal/metrics.Collector). Lintas
// seluruh tenant — device_sessions tidak menyimpan tenant_id langsung dan
// metrik ini utk operator platform, bukan dashboard tenant.
func (s *Service) CountOpenSessions(ctx context.Context) (int, error) {
	return s.sessions.CountOpen(ctx)
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

	// Aktor sistem (bukan user login) untuk aksi otomatis seperti evaluasi ZTP.
	systemActor := domain.Actor{TenantID: dev.TenantID}

	for _, ev := range in.Events {
		code, err := s.refs.GetByCode(ctx, domain.RefTableEventCodes, ev.EventCode)
		if err != nil {
			continue // event code tidak dikenal (kuirk vendor) - jangan gagalkan seluruh Inform
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
			// ZTP gagal/tidak match TIDAK boleh menggagalkan seluruh Inform —
			// device tetap tercatat, menunggu tindakan manual (FR-15).
			if _, err := s.provisioningSvc.EvaluateZeroTouch(ctx, systemActor, dev); err != nil &&
				!errors.Is(err, domain.ErrNoMatchingRule) {
				_ = err
			}
		case domain.EventCodeBoot:
			_ = s.deviceSvc.TouchLastBootEvent(ctx, dev, occurredAt)
		}
	}

	if len(in.Parameters) > 0 {
		params := make([]domain.DeviceParameter, 0, len(in.Parameters))
		for _, p := range in.Parameters {
			val := p.Value
			params = append(params, domain.DeviceParameter{DeviceID: dev.ID, ParameterName: p.Name, ParameterValue: &val})
		}
		_ = s.deviceParams.UpsertBatch(ctx, params)
	}

	_ = s.deviceSvc.MarkOnline(ctx, dev.ID)

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

	return &OutboundRPC{TaskID: t.ID, TaskUUID: t.TaskUUID, TaskType: typeRef.Code, Parameters: t.Parameters}, false, nil
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
	if err := s.taskSvc.Complete(ctx, t.ID, resp.RawResponse); err != nil {
		return err
	}

	if len(resp.ParameterValues) > 0 {
		params := make([]domain.DeviceParameter, 0, len(resp.ParameterValues))
		for _, p := range resp.ParameterValues {
			val := p.Value
			params = append(params, domain.DeviceParameter{DeviceID: sess.DeviceID, ParameterName: p.Name, ParameterValue: &val, Writable: true})
		}
		_ = s.deviceParams.UpsertBatch(ctx, params)

		// Coba korelasikan ke DeviceDiagnostic (bila task ini memang task
		// pengambilan hasil diagnostic) — abaikan bila tidak ada korelasi.
		if err := s.diagnosticsSvc.HandleResult(ctx, t.ID, resp.RawResponse, true); err != nil && !errors.Is(err, domain.ErrNotFound) {
			_ = err
		}
	}
	return nil
}

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
		if err := s.taskSvc.Complete(ctx, t.ID, payload); err != nil {
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
