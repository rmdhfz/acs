package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"acs/internal/simcpe"
)

// S31 — resource faults (migrations/0038): cwmp:Fault dari CPE tercatat sebagai masalah AKTIF per (perangkat, kode, RPC).
//
//	FA-01  fault 9005 tercatat (kode, RPC, serial, task) dan muncul di GET /faults + GET /faults/:id
//	FA-02  fault yang sama berulang menaikkan occurrence_count, bukan menambah baris
//	FA-03  scope tenant: tenant lain tidak melihatnya di daftar dan 403 pada detail/tutup
//	FA-04  RPC yang sama kemudian BERHASIL -> fault tertutup otomatis
//	FA-05  operator menutup fault (DELETE) -> hilang dari daftar; muncul lagi bila CPE mengulang
type faultJSON struct {
	ID              string  `json:"id"`
	DeviceID        uint64  `json:"device_id"`
	DeviceSerial    string  `json:"device_serial"`
	TaskID          *uint64 `json:"task_id"`
	RPCMethod       string  `json:"rpc_method"`
	FaultCode       string  `json:"fault_code"`
	FaultString     string  `json:"fault_string"`
	OccurrenceCount uint32  `json:"occurrence_count"`
}

func listFaults(c *Client, q url.Values) ([]faultJSON, error) {
	var env listEnvelope[faultJSON]
	if err := c.do(http.MethodGet, "/api/v1/faults?"+q.Encode(), nil, &env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

func scenarioFaults(ctx context.Context, w *World) result {
	key := "zte"
	dev, serial := w.devices[key], w.serials[key]
	if dev == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	prof := simcpe.Profiles[key]
	badPath := prof.RootPrefix() + "X_E2E_FAULT_" + w.runID + ".Trigger"
	q := url.Values{}
	q.Set("device_id", fmt.Sprint(d.ID))

	// triggerFault: antre SetParameterValues ke parameter yang ditolak CPE (9005), lalu jalankan satu sesi.
	triggerFault := func() error {
		if st, body := w.pilotAdmin.status(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/tasks/set-parameter-values", d.ID),
			map[string]any{"values": map[string]string{badPath: "x"}}); st >= 400 {
			return fmt.Errorf("POST set-parameter-values: HTTP %d %s", st, body)
		}
		rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}, FaultParamSubstr: "X_E2E_FAULT"})
		if !rep.OK || len(rep.FaultsSent) == 0 {
			return fmt.Errorf("CPE tidak membalas Fault (ok=%v err=%s rpc=%v)", rep.OK, rep.ErrMsg, rep.RPCsReceived)
		}
		time.Sleep(500 * time.Millisecond)
		return nil
	}

	// FA-01
	if err := triggerFault(); err != nil {
		return failr("FA-01: %v", err)
	}
	fs, err := listFaults(w.pilotAdmin, q)
	if err != nil {
		return failr("GET /faults: %v", err)
	}
	if len(fs) != 1 {
		return failr("FA-01: %d fault aktif untuk perangkat, harusnya 1", len(fs))
	}
	f := fs[0]
	if f.FaultCode != "9005" || f.RPCMethod != "SET_PARAMETER_VALUES" || f.DeviceSerial != serial || f.TaskID == nil || f.OccurrenceCount != 1 || f.ID == "" {
		return failr("FA-01: isi fault salah: %+v", f)
	}
	var one faultJSON
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/faults/"+f.ID, nil, &one); err != nil || one.ID != f.ID {
		return failr("FA-01: GET /faults/:id: %v (%+v)", err, one)
	}

	// FA-02
	if err := triggerFault(); err != nil {
		return failr("FA-02: %v", err)
	}
	fs, _ = listFaults(w.pilotAdmin, q)
	if len(fs) != 1 || fs[0].OccurrenceCount != 2 || fs[0].ID != f.ID {
		return failr("FA-02: setelah kejadian ke-2 harusnya 1 baris dengan occurrence_count 2, dapat %+v", fs)
	}

	// FA-03
	if other, _ := listFaults(w.pilot2Admin, q); len(other) != 0 {
		return failr("FA-03: tenant lain melihat %d fault perangkat ini", len(other))
	}
	if st, _ := w.pilot2Admin.status(http.MethodGet, "/api/v1/faults/"+f.ID, nil); st != http.StatusForbidden {
		return failr("FA-03: GET fault lintas tenant HTTP %d, harusnya 403", st)
	}
	if st, _ := w.pilot2Admin.status(http.MethodDelete, "/api/v1/faults/"+f.ID, nil); st != http.StatusForbidden {
		return failr("FA-03: DELETE fault lintas tenant HTTP %d, harusnya 403", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/faults/bukan-uuid-yang-ada", nil); st != http.StatusNotFound {
		return failr("FA-03: uuid tak dikenal HTTP %d, harusnya 404", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/faults?fault_code=12345678901234567", nil); st != http.StatusBadRequest {
		return failr("FA-03: fault_code kepanjangan HTTP %d, harusnya 400", st)
	}

	// FA-04: SetParameterValues yang sah selesai -> fault SET_PARAMETER_VALUES tertutup otomatis.
	ssidPath := prof.RootPrefix() + "LANDevice.1.WLANConfiguration.1.SSID"
	if prof.RootPrefix() == "Device." {
		ssidPath = "Device.WiFi.SSID.1.SSID"
	}
	if st, body := w.pilotAdmin.status(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/tasks/set-parameter-values", d.ID),
		map[string]any{"values": map[string]string{ssidPath: "e2e-fault-ok-" + w.runID}}); st >= 400 {
		return failr("FA-04: POST set-parameter-values sah: HTTP %d %s", st, body)
	}
	if rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}, FaultParamSubstr: "X_E2E_FAULT"}); !rep.OK {
		return failr("FA-04: sesi CWMP: %s", rep.ErrMsg)
	}
	time.Sleep(500 * time.Millisecond)
	if fs, _ = listFaults(w.pilotAdmin, q); len(fs) != 0 {
		return failr("FA-04: fault harusnya tertutup setelah RPC yang sama berhasil, masih ada %+v", fs)
	}

	// FA-05
	if err := triggerFault(); err != nil {
		return failr("FA-05: %v", err)
	}
	fs, _ = listFaults(w.pilotAdmin, q)
	if len(fs) != 1 || fs[0].OccurrenceCount != 1 {
		return failr("FA-05: fault baru setelah ditutup harusnya baris baru dengan count 1, dapat %+v", fs)
	}
	if st, _ := w.pilotAdmin.status(http.MethodDelete, "/api/v1/faults/"+fs[0].ID, nil); st >= 400 {
		return failr("FA-05: DELETE fault HTTP %d, harusnya sukses", st)
	}
	if fs, _ = listFaults(w.pilotAdmin, q); len(fs) != 0 {
		return failr("FA-05: fault harusnya hilang dari daftar setelah ditutup operator")
	}
	if st, _ := w.pilotAdmin.status(http.MethodDelete, "/api/v1/faults/"+one.ID, nil); st != http.StatusNotFound {
		return failr("FA-05: menutup fault yang sudah tertutup HTTP %d, harusnya 404", st)
	}
	return pass("fault 9005 tercatat (kode/RPC/serial/task), kejadian ke-2 menaikkan count (bukan baris baru), tenant lain 403 + daftar kosong, SET yang sah menutupnya otomatis, DELETE operator sukses lalu 404; fault_code kepanjangan 400")
}
