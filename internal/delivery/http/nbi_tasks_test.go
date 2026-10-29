package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
)

func httpStatusOf(err error) int {
	var he *echo.HTTPError
	if e, ok := err.(*echo.HTTPError); ok {
		he = e
	}
	if he == nil {
		return 0
	}
	return he.Code
}

func TestParseNBITask_Valid(t *testing.T) {
	typ, p, err := parseNBITask([]byte(`{"name":"setParameterValues","parameterValues":[["InternetGatewayDevice.A.SSID","Rumah","xsd:string"],["InternetGatewayDevice.A.Channel",6],["InternetGatewayDevice.A.On",true]]}`))
	if err != nil || typ != domain.TaskTypeSetParameterValues {
		t.Fatalf("typ=%q err=%v", typ, err)
	}
	vals := p["values"].(map[string]string)
	if vals["InternetGatewayDevice.A.SSID"] != "Rumah" || vals["InternetGatewayDevice.A.Channel"] != "6" || vals["InternetGatewayDevice.A.On"] != "true" || len(vals) != 3 {
		t.Errorf("values = %v", vals)
	}
	if ty := p["types"].(map[string]string); len(ty) != 1 || ty["InternetGatewayDevice.A.SSID"] != "xsd:string" {
		t.Errorf("types = %v", ty)
	}

	typ, p, err = parseNBITask([]byte(`{"name":"getParameterValues","parameterNames":["InternetGatewayDevice.DeviceInfo.UpTime"]}`))
	if err != nil || typ != domain.TaskTypeGetParameterValues || len(p["names"].([]string)) != 1 {
		t.Errorf("get: %q %v %v", typ, p, err)
	}
	if typ, _, err = parseNBITask([]byte(`{"name":"reboot"}`)); err != nil || typ != domain.TaskTypeReboot {
		t.Errorf("reboot: %q %v", typ, err)
	}
}

func TestParseNBITask_Ditolak(t *testing.T) {
	bad := map[string]string{
		"bukan JSON":           `nope`,
		"data sisa":            `{"name":"reboot"} {"x":1}`,
		"tanpa name":           `{}`,
		"factoryReset (belum)": `{"name":"factoryReset"}`,
		"download (belum)":     `{"name":"download","file":"x"}`,
		"set tanpa nilai":      `{"name":"setParameterValues"}`,
		"set dengan names":     `{"name":"setParameterValues","parameterNames":["a"],"parameterValues":[["a","b"]]}`,
		"get tanpa names":      `{"name":"getParameterValues"}`,
		"get dengan values":    `{"name":"getParameterValues","parameterNames":["a"],"parameterValues":[["a","b"]]}`,
		"reboot dengan names":  `{"name":"reboot","parameterNames":["a"]}`,
		"pasangan 1 elemen":    `{"name":"setParameterValues","parameterValues":[["a"]]}`,
		"pasangan 4 elemen":    `{"name":"setParameterValues","parameterValues":[["a","b","xsd:string","x"]]}`,
		"nama bukan string":    `{"name":"setParameterValues","parameterValues":[[1,"b"]]}`,
		"nama kosong":          `{"name":"setParameterValues","parameterValues":[["","b"]]}`,
		"nilai objek":          `{"name":"setParameterValues","parameterValues":[["a",{"x":1}]]}`,
		"nilai null":           `{"name":"setParameterValues","parameterValues":[["a",null]]}`,
		"tipe xsd tak dikenal": `{"name":"setParameterValues","parameterValues":[["a","b","xsd:ngawur"]]}`,
		"nama ganda":           `{"name":"setParameterValues","parameterValues":[["a","1"],["a","2"]]}`,
	}
	for why, body := range bad {
		_, _, err := parseNBITask([]byte(body))
		if err == nil || httpStatusOf(err) != http.StatusBadRequest {
			t.Errorf("%s: err=%v, mau 400", why, err)
		}
	}
	// pesan menyebut yang didukung
	_, _, err := parseNBITask([]byte(`{"name":"factoryReset"}`))
	if err == nil || !strings.Contains(err.Error(), nbiSupportedTasks) {
		t.Errorf("pesan harus menyebut tugas yang didukung: %v", err)
	}
	// terlalu banyak pasangan
	var sb strings.Builder
	sb.WriteString(`{"name":"setParameterValues","parameterValues":[`)
	for i := 0; i < 101; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`["P` + strings.Repeat("x", i) + `","v"]`)
	}
	sb.WriteString(`]}`)
	if _, _, err := parseNBITask([]byte(sb.String())); err == nil || httpStatusOf(err) != http.StatusBadRequest {
		t.Errorf("101 pasangan harus 400: %v", err)
	}
}

