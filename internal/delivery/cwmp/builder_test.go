package cwmp

import (
	"testing"

	"acs/internal/domain"
)

// TestBuildRequestBodyUsesGivenNamespace memverifikasi BuildRequestBody
// menaruh namespace (ns) yang diberikan pemanggil ke XMLName elemen RPC yang
// dibangun, fallback ke cwmpxml.NSCWMP hanya bila ns kosong — lihat
// pkg/cwmpxml/envelope.go#RPCName & handler.go soal dari mana ns berasal
// (namespace yang dideklarasikan CPE ybs sendiri pada sesi ini).
func TestBuildRequestBodyUsesGivenNamespace(t *testing.T) {
	const ns = "urn:dslforum-org:cwmp-1-0"

	t.Run("GetParameterValues memakai ns yang diberikan", func(t *testing.T) {
		body, err := BuildRequestBody(domain.TaskTypeGetParameterValues, "uuid-1",
			[]byte(`{"names":["Device.DeviceInfo.SerialNumber"]}`), ns)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if body.GetParameterValues == nil {
			t.Fatal("GetParameterValues nil")
		}
		if got := body.GetParameterValues.XMLName.Space; got != ns {
			t.Errorf("XMLName.Space = %q, want %q", got, ns)
		}
	})

	t.Run("Reboot fallback ke NSCWMP default saat ns kosong", func(t *testing.T) {
		body, err := BuildRequestBody(domain.TaskTypeReboot, "uuid-2", nil, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if body.Reboot == nil {
			t.Fatal("Reboot nil")
		}
		if got := body.Reboot.XMLName.Space; got == "" {
			t.Errorf("XMLName.Space kosong, want fallback ke NSCWMP default")
		}
	})

	t.Run("tipe task tidak dikenal -> error", func(t *testing.T) {
		if _, err := BuildRequestBody("TIDAK_DIKENAL", "uuid-3", nil, ns); err == nil {
			t.Fatal("expected error untuk tipe task tidak dikenal")
		}
	})
}
