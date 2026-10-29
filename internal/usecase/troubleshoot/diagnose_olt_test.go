package troubleshoot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"acs/internal/domain"
)

// ---- bukti sisi OLT (irisan B) ----

func oltAt(state string, seenAgo time.Duration) *domain.DiagnosisOLT {
	idx, vendor := "7", "vendor-"+state
	return &domain.DiagnosisOLT{
		OLTName: "OLT-Pusat", PonPort: "sim/1/2", ONUIndex: &idx, State: state, VendorState: &vendor,
		LastSeenAt: t0.Add(-seenAgo), LastStateChangeAt: t0.Add(-seenAgo - time.Hour),
	}
}

func TestDiagnose_OLT_KeadaanMenjadiKesimpulanFisik(t *testing.T) {
	cases := []struct {
		state    string
		code     string
		severity string
		inTitle  string
		inHint   string
	}{
		{domain.OLTStateLOS, "OLT_LOS", domain.SeverityCritical, "kehilangan sinyal optik", "Jalur fiber"},
		{domain.OLTStateDyingGasp, "OLT_DYING_GASP", domain.SeverityCritical, "dying gasp", "listrik"},
		{domain.OLTStateOffline, "OLT_ONU_OFFLINE", domain.SeverityWarning, "offline di OLT", "tidak merinci"},
		{domain.OLTStateOnline, "OLT_ONLINE_ACS_SILENT", domain.SeverityWarning, "online di OLT, tetapi tidak terlihat di ACS", "IP/VLAN manajemen"},
		{domain.OLTStateUnknown, "OLT_STATE_UNKNOWN", domain.SeverityInfo, "tidak diketahui", "tidak memetakan"},
	}
	for _, c := range cases {
		t.Run(c.state, func(t *testing.T) {
			e := base(3 * time.Hour)
			e.OLT = oltAt(c.state, 2*time.Minute)
			d := Diagnose(e)
			f := finding(t, d, c.code)
			if f.Severity != c.severity || !strings.Contains(f.Title, c.inTitle) || !strings.Contains(f.Hint, c.inHint) {
				t.Errorf("temuan = %+v", f)
			}
			for _, must := range []string{"OLT OLT-Pusat", "port sim/1/2", "ONU 7", "keadaan " + c.state, "data OLT 2 menit lalu"} {
				if !strings.Contains(f.Evidence, must) {
					t.Errorf("evidence tak memuat %q: %s", must, f.Evidence)
				}
			}
			if d.OLT == nil || d.OLT.PonPort != "sim/1/2" {
				t.Errorf("posisi OLT harus diteruskan ke respons: %+v", d.OLT)
			}
			if d.Limits != domain.DiagnosisLimitsWithOLT {
				t.Errorf("limits harus memuat data OLT: %q", d.Limits)
			}
		})
	}
}

func TestDiagnose_OLT_LosMenjadiDugaanUtamaDanMendahuluiOptikACS(t *testing.T) {
	e := base(3 * time.Hour)
	e.OLT = oltAt(domain.OLTStateLOS, time.Minute)
	e.Optical = []OpticalAt{{RXDBM: f64(-29.1), At: t0.Add(-4 * time.Hour)}} // juga kritis dari sisi ACS
	d := Diagnose(e)
	if d.Findings[0].Code != "OLT_LOS" {
		t.Fatalf("laporan OLT harus lebih dulu dari temuan ACS sekelas: %v", codes(d))
	}
	if !strings.HasPrefix(d.Summary, "Dugaan utama: OLT melaporkan ONU kehilangan sinyal optik (LOS)") {
		t.Errorf("summary = %q", d.Summary)
	}
}

func TestDiagnose_OLT_OnlineTapiSenyapMembebaskanLapisanFisik(t *testing.T) {
	e := base(3 * time.Hour)
	e.OLT = oltAt(domain.OLTStateOnline, time.Minute)
	d := Diagnose(e)
	if !strings.HasPrefix(d.Summary, "Dugaan utama: ONU online di OLT, tetapi tidak terlihat di ACS") {
		t.Errorf("summary = %q", d.Summary)
	}
	if strings.Contains(finding(t, d, "OLT_ONLINE_ACS_SILENT").Evidence, "belum pernah") {
		t.Error("perangkat yang pernah Inform tak boleh disebut belum pernah Inform")
	}

	// perangkat baru: ONU online di OLT tetapi ACS belum pernah menerima Inform
	n := base(0)
	n.Device.LastInformAt = nil
	n.OLT = oltAt(domain.OLTStateOnline, time.Minute)
	nd := Diagnose(n)
	f := finding(t, nd, "OLT_ONLINE_ACS_SILENT")
	if !strings.Contains(f.Evidence, "belum pernah menerima Inform") || !strings.Contains(f.Hint, "URL ACS sudah diprovisioning") {
		t.Errorf("ONU baru: %+v", f)
	}
	if nd.Verdict != domain.VerdictNeverConnected || !has(nd, "NEVER_INFORMED") {
		t.Errorf("verdict/temuan dasar tetap ada: %s %v", nd.Verdict, codes(nd))
	}
}

