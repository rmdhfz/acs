// Package sim = driver OLT simulator: deterministik, TANPA koneksi jaringan, dan tanpa meniru format keluaran vendor mana pun.
// Dipakai untuk pengembangan UI, e2e, dan demo selama driver vendor nyata belum ada (docs/arsitektur/OLT_CONTROLLER_DESIGN.md).
// Nonaktif secara bawaan (ACS_OLT_SIM_ENABLED); jangan diaktifkan di produksi karena menampilkan ONU palsu.
//
// Perilaku dipilih lewat host (alamat literal, supaya lolos validasi netguard tanpa DNS):
//
//	10.255.0.2  tidak dapat dijangkau (ErrOLTUnreachable)
//	10.255.0.3  autentikasi ditolak   (ErrOLTAuthFailed)
//	10.255.0.4  berputar: setiap polling mengembalikan himpunan berbeda (A,B,C → B,C,D → kosong → ...), untuk menguji
//	            ONU baru, ONU yang tetap, dan ONU yang menghilang. Hitungannya per (host, snmp_port) di memori proses, jadi
//	            memakai snmp_port berbeda memberi OLT dengan urutan yang mulai dari awal (berguna untuk e2e yang diulang)
//	10.255.0.5  selalu kosong
//	lainnya     selalu A,B,C
//
// ONU TERDAFTAR (ListRegisteredONUs, kemampuan opsional) mengikuti host yang sama dengan hitungan polling terpisah:
//
//	default     lima ONU di dua port (SIMREG0001..0005) dengan keadaan ONLINE, LOS, DYING_GASP, ONLINE, OFFLINE
//	10.255.0.4  berputar: semua tampil (0002 ONLINE) → 0002 menjadi LOS, 0005 hilang, 0006 baru → kosong → ...
//	10.255.0.5  selalu kosong
//	            (10.255.0.2/.3 gagal seperti di atas)
package sim

import (
	"context"
	"fmt"
	"sync"

	"acs/internal/domain"
)

const Code = "sim"

// Dipenuhi: kemampuan opsional membaca ONU terdaftar.
var _ domain.OLTInventoryDriver = (*Driver)(nil)

// Host khusus perilaku simulator.
const (
	HostUnreachable = "10.255.0.2"
	HostAuthFailed  = "10.255.0.3"
	HostRotating    = "10.255.0.4"
	HostEmpty       = "10.255.0.5"
)

type Driver struct {
	mu    sync.Mutex
	polls map[string]int
}

func New() *Driver { return &Driver{polls: map[string]int{}} }

func (d *Driver) Code() string { return Code }

func (d *Driver) fail(ctx context.Context, host string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch host {
	case HostUnreachable:
		return domain.ErrOLTUnreachable
	case HostAuthFailed:
		return domain.ErrOLTAuthFailed
	}
	return nil
}

func (d *Driver) Probe(ctx context.Context, c domain.OLTConnection) (domain.OLTInfo, error) {
	if err := d.fail(ctx, c.Host); err != nil {
		return domain.OLTInfo{}, err
	}
	return domain.OLTInfo{Name: "SIM-OLT-" + c.Host, Version: "sim-1.0"}, nil
}

var (
	onuA = domain.UnregisteredONU{PonPort: "sim/1/1", SerialNumber: "SIMONU0001", ONUType: "SIM-ONU"}
	onuB = domain.UnregisteredONU{PonPort: "sim/1/1", SerialNumber: "SIMONU0002", ONUType: "SIM-ONU"}
	onuC = domain.UnregisteredONU{PonPort: "sim/1/2", SerialNumber: "SIMONU0003", ONUType: "SIM-ONU-X"}
	onuD = domain.UnregisteredONU{PonPort: "sim/1/3", SerialNumber: "SIMONU0004", ONUType: "SIM-ONU"}
)

func (d *Driver) ListUnregisteredONUs(ctx context.Context, c domain.OLTConnection) ([]domain.UnregisteredONU, error) {
	if err := d.fail(ctx, c.Host); err != nil {
		return nil, err
	}
	switch c.Host {
	case HostEmpty:
		return nil, nil
	case HostRotating:
		key := fmt.Sprintf("%s:%d", c.Host, c.SNMPPort)
		d.mu.Lock()
		n := d.polls[key]
		d.polls[key] = n + 1
		d.mu.Unlock()
		switch n % 3 {
		case 0:
			return []domain.UnregisteredONU{onuA, onuB, onuC}, nil
		case 1:
			return []domain.UnregisteredONU{onuB, onuC, onuD}, nil
		default:
			return nil, nil
		}
	}
	return []domain.UnregisteredONU{onuA, onuB, onuC}, nil
}

func f64(v float64) *float64 { return &v }

func reg(port, idx, serial, state, vendor string, rx *float64) domain.ReportedONU {
	return domain.ReportedONU{PonPort: port, ONUIndex: idx, SerialNumber: serial, ONUType: "SIM-ONU", State: state, VendorState: vendor, RxPowerDBM: rx}
}

// registered menyusun himpunan ONU terdaftar; step memilih varian untuk mode berputar (0 = awal).
func registered(step int) []domain.ReportedONU {
	r2 := reg("sim/1/1", "2", "SIMREG0002", domain.OLTStateLOS, "sim-los", nil)
	switch step {
	case 0:
		r2 = reg("sim/1/1", "2", "SIMREG0002", domain.OLTStateOnline, "sim-working", f64(-22.1))
	case 2:
		return nil
	}
	out := []domain.ReportedONU{
		reg("sim/1/1", "1", "SIMREG0001", domain.OLTStateOnline, "sim-working", f64(-21.5)),
		r2,
		reg("sim/1/1", "3", "SIMREG0003", domain.OLTStateDyingGasp, "sim-dyinggasp", nil),
		reg("sim/1/2", "1", "SIMREG0004", domain.OLTStateOnline, "sim-working", f64(-24.8)),
	}
	if step == 1 {
		return append(out, reg("sim/1/3", "1", "SIMREG0006", domain.OLTStateOnline, "sim-working", f64(-19.9)))
	}
	return append(out, reg("sim/1/2", "2", "SIMREG0005", domain.OLTStateOffline, "sim-offline", nil))
}

func (d *Driver) ListRegisteredONUs(ctx context.Context, c domain.OLTConnection) ([]domain.ReportedONU, error) {
	if err := d.fail(ctx, c.Host); err != nil {
		return nil, err
	}
	switch c.Host {
	case HostEmpty:
		return nil, nil
	case HostRotating:
		key := fmt.Sprintf("reg:%s:%d", c.Host, c.SNMPPort)
		d.mu.Lock()
		n := d.polls[key]
		d.polls[key] = n + 1
		d.mu.Unlock()
		return registered(n % 3), nil
	}
	return registered(-1), nil
}
