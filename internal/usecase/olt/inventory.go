package olt

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// ---- Inventaris ONU terdaftar (irisan B, migrations/0047) ----
// Read-only terhadap OLT. Keadaan ONU sudah dinormalisasi driver (domain.ReportedONU); usecase ini tidak pernah bercabang per vendor.

// maxStateChangesInEvent membatasi rincian per event ONU_STATE_CHANGED; selebihnya hanya dihitung (jumlah total tetap benar).
const maxStateChangesInEvent = 50

// maxPollDuePerTick membatasi OLT yang dipoll dalam satu tick penyapu (polling OLT nyata lambat dan membatasi sesi).
const maxPollDuePerTick = 20

// errInventoryTooLarge: OLT melaporkan lebih dari domain.OLTMaxRegisteredONUs ONU; hasil tidak diterapkan.
var errInventoryTooLarge = fmt.Errorf("OLT melaporkan lebih dari %d ONU terdaftar; hasil polling tidak diterapkan", domain.OLTMaxRegisteredONUs)

// DeviceReader dipenuhi usecase/device.Service: pembacaan perangkat yang sudah menegakkan scope tenant/RBAC.
type DeviceReader interface {
	Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.Device, error)
}

// WithInventory mengaktifkan inventaris ONU terdaftar (polling, daftar, port, tautan perangkat).
func (s *Service) WithInventory(repo domain.OLTInventoryRepository) *Service {
	s.inv = repo
	return s
}

// WithPublisher mengaktifkan siaran realtime ONU_STATE_CHANGED ke WebSocket tenant pemilik OLT.
func (s *Service) WithPublisher(p domain.EventPublisher) *Service {
	s.publisher = p
	return s
}

// WithNotifier mengaktifkan notifikasi in-app ONU_STATE_ALERT saat ONU masuk LOS/dying gasp (diringkas per polling OLT).
func (s *Service) WithNotifier(n domain.Notifier) *Service {
	s.notifier = n
	return s
}

// WithWebhooks mengaktifkan webhook ONU_STATE_CHANGED untuk BSS/NMS (satu event per polling yang menemukan perubahan keadaan).
func (s *Service) WithWebhooks(w domain.WebhookEnqueuer) *Service {
	s.webhooks = w
	return s
}

// WithDevices mengaktifkan pencarian ONU milik sebuah perangkat (ForDevice) dengan scope tenant dari usecase perangkat.
func (s *Service) WithDevices(d DeviceReader) *Service {
	s.devices = d
	return s
}

// StateChange = satu ONU yang keadaannya berubah di antara dua polling.
type StateChange struct {
	SerialNumber string `json:"serial_number"`
	PonPort      string `json:"pon_port"`
	From         string `json:"from"`
	To           string `json:"to"`
}

// RegisteredSummary = ringkasan rekonsiliasi ONU terdaftar pada satu polling.
type RegisteredSummary struct {
	Seen         int            `json:"seen"`          // ONU terdaftar yang dilaporkan OLT (setelah dibersihkan)
	New          int            `json:"new"`           // belum pernah ada (atau sebelumnya hilang)
	Removed      int            `json:"removed"`       // sebelumnya ada, kini tidak dilaporkan OLT (dihapus-lunak)
	StateChanged int            `json:"state_changed"` // keadaan berbeda dari polling sebelumnya (tidak termasuk ONU baru)
	Skipped      int            `json:"skipped"`       // entri dari perangkat yang tak valid/duplikat dan dilewati
	ByState      map[string]int `json:"by_state"`      // jumlah per kode keadaan
}

