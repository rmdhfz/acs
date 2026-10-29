package troubleshoot

import (
	"fmt"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"

	"acs/internal/domain"
)

// Ambang aturan diagnosis. Semuanya heuristik atas data TR-069; angkanya sengaja konservatif dan tertulis di sini agar bisa ditinjau.
const (
	bootBeforeSilenceWindow = 30 * time.Minute // Inform terakhir sebaik ini setelah BOOT = "baru menyala lalu hilang"
	oltStaleAfter           = 30 * time.Minute // data OLT lebih tua dari ini diberi catatan "basi"
	bootFlapWindow          = 24 * time.Hour
	bootFlapCount           = 3
	opticalDropDB           = 3.0 // penurunan RX (dB) terhadap pembacaan terbaik di jendela terakhir
	opticalTrendReadings    = 6   // pembacaan sebelum yang terbaru yang dibandingkan
	// opticalPairMaxGap = selisih waktu terbesar antara pembacaan RX ONT dan polling OLT yang masih dianggap sebanding. Lebih jauh dari ini, kedua
	// angka menggambarkan keadaan jalur yang berbeda dan tidak boleh dibandingkan.
	opticalPairMaxGap = 2 * time.Hour
)

// Evidence = semua bukti yang dikumpulkan untuk satu perangkat. Diagnose adalah fungsi murni atasnya (tanpa DB/jam sistem).
type Evidence struct {
	Now          time.Time
	Device       domain.Device
	StatusCode   string
	MinThreshold time.Duration
	// DataModelRoot = "InternetGatewayDevice." / "Device." / "" (belum diketahui); menentukan nama parameter interval Inform.
	DataModelRoot string
	Events        []EventAt            // terbaru dulu
	Optical       []OpticalAt          // terbaru dulu
	Faults        []domain.DeviceFault // fault aktif
	Tasks         []TaskInfo           // terbaru dulu
	// OLT = posisi perangkat di jaringan OLT (nil bila tak ditemukan/inventaris tidak aktif); TenantHasOLT = tenant punya OLT aktif.
	OLT          *domain.DiagnosisOLT
	TenantHasOLT bool
	// Gaps = sumber bukti yang gagal dibaca; diagnosis menyatakannya agar "tidak ada data" tidak disalahartikan sebagai "tidak ada masalah".
	Gaps []string
}

type EventAt struct {
	Code string // kode event CWMP, mis. "1 BOOT"
	At   time.Time
}

type OpticalAt struct {
	RXDBM *float64
	At    time.Time
}

type TaskInfo struct {
	Status    string
	CreatedAt time.Time
}

