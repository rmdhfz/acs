package cwmpxml

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Fixture bergaya firmware nyata. Bukan capture dari perangkat tertentu —
// bentuknya mengikuti variasi yang SAH menurut TR-069/SOAP 1.1 dan yang
// diketahui beda antar vendor: prefix namespace, kutip atribut, deklarasi
// encoding, elemen kosong, dan pretty-printing. Tujuannya menangkap regresi
// parser sebelum bertemu CPE sungguhan.

// soapWrap membungkus isi body dgn envelope SOAP 1.1 standar.
func soapWrap(cwmpNS, hdr, body string) string {
	return `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"` +
		` xmlns:soapenc="http://schemas.xmlsoap.org/soap/encoding/"` +
		` xmlns:xsd="http://www.w3.org/2001/XMLSchema"` +
		` xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` +
		` xmlns:cwmp="` + cwmpNS + `">` + hdr + `<soapenv:Body>` + body + `</soapenv:Body></soapenv:Envelope>`
}

const informBody = `<cwmp:Inform>
<DeviceId><Manufacturer>ZTE</Manufacturer><OUI>00D0D0</OUI><ProductClass>F670L</ProductClass><SerialNumber>ZTEGC8A1B2C3</SerialNumber></DeviceId>
<Event soapenc:arrayType="cwmp:EventStruct[2]">
  <EventStruct><EventCode>1 BOOT</EventCode><CommandKey></CommandKey></EventStruct>
  <EventStruct><EventCode>6 CONNECTION REQUEST</EventCode><CommandKey/></EventStruct>
</Event>
<MaxEnvelopes>1</MaxEnvelopes><CurrentTime>2026-10-15T09:30:00+07:00</CurrentTime><RetryCount>0</RetryCount>
<ParameterList soapenc:arrayType="cwmp:ParameterValueStruct[4]">
  <ParameterValueStruct><Name>InternetGatewayDevice.DeviceInfo.SoftwareVersion</Name><Value xsi:type="xsd:string">V9.0.10P1N6</Value></ParameterValueStruct>
  <ParameterValueStruct><Name>InternetGatewayDevice.ManagementServer.ConnectionRequestURL</Name><Value xsi:type="xsd:string">http://10.20.30.40:58000/acs</Value></ParameterValueStruct>
  <ParameterValueStruct><Name>InternetGatewayDevice.DeviceInfo.UpTime</Name><Value xsi:type="xsd:unsignedInt">8421</Value></ParameterValueStruct>
  <ParameterValueStruct><Name>InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.Enable</Name><Value xsi:type="xsd:boolean">1</Value></ParameterValueStruct>
</ParameterList>
</cwmp:Inform>`

func mustParse(t *testing.T, data string) *Envelope {
	t.Helper()
	env, err := Unmarshal([]byte(data))
	if err != nil {
		t.Fatalf("Unmarshal gagal: %v\n--- input ---\n%.400s", err, data)
	}
	return env
}

func TestInformStyleZTE_ArrayTypeEventDanParameter(t *testing.T) {
	env := mustParse(t, `<?xml version="1.0" encoding="UTF-8"?>`+soapWrap(NSCWMP, `<soapenv:Header><cwmp:ID soapenv:mustUnderstand="1">1604</cwmp:ID></soapenv:Header>`, informBody))
	inf := env.Body.Inform
	if inf == nil {
		t.Fatal("Inform nil")
	}
	if inf.DeviceId.OUI != "00D0D0" || inf.DeviceId.SerialNumber != "ZTEGC8A1B2C3" || inf.DeviceId.ProductClass != "F670L" {
		t.Fatalf("DeviceId salah: %+v", inf.DeviceId)
	}
	if len(inf.Event.Items) != 2 || inf.Event.Items[0].EventCode != "1 BOOT" || inf.Event.Items[1].EventCode != "6 CONNECTION REQUEST" {
		t.Fatalf("Event salah (kode event harus persis, CLAUDE.md): %+v", inf.Event.Items)
	}
	if inf.Event.Items[1].CommandKey != "" {
		t.Fatalf("CommandKey elemen kosong self-closing harus string kosong: %q", inf.Event.Items[1].CommandKey)
	}
	if len(inf.ParameterList.Values) != 4 {
		t.Fatalf("ParameterList %d item, ingin 4", len(inf.ParameterList.Values))
	}
	byName := map[string]ParameterValue{}
	for _, p := range inf.ParameterList.Values {
		byName[p.Name] = p
	}
	up := byName["InternetGatewayDevice.DeviceInfo.UpTime"]
	if up.Value.Value != "8421" || up.Value.Type != "xsd:unsignedInt" {
		t.Fatalf("tipe/nilai UpTime salah: %+v", up.Value)
	}
	if byName["InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.Enable"].Value.Type != "xsd:boolean" {
		t.Fatal("tipe boolean tidak ter-capture")
	}
	if env.Header == nil || env.Header.ID == nil || env.Header.ID.Value != "1604" {
		t.Fatalf("Header ID harus ter-parse (dipakai untuk echo balik): %+v", env.Header)
	}
	if env.Body.Method() != "Inform" || env.Body.Namespace() != NSCWMP {
		t.Fatalf("Method/Namespace: %q %q", env.Body.Method(), env.Body.Namespace())
	}
}