// Perangkat yang masih terjangkau ACS tidak boleh mendapat temuan kritis dari OLT: ACS baru saja menerima Inform, jadi "LOS" pasti basi.
// Regresi (ditemukan e2e S39): untuk perangkat yang belum pernah Inform, summary tidak boleh berhenti di "belum pernah terhubung" bila OLT sudah
// melaporkan penyebabnya.
func TestDiagnose_OLT_PerangkatBaruRingkasanMemuatPenyebabOLT(t *testing.T) {
	mk := func(state string) domain.DeviceDiagnosis {
		e := base(0)
		e.Device.LastInformAt = nil
		if state != "" {
			e.OLT = oltAt(state, time.Minute)
		}
		return Diagnose(e)
	}
	if d := mk(domain.OLTStateLOS); !strings.HasPrefix(d.Summary, "Dugaan utama: OLT melaporkan ONU kehilangan sinyal optik") {
		t.Errorf("LOS: summary = %q", d.Summary)
	}
	if d := mk(domain.OLTStateOnline); !strings.HasPrefix(d.Summary, "Dugaan utama: ONU online di OLT, tetapi tidak terlihat di ACS") {
		t.Errorf("ONLINE: summary = %q", d.Summary)
	}
	// Tanpa bukti lain atau hanya info (UNKNOWN): tetap pernyataan fakta.
	for _, state := range []string{"", domain.OLTStateUnknown} {
		if d := mk(state); d.Summary != "Perangkat belum pernah terhubung ke ACS." {
			t.Errorf("state %q: summary = %q", state, d.Summary)
		}
	}
}

func TestDiagnose_OLT_PerangkatTerjangkauHanyaKonflikInfo(t *testing.T) {
	for _, state := range []string{domain.OLTStateLOS, domain.OLTStateDyingGasp, domain.OLTStateOffline, domain.OLTStateUnknown} {
		e := base(2 * time.Minute)
		e.OLT = oltAt(state, 3*time.Minute)
		d := Diagnose(e)
		if d.Verdict != domain.VerdictReachable {
			t.Fatalf("prasyarat: %s", d.Verdict)
		}
		f := finding(t, d, "OLT_STATE_CONFLICT")
		if f.Severity != domain.SeverityInfo || !strings.Contains(f.Evidence, "ACS menerima Inform") {
			t.Errorf("%s: %+v", state, f)
		}
		for _, c := range []string{"OLT_LOS", "OLT_DYING_GASP", "OLT_ONU_OFFLINE", "OLT_ONLINE_ACS_SILENT"} {
			if has(d, c) {
				t.Errorf("%s: temuan %s tak boleh muncul untuk perangkat terjangkau", state, c)
			}
		}
	}
	e := base(2 * time.Minute)
	e.OLT = oltAt(domain.OLTStateOnline, time.Minute)
	if d := Diagnose(e); has(d, "OLT_STATE_CONFLICT") || has(d, "OLT_ONLINE_ACS_SILENT") {
		t.Errorf("sepaham (terjangkau + ONLINE) tidak perlu temuan OLT: %v", codes(d))
	}
}

func TestDiagnose_OLT_DataBasiDiberiCatatan(t *testing.T) {
	e := base(3 * time.Hour)
	e.OLT = oltAt(domain.OLTStateLOS, 45*time.Minute)
	f := finding(t, Diagnose(e), "OLT_DATA_STALE")
	if f.Severity != domain.SeverityInfo || !strings.Contains(f.Evidence, "45 menit lalu") || !strings.Contains(f.Hint, "ACS_OLT_POLL_MINUTES") {
		t.Errorf("%+v", f)
	}
	e.OLT = oltAt(domain.OLTStateLOS, 29*time.Minute)
	if has(Diagnose(e), "OLT_DATA_STALE") {
		t.Error("29 menit belum basi")
	}
	// basi juga untuk perangkat terjangkau (konflik + basi)
	r := base(time.Minute)
	r.OLT = oltAt(domain.OLTStateLOS, 2*time.Hour)
	if !has(Diagnose(r), "OLT_DATA_STALE") {
		t.Error("data basi harus dinyatakan juga pada perangkat terjangkau")
	}
}

