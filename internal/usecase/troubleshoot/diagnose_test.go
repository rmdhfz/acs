package troubleshoot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"acs/internal/domain"
)

var t0 = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) *time.Time { v := t0.Add(-d); return &v }
func u32(v uint32) *uint32           { return &v }
func f64(v float64) *float64         { return &v }
func sp(v string) *string            { return &v }

// base = perangkat yang sudah diketahui kredensial/URL CR-nya dan diam selama silent.
func base(silent time.Duration) Evidence {
	return Evidence{
		Now: t0, StatusCode: "OFFLINE", MinThreshold: 15 * time.Minute,
		Device: domain.Device{
			ID: 7, LastInformAt: ago(silent), ConnectionRequestURL: sp("http://203.0.113.9:58000/x"), CRCredentialsSet: true,
		},
	}
}

func codes(d domain.DeviceDiagnosis) []string {
	out := make([]string, len(d.Findings))
	for i, f := range d.Findings {
		out[i] = f.Code
	}
	return out
}

func has(d domain.DeviceDiagnosis, code string) bool {
	for _, c := range codes(d) {
		if c == code {
			return true
		}
	}
	return false
}

func finding(t *testing.T, d domain.DeviceDiagnosis, code string) domain.DiagnosisFinding {
	t.Helper()
	for _, f := range d.Findings {
		if f.Code == code {
			return f
		}
	}
	t.Fatalf("temuan %s tidak ada; ada: %v", code, codes(d))
	return domain.DiagnosisFinding{}
}

func hasAction(d domain.DeviceDiagnosis, a string) bool {
	for _, x := range d.Actions {
		if x == a {
			return true
		}
	}
	return false
}

func TestDiagnose_BelumPernahInform(t *testing.T) {
	e := base(0)
	e.Device.LastInformAt = nil
	e.StatusCode = "UNREGISTERED"
	d := Diagnose(e)
	if d.Verdict != domain.VerdictNeverConnected || d.SilentSeconds != nil {
		t.Fatalf("verdict=%s silent=%v", d.Verdict, d.SilentSeconds)
	}
	if !has(d, "NEVER_INFORMED") || len(d.Findings) != 1 {
		t.Errorf("hanya NEVER_INFORMED yang masuk akal untuk perangkat tanpa data: %v", codes(d))
	}
	if d.Limits == "" || d.Findings == nil || d.Actions == nil {
		t.Error("Limits wajib ada dan larik tidak boleh nil (JSON [] bukan null)")
	}
}

// Kasus yang membuat status "berkedip": interval 12 jam, diam 20 menit. Harus REACHABLE, bukan offline.
func TestDiagnose_IntervalPanjangTidakDianggapOffline(t *testing.T) {
	e := base(20 * time.Minute)
	e.Device.InformIntervalSeconds = u32(43200)
	d := Diagnose(e)
	if d.Verdict != domain.VerdictReachable {
		t.Fatalf("verdict = %s, mau REACHABLE (ambang 30 jam)", d.Verdict)
	}
	if d.OfflineThresholdSeconds != 30*3600 {
		t.Errorf("ambang = %d dtk", d.OfflineThresholdSeconds)
	}
	if !has(d, "WITHIN_TOLERANCE") || !strings.Contains(d.Summary, "terjangkau") {
		t.Errorf("summary=%q findings=%v", d.Summary, codes(d))
	}
}

func TestDiagnose_BatasAmbangPersis(t *testing.T) {
	// Aturan SQL penyapu: offline hanya bila diam LEBIH BESAR dari ambang.
	if d := Diagnose(base(15 * time.Minute)); d.Verdict != domain.VerdictReachable {
		t.Errorf("diam == ambang harus REACHABLE, dapat %s", d.Verdict)
	}
	if d := Diagnose(base(15*time.Minute + time.Second)); d.Verdict != domain.VerdictOffline {
		t.Errorf("diam > ambang harus OFFLINE, dapat %s", d.Verdict)
	}
	// Jam perangkat/ACS mundur: LastInform di masa depan tidak boleh menghasilkan durasi negatif.
	e := base(0)
	e.Device.LastInformAt = ago(-time.Hour)
	if d := Diagnose(e); d.SilentSeconds == nil || *d.SilentSeconds != 0 || d.Verdict != domain.VerdictReachable {
		t.Errorf("silent=%v verdict=%s", d.SilentSeconds, d.Verdict)
	}
}

