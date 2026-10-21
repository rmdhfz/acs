package cwmp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/session"
	"acs/pkg/cwmpxml"
)

// ---- fake usecase ----

type fakeSessions struct {
	informIn   []session.InformInput
	informRes  *session.InformResult
	informErr  error
	respIn     []session.RPCResponse
	respErr    error
	nextTokens []string
	nextQueue  []nextResult // dikonsumsi berurutan; habis -> (nil, true, nil)
	nextErr    error
}

type nextResult struct {
	rpc   *session.OutboundRPC
	close bool
}

func (f *fakeSessions) HandleInform(_ context.Context, in session.InformInput) (*session.InformResult, error) {
	f.informIn = append(f.informIn, in)
	if f.informErr != nil {
		return nil, f.informErr
	}
	if f.informRes != nil {
		return f.informRes, nil
	}
	return &session.InformResult{SessionToken: "tok-1", DeviceID: 42}, nil
}
func (f *fakeSessions) HandleRPCResponse(_ context.Context, r session.RPCResponse) error {
	f.respIn = append(f.respIn, r)
	return f.respErr
}
func (f *fakeSessions) NextRequest(_ context.Context, token string) (*session.OutboundRPC, bool, error) {
	f.nextTokens = append(f.nextTokens, token)
	if f.nextErr != nil {
		return nil, true, f.nextErr
	}
	if len(f.nextQueue) == 0 {
		return nil, true, nil
	}
	n := f.nextQueue[0]
	f.nextQueue = f.nextQueue[1:]
	return n.rpc, n.close, nil
}

// ---- mock CPE (klien SOAP + cookie jar ala libcurl) ----

// mockCPE meniru perilaku CPE sungguhan terhadap cookie: cookie ber-atribut
// Secure TIDAK dikirim lewat http:// (libcurl dan kebanyakan stack CPE begitu).
// Inilah yang tidak dimodelkan simulator e2e dan menyembunyikan bug cookie
// Secure-selalu-true.
type mockCPE struct {
	t      *testing.T
	e      *echo.Echo
	https  bool // kanal ke ACS: https (lewat terminator TLS) atau http polos
	cookie *http.Cookie
}