// Diagnose menerapkan aturan di atas bukti dan menghasilkan diagnosis. Deterministik: bukti yang sama = hasil yang sama.
func Diagnose(e Evidence) domain.DeviceDiagnosis {
	d := domain.DeviceDiagnosis{
		DeviceID: e.Device.ID, Status: e.StatusCode, LastInformAt: e.Device.LastInformAt,
		InformIntervalSeconds: e.Device.InformIntervalSeconds,
		Findings:              []domain.DiagnosisFinding{}, Actions: []string{},
		Limits: domain.DiagnosisLimits, GeneratedAt: e.Now, OLT: e.OLT,
	}
	if e.OLT != nil {
		d.Limits = domain.DiagnosisLimitsWithOLT
	}
	if e.DataModelRoot != "" {
		d.InformIntervalParameter = e.DataModelRoot + "ManagementServer.PeriodicInformInterval"
	}
	threshold := domain.OfflineThreshold(e.Device.InformIntervalSeconds, e.MinThreshold)
	d.OfflineThresholdSeconds = int64(threshold / time.Second)

	var silent time.Duration
	switch {
	case e.Device.LastInformAt == nil:
		d.Verdict = domain.VerdictNeverConnected
	default:
		silent = e.Now.Sub(*e.Device.LastInformAt)
		if silent < 0 {
			silent = 0
		}
		secs := int64(silent / time.Second)
		d.SilentSeconds = &secs
		if silent > threshold {
			d.Verdict = domain.VerdictOffline
		} else {
			d.Verdict = domain.VerdictReachable
		}
	}

	a := &analysis{e: e, d: &d, silent: silent, threshold: threshold}
	switch d.Verdict {
	case domain.VerdictNeverConnected:
		a.add(domain.SeverityWarning, "NEVER_INFORMED", "Perangkat belum pernah terhubung ke ACS",
			"Belum ada Inform yang diterima untuk perangkat ini.",
			"Pastikan CPE menyala dan URL ACS beserta kredensial CWMP tenant sudah diatur di CPE, dan serial number yang didaftarkan benar.")
		a.oltRules()
	case domain.VerdictReachable:
		a.add(domain.SeverityInfo, "WITHIN_TOLERANCE", "Perangkat masih dalam batas toleransi",
			fmt.Sprintf("Inform terakhir %s lalu, di bawah ambang offline %s.", humanDuration(silent), humanDuration(threshold)),
			"Tidak ada tindakan; status dihitung ulang tiap menit.")
		a.oltRules()
		a.opticalRules(false)
		a.opticalPairRule()
		a.bootFlapRule()
		a.faultRule()
	case domain.VerdictOffline:
		a.oltRules() // lebih dulu: laporan OLT paling dekat ke penyebab fisik, dan temuan sekelas mempertahankan urutan penambahan
		a.offlineRules()
		a.opticalRules(true)
		a.opticalPairRule()
		a.bootFlapRule()
		a.faultRule()
		a.pendingTasksRule()
		a.connectionRequestRules()
	}
	if len(e.Gaps) > 0 {
		a.add(domain.SeverityInfo, "EVIDENCE_INCOMPLETE", "Sebagian bukti tidak dapat dibaca",
			"Gagal membaca: "+strings.Join(e.Gaps, ", ")+".", "Coba lagi; temuan di bawah mungkin tidak lengkap.")
	}

	// Urut dari paling parah; urutan penambahan dipertahankan di dalam tingkat yang sama. Gejala (diam melewati interval) dan keterbatasan
	// (tak bisa dibangunkan) sengaja berkeparahan info: ringkasan hanya boleh menyebut PENYEBAB berbukti sebagai dugaan utama.
	rank := map[string]int{domain.SeverityCritical: 0, domain.SeverityWarning: 1, domain.SeverityInfo: 2}
	sort.SliceStable(d.Findings, func(i, j int) bool { return rank[d.Findings[i].Severity] < rank[d.Findings[j].Severity] })
	d.Summary = a.summary()
	return d
}

type analysis struct {
	e         Evidence
	d         *domain.DeviceDiagnosis
	silent    time.Duration
	threshold time.Duration
}

func (a *analysis) add(sev, code, title, evidence, hint string) {
	a.d.Findings = append(a.d.Findings, domain.DiagnosisFinding{Code: code, Severity: sev, Title: title, Evidence: evidence, Hint: hint})
}

func (a *analysis) action(code string) {
	for _, x := range a.d.Actions {
		if x == code {
			return
		}
	}
	a.d.Actions = append(a.d.Actions, code)
}

