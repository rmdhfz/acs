package cwmpxml

import "encoding/xml"

// ValueType merepresentasikan <Value xsi:type="xsd:string">...</Value>.
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
	XMLName       xml.Name           `xml:"urn:dslforum-org:cwmp-1-2 Inform"`
	DeviceId      DeviceIDStruct     `xml:"DeviceId"`
	Event         EventList          `xml:"Event"`
	MaxEnvelopes  int                `xml:"MaxEnvelopes"`
	CurrentTime   string             `xml:"CurrentTime"`
	RetryCount    int                `xml:"RetryCount"`
	ParameterList ParameterValueList `xml:"ParameterList"`
}

type InformResponse struct {
	XMLName      xml.Name `xml:"urn:dslforum-org:cwmp-1-2 InformResponse"`
	MaxEnvelopes int      `xml:"MaxEnvelopes"`
}

// ---- GetParameterValues ----

type GetParameterValues struct {
	XMLName        xml.Name   `xml:"urn:dslforum-org:cwmp-1-2 GetParameterValues"`
	ParameterNames StringList `xml:"ParameterNames"`
}

type GetParameterValuesResponse struct {
	XMLName       xml.Name           `xml:"urn:dslforum-org:cwmp-1-2 GetParameterValuesResponse"`
	ParameterList ParameterValueList `xml:"ParameterList"`
}

// ---- SetParameterValues ----

type SetParameterValues struct {
	XMLName       xml.Name           `xml:"urn:dslforum-org:cwmp-1-2 SetParameterValues"`
	ParameterList ParameterValueList `xml:"ParameterList"`
	ParameterKey  string             `xml:"ParameterKey"`
}

type SetParameterValuesResponse struct {
	XMLName xml.Name `xml:"urn:dslforum-org:cwmp-1-2 SetParameterValuesResponse"`
	Status  int      `xml:"Status"`
}

// ---- GetParameterNames ----

type GetParameterNames struct {
	XMLName       xml.Name `xml:"urn:dslforum-org:cwmp-1-2 GetParameterNames"`
	ParameterPath string   `xml:"ParameterPath"`
	NextLevel     bool     `xml:"NextLevel"`
}

type GetParameterNamesResponse struct {
	XMLName       xml.Name          `xml:"urn:dslforum-org:cwmp-1-2 GetParameterNamesResponse"`
	ParameterList ParameterInfoList `xml:"ParameterList"`
}

// ---- AddObject / DeleteObject ----

type AddObject struct {
	XMLName      xml.Name `xml:"urn:dslforum-org:cwmp-1-2 AddObject"`
	ObjectName   string   `xml:"ObjectName"`
	ParameterKey string   `xml:"ParameterKey"`
}

type AddObjectResponse struct {
	XMLName        xml.Name `xml:"urn:dslforum-org:cwmp-1-2 AddObjectResponse"`
	InstanceNumber int      `xml:"InstanceNumber"`
	Status         int      `xml:"Status"`
}

type DeleteObject struct {
	XMLName      xml.Name `xml:"urn:dslforum-org:cwmp-1-2 DeleteObject"`
	ObjectName   string   `xml:"ObjectName"`
	ParameterKey string   `xml:"ParameterKey"`
}

type DeleteObjectResponse struct {
	XMLName xml.Name `xml:"urn:dslforum-org:cwmp-1-2 DeleteObjectResponse"`
	Status  int      `xml:"Status"`
}

// ---- Reboot / FactoryReset ----

type Reboot struct {
	XMLName    xml.Name `xml:"urn:dslforum-org:cwmp-1-2 Reboot"`
	CommandKey string   `xml:"CommandKey"`
}

type RebootResponse struct {
	XMLName xml.Name `xml:"urn:dslforum-org:cwmp-1-2 RebootResponse"`
}

type FactoryReset struct {
	XMLName xml.Name `xml:"urn:dslforum-org:cwmp-1-2 FactoryReset"`
}

