package cwmp

import (
	"strings"
	"testing"

	"acs/internal/domain"
	"acs/pkg/cwmpxml"
)

func marshalBody(t *testing.T, ns string, body cwmpxml.Body) string {
	t.Helper()
	out, err := cwmpxml.Marshal(cwmpxml.NewEnvelope("id", ns, body))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// SetParameterValues: urutan deterministik, arrayType benar, ParameterKey
// default ke task uuid, nilai berkarakter khusus selamat.
func TestBuildSetParameterValues_DeterministikDanArrayType(t *testing.T) {
	params := []byte(`{"values":{"Z.last":"3","A.first":"1","M.mid":"a&b <x>"}}`)
	var prev string
	for i := 0; i < 30; i++ { // iterasi map Go acak; output harus tetap sama
		body, err := BuildRequestBody(domain.TaskTypeSetParameterValues, "task-9", params, nsCWMP10)
		if err != nil {
			t.Fatal(err)
		}
		out := marshalBody(t, nsCWMP10, body)
		if prev != "" && out != prev {
			t.Fatalf("output tidak deterministik antar panggilan:\n%s\n---\n%s", prev, out)
		}
		prev = out
	}
	v := func() *cwmpxml.SetParameterValues {
		body, _ := BuildRequestBody(domain.TaskTypeSetParameterValues, "task-9", params, nsCWMP10)
		return body.SetParameterValues
	}()
	names := []string{}
	for _, p := range v.ParameterList.Values {
		names = append(names, p.Name)
	}
	if strings.Join(names, ",") != "A.first,M.mid,Z.last" {
		t.Fatalf("urutan = %v, ingin terurut", names)
	}
	if v.ParameterList.ArrayType != "cwmp:ParameterValueStruct[3]" {
		t.Fatalf("arrayType = %q", v.ParameterList.ArrayType)
	}
	if v.ParameterKey != "task-9" {
		t.Fatalf("ParameterKey default harus task uuid, dapat %q", v.ParameterKey)
	}
	// round-trip karakter khusus
	back, err := cwmpxml.Unmarshal([]byte(prev))
	if err != nil {
		t.Fatal(err)
	}
	if got := back.Body.SetParameterValues.ParameterList.Values[1].Value.Value; got != "a&b <x>" {
		t.Fatalf("nilai khusus rusak: %q", got)
	}
	if !strings.Contains(prev, "arrayType") {
		t.Fatalf("atribut arrayType harus ada di XML (TR-069 mewajibkan):\n%s", prev)
	}
}

func TestBuildSetParameterValues_ParameterKeyEksplisit(t *testing.T) {
	body, err := BuildRequestBody(domain.TaskTypeSetParameterValues, "u", []byte(`{"values":{"A":"1"},"parameter_key":"kunci-saya"}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if body.SetParameterValues.ParameterKey != "kunci-saya" {
		t.Fatalf("%q", body.SetParameterValues.ParameterKey)
	}
	if body.SetParameterValues.XMLName.Space != cwmpxml.NSCWMP {
		t.Fatalf("ns kosong harus fallback cwmp-1-2: %q", body.SetParameterValues.XMLName.Space)
	}
}

func TestBuildGetParameterValues_ArrayType(t *testing.T) {
	body, err := BuildRequestBody(domain.TaskTypeGetParameterValues, "u", []byte(`{"names":["A","B","C"]}`), nsCWMP10)
	if err != nil {
		t.Fatal(err)
	}
	if got := body.GetParameterValues.ParameterNames.ArrayType; got != "xsd:string[3]" {
		t.Fatalf("arrayType = %q", got)
	}
	// daftar kosong tetap valid
	empty, err := BuildRequestBody(domain.TaskTypeGetParameterValues, "u", []byte(`{"names":[]}`), nsCWMP10)
	if err != nil || empty.GetParameterValues.ParameterNames.ArrayType != "xsd:string[0]" {
		t.Fatalf("%+v err=%v", empty.GetParameterValues, err)
	}
}

func TestBuildDownload_SemuaFieldDanCommandKey(t *testing.T) {
	body, err := BuildRequestBody(domain.TaskTypeDownload, "dl-1",
		[]byte(`{"file_type":"1 Firmware Upgrade Image","url":"http://minio:9000/b/f.bin?X-Amz-Signature=s&X-Amz-Expires=3600","username":"u","password":"p","target_file_name":"f.bin","file_size":1234}`), nsCWMP10)
	if err != nil {
		t.Fatal(err)
	}
	d := body.Download
	if d.CommandKey != "dl-1" || d.FileType != "1 Firmware Upgrade Image" || d.FileSize != 1234 || d.TargetFileName != "f.bin" || d.Username != "u" || d.Password != "p" {
		t.Fatalf("%+v", d)
	}
	out := marshalBody(t, nsCWMP10, body)
	for _, tag := range []string{"<DelaySeconds>0</DelaySeconds>", "<SuccessURL></SuccessURL>", "<FailureURL></FailureURL>"} {
		if !strings.Contains(out, tag) {
			t.Errorf("Download wajib memuat %s (field wajib TR-069 walau kosong):\n%s", tag, out)
		}
	}
	back, _ := cwmpxml.Unmarshal([]byte(out))
	if !strings.Contains(back.Body.Download.URL, "X-Amz-Signature=s&X-Amz-Expires=3600") {
		t.Fatalf("URL presigned rusak: %q", back.Body.Download.URL)
	}
}

func TestBuildRequestBody_SemuaTipe(t *testing.T) {
	for taskType, tc := range map[string]struct {
		params string
		check  func(cwmpxml.Body) bool
	}{
		domain.TaskTypeGetParameterNames: {`{"path":"Device.","next_level":true}`, func(b cwmpxml.Body) bool {
			return b.GetParameterNames != nil && b.GetParameterNames.ParameterPath == "Device." && b.GetParameterNames.NextLevel
		}},
		domain.TaskTypeAddObject: {`{"object_name":"X.PM."}`, func(b cwmpxml.Body) bool {
			return b.AddObject != nil && b.AddObject.ObjectName == "X.PM." && b.AddObject.ParameterKey == "uu"
		}},
		domain.TaskTypeDeleteObject: {`{"object_name":"X.PM.2."}`, func(b cwmpxml.Body) bool {
			return b.DeleteObject != nil && b.DeleteObject.ObjectName == "X.PM.2." && b.DeleteObject.ParameterKey == "uu"
		}},
		domain.TaskTypeReboot:       {``, func(b cwmpxml.Body) bool { return b.Reboot != nil && b.Reboot.CommandKey == "uu" }},
		domain.TaskTypeFactoryReset: {``, func(b cwmpxml.Body) bool { return b.FactoryReset != nil }},
		domain.TaskTypeUpload: {`{"file_type":"1 Vendor Configuration File","url":"http://h/u","username":"a","password":"b"}`, func(b cwmpxml.Body) bool {
			return b.Upload != nil && b.Upload.URL == "http://h/u" && b.Upload.CommandKey == "uu" && b.Upload.Username == "a"
		}},
		domain.TaskTypeScheduleInform: {`{"delay_seconds":45}`, func(b cwmpxml.Body) bool {
			return b.ScheduleInform != nil && b.ScheduleInform.DelaySeconds == 45 && b.ScheduleInform.CommandKey == "uu"
		}},
	} {
		t.Run(taskType, func(t *testing.T) {
			body, err := BuildRequestBody(taskType, "uu", []byte(tc.params), nsCWMP10)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.check(body) {
				t.Fatalf("isi body salah: %+v", body)
			}
			if body.Namespace() != nsCWMP10 {
				t.Fatalf("namespace %q", body.Namespace())
			}
		})
	}
}

func TestBuildRequestBody_JSONRusakDanTipeAsing(t *testing.T) {
	for _, tt := range []string{
		domain.TaskTypeGetParameterValues, domain.TaskTypeSetParameterValues, domain.TaskTypeGetParameterNames,
		domain.TaskTypeAddObject, domain.TaskTypeDeleteObject, domain.TaskTypeDownload, domain.TaskTypeUpload, domain.TaskTypeScheduleInform,
	} {
		if _, err := BuildRequestBody(tt, "u", []byte("{rusak"), nsCWMP10); err == nil {
			t.Errorf("%s: JSON rusak harus error", tt)
		}
	}
	_, err := BuildRequestBody("TIPE_GAIB", "u", []byte("{}"), nsCWMP10)
	if err == nil || !strings.Contains(err.Error(), "TIPE_GAIB") {
		t.Fatalf("tipe asing harus error yang menyebut tipenya: %v", err)
	}
}

func TestParseResponseValues(t *testing.T) {
	got := ParseResponseValues(cwmpxml.ParameterValueList{Values: []cwmpxml.ParameterValue{
		{Name: "a", Value: cwmpxml.ValueType{Type: "xsd:string", Value: "1"}},
		{Name: "b", Value: cwmpxml.ValueType{Value: ""}},
	}})
	if len(got) != 2 || got[0].Name != "a" || got[0].Value != "1" || got[1].Name != "b" || got[1].Value != "" {
		t.Fatalf("%+v", got)
	}
	if out := ParseResponseValues(cwmpxml.ParameterValueList{}); out == nil || len(out) != 0 {
		t.Fatalf("kosong harus slice kosong non-nil: %#v", out)
	}
}

// Tipe parameter: hint "types" dipakai sebagai xsi:type; tanpa hint / tak dikenal -> xsd:string.
func TestBuildSetParameterValues_TipeXSD(t *testing.T) {
	params := []byte(`{"values":{"A.Interval":"300","A.Enable":"true","A.SSID":"x","A.Aneh":"1"},"types":{"A.Interval":"xsd:unsignedInt","A.Enable":"boolean","A.Aneh":"xsd:float"}}`)
	body, err := BuildRequestBody(domain.TaskTypeSetParameterValues, "u", params, nsCWMP10)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, v := range body.SetParameterValues.ParameterList.Values {
		got[v.Name] = v.Value.Type
	}
	want := map[string]string{"A.Interval": "xsd:unsignedInt", "A.Enable": "xsd:boolean", "A.SSID": "xsd:string", "A.Aneh": "xsd:string"}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("tipe %s = %q, want %q", k, got[k], w)
		}
	}
}

func TestBuildParameterAttributes(t *testing.T) {
	body, err := BuildRequestBody(domain.TaskTypeSetParameterAttributes, "u",
		[]byte(`{"attributes":[{"name":"Device.WiFi.SSID.1.SSID","notification_change":true,"notification":2}]}`), nsCWMP10)
	if err != nil || body.SetParameterAttributes == nil {
		t.Fatalf("%v %+v", err, body)
	}
	it := body.SetParameterAttributes.ParameterList.Items
	if len(it) != 1 || it[0].Name != "Device.WiFi.SSID.1.SSID" || !it[0].NotificationChange || it[0].Notification != 2 {
		t.Fatalf("%+v", it)
	}
	g, err := BuildRequestBody(domain.TaskTypeGetParameterAttributes, "u", []byte(`{"names":["A.B"]}`), nsCWMP10)
	if err != nil || g.GetParameterAttributes == nil || len(g.GetParameterAttributes.ParameterNames.Items) != 1 {
		t.Fatalf("%v %+v", err, g)
	}
}