func TestNBITaskDoc_SetMenyamarkanRahasiaDanTipeIkut(t *testing.T) {
	params, _ := json.Marshal(map[string]any{
		"values": map[string]string{"InternetGatewayDevice.W.1.SSID": "Rumah", "InternetGatewayDevice.W.1.KeyPassphrase": "enc:v1:TERSEGEL", "InternetGatewayDevice.W.1.PreSharedKey.1.PreSharedKey": "rahasia-mentah"},
		"types":  map[string]string{"InternetGatewayDevice.W.1.SSID": "xsd:string"},
	})
	tk := &domain.Task{TaskUUID: "task-uuid-1", Parameters: domain.JSONRawMessage(params)}
	tk.CreatedAt = time.Date(2026, 10, 29, 7, 0, 0, 0, time.UTC)
	doc := nbiTaskDoc(tk, "dev-uuid-1", domain.TaskTypeSetParameterValues, domain.TaskStatusPending)
	raw, _ := json.Marshal(doc)
	for _, leak := range []string{"TERSEGEL", "rahasia-mentah"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("dokumen membocorkan nilai rahasia %q: %s", leak, raw)
		}
	}
	if doc["_id"] != "task-uuid-1" || doc["device"] != "dev-uuid-1" || doc["name"] != "setParameterValues" || doc["status"] != "PENDING" || doc["timestamp"] != "2026-10-29T07:00:00Z" {
		t.Errorf("doc = %v", doc)
	}
	pvs := doc["parameterValues"].([][]string)
	if len(pvs) != 3 {
		t.Fatalf("pvs = %v", pvs)
	}
	for _, row := range pvs {
		switch row[0] {
		case "InternetGatewayDevice.W.1.SSID":
			if row[1] != "Rumah" || len(row) != 3 || row[2] != "xsd:string" {
				t.Errorf("SSID row = %v", row)
			}
		default:
			if row[1] != domain.RedactedValue {
				t.Errorf("nilai %s harus disamarkan: %v", row[0], row)
			}
		}
	}
	if _, has := doc["fault"]; has {
		t.Error("tugas non-gagal tak punya fault")
	}
}

func TestNBITaskDoc_GetRebootDanFault(t *testing.T) {
	params, _ := json.Marshal(map[string]any{"names": []string{"A.B"}})
	msg := "CPE menolak"
	tk := &domain.Task{TaskUUID: "t", Parameters: domain.JSONRawMessage(params), ErrorMessage: &msg}
	doc := nbiTaskDoc(tk, "d", domain.TaskTypeGetParameterValues, domain.TaskStatusFailed)
	if n := doc["parameterNames"].([]string); len(n) != 1 || n[0] != "A.B" {
		t.Errorf("names = %v", doc["parameterNames"])
	}
	if f, ok := doc["fault"].(map[string]any); !ok || f["message"] != "CPE menolak" {
		t.Errorf("fault = %v", doc["fault"])
	}
	rb := nbiTaskDoc(&domain.Task{TaskUUID: "t"}, "d", domain.TaskTypeReboot, domain.TaskStatusPending)
	if rb["name"] != "reboot" || rb["parameterValues"] != nil || rb["parameterNames"] != nil {
		t.Errorf("reboot doc = %v", rb)
	}
	if nbiTaskName("FACTORY_RESET") != "FACTORY_RESET" {
		t.Error("jenis lain: kode apa adanya")
	}
	if !errIsNotFound(nbiNotFoundOnForbidden(domain.ErrForbidden)) || errIsNotFound(nbiNotFoundOnForbidden(domain.ErrConflict)) {
		t.Error("hanya Forbidden yang dipetakan ke NotFound")
	}
}

func errIsNotFound(err error) bool { return err == domain.ErrNotFound }