// cleanReported membersihkan entri dari perangkat (tak tepercaya): serial wajib 1..32 tanpa karakter kontrol; port/nomor/tipe/keadaan asli
// dipangkas; keadaan di luar kode yang dikenal menjadi UNKNOWN (tidak ditebak); RX di luar rentang fisik dibuang; duplikat serial (tak peka
// huruf besar/kecil, seperti kolasi kolom) dibuang. Entri tak valid dilewati (dihitung), bukan menggagalkan seluruh polling.
func cleanReported(in []domain.ReportedONU) ([]domain.ReportedONU, int) {
	out := make([]domain.ReportedONU, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	skipped := 0
	for _, o := range in {
		serial, err := domain.CleanText("serial", o.SerialNumber, 1, domain.ONUSerialMaxLen, false)
		if err != nil {
			skipped++
			continue
		}
		key := strings.ToUpper(serial)
		if _, dup := seen[key]; dup {
			skipped++
			continue
		}
		port, e1 := domain.CleanText("pon_port", o.PonPort, 0, domain.ONUPortMaxLen, false)
		idx, e2 := domain.CleanText("onu_index", o.ONUIndex, 0, domain.ONUIndexMaxLen, false)
		typ, e3 := domain.CleanText("onu_type", o.ONUType, 0, domain.ONUTypeMaxLen, false)
		vst, e4 := domain.CleanText("vendor_state", o.VendorState, 0, domain.ONUVendorStateMaxLen, false)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			skipped++
			continue
		}
		state := o.State
		if !domain.ValidOLTONUState(state) {
			state = domain.OLTStateUnknown
		}
		var rx *float64
		if o.RxPowerDBM != nil && domain.ValidOLTRx(*o.RxPowerDBM) {
			v := *o.RxPowerDBM
			rx = &v
		}
		seen[key] = struct{}{}
		out = append(out, domain.ReportedONU{PonPort: port, ONUIndex: idx, SerialNumber: serial, ONUType: typ, State: state, VendorState: vst, RxPowerDBM: rx})
	}
	return out, skipped
}

// pollInventory membaca ONU terdaftar dari driver lalu merekonsiliasi snapshot. Dipanggil dari Poll dengan penjaga satu-polling-per-OLT
// yang sama. Galat dikembalikan apa adanya (Poll yang mengklasifikasikannya).
func (s *Service) pollInventory(ctx context.Context, o *domain.OLT, drv domain.OLTInventoryDriver, conn domain.OLTConnection) (*RegisteredSummary, error) {
	cctx, cancel := context.WithTimeout(ctx, driverTimeout)
	defer cancel()
	reported, err := drv.ListRegisteredONUs(cctx, conn)
	if err != nil {
		return nil, err
	}
	clean, skipped := cleanReported(reported)
	if len(clean) > domain.OLTMaxRegisteredONUs {
		return nil, errInventoryTooLarge
	}
	sum, changes, err := s.reconcileInventory(ctx, o, clean)
	if err != nil {
		return nil, err
	}
	sum.Skipped = skipped
	if len(changes) > 0 {
		shown := changes
		if len(shown) > maxStateChangesInEvent {
			shown = shown[:maxStateChangesInEvent]
		}
		payload := map[string]interface{}{
			"olt_id": o.UUID, "olt_name": o.Name, "changed": len(changes), "changes": shown,
		}
		if s.publisher != nil {
			s.publisher.BroadcastToTenant(o.TenantID, "ONU_STATE_CHANGED", payload)
		}
		s.announceStateChanges(ctx, o, changes, payload)
	}
	return sum, nil
}

// onuLOSCriticalCount: ONU LOS sebanyak ini (atau lebih) pada satu polling OLT lebih mirip putusnya kabel feeder/distribusi daripada
// gangguan di rumah satu pelanggan, jadi notifikasinya CRITICAL (di bawahnya WARNING).
const onuLOSCriticalCount = 5

// maxONUsInAlert = jumlah ONU yang disebut di isi notifikasi ringkasan; selebihnya hanya dihitung.
const maxONUsInAlert = 5

