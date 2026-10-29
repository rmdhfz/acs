package olt

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"acs/internal/domain"
	"acs/internal/oltdriver/sim"
)

// ---- fakes inventaris: meniru semantik SQL (unik per (olt, serial) tak peka huruf besar/kecil, hapus-lunak, join OLT/keadaan) ----

type fakeInv struct {
	mu       sync.Mutex
	repo     *fakeRepo
	rows     map[uint64]*domain.RegisteredONU
	deleted  map[uint64]bool
	next     uint64
	due      []domain.OLT
	activeBy map[uint64]int
	upserts  int
	events   []domain.ONUStateEvent
	eventErr error
}

func (f *fakeInv) FindRegisteredByUUID(_ context.Context, oltID uint64, onuUUID string) (*domain.RegisteredONU, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.OLTID == oltID && r.UUID == onuUUID {
			return f.join(r), nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeInv) InsertStateEvents(_ context.Context, ev []domain.ONUStateEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.eventErr != nil {
		return f.eventErr
	}
	f.events = append(f.events, ev...)
	return nil
}

func (f *fakeInv) ListStateEvents(_ context.Context, oltID uint64, serial string, p domain.Pagination) ([]domain.ONUStateEvent, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []domain.ONUStateEvent
	for i := len(f.events) - 1; i >= 0; i-- { // terbaru dulu
		e := f.events[i]
		if e.OLTID == oltID && strings.EqualFold(e.SerialNumber, serial) {
			if e.FromStateID != nil {
				c := codeOf(stateIDs, *e.FromStateID)
				e.FromState = &c
			}
			if e.ToStateID != nil {
				c := codeOf(stateIDs, *e.ToStateID)
				e.ToState = &c
			}
			all = append(all, e)
		}
	}
	lo, hi := p.Offset(), p.Offset()+p.Limit()
	if lo > len(all) {
		lo = len(all)
	}
	if hi > len(all) {
		hi = len(all)
	}
	return all[lo:hi], len(all), nil
}

func (f *fakeInv) CountStateChanges(_ context.Context, oltID uint64, serial string, since time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, e := range f.events {
		if e.OLTID == oltID && strings.EqualFold(e.SerialNumber, serial) && !e.OccurredAt.Before(since) && e.FromStateID != nil && e.ToStateID != nil {
			n++
		}
	}
	return n, nil
}

func newFakeInv(repo *fakeRepo) *fakeInv {
	return &fakeInv{repo: repo, rows: map[uint64]*domain.RegisteredONU{}, deleted: map[uint64]bool{}, activeBy: map[uint64]int{}}
}

func (f *fakeInv) join(r *domain.RegisteredONU) *domain.RegisteredONU {
	cp := *r
	cp.State = codeOf(stateIDs, cp.StateID)
	f.repo.mu.Lock()
	if o, ok := f.repo.olts[cp.OLTID]; ok {
		cp.OLTUUID, cp.OLTName, cp.TenantID = o.UUID, o.Name, o.TenantID
	}
	f.repo.mu.Unlock()
	return &cp
}

func (f *fakeInv) ListRegisteredByOLT(_ context.Context, oltID uint64) ([]domain.RegisteredONU, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []domain.RegisteredONU{}
	for _, r := range f.rows {
		if r.OLTID == oltID && !f.deleted[r.ID] {
			out = append(out, *f.join(r))
		}
	}
	return out, nil
}

func (f *fakeInv) UpsertRegisteredONUs(_ context.Context, oltID uint64, rows []domain.RegisteredONU) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserts++
	for _, in := range rows {
		var cur *domain.RegisteredONU
		for _, r := range f.rows {
			if r.OLTID == oltID && strings.EqualFold(r.SerialNumber, in.SerialNumber) {
				cur = r
			}
		}
		if cur == nil {
			f.next++
			cp := in
			cp.ID, cp.OLTID = f.next, oltID
			f.rows[cp.ID] = &cp
			continue
		}
		wasDeleted := f.deleted[cur.ID]
		stateChanged := cur.StateID != in.StateID
		id, uuid, first, lastChange := cur.ID, cur.UUID, cur.FirstSeenAt, cur.LastStateChangeAt
		*cur = in
		cur.ID, cur.OLTID, cur.UUID = id, oltID, uuid
		if wasDeleted {
			cur.FirstSeenAt, cur.LastStateChangeAt = in.FirstSeenAt, in.LastStateChangeAt
		} else {
			cur.FirstSeenAt = first
			cur.LastStateChangeAt = lastChange
			if stateChanged {
				cur.LastStateChangeAt = in.LastStateChangeAt
			}
		}
		delete(f.deleted, cur.ID)
	}
	return nil
}

