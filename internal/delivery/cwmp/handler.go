// Package cwmp adalah delivery layer untuk endpoint CWMP — satu-satunya
// tempat yang mem-parsing/mem-build XML SOAP (pkg/cwmpxml) dan
// menerjemahkannya ke/dari tipe polos usecase/session. Handler HANYA
// parsing/validasi request lalu memanggil usecase — tidak ada logic bisnis
// di sini (CLAUDE.md).
package cwmp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/metrics"
	"acs/internal/usecase/session"
	"acs/pkg/cwmpxml"
)

const sessionCookieName = "acs_session"

// SessionService adalah bagian usecase/session yang dibutuhkan handler ini.
// Dipakai sebagai interface (bukan *session.Service) supaya alur HTTP CWMP
// bisa diuji penuh dengan mock CPE tanpa merakit seluruh usecase + repository.
// *session.Service memenuhinya apa adanya.
type SessionService interface {
	HandleInform(ctx context.Context, in session.InformInput) (*session.InformResult, error)
	HandleRPCResponse(ctx context.Context, resp session.RPCResponse) error
	NextRequest(ctx context.Context, token string) (*session.OutboundRPC, bool, error)
}

type Handler struct {
	sessions SessionService
	capture  Capturer
	log      *slog.Logger
	// storage dipakai menerbitkan URL presigned Download SAAT RPC dikirim.
	storage domain.ObjectStorage
}

// Capturer = capture sesi per perangkat (usecase/capture). Opsional: nil = tidak ada capture. Jalur cepat AnyActive
// membuat biaya nol bagi kasus umum (tidak ada perangkat yang sedang di-capture).
type Capturer interface {
	AnyActive(ctx context.Context) bool
	Active(ctx context.Context, deviceID uint64) bool
	DeviceIDForToken(ctx context.Context, token string) (uint64, bool)
	Record(ctx context.Context, deviceID uint64, token string, fromCPE bool, raw []byte, httpStatus int) error
}

// WithCapture memasang capture sesi. Pesan disamarkan (pkg/cwmpxml.Redact) di Capturer sebelum disimpan.
func (h *Handler) WithCapture(c Capturer) *Handler {
	h.capture = c
	return h
}

// Kunci context untuk meneruskan hasil request ke langkah capture (tanpa mengubah tanda tangan fungsi lain).
const (
	ctxOutKey    = "cwmp.out"    // []byte: envelope yang dikirim ke CPE
	ctxDeviceKey = "cwmp.device" // uint64: id perangkat dari HandleInform
	ctxTokenKey  = "cwmp.token"  // string: token sesi (baru) dari HandleInform
)

// maybeCapture merekam pasangan request/response request ini bila perangkatnya sedang di-capture. Kegagalan hanya dicatat
// (DEBUG) — tidak boleh mengganggu sesi CWMP.
func (h *Handler) maybeCapture(c *echo.Context, reqBody []byte) {
	if h.capture == nil {
		return
	}
	ctx := c.Request().Context()
	if !h.capture.AnyActive(ctx) {
		return
	}
	token, _ := c.Get(ctxTokenKey).(string)
	if token == "" {
		token = readSessionToken(c)
	}
	deviceID, ok := c.Get(ctxDeviceKey).(uint64)
	if !ok {
		if deviceID, ok = h.capture.DeviceIDForToken(ctx, token); !ok {
			return
		}
	}
	if !h.capture.Active(ctx, deviceID) {
		return
	}
	status := 0
	if r, err := echo.UnwrapResponse(c.Response()); err == nil {
		status = r.Status
	}
	out, _ := c.Get(ctxOutKey).([]byte)
	if err := h.capture.Record(ctx, deviceID, token, true, reqBody, 0); err != nil {
		h.log.Debug("cwmp: capture request gagal disimpan", "device_id", deviceID, "error", err)
	}
	if err := h.capture.Record(ctx, deviceID, token, false, out, status); err != nil {
		h.log.Debug("cwmp: capture response gagal disimpan", "device_id", deviceID, "error", err)
	}
}

// downloadURLTTL — masa berlaku URL presigned Download sejak RPC dikirim; CPE
// mengunduh segera setelah menerima RPC.
const downloadURLTTL = time.Hour

// WithStorage memasang object storage untuk task Download ber-storage_key.
func (h *Handler) WithStorage(s domain.ObjectStorage) *Handler {
	h.storage = s
	return h
}

