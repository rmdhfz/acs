// Package cwmpxml berisi struct dan (de)serialisasi envelope CWMP 1.x
// (Broadband Forum TR-069) secara manual dengan encoding/xml stdlib —
// tanpa generator WSDL, sesuai keputusan di TECH.md §1/§12.
//
// Body memakai satu struct dengan field pointer per kemungkinan RPC method:
// encoding/xml akan mengisi field yang elemen-nya benar-benar hadir di XML
// dan membiarkan sisanya nil. Ini menghindari trik reflection/probe dan
// membuat kuirk per-vendor mudah ditangani secara eksplisit per struct.
package cwmpxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
)

const (
	NSSoapEnv = "http://schemas.xmlsoap.org/soap/envelope/"
	NSSoapEnc = "http://schemas.xmlsoap.org/soap/encoding/"
	NSXSD     = "http://www.w3.org/2001/XMLSchema"
	NSXSI     = "http://www.w3.org/2001/XMLSchema-instance"
	NSCWMP    = "urn:dslforum-org:cwmp-1-2"
)

type Envelope struct {
	XMLName      xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	XMLNSSoapEnv string   `xml:"xmlns:soapenv,attr"`
	XMLNSXSD     string   `xml:"xmlns:xsd,attr"`
	XMLNSXSI     string   `xml:"xmlns:xsi,attr"`
	XMLNSCWMP    string   `xml:"xmlns:cwmp,attr"`
	Header       *Header  `xml:"Header"`
	Body         Body     `xml:"Body"`
}

type Header struct {
	ID             *HeaderID `xml:"ID"`
	NoMoreRequests *int      `xml:"NoMoreRequests"`
}

type HeaderID struct {
	MustUnderstand string `xml:"soapenv:mustUnderstand,attr,omitempty"`
	Value          string `xml:",chardata"`
}

// Body berisi tepat satu RPC method (atau Fault) per pesan CWMP. Semua field
// lain otomatis nil saat unmarshal. Saat marshal (ACS -> CPE) isi hanya satu
// field yang relevan.
type Body struct {
	Fault *Fault `xml:"Fault"`

	Inform         *Inform         `xml:"Inform"`
	InformResponse *InformResponse `xml:"InformResponse"`

	GetParameterValues         *GetParameterValues         `xml:"GetParameterValues"`
	GetParameterValuesResponse *GetParameterValuesResponse `xml:"GetParameterValuesResponse"`

	SetParameterValues         *SetParameterValues         `xml:"SetParameterValues"`
	SetParameterValuesResponse *SetParameterValuesResponse `xml:"SetParameterValuesResponse"`

	GetParameterNames         *GetParameterNames         `xml:"GetParameterNames"`
	GetParameterNamesResponse *GetParameterNamesResponse `xml:"GetParameterNamesResponse"`

	AddObject         *AddObject         `xml:"AddObject"`
	AddObjectResponse *AddObjectResponse `xml:"AddObjectResponse"`

	DeleteObject         *DeleteObject         `xml:"DeleteObject"`
	DeleteObjectResponse *DeleteObjectResponse `xml:"DeleteObjectResponse"`

	Reboot         *Reboot         `xml:"Reboot"`
	RebootResponse *RebootResponse `xml:"RebootResponse"`

	FactoryReset         *FactoryReset         `xml:"FactoryReset"`
	FactoryResetResponse *FactoryResetResponse `xml:"FactoryResetResponse"`

	Download         *Download         `xml:"Download"`
	DownloadResponse *DownloadResponse `xml:"DownloadResponse"`

	Upload         *Upload         `xml:"Upload"`
	UploadResponse *UploadResponse `xml:"UploadResponse"`

	TransferComplete         *TransferComplete         `xml:"TransferComplete"`
	TransferCompleteResponse *TransferCompleteResponse `xml:"TransferCompleteResponse"`

	ScheduleInform         *ScheduleInform         `xml:"ScheduleInform"`
	ScheduleInformResponse *ScheduleInformResponse `xml:"ScheduleInformResponse"`

	SetParameterAttributes         *SetParameterAttributes         `xml:"SetParameterAttributes"`
	SetParameterAttributesResponse *SetParameterAttributesResponse `xml:"SetParameterAttributesResponse"`

	GetParameterAttributes         *GetParameterAttributes         `xml:"GetParameterAttributes"`
	GetParameterAttributesResponse *GetParameterAttributesResponse `xml:"GetParameterAttributesResponse"`
}

