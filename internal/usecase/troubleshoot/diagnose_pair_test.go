package troubleshoot

import (
	"strings"
	"testing"
	"time"

	"acs/internal/domain"
)

// pairEv: perangkat dengan ONU online di OLT yang melaporkan RX sisi OLT, plus satu pembacaan RX ONT. silent kecil = Reachable, besar = Offline.
func pairEv(silent time.Duration, oltRX, ontRX float64, ontAgo, oltPollAgo time.Duration) Evidence {
	e := base(silent)
	o := oltAt(domain.OLTStateOnline, time.Minute)
	o.RxPowerDBM = f64(oltRX)
	polled := t0.Add(-oltPollAgo)
	o.OLTPolledAt = &polled
	e.OLT = o
	e.Optical = []OpticalAt{{RXDBM: f64(ontRX), At: t0.Add(-ontAgo)}}
	return e
}

func TestPair_PolaKombinasiLemahDanKeparahan(t *testing.T) {
	cases := []struct {
		name         string
		oltRX, ontRX float64
		code         string
		sev          string
		inHint       string
	}{
		{"dua arah lemah (waspada)", -25.5, -26.0, "OPTICAL_BOTH_WEAK", domain.SeverityWarning, "jalur fiber yang dipakai bersama"},
		{"dua arah, satu kritis", -28.5, -26.0, "OPTICAL_BOTH_WEAK", domain.SeverityCritical, "jalur fiber yang dipakai bersama"},
		{"hanya upstream", -28.0, -20.0, "OPTICAL_UPSTREAM_WEAK", domain.SeverityCritical, "pemancar ONU"},
		{"hanya upstream waspada", -25.5, -20.0, "OPTICAL_UPSTREAM_WEAK", domain.SeverityWarning, "pemancar ONU"},
		{"hanya downstream", -20.0, -27.5, "OPTICAL_DOWNSTREAM_WEAK", domain.SeverityCritical, "path RX yang dipetakan"},
		{"overload upstream", -5.0, -20.0, "OPTICAL_UPSTREAM_WEAK", domain.SeverityCritical, "pemancar ONU"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Diagnose(pairEv(10*time.Minute, c.oltRX, c.ontRX, 30*time.Minute, 5*time.Minute))
			f := finding(t, d, c.code)
			if f.Severity != c.sev {
				t.Errorf("keparahan %q, mau %q", f.Severity, c.sev)
			}
			if !strings.Contains(f.Hint, c.inHint) {
				t.Errorf("saran %q harus memuat %q", f.Hint, c.inHint)
			}
			// bukti menyebut KEDUA angka dan arahnya (upstream/downstream), bukan selisih yang menyesatkan
			for _, want := range []string{"upstream", "downstream", "polling OLT", "dibaca"} {
				if !strings.Contains(f.Evidence, want) {
					t.Errorf("bukti %q tak memuat %q", f.Evidence, want)
				}
			}
			if !hasAction(d, domain.DiagActionOpenOptical) {
				t.Error("aksi OPEN_OPTICAL harus disarankan")
			}
		})
	}
}

func TestPair_NormalDuaArah(t *testing.T) {
	// Reachable: tidak ada temuan pasangan (tak ada yang perlu dikatakan)
	d := Diagnose(pairEv(10*time.Minute, -20.0, -19.0, 30*time.Minute, 5*time.Minute))
	for _, c := range []string{"OPTICAL_PAIR_OK", "OPTICAL_BOTH_WEAK", "OPTICAL_UPSTREAM_WEAK", "OPTICAL_DOWNSTREAM_WEAK", "OPTICAL_PAIR_NOT_COMPARABLE"} {
		if has(d, c) {
			t.Errorf("perangkat sehat tidak boleh memunculkan %s", c)
		}
	}
	// Offline tetapi ONU online di OLT: kedua arah normal membebaskan jalur optik
	d = Diagnose(pairEv(3*time.Hour, -20.0, -19.0, 3*time.Hour+10*time.Minute, 3*time.Hour))
	f := finding(t, d, "OPTICAL_PAIR_OK")
	if f.Severity != domain.SeverityInfo || !strings.Contains(f.Hint, "jalur IP/TR-069") {
		t.Errorf("OPTICAL_PAIR_OK = %+v", f)
	}
}

