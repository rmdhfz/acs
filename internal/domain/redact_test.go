package domain

import (
	"strings"
	"testing"
)

func TestIsSensitiveParamName(t *testing.T) {
	yes := []string{
		"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.PreSharedKey.1.KeyPassphrase",
		"Device.WiFi.AccessPoint.1.Security.KeyPassphrase",
		"InternetGatewayDevice.ManagementServer.ConnectionRequestPassword",
		"X.PPPoE.Password", "wan.pppoe.password", "wifi.wpa_passphrase", "api_token", "SharedSecret", "X.WEPKey",
		// celah yang ditemukan audit keamanan 22 Okt 2026
		"InternetGatewayDevice.X_HW_WebUserInfo.1.Pwd", "InternetGatewayDevice.X_HW_WPSPin", "Device.Users.User.1.UserPass",
		"Device.Services.VoiceService.1.SIP.AuthPasscode", "InternetGatewayDevice.X_SNMP.ReadCommunity", "X.Credential",
		"InternetGatewayDevice.X_HW_AdminPwd", "InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.WPAKey.1.Value",
		"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.WEPKey.1.KeyValue", "X.ACS.Pin", "X.UserPass",
	}
	no := []string{"InternetGatewayDevice.DeviceInfo.UpTime", "Device.WiFi.SSID.1.SSID", "wifi.ssid", "device.model_name",
		"Device.IP.Diagnostics.IPPing.Host", "X.Passive.Mode", "X.Bypass.Enable", "X.SpinningCount", "InternetGatewayDevice.DeviceInfo.ProvisioningCode",
		"InternetGatewayDevice.ManagementServer.PeriodicInformInterval", "Device.WiFi.AccessPoint.1.Security.ModeEnabled", "X.Compass"}
	for _, n := range yes {
		if !IsSensitiveParamName(n) {
			t.Errorf("%q harus sensitif", n)
		}
	}
	for _, n := range no {
		if IsSensitiveParamName(n) {
			t.Errorf("%q tidak boleh sensitif", n)
		}
	}
}

func TestRedactURLCredentials(t *testing.T) {
	got := RedactURLCredentials("https://user:pass@bss.example/hook?token=abc&x=1")
	for _, leak := range []string{"user", "pass", "abc"} {
		if strings.Contains(got, leak+"@") || strings.Contains(got, "token=abc") || (leak == "pass" && strings.Contains(got, ":pass@")) {
			t.Fatalf("bocor %q: %s", leak, got)
		}
	}
	if !strings.Contains(got, "x=1") || !strings.Contains(got, "bss.example") {
		t.Fatalf("bagian aman hilang: %s", got)
	}
}

func TestRedactPresetConfigurations(t *testing.T) {
	in := `[{"op":"set_parameter","key":"wifi.password","value":"rahasia"},{"op":"set_parameter","key":"wifi.ssid","value":"Rumah"},{"op":"reboot"}]`
	out := RedactPresetConfigurations(in)
	if strings.Contains(out, "rahasia") || !strings.Contains(out, "Rumah") || !strings.Contains(out, RedactedValue) {
		t.Fatalf("hanya nilai sensitif yang disamarkan: %s", out)
	}
	for _, c := range []string{"", "null", "[]", "bukan json", `{"a":1}`} {
		if got := RedactPresetConfigurations(c); got != c {
			t.Fatalf("%q harus apa adanya, dapat %q", c, got)
		}
	}
}