// resolveParameters mengganti storage_key pada parameters Download dengan URL
// presigned yang baru dibuat; tipe lain dikembalikan apa adanya.
func (h *Handler) resolveParameters(ctx context.Context, rpc *session.OutboundRPC) ([]byte, error) {
	if rpc.TaskType != domain.TaskTypeDownload {
		return rpc.Parameters, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(rpc.Parameters, &m); err != nil {
		return nil, err
	}
	key, _ := m[domain.TaskParamStorageKey].(string)
	if key == "" {
		return rpc.Parameters, nil
	}
	if h.storage == nil {
		return nil, errors.New("object storage tidak terkonfigurasi untuk task Download")
	}
	u, err := h.storage.PresignedGetURL(ctx, key, downloadURLTTL)
	if err != nil {
		return nil, err
	}
	m["url"] = u
	delete(m, domain.TaskParamStorageKey)
	return json.Marshal(m)
}

// NewHandler — logger nil diperbolehkan (fallback slog.Default()), supaya
// pemanggil yang belum siap sedia logger khusus (mis. test) tidak wajib
// menyediakannya.
func NewHandler(sessions SessionService, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{sessions: sessions, log: logger}
}

func (h *Handler) Register(e *echo.Echo, path string) {
	e.POST(path, h.handle)
}

func (h *Handler) handle(c *echo.Context) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.NoContent(http.StatusBadRequest)
	}
	err = h.serve(c, body)
	h.maybeCapture(c, body)
	return err
}

