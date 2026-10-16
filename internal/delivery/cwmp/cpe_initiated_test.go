package cwmp

import (
	"net/http"
	"strings"
	"testing"
)

// RPC yang diinisiasi CPE dijawab langsung dan TIDAK diteruskan sebagai "respons" task.
func TestGetRPCMethodsDariCPE_DijawabTanpaMenyentuhTask(t *testing.T) {
	svc := &fakeSessions{}
	cpe := openSession(t, svc)
	rec := cpe.post(soapEnv(nsCWMP10, hdrID, `<cwmp:GetRPCMethods/>`), "", "")
	out := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(out, "GetRPCMethodsResponse") ||
		!strings.Contains(out, "GetRPCMethods") || !strings.Contains(out, "TransferComplete") || !strings.Contains(out, "cpe-id-77") {
		t.Fatalf("HTTP %d, balasan GetRPCMethodsResponse tidak benar: %s", rec.Code, out)
	}
	if len(svc.respIn) != 0 {
		t.Fatalf("GetRPCMethods dari CPE tidak boleh jadi respons task: %+v", svc.respIn)
	}
}

func TestKickedDanRequestDownload_DitolakFault9000(t *testing.T) {
	for _, body := range []string{
		`<cwmp:Kicked><Command>x</Command><Referer>r</Referer><Arg>a</Arg><Next>n</Next></cwmp:Kicked>`,
		`<cwmp:RequestDownload><FileType>1 Firmware Upgrade Image</FileType></cwmp:RequestDownload>`,
	} {
		svc := &fakeSessions{}
		rec := openSession(t, svc).post(soapEnv(nsCWMP10, hdrID, body), "", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "9000") {
			t.Errorf("harus Fault 9000: HTTP %d %s", rec.Code, rec.Body.String())
		}
		if len(svc.respIn) != 0 {
			t.Errorf("tidak boleh diteruskan sebagai respons task: %+v", svc.respIn)
		}
	}
}

// cwmp:ID pada respons CPE diteruskan agar task yang dijawab bisa dikorelasikan.
func TestRespons_MembawaCWMPID(t *testing.T) {
	svc := &fakeSessions{}
	openSession(t, svc).post(soapEnv(nsCWMP10, hdrID, `<cwmp:SetParameterValuesResponse><Status>0</Status></cwmp:SetParameterValuesResponse>`), "", "")
	if len(svc.respIn) != 1 || svc.respIn[0].CWMPID != "cpe-id-77" {
		t.Fatalf("CWMPID tidak diteruskan: %+v", svc.respIn)
	}
}