// Fault adalah SOAP Fault pembungkus cwmp:Fault (CPE -> ACS saat RPC gagal).
type Fault struct {
	FaultCode   string       `xml:"faultcode"`
	FaultString string       `xml:"faultstring"`
	Detail      *FaultDetail `xml:"detail"`
}

type FaultDetail struct {
	CWMPFault CWMPFault `xml:"Fault"`
}

type CWMPFault struct {
	FaultCode   string `xml:"FaultCode"`
	FaultString string `xml:"FaultString"`
}

// Unmarshal mem-parse body HTTP POST CWMP. Body kosong (POST kosong penanda
// akhir sesi, lihat TECH.md §3) menghasilkan Envelope kosong tanpa error.
func Unmarshal(data []byte) (*Envelope, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return &Envelope{}, nil
	}
	var env Envelope
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("cwmpxml: gagal unmarshal envelope: %w", err)
	}
	return &env, nil
}

// Marshal menghasilkan XML lengkap dengan deklarasi <?xml?> di depan.
func Marshal(env *Envelope) ([]byte, error) {
	out, err := xml.MarshalIndent(env, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("cwmpxml: gagal marshal envelope: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}

// NewEnvelope membuat envelope kosong dengan namespace standar dan header ID,
// siap diisi salah satu field Body oleh pemanggil.
func NewEnvelope(id string, body Body) *Envelope {
	var header *Header
	if id != "" {
		header = &Header{ID: &HeaderID{MustUnderstand: "1", Value: id}}
	}
	return &Envelope{
		XMLNSSoapEnv: NSSoapEnv,
		XMLNSXSD:     NSXSD,
		XMLNSXSI:     NSXSI,
		XMLNSCWMP:    NSCWMP,
		Header:       header,
		Body:         body,
	}
}

// IsEmpty menandakan POST kosong dari CPE (tanda tidak ada request lanjutan,
// ACS boleh menutup sesi jika juga tidak ada task pending — lihat TECH.md §3).
func (e *Envelope) IsEmpty() bool {
	return e == nil || e.XMLName.Local == ""
}

// Method mengembalikan nama RPC method yang terisi di Body, atau "" bila
// envelope kosong / tidak dikenali.
func (b Body) Method() string {
	switch {
	case b.Fault != nil:
		return "Fault"
	case b.Inform != nil:
		return "Inform"
	case b.InformResponse != nil:
		return "InformResponse"
	case b.GetParameterValues != nil:
		return "GetParameterValues"
	case b.GetParameterValuesResponse != nil:
		return "GetParameterValuesResponse"
	case b.SetParameterValues != nil:
		return "SetParameterValues"
	case b.SetParameterValuesResponse != nil:
		return "SetParameterValuesResponse"
	case b.GetParameterNames != nil:
		return "GetParameterNames"
	case b.GetParameterNamesResponse != nil:
		return "GetParameterNamesResponse"
	case b.AddObject != nil:
		return "AddObject"
	case b.AddObjectResponse != nil:
		return "AddObjectResponse"
	case b.DeleteObject != nil:
		return "DeleteObject"
	case b.DeleteObjectResponse != nil:
		return "DeleteObjectResponse"
	case b.Reboot != nil:
		return "Reboot"
	case b.RebootResponse != nil:
		return "RebootResponse"
	case b.FactoryReset != nil:
		return "FactoryReset"
	case b.FactoryResetResponse != nil:
		return "FactoryResetResponse"
	case b.Download != nil:
		return "Download"
	case b.DownloadResponse != nil:
		return "DownloadResponse"
	case b.Upload != nil:
		return "Upload"
	case b.UploadResponse != nil:
		return "UploadResponse"
	case b.TransferComplete != nil:
		return "TransferComplete"
	case b.TransferCompleteResponse != nil:
		return "TransferCompleteResponse"
	case b.ScheduleInform != nil:
		return "ScheduleInform"
	case b.ScheduleInformResponse != nil:
		return "ScheduleInformResponse"
	case b.SetParameterAttributes != nil:
		return "SetParameterAttributes"
	case b.SetParameterAttributesResponse != nil:
		return "SetParameterAttributesResponse"
	case b.GetParameterAttributes != nil:
		return "GetParameterAttributes"
	case b.GetParameterAttributesResponse != nil:
		return "GetParameterAttributesResponse"
	default:
		return ""
	}
}