func TestDiagnose_IntervalTidakDiketahui(t *testing.T) {
	e := base(3 * time.Hour)
	e.DataModelRoot = "InternetGatewayDevice."
	d := Diagnose(e)
	f := finding(t, d, "INTERVAL_UNKNOWN")
	if f.Severity != domain.SeverityInfo || !hasAction(d, domain.DiagActionReadInformInterval) {
		t.Errorf("INTERVAL_UNKNOWN harus info + aksi baca interval: %+v actions=%v", f, d.Actions)
	}
	if d.InformIntervalParameter != "InternetGatewayDevice.ManagementServer.PeriodicInformInterval" {
		t.Errorf("nama parameter = %q", d.InformIntervalParameter)
	}
	e.DataModelRoot = "Device."
	if d2 := Diagnose(e); d2.InformIntervalParameter != "Device.ManagementServer.PeriodicInformInterval" {
		t.Errorf("TR-181: nama parameter = %q", d2.InformIntervalParameter)
	}
	// Model data belum diketahui: nama parameter tak bisa ditentukan, jadi aksi tidak ditawarkan (bukan menebak root).
	e.DataModelRoot = ""
	d3 := Diagnose(e)
	if hasAction(d3, domain.DiagActionReadInformInterval) || d3.InformIntervalParameter != "" {
		t.Errorf("tanpa model data aksi tak boleh ditawarkan: %v param=%q", d3.Actions, d3.InformIntervalParameter)
	}
	if !strings.Contains(f.Hint, "keliru") {
		t.Errorf("hint harus jujur bahwa status bisa keliru: %q", f.Hint)
	}
}

func TestDiagnose_IntervalDiketahuiMenyebutKelipatan(t *testing.T) {
	e := base(5 * time.Hour)
	e.Device.InformIntervalSeconds = u32(600) // ambang 25 mnt
	d := Diagnose(e)
	f := finding(t, d, "SILENCE_EXCEEDS_INTERVAL")
	if !strings.Contains(f.Evidence, "30.0×") || !strings.Contains(f.Evidence, "10 menit") {
		t.Errorf("evidence = %q", f.Evidence)
	}
	if has(d, "INTERVAL_UNKNOWN") || hasAction(d, domain.DiagActionReadInformInterval) {
		t.Error("interval sudah diketahui: tak perlu menyuruh membacanya")
	}
}

func TestDiagnose_RebootSebelumHilang(t *testing.T) {
	e := base(2 * time.Hour)
	boot := t0.Add(-2*time.Hour - 10*time.Minute) // Inform terakhir 10 menit setelah BOOT
	e.Device.LastBootEventAt = &boot
	d := Diagnose(e)
	f := finding(t, d, "REBOOT_BEFORE_SILENCE")
	if f.Severity != domain.SeverityWarning || !strings.Contains(f.Evidence, "10 menit") {
		t.Errorf("%+v", f)
	}
	if !strings.HasPrefix(d.Summary, "Dugaan utama: Perangkat baru menyala") {
		t.Errorf("summary harus menyebut dugaan utama: %q", d.Summary)
	}

	// Boot jauh sebelum Inform terakhir: bukan pola "baru menyala lalu hilang".
	old := t0.Add(-10 * time.Hour)
	e.Device.LastBootEventAt = &old
	if d := Diagnose(e); has(d, "REBOOT_BEFORE_SILENCE") {
		t.Error("BOOT 8 jam sebelum Inform terakhir tidak boleh dianggap pola ini")
	}
	// Boot SETELAH Inform terakhir (data tak konsisten) tidak boleh memicu.
	future := t0.Add(-time.Hour)
	e.Device.LastBootEventAt = &future
	if d := Diagnose(e); has(d, "REBOOT_BEFORE_SILENCE") {
		t.Error("BOOT setelah Inform terakhir tidak boleh memicu")
	}
}

