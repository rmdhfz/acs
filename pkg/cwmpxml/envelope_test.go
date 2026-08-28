package cwmpxml

import (
	"strings"
	"testing"
)

// TestUnmarshalAcceptsAnyCWMPNamespace adalah regresi utk bug yang ditemukan
// saat investigasi arsitektur: sebelum XMLName di rpc.go dilepas tag
// namespace hardcoded-nya, Unmarshal MENOLAK TOTAL envelope Inform yang
// namespace-nya bukan persis "urn:dslforum-org:cwmp-1-2" (CPE yang
// mendeklarasikan versi CWMP lain, mis. cwmp-1-0/1-1/1-3, akan gagal
// connect ke ACS ini sama sekali, bukan cuma "dibalas namespace salah").
func TestUnmarshalAcceptsAnyCWMPNamespace(t *testing.T) {
	cases := []string{
		"urn:dslforum-org:cwmp-1-2",
		"urn:dslforum-org:cwmp-1-0",
		"urn:dslforum-org:cwmp-1-1",
		"urn:dslforum-org:cwmp-1-3",
		"urn:dslforum-org:cwmp-1-4",
	}
	for _, ns := range cases {
		t.Run(ns, func(t *testing.T) {
			data := []byte(`<?xml version="1.0"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:cwmp="` + ns + `">
  <soapenv:Body>
    <cwmp:Inform>
      <DeviceId><Manufacturer>ACME</Manufacturer><OUI>001122</OUI><ProductClass>PC</ProductClass><SerialNumber>SN1</SerialNumber></DeviceId>
      <Event><EventStruct><EventCode>0 BOOTSTRAP</EventCode><CommandKey></CommandKey></EventStruct></Event>
      <MaxEnvelopes>1</MaxEnvelopes>
      <CurrentTime>2020-01-01T00:00:00Z</CurrentTime>
      <RetryCount>0</RetryCount>
      <ParameterList></ParameterList>
    </cwmp:Inform>
  </soapenv:Body>
</soapenv:Envelope>`)
			env, err := Unmarshal(data)
			if err != nil {
				t.Fatalf("Unmarshal gagal utk namespace %q: %v", ns, err)
			}
			if env.Body.Inform == nil {
				t.Fatalf("Body.Inform nil utk namespace %q", ns)
			}
			if env.Body.Inform.DeviceId.SerialNumber != "SN1" {
				t.Errorf("DeviceId tidak ter-parse dgn benar utk namespace %q", ns)
			}
			if got := env.Body.Namespace(); got != ns {
				t.Errorf("Body.Namespace() = %q, want %q (harus meng-capture namespace ASLI yang dipakai CPE)", got, ns)
			}
		})
	}
}

// TestNewEnvelopeEchoesNamespace memverifikasi outgoing envelope memakai
// namespace yang diberikan (echo balik dari CPE), bukan selalu NSCWMP
// hardcoded, dan fallback ke NSCWMP hanya bila ns kosong.
func TestNewEnvelopeEchoesNamespace(t *testing.T) {
	t.Run("ns non-kosong -> dipakai apa adanya di elemen RPC & atribut envelope", func(t *testing.T) {
		const ns = "urn:dslforum-org:cwmp-1-0"
		env := NewEnvelope("hdr-1", ns, Body{
			InformResponse: &InformResponse{XMLName: RPCName(ns, "InformResponse"), MaxEnvelopes: 1},
		})
		if env.XMLNSCWMP != ns {
			t.Errorf("Envelope.XMLNSCWMP = %q, want %q", env.XMLNSCWMP, ns)
		}
		out, err := Marshal(env)
		if err != nil {
			t.Fatalf("Marshal gagal: %v", err)
		}
		xmlStr := string(out)
		if !strings.Contains(xmlStr, `<InformResponse xmlns="`+ns+`">`) {
			t.Errorf("output XML tidak memakai namespace %q pada elemen InformResponse:\n%s", ns, xmlStr)
		}
		if strings.Contains(xmlStr, NSCWMP) {
			t.Errorf("output XML seharusnya TIDAK mengandung NSCWMP default (%q) ketika ns eksplisit lain diberikan:\n%s", NSCWMP, xmlStr)
		}
	})

	t.Run("ns kosong -> fallback ke NSCWMP default", func(t *testing.T) {
		env := NewEnvelope("hdr-2", "", Body{
			InformResponse: &InformResponse{XMLName: RPCName("", "InformResponse"), MaxEnvelopes: 1},
		})
		if env.XMLNSCWMP != NSCWMP {
			t.Errorf("Envelope.XMLNSCWMP = %q, want fallback %q", env.XMLNSCWMP, NSCWMP)
		}
		out, err := Marshal(env)
		if err != nil {
			t.Fatalf("Marshal gagal: %v", err)
		}
		if !strings.Contains(string(out), `<InformResponse xmlns="`+NSCWMP+`">`) {
			t.Errorf("output XML tidak fallback ke NSCWMP default:\n%s", string(out))
		}
	})
}
