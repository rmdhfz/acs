package cwmp

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"acs/pkg/cwmpxml"
)

// Uji regresi dari capture sesi NYATA: setiap berkas NDJSON di testdata/captures/ (hasil
// GET /devices/{id}/captures/export — nilai rahasia sudah disamarkan) diputar ulang terhadap handler CWMP, sehingga kuirk
// firmware yang pernah ditemukan di lapangan menjadi tes otomatis. Cara menambah fixture:
//
//  1. Console > detail perangkat > tab Capture > Nyalakan, picu Inform ("Refresh ONT"), lalu "Unduh NDJSON".
//  2. Simpan di testdata/captures/<sumber>-<vendor>-<keterangan>.ndjson (awalan "sim-" = hasil simulator, "ont-" = ONT fisik).
//  3. Jalankan go test ./internal/delivery/cwmp/ — tidak perlu kode baru.
//
// Yang ditegakkan: (a) semua pesan CPE→ACS terurai & Inform diterima (200 + InformResponse); (b) semua pesan ACS→CPE adalah
// SOAP valid dengan header ID, dan SetParameterValues memakai atribut tipe xsi:type literal (akar Fault 9002 pada ZTE F660,
// T-07) serta ParameterKey <= 32 karakter (batas TR-069).

type capturedMsg struct {
	FromCPE bool   `json:"from_cpe"`
	RPCName string `json:"rpc_name"`
	Body    string `json:"body"`
	Status  *int   `json:"http_status"`
}

func loadCapture(t *testing.T, path string) []capturedMsg {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []capturedMsg
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for n := 1; sc.Scan(); n++ {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var m capturedMsg
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("%s baris %d bukan JSON capture: %v", filepath.Base(path), n, err)
		}
		out = append(out, m)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

var (
	valueTag     = regexp.MustCompile(`<(?:[\w.-]+:)?Value\b([^>]*)>`)
	parameterKey = regexp.MustCompile(`<(?:[\w.-]+:)?ParameterKey>([^<]*)</`)
)

func TestRealCaptures_Replay(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "captures", "*.ndjson"))
	if len(files) == 0 {
		t.Skip("belum ada fixture capture di testdata/captures")
	}
	for _, file := range files {
		file := file
		t.Run(filepath.Base(file), func(t *testing.T) {
			msgs := loadCapture(t, file)
			if len(msgs) == 0 {
				t.Fatal("fixture kosong")
			}
			informs, outbound := 0, 0
			for i, m := range msgs {
				if m.FromCPE {
					if strings.TrimSpace(m.Body) == "" {
						continue // POST kosong
					}
					if _, err := cwmpxml.Unmarshal([]byte(m.Body)); err != nil {
						t.Fatalf("pesan %d (%s) dari CPE tidak terurai: %v", i, m.RPCName, err)
					}
					if m.RPCName == "Inform" {
						informs++
						svc := &fakeSessions{}
						rec := newCPE(t, svc, false).post(m.Body, "u", "p")
						if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "InformResponse") {
							t.Fatalf("Inform pesan %d ditolak: %d %s", i, rec.Code, rec.Body.String())
						}
						if len(svc.informIn) != 1 || svc.informIn[0].SerialNumber == "" || svc.informIn[0].DeviceOUI == "" {
							t.Fatalf("Inform pesan %d: identitas tidak terbaca: %+v", i, svc.informIn)
						}
					}
					continue
				}
				if strings.TrimSpace(m.Body) == "" {
					continue // 204 / tanpa body
				}
				outbound++
				env, err := cwmpxml.Unmarshal([]byte(m.Body))
				if err != nil {
					t.Fatalf("pesan %d (%s) dari ACS bukan SOAP valid: %v", i, m.RPCName, err)
				}
				if env.Header == nil || env.Header.ID == nil || env.Header.ID.Value == "" {
					t.Errorf("pesan %d (%s) dari ACS tanpa header cwmp:ID", i, m.RPCName)
				}
				if strings.Contains(m.Body, "_XMLSchema-instance") {
					t.Errorf("pesan %d (%s): atribut tipe memakai _XMLSchema-instance (ditolak firmware ketat, Fault 9002)", i, m.RPCName)
				}
				if m.RPCName == "SetParameterValues" {
					vals := valueTag.FindAllStringSubmatch(m.Body, -1)
					if len(vals) == 0 {
						t.Errorf("pesan %d: SetParameterValues tanpa <Value>", i)
					}
					for _, v := range vals {
						if !strings.Contains(v[1], "xsi:type=") {
							t.Errorf("pesan %d: <Value%s> tanpa atribut xsi:type literal", i, v[1])
						}
					}
					if k := parameterKey.FindStringSubmatch(m.Body); k != nil && len(k[1]) > 32 {
						t.Errorf("pesan %d: ParameterKey %d karakter (> 32): %q", i, len(k[1]), k[1])
					}
				}
			}
			if informs == 0 || outbound == 0 {
				t.Fatalf("fixture tak berguna: %d Inform, %d pesan keluar ACS", informs, outbound)
			}
		})
	}
}
