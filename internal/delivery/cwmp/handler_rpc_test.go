package cwmp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/session"
	"acs/pkg/cwmpxml"
)

const nsCWMP10 = "urn:dslforum-org:cwmp-1-0"

// openSession membuka sesi lewat Inform supaya CPE punya cookie, lalu mengembalikan CPE-nya.
func openSession(t *testing.T, svc *fakeSessions) *mockCPE {
	t.Helper()
	cpe := newCPE(t, svc, true) // https agar cookie Secure pun terkirim; fokus test ini bukan cookie
	if rec := cpe.post(informXML(nsCWMP10), "u", "p"); rec.Code != 200 {
		t.Fatalf("Inform: %d", rec.Code)
	}
	return cpe
}

// ================== NextRequest ==================

func TestNext_KirimBerbagaiTipeRPCDenganNamespaceSesi(t *testing.T) {
	cases := []struct {
		name   string
		taskTy string
		params string
		method string
	}{
		{"GetParameterValues", domain.TaskTypeGetParameterValues, `{"names":["A.B","C.D"]}`, "GetParameterValues"},
		{"SetParameterValues", domain.TaskTypeSetParameterValues, `{"values":{"X.SSID":"net"}}`, "SetParameterValues"},
		{"GetParameterNames", domain.TaskTypeGetParameterNames, `{"path":"InternetGatewayDevice.","next_level":true}`, "GetParameterNames"},
		{"AddObject", domain.TaskTypeAddObject, `{"object_name":"X.PM."}`, "AddObject"},
		{"DeleteObject", domain.TaskTypeDeleteObject, `{"object_name":"X.PM.1."}`, "DeleteObject"},
		{"FactoryReset", domain.TaskTypeFactoryReset, `{}`, "FactoryReset"},
		{"Download", domain.TaskTypeDownload, `{"file_type":"1 Firmware Upgrade Image","url":"http://h/f?a=1&b=2","file_size":10}`, "Download"},
		{"Upload", domain.TaskTypeUpload, `{"file_type":"1 Vendor Configuration File","url":"http://h/u"}`, "Upload"},
		{"ScheduleInform", domain.TaskTypeScheduleInform, `{"delay_seconds":30}`, "ScheduleInform"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeSessions{nextQueue: []nextResult{{rpc: &session.OutboundRPC{
				TaskUUID: "u-" + tc.name, TaskType: tc.taskTy, Parameters: []byte(tc.params), Namespace: nsCWMP10}}}}
			rec := openSession(t, svc).post(emptyPost, "", "")
			if rec.Code != 200 {
				t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
			}
			env := mustEnv(t, rec)
			if env.Body.Method() != tc.method || env.Body.Namespace() != nsCWMP10 {
				t.Fatalf("Method=%q ns=%q", env.Body.Method(), env.Body.Namespace())
			}
			if env.Header == nil || env.Header.ID == nil || env.Header.ID.Value != "u-"+tc.name {
				t.Fatalf("header ID harus = task uuid (korelasi respons): %+v", env.Header)
			}
		})
	}
}

func TestNext_NamespaceKosongFallbackCWMP12(t *testing.T) {
	svc := &fakeSessions{nextQueue: []nextResult{{rpc: &session.OutboundRPC{TaskUUID: "u", TaskType: domain.TaskTypeReboot}}}}
	env := mustEnv(t, openSession(t, svc).post(emptyPost, "", ""))
	if env.Body.Namespace() != cwmpxml.NSCWMP {
		t.Fatalf("sesi lama tanpa namespace harus fallback %q, dapat %q", cwmpxml.NSCWMP, env.Body.Namespace())
	}
}