// oltRules: bukti dari sisi OLT (irisan B). Usia data selalu disebut di tiap temuan: keadaan OLT berasal dari polling terakhir, bukan dari
// detik ini. Perangkat yang masih terjangkau ACS tidak pernah mendapat temuan kritis dari OLT (ACS baru saja menerima Inform, jadi
// "LOS" pasti basi); selisihnya dinyatakan sebagai konflik berkeparahan info.
func (a *analysis) oltRules() {
	o := a.e.OLT
	if o == nil {
		if a.e.TenantHasOLT && a.d.Verdict != domain.VerdictReachable {
			a.add(domain.SeverityInfo, "OLT_NOT_FOUND", "ONU tidak ditemukan di data OLT",
				"Tenant punya OLT terdaftar, tetapi tidak ada ONU dengan serial ini pada polling OLT terakhir.",
				"Periksa kesamaan format serial antara OLT dan TR-069, apakah ONU sudah diregistrasi di OLT, dan apakah OLT sudah dipoll.")
		}
		return
	}
	age := a.e.Now.Sub(o.LastSeenAt)
	where := fmt.Sprintf("OLT %s, port %s", o.OLTName, o.PonPort)
	if o.ONUIndex != nil && *o.ONUIndex != "" {
		where += fmt.Sprintf(", ONU %s", *o.ONUIndex)
	}
	basis := fmt.Sprintf("%s: keadaan %s, terpantau sejak %s lalu (data OLT %s lalu).", where, o.State, humanDuration(a.e.Now.Sub(o.LastStateChangeAt)), humanDuration(age))
	vendor := ""
	if o.VendorState != nil && *o.VendorState != "" {
		vendor = fmt.Sprintf(" Keadaan asli dari OLT: %q.", *o.VendorState)
	}

	if a.d.Verdict == domain.VerdictReachable {
		if o.State != domain.OLTStateOnline {
			a.add(domain.SeverityInfo, "OLT_STATE_CONFLICT", "OLT dan ACS tidak sepaham",
				basis+fmt.Sprintf(" Namun ACS menerima Inform %s lalu.", humanDuration(a.silent)),
				"Data OLT kemungkinan basi atau ONU baru saja pulih; poll ulang OLT untuk memastikan.")
		}
	} else {
		acs := fmt.Sprintf("ACS diam %s", humanDuration(a.silent))
		if a.d.Verdict == domain.VerdictNeverConnected {
			acs = "ACS belum pernah menerima Inform dari perangkat ini"
		}
		switch o.State {
		case domain.OLTStateLOS:
			a.add(domain.SeverityCritical, "OLT_LOS", "OLT melaporkan ONU kehilangan sinyal optik (LOS)", basis+" "+acs+".",
				"Jalur fiber antara OLT dan ONU terputus atau konektor terlepas; ONU kemungkinan masih berdaya. Periksa ODP/splitter, kabel drop, dan konektor di ONU.")
		case domain.OLTStateDyingGasp:
			a.add(domain.SeverityCritical, "OLT_DYING_GASP", "OLT menerima dying gasp: ONU kehilangan daya listrik", basis+" "+acs+".",
				"Periksa listrik di lokasi pelanggan dan adaptor ONU; ONU kembali sendiri begitu daya pulih.")
		case domain.OLTStateOffline:
			a.add(domain.SeverityWarning, "OLT_ONU_OFFLINE", "ONU offline di OLT", basis+vendor+" "+acs+".",
				"OLT tidak merinci penyebabnya (bisa listrik maupun fiber). Bandingkan dengan redaman optik terakhir dan riwayat BOOT pada temuan lain.")
		case domain.OLTStateOnline:
			hint := "Lapisan optik dan daya ONU sehat. Periksa jalur IP/VLAN manajemen dari ONU ke ACS, URL dan kredensial ACS pada pengaturan TR-069 ONU, serta firewall."
			if a.d.Verdict == domain.VerdictNeverConnected {
				hint += " Untuk ONU baru: pastikan URL ACS sudah diprovisioning ke ONU."
			}
			a.add(domain.SeverityWarning, "OLT_ONLINE_ACS_SILENT", "ONU online di OLT, tetapi tidak terlihat di ACS", basis+" "+acs+".", hint)
		default:
			a.add(domain.SeverityInfo, "OLT_STATE_UNKNOWN", "Keadaan ONU di OLT tidak diketahui", basis+vendor,
				"Driver OLT tidak memetakan keadaan ini ke keadaan yang dikenal.")
		}
	}
	if age > oltStaleAfter {
		a.add(domain.SeverityInfo, "OLT_DATA_STALE", "Data OLT sudah lama",
			fmt.Sprintf("Polling OLT terakhir yang memuat ONU ini %s lalu.", humanDuration(age)),
			"Poll ulang OLT dari halaman OLT, atau nyalakan polling berkala (ACS_OLT_POLL_MINUTES).")
	}
}