// announceStateChanges memberitahu perubahan keadaan ONU di luar siaran realtime. Kegagalan tidak menggagalkan polling.
//   - webhook ONU_STATE_CHANGED: SATU event per polling (payload sama dengan siaran WebSocket: jumlah total + maks maxStateChangesInEvent
//     rincian, `truncated` bila terpotong). Bukan satu per ONU: OLT yang reboot dapat mengubah ribuan ONU sekaligus.
//   - notifikasi ONU_STATE_ALERT: hanya bila ada ONU yang BARU masuk LOS atau dying gasp; diringkas per polling OLT.
func (s *Service) announceStateChanges(ctx context.Context, o *domain.OLT, changes []StateChange, payload map[string]interface{}) {
	tid := o.TenantID
	if s.webhooks != nil {
		p := make(map[string]interface{}, len(payload)+1)
		for k, v := range payload {
			p[k] = v
		}
		p["truncated"] = len(changes) > maxStateChangesInEvent
		_ = s.webhooks.Enqueue(ctx, domain.WebhookEventONUStateChanged, &tid, p)
	}
	if s.notifier == nil {
		return
	}
	if ev, ok := onuStateAlert(o, changes); ok {
		_ = s.notifier.Notify(ctx, ev)
	}
}

// onuStateAlert menyusun notifikasi ringkasan untuk ONU yang baru masuk LOS/dying gasp pada satu polling; ok=false bila tak ada.
func onuStateAlert(o *domain.OLT, changes []StateChange) (domain.NotificationEvent, bool) {
	var hit []StateChange
	los := 0
	for _, c := range changes {
		if c.To == domain.OLTStateLOS || c.To == domain.OLTStateDyingGasp {
			hit = append(hit, c)
			if c.To == domain.OLTStateLOS {
				los++
			}
		}
	}
	if len(hit) == 0 {
		return domain.NotificationEvent{}, false
	}
	tid := o.TenantID
	shown := hit
	if len(shown) > maxONUsInAlert {
		shown = shown[:maxONUsInAlert]
	}
	parts := make([]string, len(shown))
	for i, c := range shown {
		parts[i] = fmt.Sprintf("%s (%s, %s)", c.SerialNumber, c.PonPort, c.To)
	}
	body := fmt.Sprintf("OLT %s: %d ONU masuk LOS atau dying gasp: %s", o.Name, len(hit), strings.Join(parts, ", "))
	if rest := len(hit) - len(shown); rest > 0 {
		body += fmt.Sprintf(" dan %d lainnya", rest)
	}
	body += ". LOS pada banyak ONU di satu port PON biasanya berarti kabel feeder/distribusi terputus; dying gasp berarti ONU kehilangan daya."
	severity := domain.NotificationSeverityWarning
	if los >= onuLOSCriticalCount {
		severity = domain.NotificationSeverityCritical
	}
	return domain.NotificationEvent{
		TypeCode: domain.NotifONUStateAlert, TenantID: &tid, SeverityOverride: severity,
		Title: fmt.Sprintf("%d ONU LOS/dying gasp di %s", len(hit), o.Name),
		Body:  body,
		// Tautan dalam: buka OLT ini dengan filter "bermasalah saja" sudah aktif (query dibaca halaman OLT).
		Link: "/olts?olt=" + o.UUID + "&problem=1",
	}, true
}