func TestNext_GagalDanKosong(t *testing.T) {
	t.Run("usecase error -> 500", func(t *testing.T) {
		svc := &fakeSessions{nextErr: errors.New("db down")}
		if rec := openSession(t, svc).post(emptyPost, "", ""); rec.Code != http.StatusInternalServerError {
			t.Fatalf("HTTP %d", rec.Code)
		}
	})
	t.Run("tak ada task -> 204", func(t *testing.T) {
		svc := &fakeSessions{nextQueue: []nextResult{{rpc: nil, close: false}}}
		if rec := openSession(t, svc).post(emptyPost, "", ""); rec.Code != http.StatusNoContent {
			t.Fatalf("HTTP %d", rec.Code)
		}
	})
	t.Run("closeSession -> 204 walau rpc ada", func(t *testing.T) {
		svc := &fakeSessions{nextQueue: []nextResult{{rpc: &session.OutboundRPC{TaskType: domain.TaskTypeReboot}, close: true}}}
		if rec := openSession(t, svc).post(emptyPost, "", ""); rec.Code != http.StatusNoContent {
			t.Fatalf("HTTP %d", rec.Code)
		}
	})
	// Task yang tak bisa dibangun jadi RPC TIDAK boleh membuat CPE menerima 500 dan
	// task tersangkut SENT: ditandai FAILED permanen (fault ACS-BUILD) lalu sesi
	// dilanjutkan (di sini: tak ada task lagi -> 204).
	t.Run("tipe task tak dikenal builder -> task FAILED permanen + 204", func(t *testing.T) {
		svc := &fakeSessions{nextQueue: []nextResult{{rpc: &session.OutboundRPC{TaskUUID: "u", TaskType: "TIPE_GAIB"}}}}
		if rec := openSession(t, svc).post(emptyPost, "", ""); rec.Code != http.StatusNoContent {
			t.Fatalf("HTTP %d", rec.Code)
		}
		if len(svc.respIn) != 1 || svc.respIn[0].Fault == nil || svc.respIn[0].Fault.Code != domain.FaultCodeACSBuild {
			t.Fatalf("task tak-terbangun harus dilaporkan sebagai fault ACS-BUILD: %+v", svc.respIn)
		}
	})
	t.Run("JSON parameter rusak -> task FAILED permanen + 204", func(t *testing.T) {
		svc := &fakeSessions{nextQueue: []nextResult{{rpc: &session.OutboundRPC{TaskUUID: "u", TaskType: domain.TaskTypeGetParameterValues, Parameters: []byte("{")}}}}
		if rec := openSession(t, svc).post(emptyPost, "", ""); rec.Code != http.StatusNoContent {
			t.Fatalf("HTTP %d", rec.Code)
		}
		if len(svc.respIn) != 1 || svc.respIn[0].Fault == nil || svc.respIn[0].Fault.Code != domain.FaultCodeACSBuild {
			t.Fatalf("harus fault ACS-BUILD: %+v", svc.respIn)
		}
	})
}

// ================== respons RPC dari CPE ==================

func TestRespons_TanpaCookieDitolak400(t *testing.T) {
	svc := &fakeSessions{}
	rec := newCPE(t, svc, false).post(soapEnv(nsCWMP10, "", `<cwmp:RebootResponse/>`), "", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("HTTP %d, ingin 400", rec.Code)
	}
	if len(svc.respIn) != 0 {
		t.Fatal("respons tanpa sesi tidak boleh diteruskan")
	}
}

func TestRespons_ErrorUsecase500(t *testing.T) {
	svc := &fakeSessions{respErr: errors.New("boom")}
	rec := openSession(t, svc).post(soapEnv(nsCWMP10, "", `<cwmp:SetParameterValuesResponse><Status>0</Status></cwmp:SetParameterValuesResponse>`), "", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("HTTP %d", rec.Code)
	}
}