// offlineRules: bukti tentang DIAMNYA perangkat (interval, restart sebelum hilang).
func (a *analysis) offlineRules() {
	dev := a.e.Device
	if dev.InformIntervalSeconds == nil {
		a.add(domain.SeverityInfo, "INTERVAL_UNKNOWN", "Interval Inform perangkat belum diketahui",
			fmt.Sprintf("ACS belum mengetahui interval Periodic Inform perangkat ini, sehingga ambang minimum %s dipakai.", humanDuration(a.threshold)),
			"Jika perangkat memang dikonfigurasi ber-interval panjang (mis. 12 jam), status offline ini bisa keliru. Baca PeriodicInformInterval; status dihitung ulang setelahnya.")
		if a.d.InformIntervalParameter != "" { // tanpa model data yang diketahui, nama parameternya tak bisa ditentukan dengan benar
			a.action(domain.DiagActionReadInformInterval)
		}
	} else {
		interval := time.Duration(*dev.InformIntervalSeconds) * time.Second
		a.add(domain.SeverityInfo, "SILENCE_EXCEEDS_INTERVAL", "Perangkat melewatkan beberapa Inform berturut-turut",
			fmt.Sprintf("Diam %s = %.1f× interval Inform %s (ambang offline %s).", humanDuration(a.silent), a.silent.Seconds()/interval.Seconds(), humanDuration(interval), humanDuration(a.threshold)),
			"Perangkat mati atau terputus dari jalur ke ACS. Periksa daya, kabel/fiber, dan jalur jaringan ke ACS.")
	}

	if dev.LastBootEventAt != nil && dev.LastInformAt != nil {
		if gap := dev.LastInformAt.Sub(*dev.LastBootEventAt); gap >= 0 && gap <= bootBeforeSilenceWindow {
			a.add(domain.SeverityWarning, "REBOOT_BEFORE_SILENCE", "Perangkat baru menyala lalu tidak terlihat lagi",
				fmt.Sprintf("Inform terakhir (%s) terjadi %s setelah event BOOT; sesudahnya tidak ada kabar.", fmtTime(*dev.LastInformAt), humanDuration(gap)),
				"Pola mati-hidup listrik atau restart yang tak selesai. Periksa adaptor/daya, apakah ONT macet saat booting, dan lampu PON/LOS pada perangkat.")
		}
	}
}

// opticalRules: bukti lapisan fisik dari redaman yang DILAPORKAN perangkat sebelum diam. offline=true menambah catatan yang hanya relevan bila perangkat hilang.
func (a *analysis) opticalRules(offline bool) {
	var readings []OpticalAt
	for _, o := range a.e.Optical {
		if o.RXDBM != nil {
			readings = append(readings, o)
		}
	}
	if len(readings) == 0 {
		if offline {
			a.add(domain.SeverityInfo, "OPTICAL_NO_DATA", "Tidak ada riwayat redaman optik",
				"ACS tidak menyimpan satu pun pembacaan RX untuk perangkat ini.",
				"Petakan path optik vendor (tab Redaman Optik di detail perangkat) agar RX terekam sebelum gangguan berikutnya. Data sisi OLT belum tersedia.")
		}
		return
	}
	last := readings[0]
	rx := *last.RXDBM
	ago := humanDuration(a.e.Now.Sub(last.At))
	switch domain.OpticalLevelOf(last.RXDBM) {
	case domain.OpticalBad:
		hint := "Sinyal sangat lemah: bersihkan konektor, periksa tikungan atau patahan fiber, serta ODP dan splitter. Prioritaskan pengecekan lapangan."
		if rx > domain.OpticalRXOverloadAboveDBM {
			hint = "Sinyal terlalu kuat (receiver overload): periksa splitter/atenuator pada jalur. Kondisi ini dapat merusak penerima ONT."
		}
		a.add(domain.SeverityCritical, "OPTICAL_BAD", "Redaman optik terakhir kritis",
			fmt.Sprintf("RX %.1f dBm pada %s (%s sebelum diagnosis); batas kritis di bawah %.0f atau di atas %.0f dBm.", rx, fmtTime(last.At), ago, domain.OpticalRXBadBelowDBM, domain.OpticalRXOverloadAboveDBM),
			hint)
		a.action(domain.DiagActionOpenOptical)
	case domain.OpticalWarn:
		a.add(domain.SeverityWarning, "OPTICAL_WEAK", "Redaman optik terakhir lemah",
			fmt.Sprintf("RX %.1f dBm pada %s (%s sebelum diagnosis); ambang waspada di bawah %.0f dBm.", rx, fmtTime(last.At), ago, domain.OpticalRXWarnBelowDBM),
			"Jalur optik mendekati batas: periksa konektor dan tikungan fiber sebelum menjadi gangguan.")
		a.action(domain.DiagActionOpenOptical)
	default:
		if offline {
			a.add(domain.SeverityInfo, "OPTICAL_OK", "Redaman optik terakhir normal",
				fmt.Sprintf("RX %.1f dBm pada %s (%s sebelum diagnosis).", rx, fmtTime(last.At), ago),
				"Tidak ada tanda degradasi optik bertahap. Penyebab lebih mungkin daya perangkat, jalur ke ACS, atau fiber putus mendadak (butuh data OLT).")
		}
	}

	// Tren: pembacaan terakhir dibanding yang terbaik di jendela sebelumnya.
	if len(readings) >= 3 {
		window := readings[1:]
		if len(window) > opticalTrendReadings {
			window = window[:opticalTrendReadings]
		}
		best := window[0]
		for _, w := range window {
			if *w.RXDBM > *best.RXDBM {
				best = w
			}
		}
		if drop := *best.RXDBM - rx; drop >= opticalDropDB {
			a.add(domain.SeverityWarning, "OPTICAL_DROPPING", "Redaman optik menurun",
				fmt.Sprintf("RX turun %.1f dB, dari %.1f dBm (%s) ke %.1f dBm (%s) dalam %s.", drop, *best.RXDBM, fmtTime(best.At), rx, fmtTime(last.At), humanDuration(last.At.Sub(best.At))),
				"Degradasi bertahap biasanya konektor longgar/kotor atau fiber tertekuk. Jadwalkan pengecekan lapangan.")
			a.action(domain.DiagActionOpenOptical)
		}
	}
}