func (f *fakeInv) RemoveRegisteredONUs(_ context.Context, ids []uint64, _ *uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range ids {
		if _, ok := f.rows[id]; ok {
			f.deleted[id] = true
		}
	}
	return nil
}

func (f *fakeInv) ListRegisteredONUs(_ context.Context, fl domain.RegisteredONUFilter, _ domain.Pagination) ([]domain.RegisteredONU, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []domain.RegisteredONU{}
	for _, r := range f.rows {
		if f.deleted[r.ID] {
			continue
		}
		j := f.join(r)
		if fl.TenantID != nil && *fl.TenantID != j.TenantID {
			continue
		}
		if fl.OLTID != nil && *fl.OLTID != j.OLTID {
			continue
		}
		if fl.PonPort != "" && fl.PonPort != j.PonPort {
			continue
		}
		if fl.StateID != nil && *fl.StateID != j.StateID {
			continue
		}
		if fl.Search != "" && !strings.Contains(strings.ToUpper(j.SerialNumber+" "+j.PonPort), strings.ToUpper(fl.Search)) {
			continue
		}
		if fl.ProblemOnly {
			isProblem := false
			for _, c := range domain.ONUProblemStates {
				isProblem = isProblem || j.State == c
			}
			if !isProblem {
				continue
			}
		}
		out = append(out, *j)
	}
	sortRegistered(out)
	if fl.SortSeverity {
		rank := func(code string) int {
			for i, c := range domain.ONUSeverityOrder {
				if c == code {
					return i
				}
			}
			return 99
		}
		sort.SliceStable(out, func(a, b int) bool { return rank(out[a].State) < rank(out[b].State) })
	}
	return out, len(out), nil
}

func sortRegistered(rs []domain.RegisteredONU) {
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && rs[j].SerialNumber < rs[j-1].SerialNumber; j-- {
			rs[j], rs[j-1] = rs[j-1], rs[j]
		}
	}
}

func (f *fakeInv) PortSummaries(_ context.Context, oltID uint64) ([]domain.PONPortSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	byPort := map[string]*domain.PONPortSummary{}
	for _, r := range f.rows {
		if r.OLTID != oltID || f.deleted[r.ID] {
			continue
		}
		p := byPort[r.PonPort]
		if p == nil {
			p = &domain.PONPortSummary{PonPort: r.PonPort}
			byPort[r.PonPort] = p
		}
		p.Total++
		switch codeOf(stateIDs, r.StateID) {
		case domain.OLTStateOnline:
			p.Online++
		case domain.OLTStateOffline:
			p.Offline++
		case domain.OLTStateLOS:
			p.LOS++
		case domain.OLTStateDyingGasp:
			p.DyingGasp++
		default:
			p.Unknown++
		}
	}
	out := make([]domain.PONPortSummary, 0, len(byPort))
	for _, p := range byPort {
		out = append(out, *p)
	}
	return out, nil
}

func (f *fakeInv) FindRegisteredBySerial(_ context.Context, tenantID uint64, serial string) (*domain.RegisteredONU, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if f.deleted[r.ID] || !strings.EqualFold(r.SerialNumber, serial) {
			continue
		}
		if j := f.join(r); j.TenantID == tenantID {
			return j, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeInv) CountActiveOLTs(_ context.Context, tenantID uint64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.activeBy[tenantID], nil
}

func (f *fakeInv) ListOLTsDueForPoll(context.Context, time.Time, int) ([]domain.OLT, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.OLT(nil), f.due...), nil
}

type pubEvent struct {
	tenant  uint64
	typ     string
	payload map[string]interface{}
}

type fakePub struct {
	mu     sync.Mutex
	events []pubEvent
}

func (p *fakePub) BroadcastEvent(string, interface{}) {}
func (p *fakePub) BroadcastToTenant(tid uint64, typ string, payload interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m, _ := payload.(map[string]interface{})
	p.events = append(p.events, pubEvent{tid, typ, m})
}

