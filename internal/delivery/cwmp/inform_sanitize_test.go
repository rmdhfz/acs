package cwmp

import (
	"strings"
	"testing"

	"acs/pkg/cwmpxml"
)

func TestValidateInformIdentity(t *testing.T) {
	ok := []cwmpxml.DeviceIDStruct{
		{OUI: "00D0D0", SerialNumber: "ZTEGC1234567", ProductClass: "F660"},
		{OUI: "00d0d0", SerialNumber: strings.Repeat("s", maxInformSerialLen)},
	}
	for _, id := range ok {
		if err := validateInformIdentity(id); err != nil {
			t.Errorf("%+v harus lolos: %v", id, err)
		}
	}
	bad := []cwmpxml.DeviceIDStruct{
		{OUI: "", SerialNumber: "x"},
		{OUI: "00D0D0AA", SerialNumber: "x"}, // > 6 -> CHAR(6) gagal di DB
		{OUI: "00D0D", SerialNumber: "x"},    // < 6
		{OUI: "00D0 D", SerialNumber: "x"},   // bukan alnum
		{OUI: "00D0D0", SerialNumber: ""},    // serial kosong
		{OUI: "00D0D0", SerialNumber: "   "}, // spasi saja
		{OUI: "00D0D0", SerialNumber: strings.Repeat("s", maxInformSerialLen+1)},
		{OUI: "00D0D0", SerialNumber: "ab\ncd"}, // karakter kontrol
		{OUI: "00D0D0", SerialNumber: "x", ProductClass: "a\x00b"},
		{OUI: "00D0D0", SerialNumber: "\xff\xfe"}, // bukan UTF-8
	}
	for _, id := range bad {
		if err := validateInformIdentity(id); err == nil {
			t.Errorf("%+v harus ditolak", id)
		}
	}
}

func TestSanitizeNamespace(t *testing.T) {
	for _, ns := range []string{"urn:dslforum-org:cwmp-1-0", "urn:dslforum-org:cwmp-1-2", "urn:dslforum-org:cwmp-1-4"} {
		if got := sanitizeNamespace(ns); got != ns {
			t.Errorf("%q harus dipertahankan, dapat %q", ns, got)
		}
	}
	for _, ns := range []string{"", "urn:evil", "urn:dslforum-org:cwmp-1-2<x>", "urn:dslforum-org:cwmp-1-" + strings.Repeat("1", 80)} {
		if got := sanitizeNamespace(ns); got != "" {
			t.Errorf("%q harus ditolak (kosong), dapat %q", ns, got)
		}
	}
}

func TestSanitizeInformEventsAndParams(t *testing.T) {
	// Event: jumlah dibatasi; kode kosong/kontrol/terlalu panjang dibuang; command key dipotong.
	items := []cwmpxml.EventStruct{
		{EventCode: "1 BOOT", CommandKey: strings.Repeat("k", maxInformCommandKeyLen+50)},
		{EventCode: ""},
		{EventCode: "2 PERIODIC\n"},
		{EventCode: strings.Repeat("e", maxInformEventCodeLen+1)},
		{EventCode: "6 CONNECTION REQUEST", CommandKey: "a\x01b"},
	}
	ev := sanitizeInformEvents(items)
	if len(ev) != 2 || ev[0].EventCode != "1 BOOT" || ev[1].EventCode != "6 CONNECTION REQUEST" {
		t.Fatalf("event tersaring salah: %+v", ev)
	}
	if len(ev[0].CommandKey) != maxInformCommandKeyLen {
		t.Errorf("command key harus dipotong ke %d, dapat %d", maxInformCommandKeyLen, len(ev[0].CommandKey))
	}
	if ev[1].CommandKey != "" {
		t.Errorf("command key berisi kontrol harus dikosongkan, dapat %q", ev[1].CommandKey)
	}
	many := make([]cwmpxml.EventStruct, maxInformEvents*3)
	for i := range many {
		many[i] = cwmpxml.EventStruct{EventCode: "2 PERIODIC"}
	}
	if got := len(sanitizeInformEvents(many)); got != maxInformEvents {
		t.Errorf("event harus dibatasi %d, dapat %d", maxInformEvents, got)
	}

	// Parameter: jumlah dibatasi; nama cacat dibuang; nilai dipotong (bukan membuang seluruh batch).
	pv := func(name, val string) cwmpxml.ParameterValue {
		var p cwmpxml.ParameterValue
		p.Name = name
		p.Value.Value = val
		return p
	}
	params := []cwmpxml.ParameterValue{
		pv("Device.A", strings.Repeat("v", maxInformParamValueLen+100)),
		pv("", "x"),
		pv(strings.Repeat("n", maxInformParamNameLen+1), "x"),
		pv("Device.B\n", "x"),
		pv("Device.C", "ok"),
	}
	got := sanitizeInformParams(params)
	if len(got) != 2 || got[0].Name != "Device.A" || got[1].Name != "Device.C" {
		t.Fatalf("parameter tersaring salah: %+v", got)
	}
	if len(got[0].Value) != maxInformParamValueLen {
		t.Errorf("nilai harus dipotong ke %d, dapat %d", maxInformParamValueLen, len(got[0].Value))
	}
	big := make([]cwmpxml.ParameterValue, maxInformParams+500)
	for i := range big {
		big[i] = pv("Device.P", "v")
	}
	if n := len(sanitizeInformParams(big)); n != maxInformParams {
		t.Errorf("parameter harus dibatasi %d, dapat %d", maxInformParams, n)
	}
}
