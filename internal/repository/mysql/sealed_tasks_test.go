package mysql

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	taskPwdName  = "InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.PreSharedKey.1.KeyPassphrase"
	taskSSIDName = "InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID"
)

func TestSealTaskValues(t *testing.T) {
	s := testSealer(t)
	raw := `{"values":{"` + taskPwdName + `":"wifi-rahasia-123","` + taskSSIDName + `":"Rumah"},"extra":1}`

	out, changed, err := sealTaskValues(s, raw)
	if err != nil || !changed {
		t.Fatalf("seal: changed=%v err=%v", changed, err)
	}
	if strings.Contains(out, "wifi-rahasia-123") {
		t.Fatalf("plaintext masih ada: %s", out)
	}
	var top struct {
		Values map[string]string `json:"values"`
		Extra  int               `json:"extra"`
	}
	if err := json.Unmarshal([]byte(out), &top); err != nil {
		t.Fatalf("hasil bukan JSON valid: %v", err)
	}
	if top.Extra != 1 || top.Values[taskSSIDName] != "Rumah" {
		t.Fatalf("field lain/nilai tak sensitif harus tetap: %+v", top)
	}
	got, err := openValue(s, top.Values[taskPwdName])
	if err != nil || got != "wifi-rahasia-123" {
		t.Fatalf("nilai harus bisa dibuka lagi: %q %v", got, err)
	}

	// idempoten: sudah tersegel -> tidak ada perubahan
	if again, changed, err := sealTaskValues(s, out); err != nil || changed || again != out {
		t.Fatalf("harus idempoten: changed=%v err=%v", changed, err)
	}
}

func TestSealTaskValues_NoOp(t *testing.T) {
	s := testSealer(t)
	for _, raw := range []string{
		`{"values":{"` + taskSSIDName + `":"Rumah"}}`, // tidak ada nilai sensitif
		`{"values":{"` + taskPwdName + `":""}}`,       // nilai kosong
		`{"names":["A.B"]}`,                           // bukan SET_PARAMETER_VALUES
		`{"values":{"a":1}}`,                          // nilai bukan string
		`{"values":{}}`,
		`bukan json`,
		`null`,
	} {
		if out, changed, err := sealTaskValues(s, raw); err != nil || changed || out != raw {
			t.Fatalf("%s harus tak berubah: out=%q changed=%v err=%v", raw, out, changed, err)
		}
	}
}