// Gaya Huawei/FiberHome: prefix berbeda (SOAP-ENV/cwmp), kutip tunggal pada
// atribut, tanpa deklarasi XML sama sekali, namespace cwmp-1-0.
func TestInformStyleTanpaDeklarasiPrefixDanKutipBeda(t *testing.T) {
	doc := `<SOAP-ENV:Envelope xmlns:SOAP-ENV='http://schemas.xmlsoap.org/soap/envelope/' xmlns:SOAP-ENC='http://schemas.xmlsoap.org/soap/encoding/' xmlns:xsd='http://www.w3.org/2001/XMLSchema' xmlns:xsi='http://www.w3.org/2001/XMLSchema-instance' xmlns:cwmp='urn:dslforum-org:cwmp-1-0'>` +
		`<SOAP-ENV:Header><cwmp:ID SOAP-ENV:mustUnderstand='1'>abc</cwmp:ID></SOAP-ENV:Header><SOAP-ENV:Body>` +
		`<cwmp:Inform><DeviceId><Manufacturer>Huawei</Manufacturer><OUI>00E0FC</OUI><ProductClass>EG8145V5</ProductClass><SerialNumber>48575443AABBCCDD</SerialNumber></DeviceId>` +
		`<Event SOAP-ENC:arrayType='cwmp:EventStruct[1]'><EventStruct><EventCode>2 PERIODIC</EventCode><CommandKey></CommandKey></EventStruct></Event>` +
		`<MaxEnvelopes>1</MaxEnvelopes><CurrentTime>2026-10-15T02:30:00Z</CurrentTime><RetryCount>3</RetryCount><ParameterList SOAP-ENC:arrayType='cwmp:ParameterValueStruct[0]'/></cwmp:Inform>` +
		`</SOAP-ENV:Body></SOAP-ENV:Envelope>`
	env := mustParse(t, doc)
	if env.Body.Inform == nil || env.Body.Inform.RetryCount != 3 || env.Body.Namespace() != "urn:dslforum-org:cwmp-1-0" {
		t.Fatalf("parse salah: %+v ns=%q", env.Body.Inform, env.Body.Namespace())
	}
	if len(env.Body.Inform.ParameterList.Values) != 0 {
		t.Fatal("ParameterList kosong self-closing harus 0 item")
	}
}

func TestNilaiParameter_EntityCDATADanKarakterKhusus(t *testing.T) {
	body := `<cwmp:GetParameterValuesResponse><ParameterList>` +
		`<ParameterValueStruct><Name>A</Name><Value xsi:type="xsd:string">a &amp; b &lt;c&gt; &quot;d&quot; &apos;e&apos;</Value></ParameterValueStruct>` +
		`<ParameterValueStruct><Name>B</Name><Value xsi:type="xsd:string"><![CDATA[<raw>&not-entity</raw>]]></Value></ParameterValueStruct>` +
		`<ParameterValueStruct><Name>C</Name><Value xsi:type="xsd:string">caf&#233; &#x4E2D;&#x6587;</Value></ParameterValueStruct>` +
		`<ParameterValueStruct><Name>D</Name><Value xsi:type="xsd:string"></Value></ParameterValueStruct>` +
		`<ParameterValueStruct><Name>E</Name><Value xsi:type="xsd:string"/></ParameterValueStruct>` +
		`</ParameterList></cwmp:GetParameterValuesResponse>`
	env := mustParse(t, soapWrap(NSCWMP, "", body))
	got := map[string]string{}
	for _, p := range env.Body.GetParameterValuesResponse.ParameterList.Values {
		got[p.Name] = p.Value.Value
	}
	want := map[string]string{
		"A": `a & b <c> "d" 'e'`,
		"B": `<raw>&not-entity</raw>`,
		"C": "café 中文",
		"D": "",
		"E": "",
	}
	for k, v := range want {
		if g, ok := got[k]; !ok || g != v {
			t.Errorf("param %s = %q (ada=%v), ingin %q", k, g, ok, v)
		}
	}
}