func TestRespons_FaultDiteruskanDenganKode(t *testing.T) {
	svc := &fakeSessions{}
	body := `<soapenv:Fault><faultcode>Client</faultcode><faultstring>CWMP fault</faultstring><detail><cwmp:Fault><FaultCode>9005</FaultCode><FaultString>Invalid parameter name</FaultString></cwmp:Fault></detail></soapenv:Fault>`
	openSession(t, svc).post(soapEnv(nsCWMP10, "", body), "", "")
	if len(svc.respIn) != 1 || svc.respIn[0].Fault == nil || svc.respIn[0].Fault.Code != "9005" ||
		svc.respIn[0].Fault.Message != "Invalid parameter name" {
		t.Fatalf("Fault tidak diteruskan benar: %+v", svc.respIn)
	}
}

func TestRespons_TransferCompleteDiInitiasiCPE(t *testing.T) {
	svc := &fakeSessions{}
	cpe := openSession(t, svc)
	rec := cpe.post(soapEnv(nsCWMP10, hdrID, `<cwmp:TransferComplete><CommandKey>fw-1</CommandKey><FaultStruct><FaultCode>0</FaultCode><FaultString></FaultString></FaultStruct><StartTime>s</StartTime><CompleteTime>c</CompleteTime></cwmp:TransferComplete>`), "", "")
	if len(svc.respIn) != 1 || svc.respIn[0].TransferComplete == nil || !svc.respIn[0].TransferComplete.Success {
		t.Fatalf("TransferComplete sukses salah: %+v", svc.respIn)
	}
	// TR-069: TransferComplete (RPC dari CPE) WAJIB dijawab TransferCompleteResponse
	// dengan ID header yang sama — bukan 204 / RPC berikutnya.
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d, harus 200 TransferCompleteResponse", rec.Code)
	}
	out := rec.Body.String()
	if !strings.Contains(out, "TransferCompleteResponse") || !strings.Contains(out, "cpe-id-77") {
		t.Fatalf("balasan bukan TransferCompleteResponse ber-ID cpe-id-77: %s", out)
	}
}

// ================== buildRPCResponse ==================