// reconcileInventory: serial baru → baris baru; serial yang ada → diperbarui (keadaan, RX, last_seen_at); serial yang tidak lagi dilaporkan →
// dihapus-lunak. Mengembalikan juga daftar perubahan keadaan (untuk siaran).
func (s *Service) reconcileInventory(ctx context.Context, o *domain.OLT, clean []domain.ReportedONU) (*RegisteredSummary, []StateChange, error) {
	existing, err := s.inv.ListRegisteredByOLT(ctx, o.ID)
	if err != nil {
		return nil, nil, err
	}
	bySerial := make(map[string]domain.RegisteredONU, len(existing))
	for _, e := range existing {
		bySerial[strings.ToUpper(e.SerialNumber)] = e
	}
	stateIDs := map[string]uint64{}
	stateID := func(code string) (uint64, error) {
		if id, ok := stateIDs[code]; ok {
			return id, nil
		}
		r, err := s.refs.GetByCode(ctx, domain.RefTableOLTONUState, code)
		if err != nil {
			return 0, err
		}
		stateIDs[code] = r.ID
		return r.ID, nil
	}

	now := time.Now()
	sum := &RegisteredSummary{Seen: len(clean), ByState: map[string]int{}}
	var events []domain.ONUStateEvent
	var changes []StateChange
	rows := make([]domain.RegisteredONU, 0, len(clean))
	seen := make(map[string]struct{}, len(clean))
	for _, c := range clean {
		key := strings.ToUpper(c.SerialNumber)
		seen[key] = struct{}{}
		sid, err := stateID(c.State)
		if err != nil {
			return nil, nil, err
		}
		sum.ByState[c.State]++
		row := domain.RegisteredONU{
			UUID: uuid.NewString(), PonPort: c.PonPort, ONUIndex: ptrOrNil(c.ONUIndex), SerialNumber: c.SerialNumber, ONUType: ptrOrNil(c.ONUType),
			StateID: sid, VendorState: ptrOrNil(c.VendorState), RxPowerDBM: c.RxPowerDBM,
			FirstSeenAt: now, LastSeenAt: now, LastStateChangeAt: now,
		}
		if ex, ok := bySerial[key]; ok {
			row.UUID, row.FirstSeenAt, row.LastStateChangeAt = ex.UUID, ex.FirstSeenAt, ex.LastStateChangeAt
			if ex.State != c.State {
				sum.StateChanged++
				row.LastStateChangeAt = now
				changes = append(changes, StateChange{SerialNumber: c.SerialNumber, PonPort: c.PonPort, From: ex.State, To: c.State})
				from := ex.StateID
				events = append(events, domain.ONUStateEvent{OLTID: o.ID, SerialNumber: c.SerialNumber, PonPort: c.PonPort, FromStateID: &from, ToStateID: &sid, RxPowerDBM: c.RxPowerDBM, OccurredAt: now})
			}
		} else {
			sum.New++
			events = append(events, domain.ONUStateEvent{OLTID: o.ID, SerialNumber: c.SerialNumber, PonPort: c.PonPort, ToStateID: &sid, RxPowerDBM: c.RxPowerDBM, OccurredAt: now})
		}
		rows = append(rows, row)
	}
	if err := s.inv.UpsertRegisteredONUs(ctx, o.ID, rows); err != nil {
		return nil, nil, err
	}
	var gone []uint64
	for _, e := range existing {
		if _, still := seen[strings.ToUpper(e.SerialNumber)]; !still {
			gone = append(gone, e.ID)
			from := e.StateID
			events = append(events, domain.ONUStateEvent{OLTID: o.ID, SerialNumber: e.SerialNumber, PonPort: e.PonPort, FromStateID: &from, RxPowerDBM: e.RxPowerDBM, OccurredAt: now})
		}
	}
	if len(gone) > 0 {
		if err := s.inv.RemoveRegisteredONUs(ctx, gone, nil); err != nil {
			return nil, nil, err
		}
	}
	sum.Removed = len(gone)
	// Riwayat hanyalah catatan tambahan: kegagalan menyimpannya tidak boleh menggagalkan polling (inventaris sudah benar), cukup dicatat.
	if err := s.inv.InsertStateEvents(ctx, events); err != nil {
		log.Printf("olt: gagal menyimpan riwayat keadaan ONU (olt %d, %d peristiwa): %v", o.ID, len(events), err)
	}
	return sum, changes, nil
}

// StateHistory = riwayat perubahan keadaan satu ONU beserta ringkasan kestabilan.
type StateHistory struct {
	Events []domain.ONUStateEvent
	Total  int
	// Changes24h = jumlah perubahan keadaan sejati (bukan sekadar muncul/hilang) dalam 24 jam terakhir; Flapping = perubahan >= domain.ONUFlapMinChanges
	// dalam domain.ONUFlapWindow terakhir. Keduanya hanya dari perubahan yang teramati polling.
	Changes24h int
	Flapping   bool
}

