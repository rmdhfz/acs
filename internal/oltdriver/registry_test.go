package oltdriver_test

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
	"acs/internal/oltdriver"
	"acs/internal/oltdriver/sim"
)

type stubDriver struct{ code string }

func (s stubDriver) Code() string { return s.code }
func (s stubDriver) Probe(context.Context, domain.OLTConnection) (domain.OLTInfo, error) {
	return domain.OLTInfo{}, nil
}
func (s stubDriver) ListUnregisteredONUs(context.Context, domain.OLTConnection) ([]domain.UnregisteredONU, error) {
	return nil, nil
}

func TestRegistry(t *testing.T) {
	r := oltdriver.NewRegistry()
	if _, ok := r.Get("sim"); ok || len(r.Codes()) != 0 {
		t.Fatal("registry baru harus kosong")
	}
	if err := r.Register(stubDriver{"zeta"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(stubDriver{"alpha"}); err != nil {
		t.Fatal(err)
	}
	if got := r.Codes(); len(got) != 2 || got[0] != "alpha" || got[1] != "zeta" {
		t.Fatalf("Codes harus terurut: %v", got)
	}
	if d, ok := r.Get("alpha"); !ok || d.Code() != "alpha" {
		t.Fatalf("Get alpha: %v %v", d, ok)
	}
	if err := r.Register(stubDriver{"alpha"}); err == nil {
		t.Fatal("kode ganda harus ditolak")
	}
	for _, bad := range []string{"", "Sim", "a b", "-x", "x_y", "a/b", "this-driver-code-is-way-too-long-123"} {
		if err := r.Register(stubDriver{bad}); err == nil {
			t.Errorf("kode %q harus ditolak", bad)
		}
	}
}

func serials(l []domain.UnregisteredONU) []string {
	out := []string{}
	for _, o := range l {
		out = append(out, o.SerialNumber)
	}
	return out
}

func TestSim_Behaviours(t *testing.T) {
	ctx := context.Background()
	d := sim.New()
	conn := func(h string) domain.OLTConnection { return domain.OLTConnection{Host: h} }

	// tidak dapat dijangkau / autentikasi ditolak: galat membungkus ErrUpstreamUnavailable, baik Probe maupun List
	for host, want := range map[string]error{sim.HostUnreachable: domain.ErrOLTUnreachable, sim.HostAuthFailed: domain.ErrOLTAuthFailed} {
		if _, err := d.Probe(ctx, conn(host)); !errors.Is(err, want) || !errors.Is(err, domain.ErrUpstreamUnavailable) {
			t.Fatalf("Probe %s: %v", host, err)
		}
		if _, err := d.ListUnregisteredONUs(ctx, conn(host)); !errors.Is(err, want) {
			t.Fatalf("List %s: %v", host, err)
		}
	}

	// Probe normal
	if info, err := d.Probe(ctx, conn("10.1.1.1")); err != nil || info.Name != "SIM-OLT-10.1.1.1" || info.Version == "" {
		t.Fatalf("Probe: %+v %v", info, err)
	}

	// bawaan: A,B,C stabil; kosong: tidak ada
	for i := 0; i < 2; i++ {
		l, err := d.ListUnregisteredONUs(ctx, conn("10.1.1.1"))
		if err != nil || len(l) != 3 || l[0].SerialNumber != "SIMONU0001" {
			t.Fatalf("bawaan: %v %v", serials(l), err)
		}
	}
	if l, err := d.ListUnregisteredONUs(ctx, conn(sim.HostEmpty)); err != nil || len(l) != 0 {
		t.Fatalf("kosong: %v %v", serials(l), err)
	}

	// berputar: A,B,C -> B,C,D -> kosong -> A,B,C ...; hitungan per host
	want := [][]string{
		{"SIMONU0001", "SIMONU0002", "SIMONU0003"},
		{"SIMONU0002", "SIMONU0003", "SIMONU0004"},
		{},
		{"SIMONU0001", "SIMONU0002", "SIMONU0003"},
	}
	for i, w := range want {
		l, err := d.ListUnregisteredONUs(ctx, conn(sim.HostRotating))
		if err != nil {
			t.Fatal(err)
		}
		got := serials(l)
		if len(got) != len(w) {
			t.Fatalf("polling ke-%d: %v, harapan %v", i, got, w)
		}
		for j := range w {
			if got[j] != w[j] {
				t.Fatalf("polling ke-%d: %v, harapan %v", i, got, w)
			}
		}
	}

	// hitungan per (host, snmp_port): port lain memulai urutan dari awal tanpa mengganggu OLT yang sedang berjalan
	other := domain.OLTConnection{Host: sim.HostRotating, SNMPPort: 20001}
	if l, err := d.ListUnregisteredONUs(ctx, other); err != nil || len(l) != 3 || l[0].SerialNumber != "SIMONU0001" {
		t.Fatalf("port lain harus mulai dari awal: %v %v", serials(l), err)
	}
	if l, _ := d.ListUnregisteredONUs(ctx, conn(sim.HostRotating)); len(l) != 3 || l[0].SerialNumber != "SIMONU0002" {
		t.Fatalf("urutan OLT semula harus lanjut (polling ke-5 = B,C,D): %v", serials(l))
	}

	// ctx yang dibatalkan dihormati
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := d.ListUnregisteredONUs(cctx, conn("10.1.1.1")); !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx batal harus dihormati: %v", err)
	}
}