// opticalPairRule membandingkan redaman dari DUA arah: RX sisi OLT (upstream, cahaya dari ONU) dan RX sisi ONT yang dilaporkan lewat TR-069
// (downstream, cahaya dari OLT). Keduanya BUKAN besaran yang sama (panjang gelombang dan daya pemancar berbeda), jadi tidak dikurangkan menjadi
// "redaman"; yang dibandingkan hanya tingkatnya terhadap ambang yang sama (domain.OpticalLevelOf; ambang khusus OLT tertentu bisa berbeda, belum
// dikalibrasi). Pola yang dicari: lemah di dua arah => jalur fiber bersama; lemah di satu arah saja => komponen di sisi itu. Hanya bila kedua
// pembacaan sebanding waktunya dan ONU online di OLT (RX sisi OLT hanya bermakna bila ONU sedang mengirim).
func (a *analysis) opticalPairRule() {
	o := a.e.OLT
	if o == nil || o.RxPowerDBM == nil || o.State != domain.OLTStateOnline || o.OLTPolledAt == nil {
		return
	}
	var ont *OpticalAt
	for i := range a.e.Optical {
		if a.e.Optical[i].RXDBM != nil {
			ont = &a.e.Optical[i]
			break
		}
	}
	if ont == nil {
		return
	}
	gap := o.OLTPolledAt.Sub(ont.At)
	if gap < 0 {
		gap = -gap
	}
	oltRX, ontRX := *o.RxPowerDBM, *ont.RXDBM
	basis := fmt.Sprintf("RX sisi OLT %.1f dBm (upstream, polling OLT %s lalu) dan RX sisi ONT %.1f dBm (downstream, dibaca %s lalu).",
		oltRX, humanDuration(a.e.Now.Sub(*o.OLTPolledAt)), ontRX, humanDuration(a.e.Now.Sub(ont.At)))
	if gap > opticalPairMaxGap {
		if a.d.Verdict == domain.VerdictReachable {
			a.add(domain.SeverityInfo, "OPTICAL_PAIR_NOT_COMPARABLE", "Redaman OLT dan ONT tidak sebanding",
				basis+fmt.Sprintf(" Selisih waktu %s melebihi %s.", humanDuration(gap), humanDuration(opticalPairMaxGap)),
				"Kedua pembacaan berasal dari waktu yang berjauhan, jadi tidak dibandingkan. Poll OLT dan segarkan redaman optik perangkat.")
		}
		return
	}
	weak := func(l string) bool { return l == domain.OpticalWarn || l == domain.OpticalBad }
	oltLvl, ontLvl := domain.OpticalLevelOf(&oltRX), domain.OpticalLevelOf(&ontRX)
	sev := domain.SeverityWarning
	if oltLvl == domain.OpticalBad || ontLvl == domain.OpticalBad {
		sev = domain.SeverityCritical
	}
	switch {
	case weak(oltLvl) && weak(ontLvl):
		a.add(sev, "OPTICAL_BOTH_WEAK", "Redaman lemah di kedua arah", basis,
			"Kedua arah sama-sama lemah: penyebab paling mungkin ada di jalur fiber yang dipakai bersama (konektor kotor, tikungan, ODP/splitter), bukan pada satu pemancar. Prioritaskan pengecekan jalur.")
		a.action(domain.DiagActionOpenOptical)
	case weak(oltLvl):
		a.add(sev, "OPTICAL_UPSTREAM_WEAK", "Hanya arah upstream (ONU ke OLT) yang lemah", basis,
			"Downstream normal tetapi upstream lemah: curigai pemancar ONU yang menurun atau konektor/ujung fiber di sisi ONU yang kotor; jalur bersama kurang mungkin. Bandingkan dengan ONU lain di port yang sama.")
		a.action(domain.DiagActionOpenOptical)
	case weak(ontLvl):
		a.add(sev, "OPTICAL_DOWNSTREAM_WEAK", "Hanya arah downstream (OLT ke ONU) yang lemah", basis,
			"Upstream normal tetapi downstream lemah: periksa konektor penerima ONT. Pastikan juga path RX yang dipetakan untuk model ini benar, karena nilai/satuan yang salah akan tampak seperti redaman lemah.")
		a.action(domain.DiagActionOpenOptical)
	case a.d.Verdict != domain.VerdictReachable:
		a.add(domain.SeverityInfo, "OPTICAL_PAIR_OK", "Redaman normal di kedua arah", basis,
			"Tidak ada tanda masalah di jalur optik dari kedua arah; penyebab lebih mungkin di jalur IP/TR-069 atau pengaturan ONU.")
	}
}

