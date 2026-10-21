package task

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"acs/internal/domain"
	"acs/pkg/cryptoutil"
)

func testEnc(t *testing.T, fill byte) *cryptoutil.Encryptor {
	t.Helper()
	e, err := cryptoutil.NewEncryptor(bytes.Repeat([]byte{fill}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

const wlan = "InternetGatewayDevice.LANDevice.1.WLANConfiguration.2."

func TestSealOpen_RoundTripDanTidakAdaPlaintext(t *testing.T) {
	enc := testEnc(t, 'a')
	in := map[string]interface{}{"values": map[string]string{
		wlan + "SSID":                         "Rumah Budi",
		wlan + "PreSharedKey.1.KeyPassphrase": "rahasia-12345",
		wlan + "X_ZTE-COM_MaxUserNum":         "16",
	}}
	sealed, err := SealSecrets(enc, in)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(sealed)
	if bytes.Contains(raw, []byte("rahasia-12345")) {
		t.Fatalf("plaintext password masih ada: %s", raw)
	}
	if !bytes.Contains(raw, []byte(sealedPrefix)) || !bytes.Contains(raw, []byte("Rumah Budi")) || !bytes.Contains(raw, []byte(`"16"`)) {
		t.Fatalf("hanya nilai sensitif yang boleh dienkripsi: %s", raw)
	}
	// input asli tidak dimutasi
	if in["values"].(map[string]string)[wlan+"PreSharedKey.1.KeyPassphrase"] != "rahasia-12345" {
		t.Fatal("SealSecrets memutasi params asli")
	}

	opened, err := OpenSecrets(enc, raw)
	if err != nil {
		t.Fatal(err)
	}
	var back struct {
		Values map[string]string `json:"values"`
	}
	if err := json.Unmarshal(opened, &back); err != nil {
		t.Fatal(err)
	}
	if back.Values[wlan+"PreSharedKey.1.KeyPassphrase"] != "rahasia-12345" || back.Values[wlan+"SSID"] != "Rumah Budi" {
		t.Fatalf("round-trip salah: %+v", back.Values)
	}
}

func TestSeal_TanpaPerubahanBilaTidakPerlu(t *testing.T) {
	enc := testEnc(t, 'a')
	plainOnly := map[string]interface{}{"values": map[string]string{wlan + "SSID": "A", wlan + "Enable": "1"}}
	if got, _ := SealSecrets(enc, plainOnly); !sameJSON(got, plainOnly) {
		t.Fatal("tanpa nilai sensitif, params tidak boleh berubah")
	}
	emptySecret := map[string]interface{}{"values": map[string]string{wlan + "PreSharedKey.1.KeyPassphrase": ""}}
	if got, _ := SealSecrets(enc, emptySecret); !sameJSON(got, emptySecret) {
		t.Fatal("nilai rahasia kosong tidak dienkripsi")
	}
	withSecret := map[string]interface{}{"values": map[string]string{wlan + "PreSharedKey.1.KeyPassphrase": "x12345678"}}
	if got, _ := SealSecrets(nil, withSecret); !sameJSON(got, withSecret) {
		t.Fatal("tanpa kunci (enc nil) params dikembalikan apa adanya")
	}
	if got, _ := SealSecrets(enc, map[string]interface{}{"names": []string{"A.B"}}); !sameJSON(got, map[string]interface{}{"names": []string{"A.B"}}) {
		t.Fatal("task selain SET tidak disentuh")
	}
	// idempoten: yang sudah terenkripsi tidak dienkripsi dua kali
	once, _ := SealSecrets(enc, withSecret)
	twice, _ := SealSecrets(enc, once)
	if !sameJSON(once, twice) {
		t.Fatal("SealSecrets harus idempoten")
	}
}

func TestOpen_TanpaKunciAtauKunciSalahGagalAman(t *testing.T) {
	sealed, _ := SealSecrets(testEnc(t, 'a'), map[string]interface{}{"values": map[string]string{wlan + "PreSharedKey.1.KeyPassphrase": "x12345678"}})
	raw, _ := json.Marshal(sealed)
	if _, err := OpenSecrets(nil, raw); !errors.Is(err, ErrSealedNoKey) {
		t.Fatalf("tanpa kunci harus ErrSealedNoKey: %v", err)
	}
	if _, err := OpenSecrets(testEnc(t, 'b'), raw); !errors.Is(err, ErrSealedNoKey) {
		t.Fatalf("kunci salah harus ErrSealedNoKey: %v", err)
	}
	// tanpa penanda: jalur cepat, tidak butuh kunci
	plain := []byte(`{"values":{"A.B":"1"}}`)
	if got, err := OpenSecrets(nil, plain); err != nil || string(got) != string(plain) {
		t.Fatalf("tanpa penanda harus apa adanya: %s %v", got, err)
	}
}

// API tak pernah mengembalikan nilai terenkripsi — termasuk untuk ADMIN dan SUPERADMIN.
func TestRedact_PenandaTerenkripsiTertutupUntukSemuaRole(t *testing.T) {
	sealed, _ := SealSecrets(testEnc(t, 'a'), map[string]interface{}{"values": map[string]string{wlan + "PreSharedKey.1.KeyPassphrase": "x12345678", wlan + "SSID": "A"}})
	raw, _ := json.Marshal(sealed)
	for _, roles := range [][]string{{domain.RoleSuperadmin}, {domain.RoleAdmin}, {domain.RoleNOC}} {
		p := domain.JSONRawMessage(append([]byte(nil), raw...))
		redactSensitiveParams(&p, domain.Actor{Roles: roles})
		if strings.Contains(string(p), sealedPrefix) || strings.Contains(string(p), "x12345678") {
			t.Fatalf("role %v: nilai/penanda terenkripsi bocor: %s", roles, p)
		}
		if !strings.Contains(string(p), domain.RedactedValue) || !strings.Contains(string(p), `"A"`) {
			t.Fatalf("role %v: harus [redacted] hanya untuk yang rahasia: %s", roles, p)
		}
	}
}

// CreateTask: password WiFi tidak pernah masuk tasks.parameters sebagai plaintext, dan tetap dapat dibuka untuk dikirim.
func TestCreateTask_SetParameterValuesMengenkripsiNilaiRahasia(t *testing.T) {
	const typeID, pendingID = 40, 41
	refs := &fakeRefRepo{ids: map[string]uint64{domain.TaskTypeSetParameterValues: typeID, domain.TaskStatusPending: pendingID}}
	repo := &fakeTaskRepo{}
	enc := testEnc(t, 'c')
	svc := NewService(repo, &fakeDeviceRepo{dev: &domain.Device{ID: 1, TenantID: u64(100)}}, nil, nil, nil, refs, &fakeActivityRepo{}, nil, nil, nil).WithEncryptor(enc)

	if _, err := svc.CreateTask(context.Background(), superadminActor(), domain.CreateTaskInput{
		DeviceID: 1, TaskType: domain.TaskTypeSetParameterValues,
		Parameters: map[string]interface{}{"values": map[string]string{
			wlan + "SSID":                         "Rumah Budi",
			wlan + "PreSharedKey.1.KeyPassphrase": "rahasia-12345",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("task tersimpan: %d", len(repo.created))
	}
	stored := []byte(repo.created[0].Parameters)
	if bytes.Contains(stored, []byte("rahasia-12345")) {
		t.Fatalf("password tersimpan plaintext di tasks.parameters: %s", stored)
	}
	opened, err := OpenSecrets(enc, stored)
	if err != nil || !bytes.Contains(opened, []byte("rahasia-12345")) {
		t.Fatalf("nilai harus bisa dibuka lagi untuk dikirim: %s %v", opened, err)
	}
}

func sameJSON(a, b interface{}) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