// ONUHistory mengembalikan riwayat perubahan keadaan ONU (identifier publik) milik sebuah OLT, scope tenant lewat OLT-nya. ONU tak ada pada
// OLT itu = ErrNotFound. Panjang riwayat dibatasi retensi log (ACS_LOG_RETENTION_DAYS).
func (s *Service) ONUHistory(ctx context.Context, actor domain.Actor, oltUUID, onuUUID string, p domain.Pagination) (*StateHistory, error) {
	if err := s.requireInventory(); err != nil {
		return nil, err
	}
	o, err := s.load(ctx, actor, oltUUID)
	if err != nil {
		return nil, err
	}
	onu, err := s.inv.FindRegisteredByUUID(ctx, o.ID, onuUUID)
	if err != nil {
		return nil, err
	}
	events, total, err := s.inv.ListStateEvents(ctx, o.ID, onu.SerialNumber, p)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	day, err := s.inv.CountStateChanges(ctx, o.ID, onu.SerialNumber, now.Add(-24*time.Hour))
	if err != nil {
		return nil, err
	}
	hour, err := s.inv.CountStateChanges(ctx, o.ID, onu.SerialNumber, now.Add(-domain.ONUFlapWindow))
	if err != nil {
		return nil, err
	}
	return &StateHistory{Events: events, Total: total, Changes24h: day, Flapping: hour >= domain.ONUFlapMinChanges}, nil
}

// ---- pembacaan ----

// RegisteredListInput = filter daftar ONU terdaftar sebuah OLT.
type RegisteredListInput struct {
	OLTUUID string
	PonPort string
	State   string // kode ref_olt_onu_state; kosong = semua
	Search  string
	// Problem: hanya ONU LOS/dying gasp/offline. Sort: "" atau "port" = port, nomor ONU, serial; "severity" = masalah dulu.
	Problem bool
	Sort    string
}

// Nilai Sort yang sah pada RegisteredListInput.
const (
	SortByPort     = "port"
	SortBySeverity = "severity"
)

func (s *Service) requireInventory() error {
	if s.inv == nil {
		return fmt.Errorf("%w: inventaris ONU terdaftar tidak diaktifkan", domain.ErrInvalidInput)
	}
	return nil
}

// ListRegistered mengembalikan ONU terdaftar sebuah OLT (scope tenant lewat OLT-nya), dengan tautan ke perangkat ACS.
func (s *Service) ListRegistered(ctx context.Context, actor domain.Actor, in RegisteredListInput, p domain.Pagination) ([]domain.RegisteredONU, int, error) {
	if err := s.requireInventory(); err != nil {
		return nil, 0, err
	}
	o, err := s.load(ctx, actor, in.OLTUUID)
	if err != nil {
		return nil, 0, err
	}
	if in.Sort != "" && in.Sort != SortByPort && in.Sort != SortBySeverity {
		return nil, 0, fmt.Errorf("%w: sort harus port atau severity", domain.ErrInvalidInput)
	}
	f := domain.RegisteredONUFilter{OLTID: &o.ID, TenantID: &o.TenantID, PonPort: in.PonPort, Search: in.Search,
		ProblemOnly: in.Problem, SortSeverity: in.Sort == SortBySeverity}
	if in.State != "" {
		st, err := s.refs.GetByCode(ctx, domain.RefTableOLTONUState, in.State)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: keadaan tidak dikenal", domain.ErrInvalidInput)
		}
		f.StateID = &st.ID
	}
	return s.inv.ListRegisteredONUs(ctx, f, p)
}

// Ports = ringkasan per port PON dari ONU terdaftar OLT itu. Hanya baca; bukan kontrol port.
func (s *Service) Ports(ctx context.Context, actor domain.Actor, oltUUID string) ([]domain.PONPortSummary, error) {
	if err := s.requireInventory(); err != nil {
		return nil, err
	}
	o, err := s.load(ctx, actor, oltUUID)
	if err != nil {
		return nil, err
	}
	return s.inv.PortSummaries(ctx, o.ID)
}