type FactoryResetResponse struct {
	XMLName xml.Name `xml:"urn:dslforum-org:cwmp-1-2 FactoryResetResponse"`
}

// ---- Download / Upload / TransferComplete ----

type Download struct {
	XMLName        xml.Name `xml:"urn:dslforum-org:cwmp-1-2 Download"`
	CommandKey     string   `xml:"CommandKey"`
	FileType       string   `xml:"FileType"`
	URL            string   `xml:"URL"`
	Username       string   `xml:"Username"`
	Password       string   `xml:"Password"`
	FileSize       int64    `xml:"FileSize"`
	TargetFileName string   `xml:"TargetFileName"`
	DelaySeconds   int      `xml:"DelaySeconds"`
	SuccessURL     string   `xml:"SuccessURL"`
	FailureURL     string   `xml:"FailureURL"`
}

type DownloadResponse struct {
	XMLName      xml.Name `xml:"urn:dslforum-org:cwmp-1-2 DownloadResponse"`
	Status       int      `xml:"Status"`
	StartTime    string   `xml:"StartTime"`
	CompleteTime string   `xml:"CompleteTime"`
}

type Upload struct {
	XMLName      xml.Name `xml:"urn:dslforum-org:cwmp-1-2 Upload"`
	CommandKey   string   `xml:"CommandKey"`
	FileType     string   `xml:"FileType"`
	URL          string   `xml:"URL"`
	Username     string   `xml:"Username"`
	Password     string   `xml:"Password"`
	DelaySeconds int      `xml:"DelaySeconds"`
}

type UploadResponse struct {
	XMLName      xml.Name `xml:"urn:dslforum-org:cwmp-1-2 UploadResponse"`
	Status       int      `xml:"Status"`
	StartTime    string   `xml:"StartTime"`
	CompleteTime string   `xml:"CompleteTime"`
}

type TransferComplete struct {
	XMLName      xml.Name   `xml:"urn:dslforum-org:cwmp-1-2 TransferComplete"`
	CommandKey   string     `xml:"CommandKey"`
	FaultStruct  *CWMPFault `xml:"FaultStruct"`
	StartTime    string     `xml:"StartTime"`
	CompleteTime string     `xml:"CompleteTime"`
}

type TransferCompleteResponse struct {
	XMLName xml.Name `xml:"urn:dslforum-org:cwmp-1-2 TransferCompleteResponse"`
}

// ---- ScheduleInform ----

type ScheduleInform struct {
	XMLName      xml.Name `xml:"urn:dslforum-org:cwmp-1-2 ScheduleInform"`
	DelaySeconds int      `xml:"DelaySeconds"`
	CommandKey   string   `xml:"CommandKey"`
}

type ScheduleInformResponse struct {
	XMLName xml.Name `xml:"urn:dslforum-org:cwmp-1-2 ScheduleInformResponse"`
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
	ArrayType string                          `xml:"http://schemas.xmlsoap.org/soap/encoding/ arrayType,attr,omitempty"`
	Items     []SetParameterAttributesStruct `xml:"SetParameterAttributesStruct"`
}

type SetParameterAttributes struct {
	XMLName       xml.Name                    `xml:"urn:dslforum-org:cwmp-1-2 SetParameterAttributes"`
	ParameterList SetParameterAttributesList `xml:"ParameterList"`
}

type SetParameterAttributesResponse struct {
	XMLName xml.Name `xml:"urn:dslforum-org:cwmp-1-2 SetParameterAttributesResponse"`
}

type GetParameterAttributes struct {
	XMLName        xml.Name   `xml:"urn:dslforum-org:cwmp-1-2 GetParameterAttributes"`
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
	XMLName       xml.Name               `xml:"urn:dslforum-org:cwmp-1-2 GetParameterAttributesResponse"`
	ParameterList ParameterAttributeList `xml:"ParameterList"`
}