func TestDiagnose_Optik(t *testing.T) {
	mk := func(rx ...float64) []OpticalAt {
		var out []OpticalAt
		for i, v := range rx { // terbaru dulu, selang 1 jam
			v := v
			out = append(out, OpticalAt{RXDBM: &v, At: t0.Add(-3*time.Hour - time.Duration(i)*time.Hour)})
		}
		return out
	}
	cases := []struct {
		name     string
		optical  []OpticalAt
		want     string // kode temuan yang harus ada ("" = tak ada temuan optik)
		severity string
		notWant  []string
	}{
		{"kritis lemah", mk(-29.1), "OPTICAL_BAD", domain.SeverityCritical, nil},
		{"kritis overload", mk(-5.0), "OPTICAL_BAD", domain.SeverityCritical, nil},
		{"waspada", mk(-26.0), "OPTICAL_WEAK", domain.SeverityWarning, nil},
		{"normal", mk(-20.0), "OPTICAL_OK", domain.SeverityInfo, []string{"OPTICAL_BAD", "OPTICAL_WEAK", "OPTICAL_DROPPING"}},
		{"turun bertahap menuju batas", mk(-24.5, -20.0, -19.8), "OPTICAL_DROPPING", domain.SeverityWarning, nil},
		{"turun kecil bukan tren", mk(-21.0, -20.0, -19.5), "OPTICAL_OK", domain.SeverityInfo, []string{"OPTICAL_DROPPING"}},
		{"kurang dari tiga pembacaan tak ada tren", mk(-24.5, -19.0), "OPTICAL_OK", domain.SeverityInfo, []string{"OPTICAL_DROPPING"}},
		{"tanpa data", nil, "OPTICAL_NO_DATA", domain.SeverityInfo, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := base(3 * time.Hour)
			e.Optical = c.optical
			d := Diagnose(e)
			f := finding(t, d, c.want)
			if f.Severity != c.severity {
				t.Errorf("severity = %s, mau %s", f.Severity, c.severity)
			}
			for _, n := range c.notWant {
				if has(d, n) {
					t.Errorf("tidak boleh ada %s: %v", n, codes(d))
				}
			}
		})
	}

	// RX NULL (pembacaan tanpa nilai RX) diabaikan; pembacaan bernilai di belakangnya yang dipakai.
	e := base(3 * time.Hour)
	v := -28.0
	e.Optical = []OpticalAt{{RXDBM: nil, At: t0.Add(-time.Hour)}, {RXDBM: &v, At: t0.Add(-2 * time.Hour)}}
	if d := Diagnose(e); !has(d, "OPTICAL_BAD") {
		t.Errorf("RX NULL harus dilewati: %v", codes(d))
	}
	// Bukti yang hanya berisi RX NULL = tidak ada data (bukan "normal").
	e.Optical = []OpticalAt{{RXDBM: nil, At: t0}}
	if d := Diagnose(e); !has(d, "OPTICAL_NO_DATA") {
		t.Errorf("hanya RX NULL harus dianggap tanpa data: %v", codes(d))
	}
}

func TestDiagnose_OptikKritisMenjadiDugaanUtamaDanMenyarankanRiwayat(t *testing.T) {
	e := base(3 * time.Hour)
	e.Optical = []OpticalAt{{RXDBM: f64(-29.1), At: t0.Add(-4 * time.Hour)}}
	d := Diagnose(e)
	if d.Findings[0].Code != "OPTICAL_BAD" {
		t.Errorf("temuan kritis harus pertama: %v", codes(d))
	}
	if !strings.Contains(d.Summary, "Redaman optik terakhir kritis") || !strings.Contains(d.Summary, "-29.1") {
		t.Errorf("summary = %q", d.Summary)
	}
	if !hasAction(d, domain.DiagActionOpenOptical) {
		t.Errorf("harus menyarankan buka riwayat optik: %v", d.Actions)
	}
}