func (a *analysis) bootFlapRule() {
	n := 0
	for _, ev := range a.e.Events {
		if ev.Code == domain.EventCodeBoot && a.e.Now.Sub(ev.At) <= bootFlapWindow {
			n++
		}
	}
	if n >= bootFlapCount {
		a.add(domain.SeverityWarning, "BOOT_FLAPPING", "Perangkat sering restart",
			fmt.Sprintf("%d kali event BOOT dalam %s terakhir.", n, humanDuration(bootFlapWindow)),
			"Restart berulang: periksa adaptor/daya yang tidak stabil, panas berlebih, atau crash firmware.")
	}
}

func (a *analysis) faultRule() {
	if len(a.e.Faults) == 0 {
		return
	}
	faults := append([]domain.DeviceFault(nil), a.e.Faults...)
	sort.SliceStable(faults, func(i, j int) bool { return faults[i].LastSeenAt.After(faults[j].LastSeenAt) })
	var parts []string
	for i, f := range faults {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("+%d lainnya", len(faults)-3))
			break
		}
		parts = append(parts, fmt.Sprintf("%s fault %s \"%s\" ×%d (terakhir %s lalu)", f.RPCMethod, f.FaultCode, f.FaultString, f.OccurrenceCount, humanDuration(a.e.Now.Sub(f.LastSeenAt))))
	}
	a.add(domain.SeverityWarning, "ACTIVE_FAULTS", "Perangkat menolak perintah ACS",
		strings.Join(parts, "; ")+".", "Periksa penyebab di halaman Fault; tutup bila sudah ditangani.")
}

func (a *analysis) pendingTasksRule() {
	n, oldest := 0, time.Time{}
	for _, t := range a.e.Tasks {
		switch t.Status {
		case domain.TaskStatusPending, domain.TaskStatusQueued, domain.TaskStatusSent:
			n++
			if oldest.IsZero() || t.CreatedAt.Before(oldest) {
				oldest = t.CreatedAt
			}
		}
	}
	if n > 0 {
		a.add(domain.SeverityInfo, "PENDING_TASKS", "Ada tugas yang menunggu perangkat",
			fmt.Sprintf("%d tugas belum selesai, tertua %s lalu.", n, humanDuration(a.e.Now.Sub(oldest))),
			"Tugas baru berjalan saat perangkat Inform kembali; batalkan yang sudah tidak relevan.")
	}
}