type invRig struct {
	*rig
	inv *fakeInv
	pub *fakePub
}

func newInvRig(t *testing.T, extra ...domain.OLTDriver) *invRig {
	t.Helper()
	r := newRig(t, extra...)
	inv, pub := newFakeInv(r.repo), &fakePub{}
	r.svc.WithInventory(inv).WithPublisher(pub)
	return &invRig{rig: r, inv: inv, pub: pub}
}

func (r *invRig) registered(t *testing.T, o *domain.OLT) map[string]string {
	t.Helper()
	rows, _, err := r.svc.ListRegistered(context.Background(), superadmin, RegisteredListInput{OLTUUID: o.UUID}, domain.Pagination{})
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, x := range rows {
		m[x.SerialNumber] = x.State
	}
	return m
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// ---- polling & rekonsiliasi ----

func TestPoll_ONUTerdaftar_DefaultTerisiDenganKeadaanNetral(t *testing.T) {
	r := newInvRig(t)
	o := r.mkOLT(t, adminA, "10.1.1.1")
	sum, err := r.svc.Poll(context.Background(), nocA, o.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Registered == nil || sum.Registered.Seen != 5 || sum.Registered.New != 5 || sum.Registered.Removed != 0 || sum.Registered.StateChanged != 0 {
		t.Fatalf("ringkasan terdaftar = %+v", sum.Registered)
	}
	if bs := sum.Registered.ByState; bs[domain.OLTStateOnline] != 2 || bs[domain.OLTStateLOS] != 1 || bs[domain.OLTStateDyingGasp] != 1 || bs[domain.OLTStateOffline] != 1 {
		t.Errorf("per keadaan = %v", bs)
	}
	want := map[string]string{"SIMREG0001": "ONLINE", "SIMREG0002": "LOS", "SIMREG0003": "DYING_GASP", "SIMREG0004": "ONLINE", "SIMREG0005": "OFFLINE"}
	if got := r.registered(t, o); !mapsEqual(got, want) {
		t.Fatalf("inventaris = %v, mau %v", got, want)
	}
	if len(r.pub.events) != 0 {
		t.Errorf("polling pertama (semua ONU baru) tidak boleh menyiarkan perubahan keadaan: %+v", r.pub.events)
	}
	// antrean belum-terdaftar tetap bekerja berdampingan
	if got := serialsByStatus(t, r.rig, o); len(got) != 3 {
		t.Errorf("antrean belum terdaftar = %v", got)
	}
}

func TestPoll_ONUTerdaftar_BerputarBaruBerubahHilangKosongLaluKembali(t *testing.T) {
	r := newInvRig(t)
	o, err := r.svc.Create(context.Background(), adminA, CreateInput{TenantID: u64(7), Name: "rot", DriverCode: sim.Code, Host: sim.HostRotating, SNMPPort: func() *int { v := 16101; return &v }()})
	if err != nil {
		t.Fatal(err)
	}
	poll := func() *RegisteredSummary {
		sum, err := r.svc.Poll(context.Background(), nocA, o.UUID)
		if err != nil || sum.Registered == nil {
			t.Fatalf("poll: %v %+v", err, sum)
		}
		return sum.Registered
	}
	// 1: lima ONU, 0002 ONLINE
	if s := poll(); s.New != 5 || s.StateChanged != 0 {
		t.Fatalf("poll 1: %+v", s)
	}
	if got := r.registered(t, o); got["SIMREG0002"] != "ONLINE" || len(got) != 5 {
		t.Fatalf("poll 1 inventaris: %v", got)
	}
	// 2: 0002 → LOS (berubah), 0005 hilang, 0006 baru
	s := poll()
	if s.StateChanged != 1 || s.New != 1 || s.Removed != 1 || s.Seen != 5 {
		t.Fatalf("poll 2: %+v", s)
	}
	got := r.registered(t, o)
	if got["SIMREG0002"] != "LOS" || got["SIMREG0006"] != "ONLINE" || got["SIMREG0005"] != "" || len(got) != 5 {
		t.Fatalf("poll 2 inventaris: %v", got)
	}
	if len(r.pub.events) != 1 {
		t.Fatalf("harus tepat satu siaran: %+v", r.pub.events)
	}
	ev := r.pub.events[0]
	ch, _ := ev.payload["changes"].([]StateChange)
	if ev.tenant != 7 || ev.typ != "ONU_STATE_CHANGED" || ev.payload["changed"] != 1 || ev.payload["olt_id"] != o.UUID || len(ch) != 1 ||
		ch[0] != (StateChange{SerialNumber: "SIMREG0002", PonPort: "sim/1/1", From: "ONLINE", To: "LOS"}) {
		t.Fatalf("siaran = %+v", ev)
	}
	// 3: kosong → semua dihapus-lunak
	if s := poll(); s.Seen != 0 || s.Removed != 5 {
		t.Fatalf("poll 3: %+v", s)
	}
	if got := r.registered(t, o); len(got) != 0 {
		t.Fatalf("poll 3 inventaris: %v", got)
	}
	// 4: siklus ulang → semua muncul lagi sebagai baru (dihidupkan kembali), bukan perubahan keadaan
	if s := poll(); s.New != 5 || s.StateChanged != 0 {
		t.Fatalf("poll 4: %+v", s)
	}
	if len(r.pub.events) != 1 {
		t.Errorf("hidup kembali bukan perubahan keadaan, tak boleh menyiarkan lagi: %d event", len(r.pub.events))
	}
}

// ONU yang hilang lalu muncul dengan huruf berbeda tetap satu baris (kolasi tak peka huruf besar/kecil), bukan ONU baru + hilang.
func TestPoll_ONUTerdaftar_SerialTakPekaHurufBesarKecil(t *testing.T) {
	d := &scriptedInv{states: [][]domain.ReportedONU{
		{{PonPort: "p/1", SerialNumber: "ABC123", State: domain.OLTStateOnline}},
		{{PonPort: "p/1", SerialNumber: "abc123", State: domain.OLTStateLOS}},
	}}
	r := newInvRig(t, d)
	o := mkWith(t, r, d.Code())
	for i := 0; i < 2; i++ {
		if _, err := r.svc.Poll(context.Background(), nocA, o.UUID); err != nil {
			t.Fatal(err)
		}
	}
	rows, _, _ := r.svc.ListRegistered(context.Background(), superadmin, RegisteredListInput{OLTUUID: o.UUID}, domain.Pagination{})
	if len(rows) != 1 || rows[0].State != domain.OLTStateLOS {
		t.Fatalf("harus satu baris berstatus LOS: %+v", rows)
	}
	if len(r.pub.events) != 1 {
		t.Errorf("perubahan ONLINE→LOS harus disiarkan sekali: %d", len(r.pub.events))
	}
}

// scriptedInv = driver uji: setiap panggilan ListRegisteredONUs mengembalikan skrip berikutnya (yang terakhir diulang).
type scriptedInv struct {
	states  [][]domain.ReportedONU
	n       int
	listErr error
	noInv   bool
}

func (d *scriptedInv) Code() string { return "scripted" }
func (d *scriptedInv) Probe(context.Context, domain.OLTConnection) (domain.OLTInfo, error) {
	return domain.OLTInfo{Name: "x", Version: "1"}, nil
}
func (d *scriptedInv) ListUnregisteredONUs(context.Context, domain.OLTConnection) ([]domain.UnregisteredONU, error) {
	return nil, nil
}
func (d *scriptedInv) ListRegisteredONUs(context.Context, domain.OLTConnection) ([]domain.ReportedONU, error) {
	if d.listErr != nil {
		return nil, d.listErr
	}
	i := d.n
	if i >= len(d.states) {
		i = len(d.states) - 1
	}
	d.n++
	return d.states[i], nil
}

// onlyBasic = driver yang HANYA memenuhi OLTDriver (tanpa kemampuan inventaris).
type onlyBasic struct{}

func (onlyBasic) Code() string { return "basic" }
func (onlyBasic) Probe(context.Context, domain.OLTConnection) (domain.OLTInfo, error) {
	return domain.OLTInfo{Name: "b", Version: "1"}, nil
}
func (onlyBasic) ListUnregisteredONUs(context.Context, domain.OLTConnection) ([]domain.UnregisteredONU, error) {
	return nil, nil
}

func mkWith(t *testing.T, r *invRig, code string) *domain.OLT {
	t.Helper()
	o, err := r.svc.Create(context.Background(), adminA, CreateInput{TenantID: u64(7), Name: "o-" + code, DriverCode: code, Host: "10.9.9.9"})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestPoll_ONUTerdaftar_DriverTanpaKemampuanDiLewati(t *testing.T) {
	r := newInvRig(t, onlyBasic{})
	o := mkWith(t, r, "basic")
	sum, err := r.svc.Poll(context.Background(), nocA, o.UUID)
	if err != nil || sum.Registered != nil {
		t.Fatalf("driver tanpa inventaris: err=%v registered=%+v", err, sum.Registered)
	}
	if r.inv.upserts != 0 {
		t.Error("inventaris tak boleh disentuh")
	}
}

func TestPoll_ONUTerdaftar_TanpaInventarisAktifTetapBerjalanSepertiDulu(t *testing.T) {
	r := newRig(t) // tanpa WithInventory
	o := r.mkOLT(t, adminA, "10.1.1.1")
	sum, err := r.svc.Poll(context.Background(), nocA, o.UUID)
	if err != nil || sum.Registered != nil || sum.Seen != 3 {
		t.Fatalf("err=%v sum=%+v", err, sum)
	}
	if _, _, err := r.svc.ListRegistered(context.Background(), adminA, RegisteredListInput{OLTUUID: o.UUID}, domain.Pagination{}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("tanpa inventaris daftar harus ErrInvalidInput, dapat %v", err)
	}
}

func TestPoll_ONUTerdaftar_GalatDriverMencatatStatusDanTidakBocor(t *testing.T) {
	d := &scriptedInv{listErr: fmt.Errorf("raw: password=rahasia123 at 10.9.9.9: %w", domain.ErrOLTUnreachable)}
	r := newInvRig(t, d)
	o := mkWith(t, r, d.Code())
	_, err := r.svc.Poll(context.Background(), nocA, o.UUID)
	if !errors.Is(err, domain.ErrOLTUnreachable) || strings.Contains(err.Error(), "rahasia123") {
		t.Fatalf("err = %v, mau ErrOLTUnreachable tanpa teks mentah perangkat", err)
	}
	got, _ := r.svc.Get(context.Background(), adminA, o.UUID)
	if got.PollStatus != domain.OLTPollUnreachable || got.LastPollError == nil || strings.Contains(*got.LastPollError, "rahasia123") {
		t.Fatalf("status poll = %q err=%v", got.PollStatus, got.LastPollError)
	}
}

func TestPoll_ONUTerdaftar_TerlaluBanyakTidakDiterapkan(t *testing.T) {
	many := make([]domain.ReportedONU, domain.OLTMaxRegisteredONUs+1)
	for i := range many {
		many[i] = domain.ReportedONU{PonPort: "p", SerialNumber: fmt.Sprintf("S%06d", i), State: domain.OLTStateOnline}
	}
	d := &scriptedInv{states: [][]domain.ReportedONU{{{PonPort: "p/1", SerialNumber: "KEEP1", State: domain.OLTStateOnline}}, many}}
	r := newInvRig(t, d)
	o := mkWith(t, r, d.Code())
	if _, err := r.svc.Poll(context.Background(), nocA, o.UUID); err != nil {
		t.Fatal(err)
	}
	_, err := r.svc.Poll(context.Background(), nocA, o.UUID)
	if !errors.Is(err, domain.ErrUpstreamUnavailable) || !strings.Contains(err.Error(), "lebih dari") {
		t.Fatalf("err = %v", err)
	}
	if got := r.registered(t, o); !mapsEqual(got, map[string]string{"KEEP1": "ONLINE"}) {
		t.Fatalf("daftar terpotong tidak boleh diterapkan: inventaris berubah menjadi %d baris", len(got))
	}
	if g, _ := r.svc.Get(context.Background(), adminA, o.UUID); g.PollStatus != domain.OLTPollError {
		t.Errorf("status poll = %s", g.PollStatus)
	}
}

func TestCleanReported(t *testing.T) {
	rx := func(v float64) *float64 { return &v }
	long := strings.Repeat("x", 200)
	in := []domain.ReportedONU{
		{PonPort: "p/1", SerialNumber: "A1", State: "ONLINE", RxPowerDBM: rx(-21)},
		{PonPort: "p/1", SerialNumber: "a1", State: "LOS"},                            // duplikat tak peka huruf besar/kecil
		{PonPort: "p/1", SerialNumber: "", State: "ONLINE"},                           // serial kosong
		{PonPort: "p/1", SerialNumber: "B\x00C", State: "ONLINE"},                     // karakter kontrol
		{PonPort: long, SerialNumber: "LONGPORT", State: "ONLINE"},                    // port kepanjangan
		{PonPort: "p/2", SerialNumber: "WEIRD", State: "MELAYANG"},                    // keadaan tak dikenal -> UNKNOWN
		{PonPort: "p/2", SerialNumber: "RXBAD", State: "ONLINE", RxPowerDBM: rx(500)}, // RX tak masuk akal -> dibuang
		{PonPort: "p/2", SerialNumber: "VSTATE", State: "ONLINE", VendorState: long},  // keadaan asli kepanjangan
	}
	out, skipped := cleanReported(in)
	if skipped != 5 {
		t.Errorf("dilewati = %d, mau 5 (duplikat, kosong, kontrol, port panjang, vendor_state panjang)", skipped)
	}
	byS := map[string]domain.ReportedONU{}
	for _, o := range out {
		byS[o.SerialNumber] = o
	}
	if len(out) != 3 || byS["A1"].State != "ONLINE" || byS["A1"].RxPowerDBM == nil {
		t.Fatalf("hasil = %+v", out)
	}
	if byS["WEIRD"].State != domain.OLTStateUnknown {
		t.Errorf("keadaan tak dikenal harus UNKNOWN, dapat %q", byS["WEIRD"].State)
	}
	if byS["RXBAD"].RxPowerDBM != nil {
		t.Errorf("RX di luar rentang fisik harus dibuang, dapat %v", *byS["RXBAD"].RxPowerDBM)
	}
}

func TestSiaran_RincianDibatasiTetapJumlahBenar(t *testing.T) {
	up, down := make([]domain.ReportedONU, 120), make([]domain.ReportedONU, 120)
	for i := range up {
		s := fmt.Sprintf("N%03d", i)
		up[i] = domain.ReportedONU{PonPort: "p/1", SerialNumber: s, State: domain.OLTStateOnline}
		down[i] = domain.ReportedONU{PonPort: "p/1", SerialNumber: s, State: domain.OLTStateLOS}
	}
	d := &scriptedInv{states: [][]domain.ReportedONU{up, down}}
	r := newInvRig(t, d)
	o := mkWith(t, r, d.Code())
	for i := 0; i < 2; i++ {
		if _, err := r.svc.Poll(context.Background(), nocA, o.UUID); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.pub.events) != 1 {
		t.Fatalf("event = %d", len(r.pub.events))
	}
	ch, _ := r.pub.events[0].payload["changes"].([]StateChange)
	if r.pub.events[0].payload["changed"] != 120 || len(ch) != maxStateChangesInEvent {
		t.Errorf("changed=%v rincian=%d, mau 120 dan %d", r.pub.events[0].payload["changed"], len(ch), maxStateChangesInEvent)
	}
}

// ---- pembacaan & scope ----

func TestListRegisteredDanPorts_ScopeTenant(t *testing.T) {
	r := newInvRig(t)
	o := r.mkOLT(t, adminA, "10.1.1.1")
	if _, err := r.svc.Poll(context.Background(), nocA, o.UUID); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, a := range []domain.Actor{adminB, noTenant} {
		if _, _, err := r.svc.ListRegistered(ctx, a, RegisteredListInput{OLTUUID: o.UUID}, domain.Pagination{}); err == nil {
			t.Errorf("aktor %+v tak boleh membaca ONU OLT tenant lain", a.UserID)
		}
		if _, err := r.svc.Ports(ctx, a, o.UUID); err == nil {
			t.Errorf("aktor %+v tak boleh membaca port OLT tenant lain", a.UserID)
		}
	}
	for _, a := range []domain.Actor{viewerA, nocA, adminA, superadmin} {
		rows, total, err := r.svc.ListRegistered(ctx, a, RegisteredListInput{OLTUUID: o.UUID}, domain.Pagination{})
		if err != nil || total != 5 || len(rows) != 5 {
			t.Errorf("aktor %d: total=%d err=%v", a.UserID, total, err)
		}
	}
	rows, _, err := r.svc.ListRegistered(ctx, adminA, RegisteredListInput{OLTUUID: o.UUID, State: "LOS"}, domain.Pagination{})
	if err != nil || len(rows) != 1 || rows[0].SerialNumber != "SIMREG0002" {
		t.Errorf("filter keadaan: %+v err=%v", rows, err)
	}
	if _, _, err := r.svc.ListRegistered(ctx, adminA, RegisteredListInput{OLTUUID: o.UUID, State: "MELAYANG"}, domain.Pagination{}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("keadaan tak dikenal harus ErrInvalidInput: %v", err)
	}
	ports, err := r.svc.Ports(ctx, adminA, o.UUID)
	if err != nil || len(ports) != 2 {
		t.Fatalf("port = %+v err=%v", ports, err)
	}
	total := 0
	for _, p := range ports {
		total += p.Total
	}
	if total != 5 {
		t.Errorf("jumlah ONU di seluruh port = %d", total)
	}
	if _, _, err := r.svc.ListRegistered(ctx, adminA, RegisteredListInput{OLTUUID: "tidak-ada"}, domain.Pagination{}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("OLT tak ada harus ErrNotFound: %v", err)
	}
}

type fakeDevs struct {
	dev *domain.Device
	err error
}

func (f fakeDevs) Get(context.Context, domain.Actor, uint64) (*domain.Device, error) {
	return f.dev, f.err
}

func TestForDevice(t *testing.T) {
	r := newInvRig(t)
	o := r.mkOLT(t, adminA, "10.1.1.1")
	if _, err := r.svc.Poll(context.Background(), nocA, o.UUID); err != nil {
		t.Fatal(err)
	}
	r.inv.activeBy[7] = 1
	tid := uint64(7)
	ctx := context.Background()

	r.svc.WithDevices(fakeDevs{dev: &domain.Device{ID: 1, TenantID: &tid, SerialNumber: "simreg0002"}})
	v, err := r.svc.ForDevice(ctx, adminA, 1)
	if err != nil || !v.Linked || v.ONU == nil || v.ONU.State != domain.OLTStateLOS || v.ONU.PonPort != "sim/1/1" || !v.TenantHasOLT {
		t.Fatalf("tertaut (serial tak peka huruf besar/kecil): %+v err=%v", v, err)
	}

	r.svc.WithDevices(fakeDevs{dev: &domain.Device{ID: 2, TenantID: &tid, SerialNumber: "TIDAKADA"}})
	if v, err := r.svc.ForDevice(ctx, adminA, 2); err != nil || v.Linked || v.ONU != nil || !v.TenantHasOLT {
		t.Fatalf("serial tak ditemukan tetapi tenant punya OLT: %+v err=%v", v, err)
	}

	r.inv.activeBy[7] = 0
	if v, _ := r.svc.ForDevice(ctx, adminA, 2); v.TenantHasOLT {
		t.Errorf("tenant tanpa OLT: tenant_has_olt harus false")
	}

	other := uint64(8)
	r.svc.WithDevices(fakeDevs{dev: &domain.Device{ID: 3, TenantID: &other, SerialNumber: "SIMREG0001"}})
	if v, _ := r.svc.ForDevice(ctx, adminB, 3); v.Linked {
		t.Errorf("ONU milik OLT tenant lain tak boleh tertaut ke perangkat tenant ini")
	}

	r.svc.WithDevices(fakeDevs{dev: &domain.Device{ID: 4, SerialNumber: "SIMREG0001"}})
	if v, err := r.svc.ForDevice(ctx, superadmin, 4); err != nil || v.Linked {
		t.Errorf("perangkat tanpa tenant tidak punya OLT: %+v err=%v", v, err)
	}

	r.svc.WithDevices(fakeDevs{err: domain.ErrForbidden})
	if _, err := r.svc.ForDevice(ctx, adminB, 5); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("akses perangkat ditolak harus diteruskan: %v", err)
	}
}

// ---- polling berkala ----

func TestPollDue(t *testing.T) {
	r := newInvRig(t)
	ok1 := r.mkOLT(t, adminA, "10.1.1.1")
	bad := r.mkOLT(t, adminA, sim.HostUnreachable)
	ok2 := r.mkOLT(t, adminA, "10.1.1.2")
	r.inv.due = []domain.OLT{{UUID: ok1.UUID, Name: ok1.Name}, {UUID: bad.UUID, Name: bad.Name}, {UUID: ok2.UUID, Name: ok2.Name}}

	n, err := r.svc.PollDue(context.Background(), 5*time.Minute)
	if err != nil || n != 2 {
		t.Fatalf("berhasil dipoll = %d err=%v, mau 2 (satu OLT gagal tidak menghentikan yang lain)", n, err)
	}
	if got := r.registered(t, ok1); len(got) != 5 {
		t.Errorf("OLT pertama harus terisi: %v", got)
	}
	if got := r.registered(t, ok2); len(got) != 5 {
		t.Errorf("OLT sesudah yang gagal tetap dipoll: %v", got)
	}
	if g, _ := r.svc.Get(context.Background(), adminA, bad.UUID); g.PollStatus != domain.OLTPollUnreachable {
		t.Errorf("OLT yang gagal harus tercatat UNREACHABLE: %s", g.PollStatus)
	}
	if n, err := r.svc.PollDue(context.Background(), 0); err != nil || n != 0 {
		t.Errorf("interval 0 = nonaktif: n=%d err=%v", n, err)
	}
	if n, _ := newRig(t).svc.PollDue(context.Background(), time.Minute); n != 0 {
		t.Errorf("tanpa inventaris tidak memoll")
	}
}

// Filter "bermasalah saja" dan urutan keparahan diteruskan ke repositori; sort asing ditolak (400), tidak diabaikan diam-diam.
func TestListRegistered_ProblemDanSortSeverity(t *testing.T) {
	d := &scriptedInv{states: [][]domain.ReportedONU{{
		{PonPort: "p/1", ONUIndex: "1", SerialNumber: "A-ONLINE", State: domain.OLTStateOnline},
		{PonPort: "p/1", ONUIndex: "2", SerialNumber: "B-OFFLINE", State: domain.OLTStateOffline},
		{PonPort: "p/1", ONUIndex: "3", SerialNumber: "C-LOS", State: domain.OLTStateLOS},
		{PonPort: "p/1", ONUIndex: "4", SerialNumber: "D-UNKNOWN", State: domain.OLTStateUnknown},
		{PonPort: "p/1", ONUIndex: "5", SerialNumber: "E-DYING", State: domain.OLTStateDyingGasp},
	}}}
	r := newInvRig(t, d)
	o := mkWith(t, r, d.Code())
	if _, err := r.svc.Poll(context.Background(), nocA, o.UUID); err != nil {
		t.Fatal(err)
	}
	list := func(in RegisteredListInput) ([]string, error) {
		in.OLTUUID = o.UUID
		rows, _, err := r.svc.ListRegistered(context.Background(), nocA, in, domain.Pagination{})
		var out []string
		for _, x := range rows {
			out = append(out, x.SerialNumber)
		}
		return out, err
	}
	eq := func(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }

	if got, _ := list(RegisteredListInput{Problem: true}); len(got) != 3 || eq(got, []string{"A-ONLINE"}) {
		t.Errorf("problem: UNKNOWN dan ONLINE bukan masalah; dapat %v", got)
	}
	for _, s := range []string{"A-ONLINE", "D-UNKNOWN"} {
		if got, _ := list(RegisteredListInput{Problem: true}); strings.Contains(strings.Join(got, ","), s) {
			t.Errorf("%s tak boleh ikut 'bermasalah saja': %v", s, got)
		}
	}
	if got, _ := list(RegisteredListInput{Sort: SortBySeverity}); !eq(got, []string{"C-LOS", "E-DYING", "B-OFFLINE", "D-UNKNOWN", "A-ONLINE"}) {
		t.Errorf("urut keparahan = %v", got)
	}
	if got, _ := list(RegisteredListInput{Sort: SortBySeverity, Problem: true}); !eq(got, []string{"C-LOS", "E-DYING", "B-OFFLINE"}) {
		t.Errorf("keparahan + bermasalah = %v", got)
	}
	// tanpa sort: urutan lama tidak berubah (kompatibel mundur)
	if got, _ := list(RegisteredListInput{}); len(got) != 5 || got[0] != "A-ONLINE" {
		t.Errorf("bawaan harus urutan port/serial, dapat %v", got)
	}
	if _, err := list(RegisteredListInput{Sort: "acak"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("sort asing harus ErrInvalidInput: %v", err)
	}
}