func TestDiagnose_BootBerulang(t *testing.T) {
	e := base(3 * time.Hour)
	for _, h := range []time.Duration{1, 5, 9} {
		e.Events = append(e.Events, EventAt{Code: domain.EventCodeBoot, At: t0.Add(-h * time.Hour)})
	}
	if d := Diagnose(e); !has(d, "BOOT_FLAPPING") {
		t.Errorf("3 BOOT dalam 24 jam harus memicu: %v", codes(d))
	}
	e.Events = e.Events[:2]
	if d := Diagnose(e); has(d, "BOOT_FLAPPING") {
		t.Error("2 BOOT tidak boleh memicu")
	}
	// BOOT lama (di luar 24 jam) dan event lain tidak dihitung.
	e.Events = []EventAt{
		{Code: domain.EventCodeBoot, At: t0.Add(-30 * time.Hour)}, {Code: domain.EventCodeBoot, At: t0.Add(-40 * time.Hour)},
		{Code: domain.EventCodeBoot, At: t0.Add(-1 * time.Hour)}, {Code: "2 PERIODIC", At: t0.Add(-2 * time.Hour)}, {Code: "2 PERIODIC", At: t0.Add(-3 * time.Hour)},
	}
	if d := Diagnose(e); has(d, "BOOT_FLAPPING") {
		t.Error("hanya BOOT dalam 24 jam yang dihitung")
	}
}

func TestDiagnose_Fault(t *testing.T) {
	mk := func(n int) []domain.DeviceFault {
		var out []domain.DeviceFault
		for i := 0; i < n; i++ {
			out = append(out, domain.DeviceFault{RPCMethod: "SetParameterValues", FaultCode: "9002", FaultString: "Internal error", OccurrenceCount: uint32(i + 1),
				LastSeenAt: t0.Add(-time.Duration(i+1) * time.Hour)})
		}
		return out
	}
	e := base(3 * time.Hour)
	e.Faults = mk(5)
	f := finding(t, Diagnose(e), "ACTIVE_FAULTS")
	if !strings.Contains(f.Evidence, "9002") || !strings.Contains(f.Evidence, "+2 lainnya") {
		t.Errorf("evidence harus meringkas 3 teratas + sisa: %q", f.Evidence)
	}
	if strings.Index(f.Evidence, "terakhir 1 jam") > strings.Index(f.Evidence, "terakhir 3 jam") {
		t.Errorf("fault terbaru harus lebih dulu: %q", f.Evidence)
	}
	e.Faults = nil
	if has(Diagnose(e), "ACTIVE_FAULTS") {
		t.Error("tanpa fault tidak boleh ada temuan")
	}
}

func TestDiagnose_TugasMenunggu(t *testing.T) {
	e := base(3 * time.Hour)
	e.Tasks = []TaskInfo{
		{Status: domain.TaskStatusPending, CreatedAt: t0.Add(-30 * time.Minute)},
		{Status: domain.TaskStatusSent, CreatedAt: t0.Add(-2 * time.Hour)},
		{Status: domain.TaskStatusCompleted, CreatedAt: t0.Add(-9 * time.Hour)},
		{Status: domain.TaskStatusFailed, CreatedAt: t0.Add(-10 * time.Hour)},
	}
	f := finding(t, Diagnose(e), "PENDING_TASKS")
	if !strings.Contains(f.Evidence, "2 tugas") || !strings.Contains(f.Evidence, "2 jam") {
		t.Errorf("evidence = %q (hanya PENDING/QUEUED/SENT; tertua 2 jam)", f.Evidence)
	}
	e.Tasks = e.Tasks[2:]
	if has(Diagnose(e), "PENDING_TASKS") {
		t.Error("tugas selesai/gagal tidak dihitung menunggu")
	}
}

