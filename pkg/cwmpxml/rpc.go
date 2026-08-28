package cwmpxml

import "encoding/xml"

// Catatan penting soal XMLName di seluruh file ini: field XMLName xml.Name
// SENGAJA TIDAK diberi tag `xml:"..."` literal (mis. BUKAN
// `xml:"urn:dslforum-org:cwmp-1-2 Inform"` seperti sebelumnya). Ini bukan
// kelalaian -- encoding/xml memprioritaskan TAG pada field XMLName di atas
// NILAI runtime field tsb saat marshal (lihat godoc encoding/xml.Marshal:
// "the tag on the XMLName field" adalah prioritas #1, "the value of the
// XMLName field" baru #2) -- kalau tag namespace di-hardcode di sini, nilai
// Space yang di-set runtime (lihat RPCName di envelope.go) akan DIABAIKAN
// sepenuhnya saat marshal, dan Unmarshal akan MENOLAK envelope apa pun yang
// namespace-nya BUKAN persis "urn:dslforum-org:cwmp-1-2" (dikonfirmasi lewat
// percobaan langsung: Unmarshal envelope Inform dgn xmlns:cwmp="urn:...-1-0"
// gagal total dengan "expected element <Inform> in name space ... but have
// ...", BUKAN cuma "salah balas namespace" seperti dugaan awal -- CPE dgn
// versi/pernyataan namespace CWMP selain cwmp-1-2 akan gagal total connect
// ke ACS ini sebelum perbaikan ini).
//
// Dengan tag dihapus (field XMLName polos tanpa tag sama sekali):
//   - Unmarshal tetap mencocokkan elemen berdasar LOCAL NAME saja (via tag
//     pada field pembungkus di Body, lihat envelope.go -- tag-tag itu MEMANG
//     sudah tanpa namespace sejak awal), sehingga menerima envelope CWMP dgn
//     namespace/versi APAPUN, sekaligus tetap meng-capture namespace asli
//     yang dipakai CPE ke field XMLName.Space (dibaca balik lewat
//     Body.Namespace(), envelope.go).
//   - Marshal (ACS -> CPE) memakai nilai runtime XMLName yang di-set eksplisit
//     oleh pemanggil (lihat RPCName di envelope.go, dipanggil dari
//     delivery/cwmp/builder.go & handler.go) -- fallback aman ke elemen tanpa
//     namespace (bukan crash) bila pemanggil lupa men-set-nya sama sekali.
type ValueType struct {
	Type  string `xml:"http://www.w3.org/2001/XMLSchema-instance type,attr"`
	Value string `xml:",chardata"`
}

type ParameterValue struct {
	Name  string    `xml:"Name"`
	Value ValueType `xml:"Value"`
}

type ParameterValueList struct {
	ArrayType string           `xml:"http://schemas.xmlsoap.org/soap/encoding/ arrayType,attr,omitempty"`
	Values    []ParameterValue `xml:"ParameterValueStruct"`
}

type StringList struct {
	ArrayType string   `xml:"http://schemas.xmlsoap.org/soap/encoding/ arrayType,attr,omitempty"`
	Items     []string `xml:"string"`
}

type ParameterInfoStruct struct {
	Name     string `xml:"Name"`
	Writable bool   `xml:"Writable"`
}

type ParameterInfoList struct {
	ArrayType string                `xml:"http://schemas.xmlsoap.org/soap/encoding/ arrayType,attr,omitempty"`
	Items     []ParameterInfoStruct `xml:"ParameterInfoStruct"`
}

// ---- DeviceId & Event (Inform) ----

type DeviceIDStruct struct {
	Manufacturer string `xml:"Manufacturer"`
	OUI          string `xml:"OUI"`
	ProductClass string `xml:"ProductClass"`
	SerialNumber string `xml:"SerialNumber"`
}

type EventStruct struct {
	EventCode  string `xml:"EventCode"`
	CommandKey string `xml:"CommandKey"`
}

type EventList struct {
	ArrayType string        `xml:"http://schemas.xmlsoap.org/soap/encoding/ arrayType,attr,omitempty"`
	Items     []EventStruct `xml:"EventStruct"`
}

// ---- Inform ----

type Inform struct {
	XMLName       xml.Name
	DeviceId      DeviceIDStruct     `xml:"DeviceId"`
	Event         EventList          `xml:"Event"`
	MaxEnvelopes  int                `xml:"MaxEnvelopes"`
	CurrentTime   string             `xml:"CurrentTime"`
	RetryCount    int                `xml:"RetryCount"`
	ParameterList ParameterValueList `xml:"ParameterList"`
}

type InformResponse struct {
	XMLName      xml.Name
	MaxEnvelopes int `xml:"MaxEnvelopes"`
}

// ---- GetParameterValues ----

type GetParameterValues struct {
	XMLName        xml.Name
	ParameterNames StringList `xml:"ParameterNames"`
}

type GetParameterValuesResponse struct {
	XMLName       xml.Name
	ParameterList ParameterValueList `xml:"ParameterList"`
}

// ---- SetParameterValues ----