func envOf(t *testing.T, body string) *cwmpxml.Envelope {
	t.Helper()
	env, err := cwmpxml.Unmarshal([]byte(soapEnv(nsCWMP10, "", body)))
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func TestBuildRPCResponse(t *testing.T) {
	t.Run("Fault tanpa detail memakai faultstring SOAP", func(t *testing.T) {
		r := buildRPCResponse("tok", envOf(t, `<soapenv:Fault><faultcode>Server</faultcode><faultstring>Internal</faultstring></soapenv:Fault>`))
		if r.Fault == nil || r.Fault.Code != "" || r.Fault.Message != "Internal" {
			t.Fatalf("%+v", r.Fault)
		}
	})
	t.Run("Fault dengan detail memakai kode/pesan CWMP", func(t *testing.T) {
		r := buildRPCResponse("tok", envOf(t, `<soapenv:Fault><faultcode>Client</faultcode><faultstring>CWMP fault</faultstring><detail><cwmp:Fault><FaultCode>9007</FaultCode><FaultString>Invalid parameter value</FaultString></cwmp:Fault></detail></soapenv:Fault>`))
		if r.Fault.Code != "9007" || r.Fault.Message != "Invalid parameter value" {
			t.Fatalf("%+v", r.Fault)
		}
	})
	t.Run("TransferComplete: kode kosong/0 = sukses, selain itu gagal", func(t *testing.T) {
		mk := func(fs string) *session.TransferCompleteInfo {
			return buildRPCResponse("t", envOf(t, `<cwmp:TransferComplete><CommandKey>k</CommandKey>`+fs+`<StartTime>s</StartTime><CompleteTime>c</CompleteTime></cwmp:TransferComplete>`)).TransferComplete
		}
		for name, tc := range map[string]struct {
			fs      string
			success bool
			msg     string
		}{
			"tanpa FaultStruct": {"", true, ""},
			"FaultCode kosong":  {`<FaultStruct><FaultCode></FaultCode><FaultString></FaultString></FaultStruct>`, true, ""},
			"FaultCode 0":       {`<FaultStruct><FaultCode>0</FaultCode><FaultString></FaultString></FaultStruct>`, true, ""},
			"FaultCode 9010":    {`<FaultStruct><FaultCode>9010</FaultCode><FaultString>Download failure</FaultString></FaultStruct>`, false, "Download failure"},
		} {
			got := mk(tc.fs)
			if got.Success != tc.success || got.ErrorMessage != tc.msg || got.CommandKey != "k" || got.StartTime != "s" || got.CompleteTime != "c" {
				t.Errorf("%s: %+v", name, got)
			}
		}
	})
	t.Run("GetParameterValuesResponse memetakan nilai + simpan RawResponse JSON", func(t *testing.T) {
		r := buildRPCResponse("t", envOf(t, `<cwmp:GetParameterValuesResponse><ParameterList><ParameterValueStruct><Name>A</Name><Value xsi:type="xsd:string">1</Value></ParameterValueStruct><ParameterValueStruct><Name>B</Name><Value xsi:type="xsd:string"/></ParameterValueStruct></ParameterList></cwmp:GetParameterValuesResponse>`))
		if len(r.ParameterValues) != 2 || r.ParameterValues[0].Name != "A" || r.ParameterValues[0].Value != "1" || r.ParameterValues[1].Value != "" {
			t.Fatalf("%+v", r.ParameterValues)
		}
		if len(r.RawResponse) == 0 || !strings.Contains(string(r.RawResponse), `"A"`) {
			t.Fatalf("RawResponse harus JSON berisi data: %s", r.RawResponse)
		}
	})
	t.Run("respons lain -> RawResponse saja", func(t *testing.T) {
		r := buildRPCResponse("t", envOf(t, `<cwmp:SetParameterValuesResponse><Status>1</Status></cwmp:SetParameterValuesResponse>`))
		if r.Fault != nil || r.TransferComplete != nil || r.ParameterValues != nil || len(r.RawResponse) == 0 {
			t.Fatalf("%+v", r)
		}
		if r.SessionToken != "t" {
			t.Fatalf("token: %q", r.SessionToken)
		}
	})
}

func TestParamSuffix(t *testing.T) {
	ps := []session.InformParameter{
		{Name: "InternetGatewayDevice.DeviceInfo.SoftwareVersion", Value: "v1"},
		{Name: "InternetGatewayDevice.ManagementServer.X_ACME_ConnectionRequestURL", Value: "evil"},
		{Name: "InternetGatewayDevice.ManagementServer.ConnectionRequestURL", Value: "ok"},
		{Name: "X", Value: "short"},
	}
	for suffix, want := range map[string]string{
		"SoftwareVersion":                        "v1",
		".ManagementServer.ConnectionRequestURL": "ok",
		"TidakAda":                               "",
		"InternetGatewayDevice.ManagementServer.ConnectionRequestURL.Lebih.Panjang.Dari.Nama": "",
	} {
		if got := paramSuffix(ps, suffix); got != want {
			t.Errorf("paramSuffix(%q) = %q, ingin %q", suffix, got, want)
		}
	}
	if paramSuffix(nil, "x") != "" {
		t.Error("nil -> kosong")
	}
}

func TestSetSessionCookie_SecureHanyaBilaHTTPS(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request)
		secure bool
	}{
		{"http polos", func(*http.Request) {}, false},
		{"X-Forwarded-Proto https", func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") }, true},
		{"X-Forwarded-Proto HTTPS (huruf besar)", func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "HTTPS") }, true},
		{"X-Forwarded-Proto http", func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "http") }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/cwmp", nil)
			tc.mutate(req)
			rec := httptest.NewRecorder()
			setSessionCookie(echo.New().NewContext(req, rec), "tok")
			cks := rec.Result().Cookies()
			if len(cks) != 1 {
				t.Fatalf("%d cookie", len(cks))
			}
			ck := cks[0]
			if ck.Secure != tc.secure || !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode || ck.MaxAge != 3600 || ck.Value != "tok" {
				t.Fatalf("atribut salah: %+v", ck)
			}
		})
	}
}
