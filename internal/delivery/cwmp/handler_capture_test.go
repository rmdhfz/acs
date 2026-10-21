package cwmp

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
)

type recorded struct {
	device  uint64
	token   string
	fromCPE bool
	body    string
	status  int
}

type fakeCapturer struct {
	mu       sync.Mutex
	any      bool
	devices  map[uint64]bool
	tokenDev map[string]uint64
	rows     []recorded
	anyCalls int
}

func (f *fakeCapturer) AnyActive(context.Context) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.anyCalls++
	return f.any
}
func (f *fakeCapturer) Active(_ context.Context, id uint64) bool { return f.devices[id] }
func (f *fakeCapturer) DeviceIDForToken(_ context.Context, tok string) (uint64, bool) {
	id, ok := f.tokenDev[tok]
	return id, ok
}
func (f *fakeCapturer) Record(_ context.Context, id uint64, tok string, fromCPE bool, raw []byte, st int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, recorded{id, tok, fromCPE, string(raw), st})
	return nil
}

func newCapturedCPE(t *testing.T, svc SessionService, cap Capturer) *mockCPE {
	e := echo.New()
	NewHandler(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).WithCapture(cap).Register(e, "/cwmp")
	return &mockCPE{t: t, e: e}
}

func TestCapture_InformDanPostKosongTerekamKeduaArah(t *testing.T) {
	svc := &fakeSessions{}
	cap := &fakeCapturer{any: true, devices: map[uint64]bool{42: true}, tokenDev: map[string]uint64{"tok-1": 42}}
	cpe := newCapturedCPE(t, svc, cap)

	// Inform -> (request CPE + InformResponse ACS) terekam dengan device dari hasil HandleInform
	rec := cpe.post(informXML("urn:dslforum-org:cwmp-1-0"), "u", "p")
	if rec.Code != http.StatusOK {
		t.Fatalf("inform: %d", rec.Code)
	}
	// POST kosong (sesi lanjut) -> token dari cookie, device dicari lewat DeviceIDForToken
	if rec := cpe.post(emptyPost, "", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("post kosong: %d", rec.Code)
	}
	if len(cap.rows) != 4 {
		t.Fatalf("harus 4 baris (2 pasang), dapat %d: %+v", len(cap.rows), cap.rows)
	}
	in, out := cap.rows[0], cap.rows[1]
	if !in.fromCPE || in.device != 42 || !strings.Contains(in.body, "<cwmp:Inform>") || in.status != 0 {
		t.Fatalf("baris request Inform salah: %+v", in)
	}
	if out.fromCPE || out.status != http.StatusOK || !strings.Contains(out.body, "InformResponse") || out.token != "tok-1" {
		t.Fatalf("baris response InformResponse salah: %+v", out)
	}
	empty := cap.rows[3]
	if empty.fromCPE || empty.status != http.StatusNoContent || empty.body != "" || empty.device != 42 {
		t.Fatalf("baris respons 204 salah: %+v", empty)
	}
}

func TestCapture_TidakAktifTidakMerekamDanJalurCepat(t *testing.T) {
	svc := &fakeSessions{}
	// tidak ada capture di mana pun: tidak ada lookup, tidak ada rekaman
	cap := &fakeCapturer{any: false, devices: map[uint64]bool{42: true}}
	cpe := newCapturedCPE(t, svc, cap)
	cpe.post(informXML("urn:dslforum-org:cwmp-1-0"), "u", "p")
	if len(cap.rows) != 0 {
		t.Fatalf("tanpa capture aktif tidak boleh merekam: %+v", cap.rows)
	}
	// ada capture di perangkat LAIN: perangkat ini tidak direkam
	cap2 := &fakeCapturer{any: true, devices: map[uint64]bool{99: true}}
	cpe2 := newCapturedCPE(t, &fakeSessions{}, cap2)
	cpe2.post(informXML("urn:dslforum-org:cwmp-1-0"), "u", "p")
	if len(cap2.rows) != 0 {
		t.Fatalf("perangkat lain tidak boleh direkam: %+v", cap2.rows)
	}
}

func TestCapture_InformDitolakTidakDirekam(t *testing.T) {
	// 401 sebelum perangkat dikenal: tidak ada device id -> tidak ada rekaman (dan tidak panik)
	svc := &fakeSessions{informErr: domain.ErrUnauthorized}
	cap := &fakeCapturer{any: true, devices: map[uint64]bool{42: true}, tokenDev: map[string]uint64{}}
	cpe := newCapturedCPE(t, svc, cap)
	if rec := cpe.post(informXML("urn:dslforum-org:cwmp-1-0"), "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("harus 401: %d", rec.Code)
	}
	if len(cap.rows) != 0 {
		t.Fatalf("Inform ditolak tidak boleh direkam: %+v", cap.rows)
	}
}