func TestPair_TidakSebandingBilaWaktuBerjauhan(t *testing.T) {
	// Reachable + selisih 3 jam: info, tanpa membandingkan tingkat (walau salah satu sebenarnya lemah)
	d := Diagnose(pairEv(10*time.Minute, -28.5, -20.0, 4*time.Hour, 1*time.Hour))
	f := finding(t, d, "OPTICAL_PAIR_NOT_COMPARABLE")
	if f.Severity != domain.SeverityInfo || !strings.Contains(f.Evidence, "melebihi") {
		t.Errorf("tidak sebanding = %+v", f)
	}
	if has(d, "OPTICAL_UPSTREAM_WEAK") || has(d, "OPTICAL_BOTH_WEAK") {
		t.Errorf("tidak boleh menyimpulkan dari pembacaan yang berjauhan: %v", codes(d))
	}
	// Offline + selisih besar: diam (pembacaan ONT sudah usang menurut definisi), tidak ada kebisingan
	d = Diagnose(pairEv(5*time.Hour, -28.5, -20.0, 6*time.Hour, 5*time.Minute))
	for _, c := range []string{"OPTICAL_PAIR_NOT_COMPARABLE", "OPTICAL_UPSTREAM_WEAK", "OPTICAL_PAIR_OK"} {
		if has(d, c) {
			t.Errorf("offline + berjauhan: %s tak boleh muncul", c)
		}
	}
	// tepat pada batas 2 jam masih sebanding
	d = Diagnose(pairEv(10*time.Minute, -28.5, -20.0, 2*time.Hour+5*time.Minute, 5*time.Minute))
	if !has(d, "OPTICAL_UPSTREAM_WEAK") {
		t.Errorf("selisih tepat 2 jam masih sebanding: %v", codes(d))
	}
}

func TestPair_SyaratBukti(t *testing.T) {
	// ONU tidak online di OLT (RX sisi OLT tak bermakna), tanpa RX sisi OLT, tanpa pembacaan ONT, tanpa waktu polling, tanpa OLT: tak ada temuan
	mutate := map[string]func(e *Evidence){
		"ONU LOS di OLT":      func(e *Evidence) { e.OLT.State = domain.OLTStateLOS },
		"tanpa RX OLT":        func(e *Evidence) { e.OLT.RxPowerDBM = nil },
		"tanpa waktu polling": func(e *Evidence) { e.OLT.OLTPolledAt = nil },
		"tanpa riwayat ONT":   func(e *Evidence) { e.Optical = nil },
		"RX ONT kosong":       func(e *Evidence) { e.Optical = []OpticalAt{{RXDBM: nil, At: t0}} },
		"tanpa OLT":           func(e *Evidence) { e.OLT = nil },
	}
	for name, f := range mutate {
		e := pairEv(10*time.Minute, -28.5, -29.0, 30*time.Minute, 5*time.Minute)
		f(&e)
		d := Diagnose(e)
		for _, c := range []string{"OPTICAL_BOTH_WEAK", "OPTICAL_UPSTREAM_WEAK", "OPTICAL_DOWNSTREAM_WEAK", "OPTICAL_PAIR_OK", "OPTICAL_PAIR_NOT_COMPARABLE"} {
			if has(d, c) {
				t.Errorf("%s: %s tak boleh muncul", name, c)
			}
		}
	}
}

func TestPair_PembacaanTerbaruDipakaiBukanYangLama(t *testing.T) {
	e := pairEv(10*time.Minute, -20.0, -20.0, 30*time.Minute, 5*time.Minute)
	// Optical terbaru dulu: yang terbaru normal, yang lama kritis -> tidak ada temuan lemah
	e.Optical = append(e.Optical, OpticalAt{RXDBM: f64(-29.0), At: t0.Add(-40 * time.Minute)})
	if d := Diagnose(e); has(d, "OPTICAL_BOTH_WEAK") || has(d, "OPTICAL_DOWNSTREAM_WEAK") {
		t.Errorf("pembacaan lama tak boleh dipakai: %v", codes(d))
	}
	// pembacaan terbaru kosong dilewati; yang berikutnya yang bernilai dipakai
	e.Optical = []OpticalAt{{RXDBM: nil, At: t0.Add(-10 * time.Minute)}, {RXDBM: f64(-27.5), At: t0.Add(-30 * time.Minute)}}
	if d := Diagnose(e); !has(d, "OPTICAL_DOWNSTREAM_WEAK") {
		t.Errorf("pembacaan bernilai terbaru harus dipakai: %v", codes(d))
	}
}