func (h *Handler) serve(c *echo.Context, body []byte) error {
	env, err := cwmpxml.Unmarshal(body)
	if err != nil {
		return c.NoContent(http.StatusBadRequest)
	}

	token := readSessionToken(c)

	// POST kosong dari CPE: baik penanda "siap terima RPC" maupun penutup
	// sesi — keduanya ditangani NextRequest (lihat TECH.md §3). Request ini
	// sendiri tidak membawa body XML sama sekali sehingga tidak ada
	// namespace CWMP yang bisa dibaca darinya -- TAPI ini justru jalur
	// MAYORITAS pengiriman RPC proaktif pertama dalam sesi (langsung sesudah
	// InformResponse), bukan kasus langka. NextRequest (usecase/session)
	// yang menyediakan namespace yang benar, dibaca dari device_sessions
	// (diisi sekali saat Inform -- migrations/0012), bukan dari request ini.
	if env.IsEmpty() {
		return h.next(c, token)
	}

	if env.Body.Inform != nil {
		return h.handleInform(c, token, env)
	}

	if token == "" {
		// RPC response tanpa sesi yang dikenal — tidak ada yang bisa dikorelasikan.
		return c.NoContent(http.StatusBadRequest)
	}

	// RPC yang DIINISIASI CPE (selain TransferComplete di bawah) bukan respons atas task
	// ACS: jawab langsung dan JANGAN korelasikan ke task SENT (sebelumnya GetRPCMethods
	// dari CPE keliru menyelesaikan task SENT yang sedang menunggu respons aslinya).
	if env.Body.GetRPCMethods != nil {
		return h.respondGetRPCMethods(c, env)
	}
	if env.Body.Kicked != nil || env.Body.RequestDownload != nil {
		return h.respondUnsupportedCPEMethod(c, env)
	}

	rpcResp := buildRPCResponse(token, env)
	if env.Header != nil && env.Header.ID != nil {
		rpcResp.CWMPID = env.Header.ID.Value
	}
	h.logRPCResponse(token, rpcResp)
	if err := h.sessions.HandleRPCResponse(c.Request().Context(), rpcResp); err != nil {
		h.log.Error("cwmp: HandleRPCResponse gagal", "session_token", session.RedactToken(token), "error", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	// TransferComplete adalah RPC yang DIINISIASI CPE: TR-069 mewajibkan ACS
	// membalasnya dengan TransferCompleteResponse pada HTTP response ini
	// (bukan RPC berikutnya / 204). CPE lalu mengirim POST kosong yang ditangani
	// next(). Tanpa balasan ini CPE nyata bisa mengulang TransferComplete
	// berkali-kali atau menganggap sesi rusak.
	if env.Body.TransferComplete != nil {
		return writeTransferCompleteResponse(c, env)
	}
	return h.next(c, token)
}

// acsMethods — method CWMP yang DIDUKUNG ACS untuk dipanggil CPE (TR-069 §A.3.1:
// GetRPCMethods, Inform, TransferComplete wajib). Kicked/RequestDownload (opsional)
// belum didukung.
var acsMethods = []string{"GetRPCMethods", "Inform", "TransferComplete"}

func (h *Handler) respondGetRPCMethods(c *echo.Context, env *cwmpxml.Envelope) error {
	ns := env.Body.Namespace()
	id := ""
	if env.Header != nil && env.Header.ID != nil {
		id = env.Header.ID.Value
	}
	return writeEnvelope(c, cwmpxml.NewEnvelope(id, ns, cwmpxml.Body{
		GetRPCMethodsResponse: &cwmpxml.GetRPCMethodsResponse{
			XMLName:    cwmpxml.RPCName(ns, "GetRPCMethodsResponse"),
			MethodList: cwmpxml.StringList{ArrayType: fmt.Sprintf("xsd:string[%d]", len(acsMethods)), Items: acsMethods},
		},
	}))
}

// respondUnsupportedCPEMethod membalas SOAP Fault 9000 "Method not supported".
func (h *Handler) respondUnsupportedCPEMethod(c *echo.Context, env *cwmpxml.Envelope) error {
	ns := env.Body.Namespace()
	id := ""
	if env.Header != nil && env.Header.ID != nil {
		id = env.Header.ID.Value
	}
	return writeEnvelope(c, cwmpxml.NewEnvelope(id, ns, cwmpxml.Body{Fault: &cwmpxml.Fault{
		FaultCode: "Client", FaultString: "CWMP fault",
		Detail: &cwmpxml.FaultDetail{CWMPFault: cwmpxml.CWMPFault{FaultCode: "9000", FaultString: "Method not supported"}},
	}}))
}

// writeTransferCompleteResponse membalas TransferComplete dengan ID header
// dan namespace yang sama seperti yang dipakai CPE.
func writeTransferCompleteResponse(c *echo.Context, env *cwmpxml.Envelope) error {
	ns := env.Body.Namespace()
	id := ""
	if env.Header != nil && env.Header.ID != nil {
		id = env.Header.ID.Value
	}
	return writeEnvelope(c, cwmpxml.NewEnvelope(id, ns, cwmpxml.Body{
		TransferCompleteResponse: &cwmpxml.TransferCompleteResponse{XMLName: cwmpxml.RPCName(ns, "TransferCompleteResponse")},
	}))
}

func (h *Handler) handleInform(c *echo.Context, token string, env *cwmpxml.Envelope) error {
	// Latensi Inform -> InformResponse (metrik observability, TECH.md §10) —
	// diukur dari titik ini (Inform baru saja selesai di-parse) sampai
	// InformResponse berhasil ditulis di akhir fungsi ini. Lihat komentar di
	// internal/metrics/inform_latency.go soal kenapa ini histogram real-time,
	// BUKAN gauge query-on-scrape spt metrik lain di internal/metrics.
	start := time.Now()

	inf := env.Body.Inform
	// ns: namespace CWMP yang BENAR-BENAR dideklarasikan CPE ini pada Inform
	// (hasil capture Unmarshal, lihat cwmpxml.Body.Namespace()) — dipakai utk
	// InformResponse balasan supaya konsisten dgn yang dipakai CPE ybs,
	// bukan cwmp-1-2 hardcoded (lihat TECH.md §3, catatan investigasi
	// arsitektur soal Huawei & vendor lain yang sensitif terhadap ini).
	// Namespace di luar pola urn:dslforum-org:cwmp-1-N dianggap cacat dan diganti default saat membalas.
	ns := sanitizeNamespace(env.Body.Namespace())

	// Isi Inform tidak dipercaya: identitas cacat ditolak 400; event/parameter dibatasi jumlah & ukurannya
	// (lihat inform_sanitize.go) supaya satu nilai buruk tidak membuat INSERT gagal atau membanjiri DB.
	if err := validateInformIdentity(inf.DeviceId); err != nil {
		h.log.Warn("cwmp: Inform ditolak (identitas tidak valid)", "remote_ip", c.RealIP())
		return c.NoContent(http.StatusBadRequest)
	}
	events := sanitizeInformEvents(inf.Event.Items)
	params := sanitizeInformParams(inf.ParameterList.Values)
	crURL := paramSuffix(params, ".ManagementServer.ConnectionRequestURL")
	if len(crURL) > maxInformCRURLLen {
		crURL = ""
	}

	// Basic Auth wajib untuk setiap Inform (session baru maupun lanjutan) —
	// tidak divalidasi ulang di request lain dalam sesi yang sama (Inform
	// adalah titik pembentukan sesi, lihat TECH.md §3 & usecase/session).
	username, password, _ := c.Request().BasicAuth()

	h.log.Debug("cwmp: Inform diterima",
		"session_token", session.RedactToken(token), "oui", inf.DeviceId.OUI, "serial_number", inf.DeviceId.SerialNumber,
		"cwmp_namespace", ns, "event_count", len(events))

	result, err := h.sessions.HandleInform(c.Request().Context(), session.InformInput{
		SessionToken:    token,
		RemoteIP:        c.RealIP(),
		DeviceOUI:       inf.DeviceId.OUI,
		SerialNumber:    inf.DeviceId.SerialNumber,
		ProductClass:    clampRunes(inf.DeviceId.ProductClass, maxInformProductClass),
		SoftwareVersion: clampRunes(paramSuffix(params, "SoftwareVersion"), maxInformVersionLen),
		HardwareVersion: clampRunes(paramSuffix(params, "HardwareVersion"), maxInformVersionLen),
		// Suffix segmen lengkap (bukan cuma "ConnectionRequestURL") supaya
		// vendor-extension yang kebetulan berakhiran sama — mis.
		// X_ACME_ConnectionRequestURL — tidak keliru dipakai sebagai target
		// GET Connection Request. Cocok utk TR-098 & TR-181 (keduanya
		// berakhiran .ManagementServer.ConnectionRequestURL).
		ConnectionRequestURL: crURL,
		InformUsername:       username,
		InformPassword:       password,
		Events:               events,
		Parameters:           params,
		Namespace:            ns,
	})
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			// CPE hampir selalu mencoba TANPA kredensial dulu, menunggu tantangan 401, lalu mengulang dengan Basic Auth.
			// Percobaan pertama itu normal (DEBUG); hanya penolakan atas kredensial yang DIBERIKAN yang layak WARN.
			if username == "" && password == "" {
				h.log.Debug("cwmp: Inform tanpa kredensial, mengirim tantangan Basic", "oui", inf.DeviceId.OUI, "serial_number", inf.DeviceId.SerialNumber)
			} else {
				h.log.Warn("cwmp: Inform ditolak (kredensial salah)", "oui", inf.DeviceId.OUI, "serial_number", inf.DeviceId.SerialNumber, "remote_ip", c.RealIP())
			}
			c.Response().Header().Set("WWW-Authenticate", `Basic realm="ACS"`)
			return c.NoContent(http.StatusUnauthorized)
		}
		if errors.Is(err, domain.ErrConflict) {
			// Mis. device dengan OUI+serial ini dulu dihapus oleh tenant LAIN: revive
			// lintas tenant sengaja tidak dilakukan (lihat deviceRepository.Create).
			h.log.Warn("cwmp: Inform ditolak (konflik kepemilikan device)", "oui", inf.DeviceId.OUI, "serial_number", inf.DeviceId.SerialNumber)
			return c.NoContent(http.StatusConflict)
		}
		if errors.Is(err, domain.ErrQuotaExceeded) {
			// Batas perangkat tenant tercapai: bukan galat server. 503 + Retry-After supaya CPE mencoba lagi nanti
			// (setelah paket dinaikkan) tanpa membanjiri ACS.
			h.log.Warn("cwmp: Inform ditolak, batas perangkat tenant tercapai", "oui", inf.DeviceId.OUI, "serial_number", inf.DeviceId.SerialNumber)
			c.Response().Header().Set("Retry-After", "3600")
			return c.NoContent(http.StatusServiceUnavailable)
		}
		h.log.Error("cwmp: HandleInform gagal", "oui", inf.DeviceId.OUI, "serial_number", inf.DeviceId.SerialNumber, "error", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	setSessionCookie(c, result.SessionToken)
	c.Set(ctxDeviceKey, result.DeviceID)
	c.Set(ctxTokenKey, result.SessionToken)

	h.log.Info("cwmp: sesi dibuka/dilanjutkan dari Inform",
		"device_id", result.DeviceID, "session_token", session.RedactToken(result.SessionToken))

	respEnv := cwmpxml.NewEnvelope(headerIDFrom(env), ns, cwmpxml.Body{
		InformResponse: &cwmpxml.InformResponse{XMLName: cwmpxml.RPCName(ns, "InformResponse"), MaxEnvelopes: 1},
	})
	if err := writeEnvelope(c, respEnv); err != nil {
		return err
	}
	metrics.ObserveInformResponseLatency(time.Since(start))
	return nil
}

func (h *Handler) next(c *echo.Context, token string) error {
	if token == "" {
		return c.NoContent(http.StatusNoContent)
	}
	rpc, closeSession, err := h.sessions.NextRequest(c.Request().Context(), token)
	if err != nil {
		h.log.Error("cwmp: NextRequest gagal", "session_token", session.RedactToken(token), "error", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	if closeSession {
		// Log "sesi ditutup" dicatat di usecase/session.Service.NextRequest
		// (titik otoritatif state transition-nya), bukan di sini — hindari
		// duplikat baris log utk satu event yang sama.
		return c.NoContent(http.StatusNoContent)
	}
	if rpc == nil {
		return c.NoContent(http.StatusNoContent)
	}

	// ns dari device_sessions.cwmp_namespace (lewat OutboundRPC.Namespace,
	// diisi NextRequest -- migrations/0012), BUKAN dari request HTTP saat
	// ini (request "siap terima RPC" tidak membawa body XML sama sekali).
	// Fallback ke cwmpxml.NSCWMP hanya utk sesi lama dari sebelum
	// migrations/0012 yang belum pernah tercatat namespace-nya.
	ns := rpc.Namespace
	if ns == "" {
		ns = cwmpxml.NSCWMP
	}

	params, err := h.resolveParameters(c.Request().Context(), rpc)
	var body cwmpxml.Body
	if err == nil {
		body, err = BuildRequestBody(rpc.TaskType, rpc.TaskUUID, params, ns)
	}
	if err != nil {
		h.log.Error("cwmp: BuildRequestBody gagal", "session_token", session.RedactToken(token), "task_type", rpc.TaskType, "error", err)
		// Task sudah berstatus SENT: tanpa ini ia tersangkut dan CPE menerima 500.
		// Tandai FAILED permanen lalu lanjutkan sesi dengan task berikutnya / 204.
		if ferr := h.sessions.HandleRPCResponse(c.Request().Context(), session.RPCResponse{
			SessionToken: token,
			Fault:        &session.FaultInfo{Code: domain.FaultCodeACSBuild, Message: "ACS gagal membangun RPC: " + err.Error()},
		}); ferr != nil {
			h.log.Error("cwmp: gagal menandai task tak-terbangun sebagai FAILED", "session_token", session.RedactToken(token), "error", ferr)
			return c.NoContent(http.StatusInternalServerError)
		}
		return h.next(c, token)
	}
	h.log.Debug("cwmp: mengirim RPC ke CPE", "session_token", session.RedactToken(token), "task_uuid", rpc.TaskUUID, "task_type", rpc.TaskType, "cwmp_namespace", ns)
	return writeEnvelope(c, cwmpxml.NewEnvelope(rpc.TaskUUID, ns, body))
}

// logRPCResponse mencatat respons RPC dari CPE (Debug utk hasil normal, Error
// utk cwmp:Fault — CLAUDE.md soal level log supaya production bisa
// menyesuaikan verbosity). Dipanggil SEBELUM diteruskan ke usecase/session
// supaya tetap tercatat walau HandleRPCResponse gagal.
func (h *Handler) logRPCResponse(token string, resp session.RPCResponse) {
	switch {
	case resp.Fault != nil:
		h.log.Error("cwmp: menerima cwmp:Fault dari CPE",
			"session_token", session.RedactToken(token), "fault_code", resp.Fault.Code, "fault_string", resp.Fault.Message)
	case resp.TransferComplete != nil:
		h.log.Debug("cwmp: menerima TransferComplete dari CPE",
			"session_token", session.RedactToken(token), "command_key", resp.TransferComplete.CommandKey, "success", resp.TransferComplete.Success)
	default:
		h.log.Debug("cwmp: menerima respons RPC dari CPE", "session_token", session.RedactToken(token))
	}
}

// faultMessage = FaultString + (bila CPE menyertakan) rincian per parameter SetParameterValuesFault, mis.
// "Internal error [InternetGatewayDevice...SSID: 9007 Invalid parameter value]", agar operator tahu parameter mana yang ditolak.
func faultMessage(f *cwmpxml.CWMPFault) string {
	msg := f.FaultString
	var parts []string
	for i, d := range f.SetParameterValuesFaults {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("+%d lainnya", len(f.SetParameterValuesFaults)-3))
			break
		}
		parts = append(parts, strings.TrimSpace(fmt.Sprintf("%s: %s %s", clampRunes(d.ParameterName, 200), d.FaultCode, clampRunes(d.FaultString, 80))))
	}
	if len(parts) > 0 {
		msg += " [" + strings.Join(parts, "; ") + "]"
	}
	return msg
}

// buildRPCResponse menerjemahkan Body respons CPE menjadi session.RPCResponse
// polos. Ini satu-satunya tempat yang tahu bentuk XML respons.
func buildRPCResponse(token string, env *cwmpxml.Envelope) session.RPCResponse {
	b := env.Body
	resp := session.RPCResponse{SessionToken: token}

	switch {
	case b.Fault != nil:
		code, msg := "", b.Fault.FaultString
		if b.Fault.Detail != nil {
			code = b.Fault.Detail.CWMPFault.FaultCode
			msg = faultMessage(&b.Fault.Detail.CWMPFault)
		}
		resp.Fault = &session.FaultInfo{Code: code, Message: msg}
		return resp

	case b.TransferComplete != nil:
		tc := b.TransferComplete
		success := tc.FaultStruct == nil || tc.FaultStruct.FaultCode == "" || tc.FaultStruct.FaultCode == "0"
		errMsg := ""
		if !success {
			errMsg = tc.FaultStruct.FaultString
		}
		resp.TransferComplete = &session.TransferCompleteInfo{
			CommandKey: tc.CommandKey, Success: success, ErrorMessage: errMsg,
			StartTime: tc.StartTime, CompleteTime: tc.CompleteTime,
		}
		return resp

	case b.GetParameterValuesResponse != nil:
		nv := ParseResponseValues(b.GetParameterValuesResponse.ParameterList)
		resp.ParameterValues = make([]session.InformParameter, 0, len(nv))
		for _, v := range nv {
			resp.ParameterValues = append(resp.ParameterValues, session.InformParameter{Name: v.Name, Value: v.Value, Type: v.Type})
		}
		resp.RawResponse, _ = json.Marshal(b.GetParameterValuesResponse)
		return resp

	default:
		resp.RawResponse, _ = json.Marshal(b)
		return resp
	}
}

func paramSuffix(params []session.InformParameter, suffix string) string {
	for _, p := range params {
		if len(p.Name) >= len(suffix) && p.Name[len(p.Name)-len(suffix):] == suffix {
			return p.Value
		}
	}
	return ""
}

func writeEnvelope(c *echo.Context, env *cwmpxml.Envelope) error {
	out, err := cwmpxml.Marshal(env)
	if err != nil {
		return err
	}
	c.Set(ctxOutKey, out)
	return c.Blob(http.StatusOK, "text/xml; charset=utf-8", out)
}

func readSessionToken(c *echo.Context) string {
	if ck, err := c.Cookie(sessionCookieName); err == nil {
		return ck.Value
	}
	return ""
}

// requestIsSecure — true bila request datang lewat TLS langsung atau lewat
// reverse proxy yang menandainya https.
func requestIsSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setSessionCookie(c *echo.Context, token string) {
	// Secure HANYA bila request memang https. Sebelumnya selalu true padahal
	// acsd melayani HTTP polos: CPE berbasis libcurl TIDAK mengirim cookie
	// Secure lewat http://, sehingga POST kosong sesudah InformResponse tiba
	// tanpa cookie -> token kosong -> sesi ditutup (204) dan TIDAK ADA RPC
	// (provisioning/reboot/dst.) yang pernah terkirim di deployment http.
	// Simulator CPE tidak menangkap ini karena mengabaikan atribut Secure.
	// TECH.md §8 tetap mewajibkan TLS di produksi; di sana (TLS langsung atau
	// X-Forwarded-Proto: https dari terminator) flag Secure menyala sendiri.
	// HttpOnly=true: tidak bisa dibaca JavaScript.
	// SameSite=Strict: request cross-site tidak membawa cookie.
	// MaxAge=3600: 1 jam, sejalan dengan umur sesi CWMP tipikal (periodic inform
	// interval biasanya 15-60 menit). Cookie kedaluwarsa = CPE harus Inform ulang.
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsSecure(c.Request()),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   3600,
	})
}

func headerIDFrom(env *cwmpxml.Envelope) string {
	if env.Header != nil && env.Header.ID != nil {
		return env.Header.ID.Value
	}
	return ""
}