type SetParameterValues struct {
	XMLName       xml.Name
	ParameterList ParameterValueList `xml:"ParameterList"`
	ParameterKey  string             `xml:"ParameterKey"`
}

type SetParameterValuesResponse struct {
	XMLName xml.Name
	Status  int `xml:"Status"`
}

// ---- GetParameterNames ----

type GetParameterNames struct {
	XMLName       xml.Name
	ParameterPath string `xml:"ParameterPath"`
	NextLevel     bool   `xml:"NextLevel"`
}

type GetParameterNamesResponse struct {
	XMLName       xml.Name
	ParameterList ParameterInfoList `xml:"ParameterList"`
}

// ---- AddObject / DeleteObject ----

type AddObject struct {
	XMLName      xml.Name
	ObjectName   string `xml:"ObjectName"`
	ParameterKey string `xml:"ParameterKey"`
}

type AddObjectResponse struct {
	XMLName        xml.Name
	InstanceNumber int `xml:"InstanceNumber"`
	Status         int `xml:"Status"`
}

type DeleteObject struct {
	XMLName      xml.Name
	ObjectName   string `xml:"ObjectName"`
	ParameterKey string `xml:"ParameterKey"`
}

type DeleteObjectResponse struct {
	XMLName xml.Name
	Status  int `xml:"Status"`
}

// ---- Reboot / FactoryReset ----

type Reboot struct {
	XMLName    xml.Name
	CommandKey string `xml:"CommandKey"`
}

type RebootResponse struct {
	XMLName xml.Name
}

type FactoryReset struct {
	XMLName xml.Name
}

type FactoryResetResponse struct {
	XMLName xml.Name
}

// ---- Download / Upload / TransferComplete ----

type Download struct {
	XMLName        xml.Name
	CommandKey     string `xml:"CommandKey"`
	FileType       string `xml:"FileType"`
	URL            string `xml:"URL"`
	Username       string `xml:"Username"`
	Password       string `xml:"Password"`
	FileSize       int64  `xml:"FileSize"`
	TargetFileName string `xml:"TargetFileName"`
	DelaySeconds   int    `xml:"DelaySeconds"`
	SuccessURL     string `xml:"SuccessURL"`
	FailureURL     string `xml:"FailureURL"`
}

type DownloadResponse struct {
	XMLName      xml.Name
	Status       int    `xml:"Status"`
	StartTime    string `xml:"StartTime"`
	CompleteTime string `xml:"CompleteTime"`
}

type Upload struct {
	XMLName      xml.Name
	CommandKey   string `xml:"CommandKey"`
	FileType     string `xml:"FileType"`
	URL          string `xml:"URL"`
	Username     string `xml:"Username"`
	Password     string `xml:"Password"`
	DelaySeconds int    `xml:"DelaySeconds"`
}

type UploadResponse struct {
	XMLName      xml.Name
	Status       int    `xml:"Status"`
	StartTime    string `xml:"StartTime"`
	CompleteTime string `xml:"CompleteTime"`
}

type TransferComplete struct {
	XMLName      xml.Name
	CommandKey   string     `xml:"CommandKey"`
	FaultStruct  *CWMPFault `xml:"FaultStruct"`
	StartTime    string     `xml:"StartTime"`
	CompleteTime string     `xml:"CompleteTime"`
}

type TransferCompleteResponse struct {
	XMLName xml.Name
}

// ---- ScheduleInform ----

type ScheduleInform struct {
	XMLName      xml.Name
	DelaySeconds int    `xml:"DelaySeconds"`
	CommandKey   string `xml:"CommandKey"`
}

type ScheduleInformResponse struct {
	XMLName xml.Name
}

// ---- SetParameterAttributes / GetParameterAttributes ----

type SetParameterAttributesStruct struct {
	Name               string     `xml:"Name"`
	NotificationChange bool       `xml:"NotificationChange"`
	Notification       int        `xml:"Notification"`
	AccessListChange   bool       `xml:"AccessListChange"`
	AccessList         StringList `xml:"AccessList"`
}

type SetParameterAttributesList struct {
	ArrayType string                         `xml:"http://schemas.xmlsoap.org/soap/encoding/ arrayType,attr,omitempty"`
	Items     []SetParameterAttributesStruct `xml:"SetParameterAttributesStruct"`
}

type SetParameterAttributes struct {
	XMLName       xml.Name
	ParameterList SetParameterAttributesList `xml:"ParameterList"`
}

type SetParameterAttributesResponse struct {
	XMLName xml.Name
}

type GetParameterAttributes struct {
	XMLName        xml.Name
	ParameterNames StringList `xml:"ParameterNames"`
}

type ParameterAttributeStruct struct {
	Name         string     `xml:"Name"`
	Notification int        `xml:"Notification"`
	AccessList   StringList `xml:"AccessList"`
}

type ParameterAttributeList struct {
	ArrayType string                     `xml:"http://schemas.xmlsoap.org/soap/encoding/ arrayType,attr,omitempty"`
	Items     []ParameterAttributeStruct `xml:"ParameterAttributeStruct"`
}

type GetParameterAttributesResponse struct {
	XMLName       xml.Name
	ParameterList ParameterAttributeList `xml:"ParameterList"`
}
