package task

import (
	"errors"
	"strings"
	"testing"

	"acs/internal/domain"
)

func names(n int) []interface{} {
	out := make([]interface{}, n)
	for i := range out {
		out[i] = "Device.X." + strings.Repeat("a", 5)
	}
	return out
}

// Batas jumlah/ukuran parameter task: mencegah satu request memicu ribuan task chunk atau SOAP raksasa.
func TestValidateTaskInput_Limits(t *testing.T) {
	longName := strings.Repeat("x", MaxParameterNameLen+1)
	longVal := strings.Repeat("v", MaxParameterValueLen+1)
	manyValues := map[string]interface{}{}
	for i := 0; i < MaxSetValues+1; i++ {
		manyValues[strings.Repeat("k", 3)+string(rune('a'+i%26))+strings.Repeat("z", i/26)] = "v"
	}
	manyAttrs := make([]interface{}, MaxSetAttributes+1)
	for i := range manyAttrs {
		manyAttrs[i] = map[string]interface{}{"name": "Device.X", "notification_change": true, "notification": 1}
	}

	bad := []struct {
		name string
		typ  string
		p    map[string]interface{}
	}{
		{"names > batas", domain.TaskTypeGetParameterValues, map[string]interface{}{"names": names(MaxParameterNames + 1)}},
		{"nama terlalu panjang", domain.TaskTypeGetParameterValues, map[string]interface{}{"names": []interface{}{longName}}},
		{"nama berisi newline", domain.TaskTypeGetParameterValues, map[string]interface{}{"names": []interface{}{"Device.X\n.Y"}}},
		{"values > batas", domain.TaskTypeSetParameterValues, map[string]interface{}{"values": manyValues}},
		{"nilai terlalu panjang", domain.TaskTypeSetParameterValues, map[string]interface{}{"values": map[string]interface{}{"Device.X": longVal}}},
		{"attr names > batas", domain.TaskTypeGetParameterAttributes, map[string]interface{}{"names": names(MaxParameterNames + 1)}},
		{"attr nama kosong", domain.TaskTypeGetParameterAttributes, map[string]interface{}{"names": []interface{}{" "}}},
		{"attributes > batas", domain.TaskTypeSetParameterAttributes, map[string]interface{}{"attributes": manyAttrs}},
		{"access_list kepanjangan", domain.TaskTypeSetParameterAttributes, map[string]interface{}{"attributes": []interface{}{
			map[string]interface{}{"name": "Device.X", "access_list_change": true, "access_list": names(MaxAccessListEntries + 1)},
		}}},
		{"url berkredensial", domain.TaskTypeDownload, map[string]interface{}{"url": "https://user:pw@host/f.bin"}},
		{"url kepanjangan", domain.TaskTypeDownload, map[string]interface{}{"url": "https://h/" + strings.Repeat("a", MaxTransferURLLen)}},
		{"delay terlalu besar", domain.TaskTypeScheduleInform, map[string]interface{}{"delay_seconds": MaxScheduleInformSec + 1}},
		{"path berisi kontrol", domain.TaskTypeGetParameterNames, map[string]interface{}{"path": "Device.\x00X"}},
	}
	for _, c := range bad {
		err := ValidateTaskInput(c.typ, c.p)
		if err == nil {
			t.Errorf("%s: harus ditolak", c.name)
			continue
		}
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: harus membungkus ErrInvalidInput, dapat %v", c.name, err)
		}
	}

	// Tepat di batas harus lolos.
	good := []struct {
		typ string
		p   map[string]interface{}
	}{
		{domain.TaskTypeGetParameterValues, map[string]interface{}{"names": names(MaxParameterNames)}},
		{domain.TaskTypeSetParameterValues, map[string]interface{}{"values": map[string]interface{}{"Device.X": strings.Repeat("v", MaxParameterValueLen)}}},
		{domain.TaskTypeScheduleInform, map[string]interface{}{"delay_seconds": MaxScheduleInformSec}},
	}
	for _, c := range good {
		if err := ValidateTaskInput(c.typ, c.p); err != nil {
			t.Errorf("%s di batas harus lolos: %v", c.typ, err)
		}
	}
}