func TestDiagnose_OLT_TidakDitemukan(t *testing.T) {
	e := base(3 * time.Hour)
	e.TenantHasOLT = true
	d := Diagnose(e)
	f := finding(t, d, "OLT_NOT_FOUND")
	if f.Severity != domain.SeverityInfo || !strings.Contains(f.Hint, "format serial") {
		t.Errorf("%+v", f)
	}
	if d.OLT != nil || d.Limits != domain.DiagnosisLimits {
		t.Errorf("tanpa ONU di OLT, limits kembali ke versi ACS saja: %q", d.Limits)
	}
	e.TenantHasOLT = false
	if has(Diagnose(e), "OLT_NOT_FOUND") {
		t.Error("tenant tanpa OLT: jangan menyinggung OLT")
	}
	r := base(time.Minute)
	r.TenantHasOLT = true
	if has(Diagnose(r), "OLT_NOT_FOUND") {
		t.Error("perangkat terjangkau tidak perlu 'tidak ditemukan di OLT'")
	}
}

// ---- Service: bukti OLT ----

type fakeLookup struct {
	onu    *domain.RegisteredONU
	hasOLT bool
	err    error
	called int
	gotSN  string
	gotTID uint64
}

func (f *fakeLookup) LookupONUBySerial(_ context.Context, tid uint64, serial string) (*domain.RegisteredONU, bool, error) {
	f.called++
	f.gotSN, f.gotTID = serial, tid
	return f.onu, f.hasOLT, f.err
}

func TestService_BuktiOLT(t *testing.T) {
	tid := uint64(7)
	mk := func(dev *domain.Device, l *fakeLookup) *domain.DeviceDiagnosis {
		svc := newSvc(fakeDevices{dev: dev}, fakeEvents{}, fakeOptical{}, &fakeFaults{}, fakeTasks{}).WithOLT(l)
		d, err := svc.Diagnose(context.Background(), domain.Actor{}, dev.ID)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	dev := &domain.Device{ID: 7, TenantID: &tid, SerialNumber: "ZTEG1234", LastInformAt: ago(3 * time.Hour)}
	idx := "5"

	l := &fakeLookup{onu: &domain.RegisteredONU{OLTName: "OLT-1", PonPort: "p/3", ONUIndex: &idx, State: domain.OLTStateLOS, LastSeenAt: t0.Add(-time.Minute), LastStateChangeAt: t0.Add(-time.Hour)}}
	d := mk(dev, l)
	if l.gotSN != "ZTEG1234" || l.gotTID != 7 {
		t.Errorf("lookup dengan serial & tenant perangkat: %q %d", l.gotSN, l.gotTID)
	}
	if !has(*d, "OLT_LOS") || d.OLT == nil || d.OLT.OLTName != "OLT-1" || d.OLT.PonPort != "p/3" {
		t.Fatalf("ONU harus dipakai sebagai bukti: %v olt=%+v", codes(*d), d.OLT)
	}

	// tak ditemukan, tenant punya OLT
	if d := mk(dev, &fakeLookup{hasOLT: true}); !has(*d, "OLT_NOT_FOUND") || d.OLT != nil {
		t.Errorf("tak ditemukan: %v", codes(*d))
	}
	// galat lookup = celah bukti, bukan kegagalan diagnosis
	d = mk(dev, &fakeLookup{err: errors.New("db")})
	if f := finding(t, *d, "EVIDENCE_INCOMPLETE"); !strings.Contains(f.Evidence, "data OLT") {
		t.Errorf("celah harus menyebut data OLT: %q", f.Evidence)
	}
	// perangkat tanpa tenant: lookup tidak dipanggil
	l2 := &fakeLookup{}
	mk(&domain.Device{ID: 8, SerialNumber: "X", LastInformAt: ago(3 * time.Hour)}, l2)
	if l2.called != 0 {
		t.Error("perangkat tanpa tenant tidak boleh memicu pencarian OLT")
	}
	// tanpa WithOLT: seperti sebelumnya
	svc := newSvc(fakeDevices{dev: dev}, fakeEvents{}, fakeOptical{}, &fakeFaults{}, fakeTasks{})
	if d, err := svc.Diagnose(context.Background(), domain.Actor{}, 7); err != nil || d.OLT != nil || d.Limits != domain.DiagnosisLimits {
		t.Errorf("tanpa OLT: err=%v olt=%+v", err, d.OLT)
	}
}
