package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"acs/internal/simcpe"
)

// S15 — redaman optik berbasis mapping: perangkat TR-181 (nokia) yang model-nya belum terdaftar.
// Setelah Inform, ACS mendeteksi data model dari parameter, me-resolve logical key optical.*
// (standar Device.Optical.Interface, skala 0,001) dan mengantre GetParameterValues; hasilnya
// disimpan sebagai metrik dBm (−23500 → −23,5 dBm; 2100 → +2,1 dBm).
func scenarioOpticalPoll(ctx context.Context, w *World) result {
	key := "nokia"
	dev, serial := w.devices[key], w.serials[key]
	if dev == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	// sesi 1: Inform -> ACS mengantre polling optik; sesi 2: GPV dijawab
	for i := 0; i < 2; i++ {
		rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}})
		if !rep.OK {
			return failr("sesi %d gagal: %s", i+1, rep.ErrMsg)
		}
	}
	var list struct {
		Data []struct {
			Rx *float64 `json:"rx_power_dbm"`
			Tx *float64 `json:"tx_power_dbm"`
		} `json:"data"`
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := w.pilotAdmin.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/optical-metrics?page_size=5", d.ID), nil, &list); err != nil {
			return failr("GET optical-metrics: %v", err)
		}
		if len(list.Data) > 0 {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if len(list.Data) == 0 {
		return failr("tidak ada metrik optik tersimpan (task: %s)", w.taskDump(d.ID))
	}
	m := list.Data[0]
	if m.Rx == nil || math.Abs(*m.Rx-(-23.5)) > 0.01 || m.Tx == nil || math.Abs(*m.Tx-2.1) > 0.01 {
		return failr("nilai metrik salah (rx=%v tx=%v), harus -23,5 / 2,1 dBm", m.Rx, m.Tx)
	}
	// Daftar perangkat membawa metrik terakhir itu (kolom Redaman): rx/tx/optical_at dari GET /devices.
	var devList struct {
		Data []struct {
			ID        uint64     `json:"id"`
			Rx        *float64   `json:"rx_power_dbm"`
			Tx        *float64   `json:"tx_power_dbm"`
			OpticalAt *time.Time `json:"optical_at"`
		} `json:"data"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/devices?search="+serial, nil, &devList); err != nil {
		return failr("GET /devices: %v", err)
	}
	var row *struct {
		ID        uint64     `json:"id"`
		Rx        *float64   `json:"rx_power_dbm"`
		Tx        *float64   `json:"tx_power_dbm"`
		OpticalAt *time.Time `json:"optical_at"`
	}
	for i := range devList.Data {
		if devList.Data[i].ID == d.ID {
			row = &devList.Data[i]
		}
	}
	if row == nil || row.Rx == nil || math.Abs(*row.Rx-(-23.5)) > 0.01 || row.Tx == nil || row.OpticalAt == nil {
		return failr("daftar perangkat tidak membawa metrik optik terakhir (row=%+v)", row)
	}
	if r := opticalAlertAndFilter(ctx, w, dev, d.ID, serial); !r.ok {
		return r
	}
	return pass("polling optik via mapping TR-181: rx %.1f dBm, tx %.1f dBm (skala 0,001 dari nilai mentah CPE); daftar perangkat ikut membawa rx/tx terakhir; RX kritis -28,5 dBm -> notifikasi OPTICAL_ALERT CRITICAL (sekali per 24 jam), filter optical_level & no_vendor benar", *m.Rx, *m.Tx)
}

// opticalAlertAndFilter: RX perangkat diturunkan ke -28,5 dBm (kritis) lalu dibaca ulang lewat refresh-summary:
// ADMIN tenant menerima OPTICAL_ALERT CRITICAL (dedupe: pembacaan kritis kedua tidak menggandakan), daftar perangkat
// dapat disaring optical_level=bad (dan tidak muncul di ok), no_vendor=true tidak memuatnya (vendor sudah terpetakan),
// dan nilai tak dikenal ditolak 400. Di akhir RX dikembalikan ke -23,5 dBm agar skenario lain tak terpengaruh.
func opticalAlertAndFilter(ctx context.Context, w *World, dev *simcpe.Device, id uint64, serial string) result {
	const rxPath = "Device.Optical.Interface.1.OpticalSignalLevel"
	readAt := func(raw string, want float64) error {
		dev.Set(map[string]string{rxPath: raw})
		if st, b := w.pilotAdmin.status(http.MethodPost, "/api/v1/devices/refresh-summary", map[string]any{"device_ids": []uint64{id}}); st != http.StatusOK {
			return fmt.Errorf("refresh-summary harus sukses (2xx), dapat %d %s", st, b)
		}
		for i := 0; i < 2; i++ {
			if rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}}); !rep.OK {
				return fmt.Errorf("sesi %d gagal: %s", i+1, rep.ErrMsg)
			}
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			var list struct {
				Data []struct {
					Rx *float64 `json:"rx_power_dbm"`
				} `json:"data"`
			}
			if err := w.pilotAdmin.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/optical-metrics?page_size=1", id), nil, &list); err != nil {
				return err
			}
			if len(list.Data) > 0 && list.Data[0].Rx != nil && math.Abs(*list.Data[0].Rx-want) < 0.01 {
				return nil
			}
			time.Sleep(300 * time.Millisecond)
		}
		return fmt.Errorf("metrik terbaru tidak menjadi %.1f dBm (task: %s)", want, w.taskDump(id))
	}
	countAlerts := func() (int, *notifItem, error) {
		var lp notifList
		if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/notifications?page_size=100", nil, &lp); err != nil {
			return 0, nil, err
		}
		n := 0
		var first *notifItem
		for i := range lp.Data {
			if lp.Data[i].Type == "OPTICAL_ALERT" && strings.Contains(lp.Data[i].Body, serial) {
				n++
				if first == nil {
					first = &lp.Data[i]
				}
			}
		}
		return n, first, nil
	}

	if err := readAt("-28500", -28.5); err != nil {
		return failr("pembacaan kritis: %v", err)
	}
	n, item, err := countAlerts()
	if err != nil {
		return failr("GET notifications: %v", err)
	}
	if n != 1 || item == nil || item.Severity != "CRITICAL" || item.Link == nil || *item.Link != fmt.Sprintf("/devices/%d", id) {
		return failr("RX kritis harus menghasilkan tepat 1 notifikasi OPTICAL_ALERT CRITICAL berlink perangkat (n=%d item=%+v)", n, item)
	}
	if err := readAt("-29000", -29.0); err != nil {
		return failr("pembacaan kritis kedua: %v", err)
	}
	if n2, _, err := countAlerts(); err != nil || n2 != 1 {
		return failr("pembacaan kritis berulang tidak boleh menggandakan notifikasi (n=%d err=%v)", n2, err)
	}

	has := func(query string) (bool, error) {
		var out struct {
			Data []struct {
				ID uint64 `json:"id"`
			} `json:"data"`
		}
		if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/devices?search="+serial+query, nil, &out); err != nil {
			return false, err
		}
		for _, d := range out.Data {
			if d.ID == id {
				return true, nil
			}
		}
		return false, nil
	}
	for q, want := range map[string]bool{"&optical_level=bad": true, "&optical_level=ok": false, "&optical_level=warn": false, "&optical_level=unknown": false, "&no_vendor=true": false} {
		got, err := has(q)
		if err != nil {
			return failr("GET /devices%s: %v", q, err)
		}
		if got != want {
			return failr("filter %s: perangkat muncul=%v, harus %v", q, got, want)
		}
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/devices?optical_level=ngawur", nil); st != http.StatusBadRequest {
		return failr("optical_level tak dikenal harus 400, dapat %d", st)
	}

	if err := readAt("-23500", -23.5); err != nil {
		return failr("memulihkan RX: %v", err)
	}
	if got, err := has("&optical_level=ok"); err != nil || !got {
		return failr("setelah pulih, filter optical_level=ok harus memuat perangkat (err=%v)", err)
	}
	return pass("")
}
