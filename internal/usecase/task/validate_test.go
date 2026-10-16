package task

import (
	"errors"
	"testing"

	"acs/internal/domain"
)

func TestValidateTaskInput(t *testing.T) {
	ok := []struct {
		typ string
		p   map[string]interface{}
	}{
		{domain.TaskTypeGetParameterValues, map[string]interface{}{"names": []interface{}{"InternetGatewayDevice.DeviceInfo.UpTime"}}},
		{domain.TaskTypeSetParameterValues, map[string]interface{}{"values": map[string]interface{}{"Device.WiFi.SSID.1.SSID": "UAT"}}},
		{domain.TaskTypeGetParameterNames, map[string]interface{}{"path": "", "next_level": true}},
		{domain.TaskTypeGetParameterNames, nil},
		{domain.TaskTypeAddObject, map[string]interface{}{"object_name": "InternetGatewayDevice.X."}},
		{domain.TaskTypeDeleteObject, map[string]interface{}{"object_name": "InternetGatewayDevice.X.1."}},
		{domain.TaskTypeReboot, nil},
		{domain.TaskTypeFactoryReset, map[string]interface{}{}},
		{domain.TaskTypeDownload, map[string]interface{}{"url": "https://fw.example/x.bin", "file_type": "1 Firmware Upgrade Image"}},
		{domain.TaskTypeScheduleInform, map[string]interface{}{"delay_seconds": 30}},
	}
	for _, c := range ok {
		if err := ValidateTaskInput(c.typ, c.p); err != nil {
			t.Errorf("%s %v harus valid: %v", c.typ, c.p, err)
		}
	}

	bad := []struct {
		typ  string
		p    map[string]interface{}
		name string
	}{
		{domain.TaskTypeSetParameterAttributes, map[string]interface{}{"x": 1}, "tipe tanpa builder"},
		{domain.TaskTypeGetParameterAttributes, nil, "tipe tanpa builder"},
		{"TIDAK_ADA", nil, "tipe tak dikenal"},
		{domain.TaskTypeGetParameterValues, nil, "names kosong"},
		{domain.TaskTypeGetParameterValues, map[string]interface{}{"names": "bukan-array"}, "names bukan array"},
		{domain.TaskTypeGetParameterValues, map[string]interface{}{"names": []interface{}{""}}, "nama kosong"},
		{domain.TaskTypeSetParameterValues, nil, "values kosong"},
		{domain.TaskTypeSetParameterValues, map[string]interface{}{"values": map[string]interface{}{"a": 123}}, "nilai bukan string (builder gagal Unmarshal)"},
		{domain.TaskTypeAddObject, map[string]interface{}{}, "object_name kosong"},
		{domain.TaskTypeDownload, map[string]interface{}{"url": "ftp://x/y"}, "skema bukan http(s)"},
		{domain.TaskTypeDownload, nil, "url kosong"},
		{domain.TaskTypeScheduleInform, map[string]interface{}{"delay_seconds": 0}, "delay < 1"},
	}
	for _, c := range bad {
		err := ValidateTaskInput(c.typ, c.p)
		if err == nil {
			t.Errorf("%s: %s %v harus ditolak", c.name, c.typ, c.p)
			continue
		}
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: harus membungkus ErrInvalidInput (jadi 400), dapat %v", c.name, err)
		}
	}
}
