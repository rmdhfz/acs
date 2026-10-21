package cwmpxml

import (
	"strings"
	"testing"
)

const informSample = `<?xml version="1.0"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/" xmlns:cwmp="urn:dslforum-org:cwmp-1-0" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<soap:Header><cwmp:ID soap:mustUnderstand="1">1</cwmp:ID></soap:Header>
<soap:Body><cwmp:Inform><ParameterList soap-enc:arrayType="cwmp:ParameterValueStruct[4]">
<ParameterValueStruct><Name>InternetGatewayDevice.DeviceInfo.SoftwareVersion</Name><Value xsi:type="xsd:string">V5.2.10P2T10</Value></ParameterValueStruct>
<ParameterValueStruct><Name>InternetGatewayDevice.ManagementServer.ConnectionRequestPassword</Name><Value xsi:type="xsd:string">s3cretCR</Value></ParameterValueStruct>
<ParameterValueStruct><Name>InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.PreSharedKey.1.KeyPassphrase</Name><Value xsi:type="xsd:string">wifi-pass-123</Value></ParameterValueStruct>
<ParameterValueStruct><Name>InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID</Name><Value xsi:type="xsd:string">Rumah</Value></ParameterValueStruct>
</ParameterList></cwmp:Inform></soap:Body></soap:Envelope>`

func TestRedact_NilaiRahasiaDisamarkanStrukturUtuh(t *testing.T) {
	out := string(Redact([]byte(informSample)))
	for _, secret := range []string{"s3cretCR", "wifi-pass-123"} {
		if strings.Contains(out, secret) {
			t.Fatalf("rahasia %q bocor: %s", secret, out)
		}
	}
	if strings.Count(out, RedactedValue) != 2 {
		t.Fatalf("harus tepat 2 nilai disamarkan: %s", out)
	}
	// yang bukan rahasia tetap utuh, termasuk atribut xsi:type
	for _, keep := range []string{"V5.2.10P2T10", ">Rumah<", `xsi:type="xsd:string">Rumah`} {
		if !strings.Contains(out, keep) {
			t.Fatalf("nilai non-rahasia %q hilang: %s", keep, out)
		}
	}
	if len(out) <= len(informSample)-40 || !strings.HasSuffix(out, "</soap:Envelope>") {
		t.Fatal("struktur XML berubah")
	}
	// idempoten
	if string(Redact([]byte(out))) != out {
		t.Fatal("Redact harus idempoten")
	}
}

func TestRedact_VarianNamespaceDanNilaiKosong(t *testing.T) {
	in := `<Body><cwmp:SetParameterValues><ParameterList><cwmp:ParameterValueStruct><Name>Device.WiFi.AccessPoint.1.Security.KeyPassphrase</Name><Value xsi:type="xsd:string">abc</Value></cwmp:ParameterValueStruct>` +
		`<ParameterValueStruct><Name>Device.PPP.Interface.1.Password</Name><Value xsi:type="xsd:string"></Value></ParameterValueStruct>` +
		`<ParameterValueStruct><Name>Device.Users.User.1.Username</Name><Value>admin</Value></ParameterValueStruct></ParameterList></cwmp:SetParameterValues></Body>`
	out := string(Redact([]byte(in)))
	if strings.Contains(out, ">abc<") || !strings.Contains(out, RedactedValue) {
		t.Fatalf("passphrase harus disamarkan: %s", out)
	}
	if !strings.Contains(out, "<Value xsi:type=\"xsd:string\"></Value>") || !strings.Contains(out, ">admin<") {
		t.Fatalf("nilai kosong dan username tidak boleh berubah: %s", out)
	}
}

func TestRedact_BukanXMLDikembalikanApaAdanya(t *testing.T) {
	for _, in := range []string{"", "bukan xml password=abc", "<a/>"} {
		if string(Redact([]byte(in))) != in {
			t.Fatalf("%q tidak boleh berubah", in)
		}
	}
}

func TestBodyRPCName(t *testing.T) {
	cases := map[string]string{
		informSample: "Inform",
		`<soap:Envelope><soap:Body><cwmp:GetParameterValuesResponse/></soap:Body></soap:Envelope>`: "GetParameterValuesResponse",
		`<Envelope><Body><Fault><faultcode>Client</faultcode></Fault></Body></Envelope>`:           "Fault",
		``:                               "",
		`<Envelope><Header/></Envelope>`: "",
	}
	for in, want := range cases {
		if got := BodyRPCName([]byte(in)); got != want {
			t.Errorf("BodyRPCName = %q, mau %q", got, want)
		}
	}
}
