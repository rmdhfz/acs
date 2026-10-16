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
	}
	no := []string{"InternetGatewayDevice.DeviceInfo.UpTime", "Device.WiFi.SSID.1.SSID", "wifi.ssid", "device.model_name"}
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