func newCPE(t *testing.T, svc SessionService, https bool) *mockCPE {
	return newCPEWithLog(t, svc, https, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func newCPEWithLog(t *testing.T, svc SessionService, https bool, log *slog.Logger) *mockCPE {
	e := echo.New()
	NewHandler(svc, log).Register(e, "/cwmp")
	return &mockCPE{t: t, e: e, https: https}
}

func (c *mockCPE) post(body string, user, pass string) *httptest.ResponseRecorder {
	c.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/cwmp", strings.NewReader(body))
	req.RemoteAddr = "203.0.113.7:51234"
	if c.https {
		req.Header.Set("X-Forwarded-Proto", "https")
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	if c.cookie != nil && (!c.cookie.Secure || c.https) {
		req.AddCookie(&http.Cookie{Name: c.cookie.Name, Value: c.cookie.Value})
	}
	rec := httptest.NewRecorder()
	c.e.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		c.cookie = ck
	}
	return rec
}

// ---- fixture SOAP ----

func soapEnv(ns, hdr, body string) string {
	return `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:soapenc="http://schemas.xmlsoap.org/soap/encoding/" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:cwmp="` + ns + `">` +
		hdr + `<soapenv:Body>` + body + `</soapenv:Body></soapenv:Envelope>`
}

const hdrID = `<soapenv:Header><cwmp:ID soapenv:mustUnderstand="1">cpe-id-77</cwmp:ID></soapenv:Header>`

func informXML(ns string) string {
	return soapEnv(ns, hdrID, `<cwmp:Inform>
<DeviceId><Manufacturer>ZTE</Manufacturer><OUI>00D0D0</OUI><ProductClass>F670L</ProductClass><SerialNumber>ZTE123</SerialNumber></DeviceId>
<Event><EventStruct><EventCode>1 BOOT</EventCode><CommandKey/></EventStruct><EventStruct><EventCode>6 CONNECTION REQUEST</EventCode><CommandKey/></EventStruct></Event>
<MaxEnvelopes>1</MaxEnvelopes><CurrentTime>2026-10-15T00:00:00Z</CurrentTime><RetryCount>0</RetryCount>
<ParameterList>
<ParameterValueStruct><Name>InternetGatewayDevice.DeviceInfo.SoftwareVersion</Name><Value xsi:type="xsd:string">V9.0.10</Value></ParameterValueStruct>
<ParameterValueStruct><Name>InternetGatewayDevice.DeviceInfo.HardwareVersion</Name><Value xsi:type="xsd:string">HW-B</Value></ParameterValueStruct>
<ParameterValueStruct><Name>InternetGatewayDevice.ManagementServer.X_ACME_ConnectionRequestURL</Name><Value xsi:type="xsd:string">http://evil.example/</Value></ParameterValueStruct>
<ParameterValueStruct><Name>InternetGatewayDevice.ManagementServer.ConnectionRequestURL</Name><Value xsi:type="xsd:string">http://203.0.113.7:58000/cr</Value></ParameterValueStruct>
</ParameterList></cwmp:Inform>`)
}

const emptyPost = ""

func mustEnv(t *testing.T, rec *httptest.ResponseRecorder) *cwmpxml.Envelope {
	t.Helper()
	env, err := cwmpxml.Unmarshal(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("respons ACS bukan SOAP valid: %v\n%s", err, rec.Body.String())
	}
	return env
}

// ================== alur sesi penuh ==================

// Regresi inti: sesi lengkap Inform -> POST kosong -> RPC -> respons -> POST
// kosong -> tutup, dengan CPE yang MENOLAK mengirim cookie Secure lewat http.
func TestSesiLengkap_Reboot(t *testing.T) {
	for _, tc := range []struct {
		name  string
		https bool
	}{
		{"http polos (deployment acsd tanpa TLS / uji lapangan LAN)", false},
		{"https lewat terminator TLS (X-Forwarded-Proto)", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ns := "urn:dslforum-org:cwmp-1-0"
			svc := &fakeSessions{nextQueue: []nextResult{
				{rpc: &session.OutboundRPC{TaskID: 9, TaskUUID: "task-uuid-9", TaskType: domain.TaskTypeReboot, Namespace: ns}},
				{close: true},
			}}
			cpe := newCPE(t, svc, tc.https)

			// 1) Inform dgn Basic Auth
			rec := cpe.post(informXML(ns), "tenant-user", "tenant-secret-0123456789")
			if rec.Code != 200 {
				t.Fatalf("Inform: HTTP %d: %s", rec.Code, rec.Body.String())
			}
			resp := mustEnv(t, rec)
			if resp.Body.InformResponse == nil || resp.Body.InformResponse.MaxEnvelopes != 1 {
				t.Fatalf("harus InformResponse: %s", rec.Body.String())
			}
			if resp.Body.Namespace() != ns {
				t.Fatalf("InformResponse harus echo namespace CPE %q, dapat %q", ns, resp.Body.Namespace())
			}
			if resp.Header == nil || resp.Header.ID == nil || resp.Header.ID.Value != "cpe-id-77" {
				t.Fatalf("Header ID CPE harus di-echo balik: %+v", resp.Header)
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/xml") {
				t.Fatalf("Content-Type = %q", got)
			}
			if cpe.cookie == nil || cpe.cookie.Value != "tok-1" || !cpe.cookie.HttpOnly {
				t.Fatalf("cookie sesi tidak dipasang benar: %+v", cpe.cookie)
			}
			if cpe.cookie.Secure != tc.https {
				t.Fatalf("Secure=%v, ingin %v (Secure hanya bila request https)", cpe.cookie.Secure, tc.https)
			}

			// 2) POST kosong: CPE siap menerima RPC -> ACS kirim Reboot
			rec = cpe.post(emptyPost, "", "")
			if rec.Code != 200 {
				t.Fatalf("POST kosong #1: HTTP %d (sesi tertutup dini? cookie tidak terkirim?) — token yg diterima NextRequest: %v", rec.Code, svc.nextTokens)
			}
			if len(svc.nextTokens) != 1 || svc.nextTokens[0] != "tok-1" {
				t.Fatalf("NextRequest harus dipanggil dgn token sesi, dapat %v", svc.nextTokens)
			}
			rpc := mustEnv(t, rec)
			if rpc.Body.Reboot == nil || rpc.Body.Reboot.CommandKey != "task-uuid-9" {
				t.Fatalf("harus Reboot dgn CommandKey=task uuid: %s", rec.Body.String())
			}
			if rpc.Body.Namespace() != ns || rpc.Header.ID.Value != "task-uuid-9" {
				t.Fatalf("ns/header RPC salah: ns=%q hdr=%+v", rpc.Body.Namespace(), rpc.Header)
			}

			// 3) CPE membalas RebootResponse
			rec = cpe.post(soapEnv(ns, hdrID, `<cwmp:RebootResponse/>`), "", "")
			if len(svc.respIn) != 1 || svc.respIn[0].SessionToken != "tok-1" || svc.respIn[0].Fault != nil {
				t.Fatalf("HandleRPCResponse harus dipanggil sekali dgn token: %+v", svc.respIn)
			}
			// tidak ada task lagi -> 204, sesi tutup
			if rec.Code != http.StatusNoContent {
				t.Fatalf("setelah respons & tanpa task: harus 204, dapat %d", rec.Code)
			}
		})
	}
}

func TestInform_PemetaanFieldKeUsecase(t *testing.T) {
	svc := &fakeSessions{}
	cpe := newCPE(t, svc, false)
	rec := cpe.post(informXML("urn:dslforum-org:cwmp-1-1"), "u", "p")
	if rec.Code != 200 || len(svc.informIn) != 1 {
		t.Fatalf("HTTP %d, panggilan=%d", rec.Code, len(svc.informIn))
	}
	in := svc.informIn[0]
	if in.DeviceOUI != "00D0D0" || in.SerialNumber != "ZTE123" || in.ProductClass != "F670L" {
		t.Fatalf("identitas device salah: %+v", in)
	}
	if in.SoftwareVersion != "V9.0.10" || in.HardwareVersion != "HW-B" {
		t.Fatalf("versi salah: sw=%q hw=%q", in.SoftwareVersion, in.HardwareVersion)
	}
	// Vendor-extension X_ACME_ConnectionRequestURL TIDAK boleh dipakai sbg CR URL.
	if in.ConnectionRequestURL != "http://203.0.113.7:58000/cr" {
		t.Fatalf("ConnectionRequestURL = %q (suffix segmen lengkap, bukan vendor-extension)", in.ConnectionRequestURL)
	}
	if in.InformUsername != "u" || in.InformPassword != "p" {
		t.Fatalf("Basic Auth tidak diteruskan: %q/%q", in.InformUsername, in.InformPassword)
	}
	if in.RemoteIP != "203.0.113.7" {
		t.Fatalf("RemoteIP = %q", in.RemoteIP)
	}
	if in.Namespace != "urn:dslforum-org:cwmp-1-1" {
		t.Fatalf("Namespace = %q", in.Namespace)
	}
	if len(in.Events) != 2 || in.Events[0].EventCode != "1 BOOT" || in.Events[1].EventCode != "6 CONNECTION REQUEST" {
		t.Fatalf("event code harus persis: %+v", in.Events)
	}
	if len(in.Parameters) != 4 {
		t.Fatalf("%d parameter", len(in.Parameters))
	}
}

func TestInform_TanpaBasicAuthDiteruskanKosong(t *testing.T) {
	svc := &fakeSessions{informErr: domain.ErrUnauthorized}
	cpe := newCPE(t, svc, false)
	rec := cpe.post(informXML(cwmpxml.NSCWMP), "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("HTTP %d, ingin 401", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") != `Basic realm="ACS"` {
		t.Fatalf("WWW-Authenticate = %q (CPE butuh tantangan untuk mengulang dgn kredensial)", rec.Header().Get("WWW-Authenticate"))
	}
	if svc.informIn[0].InformUsername != "" {
		t.Fatal("username harus kosong")
	}
	if cpe.cookie != nil {
		t.Fatalf("Inform ditolak tidak boleh memasang cookie sesi: %+v", cpe.cookie)
	}
}

// Percobaan pertama CPE tanpa kredensial = tantangan normal (tidak boleh WARN); kredensial yang DIBERIKAN lalu ditolak = WARN.
func TestInform_LogUnauthorized_TantanganNormalBukanWarn(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cpe := newCPEWithLog(t, &fakeSessions{informErr: domain.ErrUnauthorized}, false, log)

	if rec := cpe.post(informXML(cwmpxml.NSCWMP), "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("HTTP %d, ingin 401", rec.Code)
	}
	out := buf.String()
	if strings.Contains(out, "level=WARN") || !strings.Contains(out, "level=DEBUG") || !strings.Contains(out, "tanpa kredensial") {
		t.Fatalf("tantangan tanpa kredensial harus DEBUG, bukan WARN:\n%s", out)
	}

	buf.Reset()
	if rec := cpe.post(informXML(cwmpxml.NSCWMP), "depok", "salah"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("HTTP %d, ingin 401", rec.Code)
	}
	out = buf.String()
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "kredensial salah") {
		t.Fatalf("kredensial yang diberikan lalu ditolak harus WARN:\n%s", out)
	}
	if strings.Contains(out, "depok") {
		t.Fatalf("username tidak boleh masuk log:\n%s", out)
	}
}

