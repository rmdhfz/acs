package olt

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

func snap(states ...string) []domain.ReportedONU {
	var out []domain.ReportedONU
	for _, s := range states {
		out = append(out, domain.ReportedONU{PonPort: "1/1/1", ONUIndex: "1", SerialNumber: "HIST0001", State: s})
	}
	return out
}

// histRig: skrip polling satu ONU (HIST0001); setiap elemen = satu polling (nil = ONU tidak dilaporkan).
func histRig(t *testing.T, polls ...[]domain.ReportedONU) (*invRig, *domain.OLT, string) {
	t.Helper()
	d := &scriptedInv{states: polls}
	r := newInvRig(t, d)
	o := mkWith(t, r, d.Code())
	var onuUUID string
	for i := range polls {
		if _, err := r.svc.Poll(context.Background(), nocA, o.UUID); err != nil {
			t.Fatalf("poll %d: %v", i+1, err)
		}
		if onuUUID == "" {
			if rows, _, _ := r.svc.ListRegistered(context.Background(), nocA, RegisteredListInput{OLTUUID: o.UUID}, domain.Pagination{}); len(rows) > 0 {
				onuUUID = rows[0].UUID
			}
		}
	}
	return r, o, onuUUID
}

func codeStr(p *string) string {
	if p == nil {
		return "-"
	}
	return *p
}

func trail(h *StateHistory) []string {
	var out []string
	for _, e := range h.Events {
		out = append(out, codeStr(e.FromState)+">"+codeStr(e.ToState))
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestHistory_MencatatBaruPerubahanHilangDanMuncul(t *testing.T) {
	r, o, onu := histRig(t,
		snap(domain.OLTStateOnline),
		snap(domain.OLTStateOnline), // tidak berubah: tidak ada peristiwa
		snap(domain.OLTStateLOS),
		nil,                         // hilang dari OLT
		snap(domain.OLTStateOnline), // muncul lagi
	)
	h, err := r.svc.ONUHistory(context.Background(), nocA, o.UUID, onu, domain.Pagination{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-" + ">ONLINE", "ONLINE>LOS", "LOS>-", "-" + ">ONLINE"}
	// terbaru dulu: muncul lagi, hilang, LOS, baru
	wantNewestFirst := []string{want[3], want[2], want[1], want[0]}
	if !eq(trail(h), wantNewestFirst) || h.Total != 4 {
		t.Fatalf("riwayat = %v (total %d), mau %v", trail(h), h.Total, wantNewestFirst)
	}
	if h.Events[3].PonPort != "1/1/1" {
		t.Errorf("port harus ikut tercatat: %+v", h.Events[3])
	}
	// hanya ONLINE>LOS yang perubahan sejati; muncul/hilang bukan perubahan keadaan
	if h.Changes24h != 1 || h.Flapping {
		t.Errorf("changes24h=%d flapping=%v, mau 1/false", h.Changes24h, h.Flapping)
	}
}

func TestHistory_FlappingMemerlukanEmpatPerubahanDalamSatuJam(t *testing.T) {
	// 3 perubahan: belum flapping
	r, o, onu := histRig(t, snap(domain.OLTStateOnline), snap(domain.OLTStateLOS), snap(domain.OLTStateOnline), snap(domain.OLTStateLOS))
	h, _ := r.svc.ONUHistory(context.Background(), nocA, o.UUID, onu, domain.Pagination{})
	if h.Changes24h != 3 || h.Flapping {
		t.Errorf("3 perubahan: changes=%d flapping=%v", h.Changes24h, h.Flapping)
	}
	// 4 perubahan: flapping
	r, o, onu = histRig(t, snap(domain.OLTStateOnline), snap(domain.OLTStateLOS), snap(domain.OLTStateOnline), snap(domain.OLTStateLOS), snap(domain.OLTStateOnline))
	h, _ = r.svc.ONUHistory(context.Background(), nocA, o.UUID, onu, domain.Pagination{})
	if h.Changes24h != 4 || !h.Flapping {
		t.Errorf("4 perubahan: changes=%d flapping=%v", h.Changes24h, h.Flapping)
	}
}

func TestHistory_PaginasiTerbaruDulu(t *testing.T) {
	r, o, onu := histRig(t, snap(domain.OLTStateOnline), snap(domain.OLTStateLOS), snap(domain.OLTStateOnline))
	h, err := r.svc.ONUHistory(context.Background(), nocA, o.UUID, onu, domain.Pagination{Page: 1, PageSize: 2})
	if err != nil || len(h.Events) != 2 || h.Total != 3 || codeStr(h.Events[0].ToState) != "ONLINE" || codeStr(h.Events[0].FromState) != "LOS" {
		t.Fatalf("halaman 1: %v total=%d err=%v", trail(h), h.Total, err)
	}
	h, _ = r.svc.ONUHistory(context.Background(), nocA, o.UUID, onu, domain.Pagination{Page: 2, PageSize: 2})
	if len(h.Events) != 1 || codeStr(h.Events[0].FromState) != "-" {
		t.Errorf("halaman 2 harus berisi peristiwa tertua: %v", trail(h))
	}
}

func TestHistory_KegagalanMenyimpanRiwayatTidakMenggagalkanPolling(t *testing.T) {
	d := &scriptedInv{states: [][]domain.ReportedONU{snap(domain.OLTStateOnline), snap(domain.OLTStateLOS)}}
	r := newInvRig(t, d)
	r.inv.eventErr = errors.New("tabel log rusak")
	o := mkWith(t, r, d.Code())
	for i := 0; i < 2; i++ {
		if _, err := r.svc.Poll(context.Background(), nocA, o.UUID); err != nil {
			t.Fatalf("poll %d harus tetap berhasil: %v", i+1, err)
		}
	}
	if got := r.registered(t, o); got["HIST0001"] != domain.OLTStateLOS {
		t.Errorf("inventaris harus tetap benar walau riwayat gagal: %v", got)
	}
}

func TestHistory_AksesDanKetidakcocokan(t *testing.T) {
	r, o, onu := histRig(t, snap(domain.OLTStateOnline))
	ctx := context.Background()
	if _, err := r.svc.ONUHistory(ctx, adminB, o.UUID, onu, domain.Pagination{}); !errors.Is(err, domain.ErrForbidden) && !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("tenant lain harus ditolak: %v", err)
	}
	if _, err := r.svc.ONUHistory(ctx, nocA, o.UUID, "00000000-0000-0000-0000-000000000000", domain.Pagination{}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ONU tak ada: %v", err)
	}
	// ONU milik OLT lain di tenant yang sama tidak boleh dibaca lewat OLT ini
	o2 := mkWith(t, r, "scripted")
	if _, err := r.svc.ONUHistory(ctx, nocA, o2.UUID, onu, domain.Pagination{}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ONU dari OLT lain: %v", err)
	}
	if _, err := r.svc.ONUHistory(ctx, superadmin, o.UUID, onu, domain.Pagination{}); err != nil {
		t.Errorf("superadmin: %v", err)
	}
}
