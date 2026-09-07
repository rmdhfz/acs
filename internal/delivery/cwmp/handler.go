// Package cwmp adalah delivery layer untuk endpoint CWMP — satu-satunya
// tempat yang mem-parsing/mem-build XML SOAP (pkg/cwmpxml) dan
// menerjemahkannya ke/dari tipe polos usecase/session. Handler HANYA
// parsing/validasi request lalu memanggil usecase — tidak ada logic bisnis
// di sini (CLAUDE.md).
package cwmp

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/metrics"
	"acs/internal/usecase/session"
	"acs/pkg/cwmpxml"
)

const sessionCookieName = "acs_session"

type Handler struct {
	sessions *session.Service
	log      *slog.Logger
}

// NewHandler — logger nil diperbolehkan (fallback slog.Default()), supaya
// pemanggil yang belum siap sedia logger khusus (mis. test) tidak wajib
// menyediakannya.
func NewHandler(sessions *session.Service, logger *slog.Logger) *Handler {
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

	rpcResp := buildRPCResponse(token, env)
	h.logRPCResponse(token, rpcResp)
	if err := h.sessions.HandleRPCResponse(c.Request().Context(), rpcResp); err != nil {
		h.log.Error("cwmp: HandleRPCResponse gagal", "session_token", token, "error", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	return h.next(c, token)
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
	ns := env.Body.Namespace()

	events := make([]session.InformEvent, 0, len(inf.Event.Items))
	for _, e := range inf.Event.Items {
		events = append(events, session.InformEvent{EventCode: e.EventCode, CommandKey: e.CommandKey})
	}
	params := make([]session.InformParameter, 0, len(inf.ParameterList.Values))
	for _, p := range inf.ParameterList.Values {
		params = append(params, session.InformParameter{Name: p.Name, Value: p.Value.Value})
	}

	// Basic Auth wajib untuk setiap Inform (session baru maupun lanjutan) —
	// tidak divalidasi ulang di request lain dalam sesi yang sama (Inform
	// adalah titik pembentukan sesi, lihat TECH.md §3 & usecase/session).
	username, password, _ := c.Request().BasicAuth()

	h.log.Debug("cwmp: Inform diterima",
		"session_token", token, "oui", inf.DeviceId.OUI, "serial_number", inf.DeviceId.SerialNumber,
		"cwmp_namespace", ns, "event_count", len(events))

	result, err := h.sessions.HandleInform(c.Request().Context(), session.InformInput{
		SessionToken:    token,
		RemoteIP:        c.RealIP(),
		DeviceOUI:       inf.DeviceId.OUI,
		SerialNumber:    inf.DeviceId.SerialNumber,
		ProductClass:    inf.DeviceId.ProductClass,
		SoftwareVersion: paramSuffix(params, "SoftwareVersion"),
		HardwareVersion: paramSuffix(params, "HardwareVersion"),
		// Suffix segmen lengkap (bukan cuma "ConnectionRequestURL") supaya
		// vendor-extension yang kebetulan berakhiran sama — mis.
		// X_ACME_ConnectionRequestURL — tidak keliru dipakai sebagai target
		// GET Connection Request. Cocok utk TR-098 & TR-181 (keduanya
		// berakhiran .ManagementServer.ConnectionRequestURL).
		ConnectionRequestURL: paramSuffix(params, ".ManagementServer.ConnectionRequestURL"),
		InformUsername:       username,
		InformPassword:       password,
		Events:               events,
		Parameters:           params,
		Namespace:            ns,
	})
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			h.log.Warn("cwmp: Inform ditolak (unauthorized)", "oui", inf.DeviceId.OUI, "serial_number", inf.DeviceId.SerialNumber)
			c.Response().Header().Set("WWW-Authenticate", `Basic realm="ACS"`)
			return c.NoContent(http.StatusUnauthorized)
		}
		h.log.Error("cwmp: HandleInform gagal", "oui", inf.DeviceId.OUI, "serial_number", inf.DeviceId.SerialNumber, "error", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	setSessionCookie(c, result.SessionToken)

	h.log.Info("cwmp: sesi dibuka/dilanjutkan dari Inform",
		"device_id", result.DeviceID, "session_token", result.SessionToken)

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
		h.log.Error("cwmp: NextRequest gagal", "session_token", token, "error", err)
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

	body, err := BuildRequestBody(rpc.TaskType, rpc.TaskUUID, rpc.Parameters, ns)
	if err != nil {
		h.log.Error("cwmp: BuildRequestBody gagal", "session_token", token, "task_type", rpc.TaskType, "error", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	h.log.Debug("cwmp: mengirim RPC ke CPE", "session_token", token, "task_uuid", rpc.TaskUUID, "task_type", rpc.TaskType, "cwmp_namespace", ns)
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
			"session_token", token, "fault_code", resp.Fault.Code, "fault_string", resp.Fault.Message)
	case resp.TransferComplete != nil:
		h.log.Debug("cwmp: menerima TransferComplete dari CPE",
			"session_token", token, "command_key", resp.TransferComplete.CommandKey, "success", resp.TransferComplete.Success)
	default:
		h.log.Debug("cwmp: menerima respons RPC dari CPE", "session_token", token)
	}
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
			msg = b.Fault.Detail.CWMPFault.FaultString
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
			resp.ParameterValues = append(resp.ParameterValues, session.InformParameter{Name: v.Name, Value: v.Value})
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
	return c.Blob(http.StatusOK, "text/xml; charset=utf-8", out)
}

func readSessionToken(c *echo.Context) string {
	if ck, err := c.Cookie(sessionCookieName); err == nil {
		return ck.Value
	}
	return ""
}

func setSessionCookie(c *echo.Context, token string) {
	// Secure=true: hanya dikirim via HTTPS (TECH.md §8: endpoint CWMP wajib TLS)
	// HttpOnly=true: tidak bisa dibaca JavaScript (XSS protection)
	// SameSite=Strict: mencegah CSRF — request cross-site tidak membawa cookie
	// MaxAge=3600: 1 jam, sejalan dengan umur sesi CWMP tipikal (device
	// periodic inform interval biasanya 15-60 menit; sesi yang lebih panjang
	// dari ini sangat tidak umum). Cookie kedaluwarsa = CPE harus Inform ulang.
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
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