func TestDiagnose_ConnectionRequest(t *testing.T) {
	cases := []struct {
		name       string
		url        *string
		creds      bool
		wantCode   string
		wantAction bool
	}{
		{"tanpa URL", nil, true, "CR_UNAVAILABLE", false},
		{"URL kosong", sp(""), true, "CR_UNAVAILABLE", false},
		{"tanpa kredensial", sp("http://203.0.113.9:58000/x"), false, "CR_NO_CREDENTIALS", false},
		{"siap, alamat publik", sp("http://203.0.113.9:58000/x"), true, "", true},
		{"siap, RFC1918", sp("http://10.0.123.22:58000/x"), true, "CR_PRIVATE_ADDRESS", true},
		{"siap, CGNAT 100.64/10", sp("http://100.72.1.5:7547"), true, "CR_PRIVATE_ADDRESS", true},
		{"siap, link-local", sp("http://169.254.1.1:7547"), true, "CR_PRIVATE_ADDRESS", true},
		{"siap, 100.128 bukan CGNAT", sp("http://100.128.1.5:7547"), true, "", true},
		{"siap, hostname DNS (bukan literal IP)", sp("http://cpe.example.net:7547"), true, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := base(3 * time.Hour)
			e.Device.ConnectionRequestURL, e.Device.CRCredentialsSet = c.url, c.creds
			d := Diagnose(e)
			if c.wantCode != "" {
				if f := finding(t, d, c.wantCode); f.Severity != domain.SeverityInfo {
					t.Errorf("%s harus info (keterbatasan, bukan penyebab): %s", c.wantCode, f.Severity)
				}
			}
			for _, other := range []string{"CR_UNAVAILABLE", "CR_NO_CREDENTIALS", "CR_PRIVATE_ADDRESS"} {
				if other != c.wantCode && has(d, other) {
					t.Errorf("tidak boleh ada %s: %v", other, codes(d))
				}
			}
			if got := hasAction(d, domain.DiagActionConnectionRequest); got != c.wantAction {
				t.Errorf("aksi CONNECTION_REQUEST = %v, mau %v", got, c.wantAction)
			}
		})
	}
}

// Aturan khusus offline tidak boleh muncul pada perangkat yang masih terjangkau.
func TestDiagnose_AturanOfflineTidakBocorKeReachable(t *testing.T) {
	e := base(5 * time.Minute)
	boot := t0.Add(-6 * time.Minute)
	e.Device.LastBootEventAt = &boot
	e.Device.ConnectionRequestURL = nil
	e.Tasks = []TaskInfo{{Status: domain.TaskStatusPending, CreatedAt: t0}}
	d := Diagnose(e)
	for _, c := range []string{"REBOOT_BEFORE_SILENCE", "CR_UNAVAILABLE", "PENDING_TASKS", "INTERVAL_UNKNOWN", "OPTICAL_NO_DATA"} {
		if has(d, c) {
			t.Errorf("%s tidak relevan untuk perangkat REACHABLE", c)
		}
	}
	if len(d.Actions) != 0 {
		t.Errorf("tak ada aksi untuk REACHABLE: %v", d.Actions)
	}
}

// Gejala/keterbatasan saja (tanpa penyebab berbukti) tidak boleh dijadikan "dugaan utama".
func TestDiagnose_RingkasanJujurTanpaBukti(t *testing.T) {
	e := base(3 * time.Hour)
	e.Device.InformIntervalSeconds = u32(600)
	e.Device.ConnectionRequestURL = nil
	e.Optical = []OpticalAt{{RXDBM: f64(-19), At: t0.Add(-4 * time.Hour)}}
	d := Diagnose(e)
	if strings.Contains(d.Summary, "Dugaan utama") {
		t.Errorf("tanpa penyebab berbukti summary tidak boleh menyebut dugaan utama: %q", d.Summary)
	}
	if !strings.Contains(d.Summary, "Tidak ada bukti") || !strings.Contains(d.Summary, "data OLT") {
		t.Errorf("summary harus jujur & menyebut batas OLT: %q", d.Summary)
	}
}