func TestResponsTipeAngkaDanBoolean(t *testing.T) {
	t.Run("SetParameterValuesResponse Status 0 dan 1", func(t *testing.T) {
		for st, want := range map[string]int{"0": 0, "1": 1, " 1 ": 1} {
			env := mustParse(t, soapWrap(NSCWMP, "", `<cwmp:SetParameterValuesResponse><Status>`+st+`</Status></cwmp:SetParameterValuesResponse>`))
			if got := env.Body.SetParameterValuesResponse.Status; got != want {
				t.Errorf("Status %q -> %d, ingin %d", st, got, want)
			}
		}
	})
	t.Run("GetParameterNamesResponse Writable 0/1/true/false", func(t *testing.T) {
		body := `<cwmp:GetParameterNamesResponse><ParameterList soapenc:arrayType="cwmp:ParameterInfoStruct[4]">` +
			`<ParameterInfoStruct><Name>a.</Name><Writable>0</Writable></ParameterInfoStruct>` +
			`<ParameterInfoStruct><Name>b.</Name><Writable>1</Writable></ParameterInfoStruct>` +
			`<ParameterInfoStruct><Name>c.</Name><Writable>true</Writable></ParameterInfoStruct>` +
			`<ParameterInfoStruct><Name>d.</Name><Writable>false</Writable></ParameterInfoStruct>` +
			`</ParameterList></cwmp:GetParameterNamesResponse>`
		env := mustParse(t, soapWrap(NSCWMP, "", body))
		items := env.Body.GetParameterNamesResponse.ParameterList.Items
		want := []bool{false, true, true, false}
		if len(items) != 4 {
			t.Fatalf("%d item", len(items))
		}
		for i, w := range want {
			if items[i].Writable != w {
				t.Errorf("item %d (%s) Writable=%v, ingin %v", i, items[i].Name, items[i].Writable, w)
			}
		}
	})
	t.Run("AddObjectResponse", func(t *testing.T) {
		env := mustParse(t, soapWrap(NSCWMP, "", `<cwmp:AddObjectResponse><InstanceNumber>7</InstanceNumber><Status>0</Status></cwmp:AddObjectResponse>`))
		if r := env.Body.AddObjectResponse; r.InstanceNumber != 7 || r.Status != 0 {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("DownloadResponse dengan waktu unknown 0001-01-01", func(t *testing.T) {
		env := mustParse(t, soapWrap(NSCWMP, "", `<cwmp:DownloadResponse><Status>1</Status><StartTime>0001-01-01T00:00:00Z</StartTime><CompleteTime>0001-01-01T00:00:00Z</CompleteTime></cwmp:DownloadResponse>`))
		if r := env.Body.DownloadResponse; r.Status != 1 || r.StartTime != "0001-01-01T00:00:00Z" {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("RebootResponse/FactoryResetResponse elemen kosong", func(t *testing.T) {
		for _, tc := range []struct{ body, method string }{
			{`<cwmp:RebootResponse/>`, "RebootResponse"},
			{`<cwmp:FactoryResetResponse></cwmp:FactoryResetResponse>`, "FactoryResetResponse"},
		} {
			env := mustParse(t, soapWrap(NSCWMP, "", tc.body))
			if env.Body.Method() != tc.method {
				t.Errorf("Method() = %q, ingin %q", env.Body.Method(), tc.method)
			}
		}
	})
}

func TestTransferComplete_SuksesDanGagal(t *testing.T) {
	ok := mustParse(t, soapWrap(NSCWMP, "", `<cwmp:TransferComplete><CommandKey>fw-77</CommandKey><FaultStruct><FaultCode>0</FaultCode><FaultString></FaultString></FaultStruct><StartTime>2026-10-15T01:00:00Z</StartTime><CompleteTime>2026-10-15T01:03:00Z</CompleteTime></cwmp:TransferComplete>`))
	tc := ok.Body.TransferComplete
	if tc.CommandKey != "fw-77" || tc.FaultStruct == nil || tc.FaultStruct.FaultCode != "0" {
		t.Fatalf("TransferComplete sukses salah: %+v", tc)
	}
	bad := mustParse(t, soapWrap(NSCWMP, "", `<cwmp:TransferComplete><CommandKey>fw-77</CommandKey><FaultStruct><FaultCode>9010</FaultCode><FaultString>Download failure: file corrupt</FaultString></FaultStruct><StartTime>2026-10-15T01:00:00Z</StartTime><CompleteTime>2026-10-15T01:00:09Z</CompleteTime></cwmp:TransferComplete>`))
	if f := bad.Body.TransferComplete.FaultStruct; f == nil || f.FaultCode != "9010" || !strings.Contains(f.FaultString, "corrupt") {
		t.Fatalf("FaultStruct gagal salah: %+v", f)
	}
}

func TestFaultDariCPE(t *testing.T) {
	// Fault SOAP yang membungkus cwmp:Fault, plus SetParameterValuesFault turunan
	// (diabaikan parser tapi tidak boleh merusak parse).
	body := `<soapenv:Fault><faultcode>Client</faultcode><faultstring>CWMP fault</faultstring><detail>` +
		`<cwmp:Fault><FaultCode>9005</FaultCode><FaultString>Invalid parameter name</FaultString>` +
		`<SetParameterValuesFault><ParameterName>X.Y</ParameterName><FaultCode>9005</FaultCode><FaultString>Invalid parameter name</FaultString></SetParameterValuesFault>` +
		`</cwmp:Fault></detail></soapenv:Fault>`
	env := mustParse(t, soapWrap(NSCWMP, "", body))
	f := env.Body.Fault
	if f == nil || env.Body.Method() != "Fault" {
		t.Fatal("Fault tidak terdeteksi")
	}
	if f.Detail == nil || f.Detail.CWMPFault.FaultCode != "9005" || f.Detail.CWMPFault.FaultString != "Invalid parameter name" {
		t.Fatalf("cwmp:Fault detail salah: %+v", f.Detail)
	}
	if ns := env.Body.Namespace(); ns != "" {
		t.Fatalf("Fault bukan elemen ber-namespace cwmp, Namespace() harus kosong, dapat %q", ns)
	}
}

func TestBodyKosongDanPOSTKosong(t *testing.T) {
	for name, in := range map[string]string{"string kosong": "", "spasi & newline": " \r\n\t  \n"} {
		t.Run(name, func(t *testing.T) {
			env, err := Unmarshal([]byte(in))
			if err != nil {
				t.Fatalf("POST kosong penutup sesi bukan error: %v", err)
			}
			if !env.IsEmpty() || env.Body.Method() != "" {
				t.Fatalf("harus IsEmpty: %+v", env)
			}
		})
	}
	var nilEnv *Envelope
	if !nilEnv.IsEmpty() {
		t.Fatal("envelope nil harus IsEmpty")
	}
	// Envelope valid tapi Body kosong -> bukan IsEmpty, Method kosong.
	env := mustParse(t, soapWrap(NSCWMP, "", ""))
	if env.IsEmpty() || env.Body.Method() != "" || env.Body.Namespace() != "" {
		t.Fatalf("envelope tanpa RPC: IsEmpty=%v Method=%q", env.IsEmpty(), env.Body.Method())
	}
}

func TestInputRusakDitolakTanpaPanic(t *testing.T) {
	cases := map[string]string{
		"terpotong di tengah":         soapWrap(NSCWMP, "", informBody)[:200],
		"bukan XML":                   "HTTP/1.1 200 OK\r\n\r\nhello",
		"root bukan Envelope":         `<html><body>login</body></html>`,
		"SOAP 1.2 (TR-069 wajib 1.1)": `<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope"><env:Body/></env:Envelope>`,
		"tag tak tertutup":            `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body><cwmp:Inform>`,
		"integer non-numerik":         soapWrap(NSCWMP, "", `<cwmp:SetParameterValuesResponse><Status>abc</Status></cwmp:SetParameterValuesResponse>`),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			env, err := Unmarshal([]byte(in))
			if err == nil {
				t.Fatalf("seharusnya error, dapat env=%+v", env)
			}
			if env != nil {
				t.Fatal("env harus nil bila error")
			}
			if !strings.HasPrefix(err.Error(), "cwmpxml:") {
				t.Fatalf("error harus dibungkus dgn prefix paket: %v", err)
			}
		})
	}
}

// ---- Kuirk encoding yang HARUS ditoleransi (CPE lama / vendor tertentu) ----

// Deklarasi encoding non-UTF-8 sah menurut XML; sebagian CPE lama mengirim
// ISO-8859-1. Tanpa CharsetReader encoding/xml menolak SELURUH Inform sehingga
// CPE tak pernah bisa terhubung ke ACS.
func TestEncodingISO88591DanWindows1252(t *testing.T) {
	for _, label := range []string{"ISO-8859-1", "iso-8859-1", "windows-1252", "latin1", "US-ASCII"} {
		t.Run(label, func(t *testing.T) {
			// 0xE9 = 'é' di ISO-8859-1/Windows-1252 (bukan UTF-8 valid).
			xmlBytes := []byte(`<?xml version="1.0" encoding="` + label + `"?>` +
				soapWrap(NSCWMP, "", `<cwmp:GetParameterValuesResponse><ParameterList><ParameterValueStruct><Name>N</Name><Value xsi:type="xsd:string">caf`)) // dipotong; lengkapi di bawah
			xmlBytes = xmlBytes[:len(xmlBytes)-len(`</soapenv:Body></soapenv:Envelope>`)]
			xmlBytes = append(xmlBytes, 0xE9)
			xmlBytes = append(xmlBytes, []byte(`</Value></ParameterValueStruct></ParameterList></cwmp:GetParameterValuesResponse></soapenv:Body></soapenv:Envelope>`)...)
			env, err := Unmarshal(xmlBytes)
			if err != nil {
				t.Fatalf("encoding %q harus ditoleransi: %v", label, err)
			}
			if got := env.Body.GetParameterValuesResponse.ParameterList.Values[0].Value.Value; label != "US-ASCII" && got != "café" {
				t.Fatalf("nilai = %q, ingin %q", got, "café")
			}
		})
	}
}

func TestEncodingTakDikenalDitolakBersih(t *testing.T) {
	_, err := Unmarshal([]byte(`<?xml version="1.0" encoding="x-klingon"?>` + soapWrap(NSCWMP, "", informBody)))
	if err == nil {
		t.Fatal("encoding tak dikenal harus error (bukan diam-diam salah baca)")
	}
}

func TestBOMUTF8Ditoleransi(t *testing.T) {
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`<?xml version="1.0" encoding="utf-8"?>`+soapWrap(NSCWMP, "", informBody))...)
	env, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("BOM UTF-8 harus ditoleransi: %v", err)
	}
	if env.Body.Inform == nil {
		t.Fatal("Inform nil")
	}
}

// OUI+Serial adalah kunci identitas device (unique key). Pretty-printing yang
// menambah spasi/newline di sekitar nilai tidak boleh menghasilkan identitas
// "baru" (device duplikat / tak ketemu) untuk CPE yang sama.
func TestDeviceIdDiNormalisasiSpasi(t *testing.T) {
	body := "<cwmp:Inform><DeviceId>\n  <Manufacturer>\n    ZTE\n  </Manufacturer>\n  <OUI> 00D0D0 </OUI>\n  <ProductClass>\tF670L\n</ProductClass>\n  <SerialNumber>\n ZTEGC8A1B2C3 \n</SerialNumber>\n</DeviceId><Event/><MaxEnvelopes>1</MaxEnvelopes><CurrentTime>x</CurrentTime><RetryCount>0</RetryCount><ParameterList/></cwmp:Inform>"
	env := mustParse(t, soapWrap(NSCWMP, "", body))
	d := env.Body.Inform.DeviceId
	if d.Manufacturer != "ZTE" || d.OUI != "00D0D0" || d.ProductClass != "F670L" || d.SerialNumber != "ZTEGC8A1B2C3" {
		t.Fatalf("DeviceId harus di-trim: %+v", d)
	}
}

// Nilai PARAMETER tidak boleh di-trim: spasi bisa bermakna (mis. SSID).
func TestNilaiParameterTidakDiTrim(t *testing.T) {
	body := `<cwmp:GetParameterValuesResponse><ParameterList><ParameterValueStruct><Name>SSID</Name><Value xsi:type="xsd:string">  My Wifi  </Value></ParameterValueStruct></ParameterList></cwmp:GetParameterValuesResponse>`
	env := mustParse(t, soapWrap(NSCWMP, "", body))
	if got := env.Body.GetParameterValuesResponse.ParameterList.Values[0].Value.Value; got != "  My Wifi  " {
		t.Fatalf("spasi nilai parameter harus dipertahankan, dapat %q", got)
	}
}

// ---- Marshal (ACS -> CPE): round-trip semua RPC outbound ----

func TestMarshalRoundTripSemuaRPCOutbound(t *testing.T) {
	const ns = "urn:dslforum-org:cwmp-1-1"
	n := func(local string) (x struct{ xmlName string }) { return }
	_ = n
	cases := []struct {
		method string
		body   Body
		check  func(t *testing.T, b Body)
	}{
		{"GetParameterValues", Body{GetParameterValues: &GetParameterValues{XMLName: RPCName(ns, "GetParameterValues"),
			ParameterNames: StringList{ArrayType: "xsd:string[2]", Items: []string{"A.B", "C.D"}}}},
			func(t *testing.T, b Body) {
				if got := b.GetParameterValues.ParameterNames.Items; len(got) != 2 || got[1] != "C.D" {
					t.Fatalf("%v", got)
				}
			}},
		{"SetParameterValues", Body{SetParameterValues: &SetParameterValues{XMLName: RPCName(ns, "SetParameterValues"),
			ParameterList: ParameterValueList{ArrayType: "cwmp:ParameterValueStruct[1]", Values: []ParameterValue{
				{Name: "X.SSID", Value: ValueType{Type: "xsd:string", Value: `A&B <"quoted"> 'x' café 中文`}}}},
			ParameterKey: "key-1"}},
			func(t *testing.T, b Body) {
				v := b.SetParameterValues
				if v.ParameterKey != "key-1" || len(v.ParameterList.Values) != 1 ||
					v.ParameterList.Values[0].Value.Value != `A&B <"quoted"> 'x' café 中文` || v.ParameterList.Values[0].Value.Type != "xsd:string" {
					t.Fatalf("karakter khusus/tipe tidak selamat round-trip: %+v", v)
				}
			}},
		{"GetParameterNames", Body{GetParameterNames: &GetParameterNames{XMLName: RPCName(ns, "GetParameterNames"), ParameterPath: "InternetGatewayDevice.", NextLevel: true}},
			func(t *testing.T, b Body) {
				if g := b.GetParameterNames; g.ParameterPath != "InternetGatewayDevice." || !g.NextLevel {
					t.Fatalf("%+v", g)
				}
			}},
		{"AddObject", Body{AddObject: &AddObject{XMLName: RPCName(ns, "AddObject"), ObjectName: "X.PortMapping.", ParameterKey: "k"}},
			func(t *testing.T, b Body) {
				if b.AddObject.ObjectName != "X.PortMapping." {
					t.Fatalf("%+v", b.AddObject)
				}
			}},
		{"DeleteObject", Body{DeleteObject: &DeleteObject{XMLName: RPCName(ns, "DeleteObject"), ObjectName: "X.PortMapping.3.", ParameterKey: "k"}},
			func(t *testing.T, b Body) {
				if b.DeleteObject.ObjectName != "X.PortMapping.3." {
					t.Fatalf("%+v", b.DeleteObject)
				}
			}},
		{"Reboot", Body{Reboot: &Reboot{XMLName: RPCName(ns, "Reboot"), CommandKey: "reboot-9"}},
			func(t *testing.T, b Body) {
				if b.Reboot.CommandKey != "reboot-9" {
					t.Fatalf("%+v", b.Reboot)
				}
			}},
		{"FactoryReset", Body{FactoryReset: &FactoryReset{XMLName: RPCName(ns, "FactoryReset")}}, func(*testing.T, Body) {}},
		{"Download", Body{Download: &Download{XMLName: RPCName(ns, "Download"), CommandKey: "fw-1", FileType: "1 Firmware Upgrade Image",
			URL: "http://minio.example:9000/acs-firmware/f.bin?X-Amz-Signature=abc&X-Amz-Expires=3600", Username: "", Password: "",
			FileSize: 33554432, TargetFileName: "f.bin", DelaySeconds: 0}},
			func(t *testing.T, b Body) {
				d := b.Download
				if d.FileType != "1 Firmware Upgrade Image" || d.FileSize != 33554432 ||
					d.URL != "http://minio.example:9000/acs-firmware/f.bin?X-Amz-Signature=abc&X-Amz-Expires=3600" {
					t.Fatalf("URL presigned (memuat &) rusak saat round-trip: %+v", d)
				}
			}},
		{"Upload", Body{Upload: &Upload{XMLName: RPCName(ns, "Upload"), CommandKey: "u", FileType: "1 Vendor Configuration File", URL: "http://h/u?a=1&b=2"}},
			func(t *testing.T, b Body) {
				if b.Upload.URL != "http://h/u?a=1&b=2" {
					t.Fatalf("%+v", b.Upload)
				}
			}},
		{"ScheduleInform", Body{ScheduleInform: &ScheduleInform{XMLName: RPCName(ns, "ScheduleInform"), DelaySeconds: 30, CommandKey: "s"}},
			func(t *testing.T, b Body) {
				if b.ScheduleInform.DelaySeconds != 30 {
					t.Fatalf("%+v", b.ScheduleInform)
				}
			}},
		{"SetParameterAttributes", Body{SetParameterAttributes: &SetParameterAttributes{XMLName: RPCName(ns, "SetParameterAttributes"),
			ParameterList: SetParameterAttributesList{Items: []SetParameterAttributesStruct{{Name: "X.Y", NotificationChange: true, Notification: 2}}}}},
			func(t *testing.T, b Body) {
				it := b.SetParameterAttributes.ParameterList.Items
				if len(it) != 1 || !it[0].NotificationChange || it[0].Notification != 2 {
					t.Fatalf("%+v", it)
				}
			}},
		{"GetParameterAttributes", Body{GetParameterAttributes: &GetParameterAttributes{XMLName: RPCName(ns, "GetParameterAttributes"),
			ParameterNames: StringList{Items: []string{"X.Y"}}}},
			func(t *testing.T, b Body) {
				if b.GetParameterAttributes.ParameterNames.Items[0] != "X.Y" {
					t.Fatalf("%+v", b.GetParameterAttributes)
				}
			}},
		{"InformResponse", Body{InformResponse: &InformResponse{XMLName: RPCName(ns, "InformResponse"), MaxEnvelopes: 1}},
			func(t *testing.T, b Body) {
				if b.InformResponse.MaxEnvelopes != 1 {
					t.Fatalf("%+v", b.InformResponse)
				}
			}},
		{"TransferCompleteResponse", Body{TransferCompleteResponse: &TransferCompleteResponse{XMLName: RPCName(ns, "TransferCompleteResponse")}}, func(*testing.T, Body) {}},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			out, err := Marshal(NewEnvelope("id-"+tc.method, ns, tc.body))
			if err != nil {
				t.Fatal(err)
			}
			s := string(out)
			if !strings.HasPrefix(s, `<?xml version="1.0" encoding="UTF-8"?>`) {
				t.Fatalf("harus diawali deklarasi XML UTF-8:\n%.120s", s)
			}
			if !strings.Contains(s, `mustUnderstand="1"`) || !strings.Contains(s, "id-"+tc.method) {
				t.Fatalf("Header ID dengan mustUnderstand harus ada (TR-069 §3.4.1):\n%s", s)
			}
			back, err := Unmarshal(out)
			if err != nil {
				t.Fatalf("hasil Marshal tidak bisa di-Unmarshal lagi: %v\n%s", err, s)
			}
			if back.Body.Method() != tc.method {
				t.Fatalf("Method() = %q, ingin %q", back.Body.Method(), tc.method)
			}
			if back.Body.Namespace() != ns {
				t.Fatalf("Namespace() = %q, ingin %q (echo namespace CPE)", back.Body.Namespace(), ns)
			}
			if back.Header == nil || back.Header.ID == nil || back.Header.ID.Value != "id-"+tc.method {
				t.Fatalf("Header ID tidak selamat: %+v", back.Header)
			}
			tc.check(t, back.Body)
		})
	}
}