// connectionRequestRules: apakah ACS BISA membangunkan perangkat sekarang, dan mengapa tidak bila tidak.
func (a *analysis) connectionRequestRules() {
	dev := a.e.Device
	if dev.ConnectionRequestURL == nil || *dev.ConnectionRequestURL == "" {
		a.add(domain.SeverityInfo, "CR_UNAVAILABLE", "ACS tidak bisa membangunkan perangkat",
			"ACS tidak menyimpan ConnectionRequestURL perangkat ini.",
			"ACS hanya bisa menunggu Periodic Inform berikutnya. Pastikan CPE melaporkan URL itu; bila ACS berada di balik NAT/Docker, atur ACS_CR_TRUSTED_CIDRS.")
		return
	}
	if !dev.CRCredentialsSet {
		a.add(domain.SeverityInfo, "CR_NO_CREDENTIALS", "Kredensial Connection Request belum diisi",
			"URL Connection Request tersimpan, tetapi username/password-nya belum diisi di ACS.",
			"Isi di detail perangkat (Edit); tanpa itu Connection Request ditolak perangkat (401).")
		return
	}
	a.action(domain.DiagActionConnectionRequest)
	if host := privateHost(*dev.ConnectionRequestURL); host != "" {
		a.add(domain.SeverityInfo, "CR_PRIVATE_ADDRESS", "Alamat Connection Request bersifat privat",
			fmt.Sprintf("ConnectionRequestURL memakai alamat privat/CGNAT (%s).", host),
			"Hanya terjangkau bila ACS punya rute ke jaringan itu; di balik CGNAT Connection Request biasanya gagal dan ACS hanya menunggu Periodic Inform.")
	}
}

// privateHost mengembalikan host bila URL memakai IP literal privat/link-local/CGNAT (100.64.0.0/10), selain itu "".
func privateHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	addr, err := netip.ParseAddr(u.Hostname())
	if err != nil {
		return ""
	}
	cgnat := netip.MustParsePrefix("100.64.0.0/10")
	if addr.IsPrivate() || addr.IsLinkLocalUnicast() || cgnat.Contains(addr) {
		return addr.String()
	}
	return ""
}

func (a *analysis) summary() string {
	switch a.d.Verdict {
	case domain.VerdictNeverConnected:
		// NEVER_INFORMED hanyalah fakta, bukan penyebab. Bila ada bukti lain yang berbobot (mis. OLT melaporkan LOS atau ONU online tetapi belum
		// pernah Inform), itulah yang dibaca operator pertama kali.
		for _, f := range a.d.Findings {
			if f.Severity != domain.SeverityInfo && f.Code != "NEVER_INFORMED" {
				return fmt.Sprintf("Dugaan utama: %s. %s", f.Title, f.Evidence)
			}
		}
		return "Perangkat belum pernah terhubung ke ACS."
	case domain.VerdictReachable:
		return fmt.Sprintf("Perangkat terjangkau: Inform terakhir %s lalu, di bawah ambang offline %s.", humanDuration(a.silent), humanDuration(a.threshold))
	}
	for _, f := range a.d.Findings {
		if f.Severity != domain.SeverityInfo {
			return fmt.Sprintf("Dugaan utama: %s. %s", f.Title, f.Evidence)
		}
	}
	return fmt.Sprintf("Tidak ada bukti dari sisi ACS yang menjelaskan penyebabnya; perangkat diam %s (ambang %s). Penyebab fisik membutuhkan data OLT.", humanDuration(a.silent), humanDuration(a.threshold))
}

func fmtTime(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") }

// humanDuration menulis durasi dalam bahasa Indonesia dua satuan terbesar: "3 jam 20 menit", "2 hari 4 jam", "45 detik".
func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d detik", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d menit", int(d.Minutes()))
	case d < 48*time.Hour:
		h, m := int(d.Hours()), int(d.Minutes())%60
		if m == 0 {
			return fmt.Sprintf("%d jam", h)
		}
		return fmt.Sprintf("%d jam %d menit", h, m)
	default:
		days, h := int(d.Hours())/24, int(d.Hours())%24
		if h == 0 {
			return fmt.Sprintf("%d hari", days)
		}
		return fmt.Sprintf("%d hari %d jam", days, h)
	}
}