// DeviceONU = posisi sebuah perangkat ACS di jaringan OLT.
type DeviceONU struct {
	// Linked: ada ONU dengan serial yang sama di OLT milik tenant perangkat.
	Linked bool                  `json:"linked"`
	ONU    *domain.RegisteredONU `json:"onu"`
	// TenantHasOLT: tenant punya OLT aktif. Linked=false + TenantHasOLT=true = serial tidak ditemukan di data OLT terakhir (atau belum dipoll).
	TenantHasOLT bool `json:"tenant_has_olt"`
}

// LookupONUBySerial mencari ONU di OLT milik tenant itu. Tidak memeriksa hak akses: pemanggil WAJIB sudah memverifikasi akses ke perangkat
// (usecase/device.Get). Mengembalikan (ONU, true) bila ditemukan; (nil, apakah tenant punya OLT aktif) bila tidak.
func (s *Service) LookupONUBySerial(ctx context.Context, tenantID uint64, serial string) (*domain.RegisteredONU, bool, error) {
	if s.inv == nil {
		return nil, false, nil
	}
	onu, err := s.inv.FindRegisteredBySerial(ctx, tenantID, serial)
	if err == nil {
		return onu, true, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, false, err
	}
	n, err := s.inv.CountActiveOLTs(ctx, tenantID)
	if err != nil {
		return nil, false, err
	}
	return nil, n > 0, nil
}

// ForDevice mengembalikan posisi perangkat di OLT. Akses mengikuti usecase perangkat (perangkat tenant lain = ErrNotFound/ErrForbidden).
func (s *Service) ForDevice(ctx context.Context, actor domain.Actor, deviceID uint64) (*DeviceONU, error) {
	if err := s.requireInventory(); err != nil {
		return nil, err
	}
	if s.devices == nil {
		return nil, fmt.Errorf("%w: pencarian per perangkat tidak diaktifkan", domain.ErrInvalidInput)
	}
	dev, err := s.devices.Get(ctx, actor, deviceID)
	if err != nil {
		return nil, err
	}
	if dev.TenantID == nil { // perangkat tanpa tenant tidak punya OLT
		return &DeviceONU{}, nil
	}
	onu, tenantHasOLT, err := s.LookupONUBySerial(ctx, *dev.TenantID, dev.SerialNumber)
	if err != nil {
		return nil, err
	}
	return &DeviceONU{Linked: onu != nil, ONU: onu, TenantHasOLT: tenantHasOLT}, nil
}

// ---- polling berkala ----

// PollDue memoll OLT aktif yang belum dipoll dalam `every` (terlama dulu, maks maxPollDuePerTick per panggilan, berurutan). Hasil per OLT
// tercatat pada baris OLT (status/galat); galat satu OLT tidak menghentikan yang lain. Dipanggil penyapu; aman dari instance mana pun
// (penjaga satu-polling-per-OLT berlaku per instance, rekonsiliasi idempoten). Mengembalikan jumlah OLT yang berhasil dipoll.
func (s *Service) PollDue(ctx context.Context, every time.Duration) (int, error) {
	if s.inv == nil || every <= 0 {
		return 0, nil
	}
	due, err := s.inv.ListOLTsDueForPoll(ctx, time.Now().Add(-every), maxPollDuePerTick)
	if err != nil {
		return 0, err
	}
	system := domain.Actor{Roles: []string{domain.RoleSuperadmin}}
	ok := 0
	for i := range due {
		if ctx.Err() != nil {
			break
		}
		if _, err := s.Poll(ctx, system, due[i].UUID); err != nil {
			log.Printf("olt: polling berkala %s gagal: %v", due[i].Name, err)
			continue
		}
		ok++
	}
	return ok, nil
}