func TestMarshalTanpaIDTidakMenulisHeader(t *testing.T) {
	out, err := Marshal(NewEnvelope("", "", Body{RebootResponse: &RebootResponse{XMLName: RPCName("", "RebootResponse")}}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "Header") {
		t.Fatalf("tanpa ID tidak boleh ada Header:\n%s", out)
	}
}

// Body.Method() dan Body.Namespace() harus konsisten untuk SETIAP field Body:
// tabel ini menjaga switch panjang di envelope.go dari salah-ketik/terlewat.
func TestMethodDanNamespaceUntukSemuaRPC(t *testing.T) {
	const ns = "urn:dslforum-org:cwmp-1-3"
	names := []string{
		"Inform", "InformResponse", "GetParameterValues", "GetParameterValuesResponse",
		"SetParameterValues", "SetParameterValuesResponse", "GetParameterNames", "GetParameterNamesResponse",
		"AddObject", "AddObjectResponse", "DeleteObject", "DeleteObjectResponse", "Reboot", "RebootResponse",
		"FactoryReset", "FactoryResetResponse", "Download", "DownloadResponse", "Upload", "UploadResponse",
		"TransferComplete", "TransferCompleteResponse", "ScheduleInform", "ScheduleInformResponse",
		"SetParameterAttributes", "SetParameterAttributesResponse", "GetParameterAttributes", "GetParameterAttributesResponse",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			env := mustParse(t, soapWrap(ns, "", `<cwmp:`+name+`/>`))
			if got := env.Body.Method(); got != name {
				t.Fatalf("Method() = %q, ingin %q", got, name)
			}
			if got := env.Body.Namespace(); got != ns {
				t.Fatalf("Namespace() = %q, ingin %q", got, ns)
			}
		})
	}
}

