// Package cwmp adalah delivery layer untuk endpoint CWMP — satu-satunya
// tempat yang mem-parsing/mem-build XML SOAP (pkg/cwmpxml) dan
// menerjemahkannya ke/dari tipe polos usecase/session. Handler HANYA
// parsing/validasi request lalu memanggil usecase — tidak ada logic bisnis
// di sini (CLAUDE.md).
package cwmp

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v5"

	"acs/internal/usecase/session"
	"acs/pkg/cwmpxml"
)

const sessionCookieName = "acs_session"

type Handler struct {
	sessions *session.Service
}

func NewHandler(sessions *session.Service) *Handler {
	return &Handler{sessions: sessions}
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
	// sesi — keduanya ditangani NextRequest (lihat TECH.md §3).
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
	if err := h.sessions.HandleRPCResponse(c.Request().Context(), buildRPCResponse(token, env)); err != nil {
		c.Logger().Error("cwmp handler error", "error", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	return h.next(c, token)
}

func (h *Handler) handleInform(c *echo.Context, token string, env *cwmpxml.Envelope) error {
	inf := env.Body.Inform

	events := make([]session.InformEvent, 0, len(inf.Event.Items))
	for _, e := range inf.Event.Items {
		events = append(events, session.InformEvent{EventCode: e.EventCode, CommandKey: e.CommandKey})
	}
	params := make([]session.InformParameter, 0, len(inf.ParameterList.Values))
	for _, p := range inf.ParameterList.Values {
		params = append(params, session.InformParameter{Name: p.Name, Value: p.Value.Value})
	}

	result, err := h.sessions.HandleInform(c.Request().Context(), session.InformInput{
		SessionToken:    token,
		RemoteIP:        c.RealIP(),
		DeviceOUI:       inf.DeviceId.OUI,
		SerialNumber:    inf.DeviceId.SerialNumber,
		ProductClass:    inf.DeviceId.ProductClass,
		SoftwareVersion: paramSuffix(params, "SoftwareVersion"),
		HardwareVersion: paramSuffix(params, "HardwareVersion"),
		Events:          events,
		Parameters:      params,
	})
	if err != nil {
		c.Logger().Error("cwmp handler error", "error", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	setSessionCookie(c, result.SessionToken)

	respEnv := cwmpxml.NewEnvelope(headerIDFrom(env), cwmpxml.Body{
		InformResponse: &cwmpxml.InformResponse{MaxEnvelopes: 1},
	})
	return writeEnvelope(c, respEnv)
}

func (h *Handler) next(c *echo.Context, token string) error {
	if token == "" {
		return c.NoContent(http.StatusNoContent)
	}
	rpc, closeSession, err := h.sessions.NextRequest(c.Request().Context(), token)
	if err != nil {
		c.Logger().Error("cwmp handler error", "error", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	if closeSession || rpc == nil {
		return c.NoContent(http.StatusNoContent)
	}

	body, err := BuildRequestBody(rpc.TaskType, rpc.TaskUUID, rpc.Parameters)
	if err != nil {
		c.Logger().Error("cwmp handler error", "error", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	return writeEnvelope(c, cwmpxml.NewEnvelope(rpc.TaskUUID, body))
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
	c.SetCookie(&http.Cookie{Name: sessionCookieName, Value: token, Path: "/", HttpOnly: true})
}

func headerIDFrom(env *cwmpxml.Envelope) string {
	if env.Header != nil && env.Header.ID != nil {
		return env.Header.ID.Value
	}
	return ""
}