func TestDiagnose_UrutanKeparahanStabil(t *testing.T) {
	e := base(3 * time.Hour)
	e.Optical = []OpticalAt{{RXDBM: f64(-29), At: t0.Add(-4 * time.Hour)}}
	e.Faults = []domain.DeviceFault{{RPCMethod: "Reboot", FaultCode: "9002", LastSeenAt: t0}}
	boot := t0.Add(-3*time.Hour - 5*time.Minute)
	e.Device.LastBootEventAt = &boot
	d := Diagnose(e)
	rank := map[string]int{domain.SeverityCritical: 0, domain.SeverityWarning: 1, domain.SeverityInfo: 2}
	for i := 1; i < len(d.Findings); i++ {
		if rank[d.Findings[i-1].Severity] > rank[d.Findings[i].Severity] {
			t.Fatalf("urutan keparahan salah: %v", codes(d))
		}
	}
	// Deterministik: bukti sama = hasil sama.
	again := Diagnose(e)
	if strings.Join(codes(d), ",") != strings.Join(codes(again), ",") || d.Summary != again.Summary {
		t.Error("Diagnose tidak deterministik")
	}
}

func TestDiagnose_CelahBukti(t *testing.T) {
	e := base(3 * time.Hour)
	e.Gaps = []string{"riwayat redaman optik", "fault aktif"}
	f := finding(t, Diagnose(e), "EVIDENCE_INCOMPLETE")
	if !strings.Contains(f.Evidence, "riwayat redaman optik, fault aktif") {
		t.Errorf("evidence = %q", f.Evidence)
	}
}

func TestHumanDuration(t *testing.T) {
	for in, want := range map[time.Duration]string{
		-time.Hour: "0 detik", 45 * time.Second: "45 detik", 90 * time.Second: "1 menit", 59 * time.Minute: "59 menit",
		time.Hour: "1 jam", 200 * time.Minute: "3 jam 20 menit", 47 * time.Hour: "47 jam", 48 * time.Hour: "2 hari", 52 * time.Hour: "2 hari 4 jam",
	} {
		if got := humanDuration(in); got != want {
			t.Errorf("humanDuration(%v) = %q, mau %q", in, got, want)
		}
	}
}

// ---- Service: pengumpulan bukti, scope, dan degradasi ----

type fakeDevices struct {
	dev *domain.Device
	err error
}

func (f fakeDevices) Get(context.Context, domain.Actor, uint64) (*domain.Device, error) {
	return f.dev, f.err
}

type fakeEvents struct {
	domain.DeviceEventRepository
	rows []domain.DeviceEvent
	err  error
}

func (f fakeEvents) ListByDevice(context.Context, uint64, domain.Pagination) ([]domain.DeviceEvent, int, error) {
	return f.rows, len(f.rows), f.err
}

type fakeOptical struct {
	domain.DeviceOpticalMetricRepository
	rows []domain.DeviceOpticalMetric
	err  error
}

func (f fakeOptical) ListByDevice(context.Context, uint64, domain.Pagination) ([]domain.DeviceOpticalMetric, int, error) {
	return f.rows, len(f.rows), f.err
}

type fakeFaults struct {
	domain.FaultRepository
	rows []domain.DeviceFault
	err  error
	got  domain.FaultFilter
}

func (f *fakeFaults) List(_ context.Context, fl domain.FaultFilter, _ domain.Pagination) ([]domain.DeviceFault, int, error) {
	f.got = fl
	return f.rows, len(f.rows), f.err
}

type fakeTasks struct {
	domain.TaskRepository
	rows []domain.Task
	err  error
}

func (f fakeTasks) List(context.Context, domain.TaskFilter, domain.Pagination) ([]domain.Task, int, error) {
	return f.rows, len(f.rows), f.err
}

type fakeRefs struct{ domain.RefRepository }

func (fakeRefs) GetByID(_ context.Context, table string, id uint64) (domain.RefLookup, error) {
	switch table {
	case domain.RefTableDeviceStatus:
		return domain.RefLookup{ID: id, Code: "OFFLINE"}, nil
	case domain.RefTableEventCodes:
		return domain.RefLookup{ID: id, Code: map[uint64]string{1: domain.EventCodeBoot, 2: "2 PERIODIC"}[id]}, nil
	case domain.RefTableTaskStatus:
		return domain.RefLookup{ID: id, Code: domain.TaskStatusPending}, nil
	}
	return domain.RefLookup{}, domain.ErrNotFound
}

type fakeParams struct{ roots map[string]bool }

func (f fakeParams) HasPrefix(_ context.Context, _ uint64, prefix string) (bool, error) {
	return f.roots[prefix], nil
}