func TestRPCNameFallbackNamespace(t *testing.T) {
	if got := RPCName("", "Reboot"); got.Space != NSCWMP || got.Local != "Reboot" {
		t.Fatalf("%+v", got)
	}
	if got := RPCName("urn:x", "Reboot"); got.Space != "urn:x" {
		t.Fatalf("%+v", got)
	}
}

// ---- Skala & ketahanan ----

func TestInformBesarSeribuParameter(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<cwmp:Inform><DeviceId><Manufacturer>M</Manufacturer><OUI>001122</OUI><ProductClass>P</ProductClass><SerialNumber>S</SerialNumber></DeviceId><Event/><MaxEnvelopes>1</MaxEnvelopes><CurrentTime>x</CurrentTime><RetryCount>0</RetryCount><ParameterList>`)
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&sb, `<ParameterValueStruct><Name>InternetGatewayDevice.X.%d.Value</Name><Value xsi:type="xsd:string">v%d</Value></ParameterValueStruct>`, i, i)
	}
	sb.WriteString(`</ParameterList></cwmp:Inform>`)
	start := time.Now()
	env := mustParse(t, soapWrap(NSCWMP, "", sb.String()))
	if n := len(env.Body.Inform.ParameterList.Values); n != 1000 {
		t.Fatalf("%d parameter, ingin 1000", n)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("parse 1000 parameter terlalu lambat: %v", d)
	}
}

func TestUnmarshalErrorMembungkusSentinel(t *testing.T) {
	_, err := Unmarshal([]byte("<a>"))
	var target interface{ Unwrap() error }
	if !errors.As(err, &target) {
		t.Fatalf("error harus bisa di-unwrap (%%w): %v", err)
	}
}

// FuzzUnmarshal — input acak dari jaringan tidak boleh membuat panic.
// `go test` menjalankan seed di bawah; `go test -fuzz=FuzzUnmarshal` untuk menggali.
func FuzzUnmarshal(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte(soapWrap(NSCWMP, "", informBody)))
	f.Add([]byte(`<?xml version="1.0" encoding="ISO-8859-1"?><a/>`))
	f.Add([]byte("\xEF\xBB\xBF<x/>"))
	f.Add([]byte(`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body><soapenv:Fault/></soapenv:Body></soapenv:Envelope>`))
	f.Fuzz(func(t *testing.T, data []byte) {
		env, err := Unmarshal(data)
		if err == nil && env != nil {
			_ = env.IsEmpty()
			_ = env.Body.Method()
			_ = env.Body.Namespace()
		}
	})
}
