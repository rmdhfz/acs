package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
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
	return pass("polling optik via mapping TR-181: rx %.1f dBm, tx %.1f dBm (skala 0,001 dari nilai mentah CPE)", *m.Rx, *m.Tx)
}