func newSvc(dev fakeDevices, ev fakeEvents, op fakeOptical, fl *fakeFaults, tk fakeTasks) *Service {
	s := NewService(dev, ev, op, fl, tk, fakeRefs{}, fakeParams{roots: map[string]bool{"Device.": true}}, 15*time.Minute)
	s.now = func() time.Time { return t0 }
	return s
}

func TestService_MengumpulkanBuktiDanMenerapkanAturan(t *testing.T) {
	dev := &domain.Device{ID: 7, LastInformAt: ago(3 * time.Hour)}
	rx := -29.0
	svc := newSvc(fakeDevices{dev: dev},
		fakeEvents{rows: []domain.DeviceEvent{{EventCodeID: 1, OccurredAt: t0.Add(-time.Hour)}, {EventCodeID: 1, OccurredAt: t0.Add(-2 * time.Hour)}, {EventCodeID: 1, OccurredAt: t0.Add(-3 * time.Hour)}}},
		fakeOptical{rows: []domain.DeviceOpticalMetric{{RxPowerDBM: &rx, RecordedAt: t0.Add(-4 * time.Hour)}}},
		&fakeFaults{}, fakeTasks{rows: []domain.Task{{TaskStatusID: 1, Audit: domain.Audit{CreatedAt: t0.Add(-time.Hour)}}}})
	d, err := svc.Diagnose(context.Background(), domain.Actor{}, 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"OPTICAL_BAD", "BOOT_FLAPPING", "PENDING_TASKS"} {
		if !has(*d, want) {
			t.Errorf("temuan %s hilang: %v", want, codes(*d))
		}
	}
	if d.Status != "OFFLINE" || d.Verdict != domain.VerdictOffline {
		t.Errorf("status=%s verdict=%s", d.Status, d.Verdict)
	}
}

func TestService_AksesDitolakTidakMembocorkanApaPun(t *testing.T) {
	faults := &fakeFaults{}
	for _, boom := range []error{domain.ErrNotFound, domain.ErrForbidden} {
		svc := newSvc(fakeDevices{err: boom}, fakeEvents{}, fakeOptical{}, faults, fakeTasks{})
		d, err := svc.Diagnose(context.Background(), domain.Actor{}, 7)
		if !errors.Is(err, boom) || d != nil {
			t.Errorf("err=%v d=%v, mau galat %v tanpa diagnosis", err, d, boom)
		}
	}
	if faults.got.DeviceID != nil {
		t.Error("bukti tidak boleh dibaca bila perangkat tidak boleh diakses")
	}
}

func TestService_SumberBuktiGagalMenjadiCelahBukanKegagalan(t *testing.T) {
	boom := errors.New("db")
	svc := newSvc(fakeDevices{dev: &domain.Device{ID: 7, LastInformAt: ago(3 * time.Hour)}},
		fakeEvents{err: boom}, fakeOptical{err: boom}, &fakeFaults{err: boom}, fakeTasks{err: boom})
	d, err := svc.Diagnose(context.Background(), domain.Actor{}, 7)
	if err != nil {
		t.Fatalf("galat sumber bukti tidak boleh menggagalkan diagnosis: %v", err)
	}
	f := finding(t, *d, "EVIDENCE_INCOMPLETE")
	for _, src := range []string{"riwayat event", "riwayat redaman optik", "fault aktif", "antrean task"} {
		if !strings.Contains(f.Evidence, src) {
			t.Errorf("celah %q tidak disebut: %q", src, f.Evidence)
		}
	}
}

func TestService_FaultDibatasiKePerangkatItu(t *testing.T) {
	faults := &fakeFaults{}
	svc := newSvc(fakeDevices{dev: &domain.Device{ID: 7, LastInformAt: ago(time.Hour)}}, fakeEvents{}, fakeOptical{}, faults, fakeTasks{})
	if _, err := svc.Diagnose(context.Background(), domain.Actor{}, 7); err != nil {
		t.Fatal(err)
	}
	if faults.got.DeviceID == nil || *faults.got.DeviceID != 7 {
		t.Errorf("filter fault harus DeviceID=7: %+v", faults.got)
	}
}
