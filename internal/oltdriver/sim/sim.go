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
package sim

import (
	"context"
	"fmt"
	"sync"

	"acs/internal/domain"
)

const Code = "sim"

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