func TestInform_ErrorSistem500DanTanpaBocorDetail(t *testing.T) {
	svc := &fakeSessions{informErr: errors.New("dial tcp 10.0.0.5:3306: connection refused")}
	rec := newCPE(t, svc, false).post(informXML(cwmpxml.NSCWMP), "u", "p")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("HTTP %d, ingin 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Fatal("detail infrastruktur bocor ke CPE")
	}
}

func TestRequestRusak(t *testing.T) {
	svc := &fakeSessions{}
	cpe := newCPE(t, svc, false)
	for name, body := range map[string]string{
		"bukan XML":       "garbage",
		"XML tak lengkap": "<soapenv:Envelope",
		"root bukan SOAP": "<html/>",
	} {
		if rec := cpe.post(body, "u", "p"); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: HTTP %d, ingin 400", name, rec.Code)
		}
	}
	if len(svc.informIn) != 0 || len(svc.respIn) != 0 || len(svc.nextTokens) != 0 {
		t.Fatal("request rusak tidak boleh menyentuh usecase")
	}
}

func TestPOSTKosongTanpaCookie204TanpaPanggilUsecase(t *testing.T) {
	svc := &fakeSessions{}
	rec := newCPE(t, svc, false).post(emptyPost, "", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("HTTP %d, ingin 204", rec.Code)
	}
	if len(svc.nextTokens) != 0 {
		t.Fatal("tanpa token tidak boleh memanggil NextRequest")
	}
}
