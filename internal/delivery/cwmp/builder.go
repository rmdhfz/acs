package cwmp

import (
	"encoding/json"
	"fmt"
	"sort"

	"acs/internal/domain"
	"acs/pkg/cwmpxml"
)

// BuildRequestBody menerjemahkan payload JSON task.Parameters (dibentuk
// usecase/task sesuai TaskType, lihat OutboundRPC di usecase/session) menjadi
// Body RPC CWMP siap kirim. Ini satu-satunya tempat yang tahu bentuk JSON
// task.Parameters DAN struct cwmpxml — kontrak internal antara kedua sisi.
//
// ns adalah namespace CWMP yang dipakai utk elemen RPC yang dibangun (lihat
// cwmpxml.RPCName) — diresolve pemanggil (handler.go) dari namespace yang
// dideklarasikan CPE ybs sendiri pada request sesi ini, fallback ke
// cwmpxml.NSCWMP bila tidak ada informasi tsb (lihat komentar handler.go
// soal kapan ini terjadi).
func BuildRequestBody(taskTypeCode, taskUUID string, parameters []byte, ns string) (cwmpxml.Body, error) {
	switch taskTypeCode {
	case domain.TaskTypeGetParameterValues:
		var p struct {
			Names []string `json:"names"`
		}
		if err := json.Unmarshal(parameters, &p); err != nil {
			return cwmpxml.Body{}, err
		}
		return cwmpxml.Body{GetParameterValues: &cwmpxml.GetParameterValues{
			XMLName: cwmpxml.RPCName(ns, "GetParameterValues"),
			// soap-enc:arrayType WAJIB menurut TR-069 (array SOAP); firmware yang
			// ketat menolak list tanpanya.
			ParameterNames: cwmpxml.StringList{ArrayType: fmt.Sprintf("xsd:string[%d]", len(p.Names)), Items: p.Names},
		}}, nil

	case domain.TaskTypeSetParameterValues:
		var p struct {
			Values       map[string]string `json:"values"`
			Types        map[string]string `json:"types"` // path -> xsd:type (opsional; default xsd:string)
			ParameterKey string            `json:"parameter_key"`
		}
		if err := json.Unmarshal(parameters, &p); err != nil {
			return cwmpxml.Body{}, err
		}
		// Urutan nama diurutkan: iterasi map Go acak, dan RPC yang tidak
		// deterministik menyulitkan debugging (log/capture berbeda tiap kirim).
		names := make([]string, 0, len(p.Values))
		for name := range p.Values {
			names = append(names, name)
		}
		sort.Strings(names)
		var pl cwmpxml.ParameterValueList
		for _, name := range names {
			pl.Values = append(pl.Values, cwmpxml.ParameterValue{
				Name:  name,
				Value: cwmpxml.ValueType{Type: xsdTypeFor(p.Types[name]), Value: p.Values[name]},
			})
		}
		pl.ArrayType = fmt.Sprintf("cwmp:ParameterValueStruct[%d]", len(pl.Values))
		key := p.ParameterKey
		if key == "" {
			key = taskUUID
		}
		return cwmpxml.Body{SetParameterValues: &cwmpxml.SetParameterValues{
			XMLName: cwmpxml.RPCName(ns, "SetParameterValues"), ParameterList: pl, ParameterKey: key,
		}}, nil

	case domain.TaskTypeGetParameterNames:
		var p struct {
			Path      string `json:"path"`
			NextLevel bool   `json:"next_level"`
		}
		if err := json.Unmarshal(parameters, &p); err != nil {
			return cwmpxml.Body{}, err
		}
		return cwmpxml.Body{GetParameterNames: &cwmpxml.GetParameterNames{
			XMLName: cwmpxml.RPCName(ns, "GetParameterNames"), ParameterPath: p.Path, NextLevel: p.NextLevel,
		}}, nil

	case domain.TaskTypeAddObject:
		var p struct {
			ObjectName string `json:"object_name"`
		}
		if err := json.Unmarshal(parameters, &p); err != nil {
			return cwmpxml.Body{}, err
		}
		return cwmpxml.Body{AddObject: &cwmpxml.AddObject{
			XMLName: cwmpxml.RPCName(ns, "AddObject"), ObjectName: p.ObjectName, ParameterKey: taskUUID,
		}}, nil

	case domain.TaskTypeDeleteObject:
		var p struct {
			ObjectName string `json:"object_name"`
		}
		if err := json.Unmarshal(parameters, &p); err != nil {
			return cwmpxml.Body{}, err
		}
		return cwmpxml.Body{DeleteObject: &cwmpxml.DeleteObject{
			XMLName: cwmpxml.RPCName(ns, "DeleteObject"), ObjectName: p.ObjectName, ParameterKey: taskUUID,
		}}, nil

	case domain.TaskTypeReboot:
		return cwmpxml.Body{Reboot: &cwmpxml.Reboot{XMLName: cwmpxml.RPCName(ns, "Reboot"), CommandKey: taskUUID}}, nil

	case domain.TaskTypeFactoryReset:
		return cwmpxml.Body{FactoryReset: &cwmpxml.FactoryReset{XMLName: cwmpxml.RPCName(ns, "FactoryReset")}}, nil

	case domain.TaskTypeDownload:
		var p struct {
			FileType       string `json:"file_type"`
			URL            string `json:"url"`
			Username       string `json:"username"`
			Password       string `json:"password"`
			TargetFileName string `json:"target_file_name"`
			FileSize       int64  `json:"file_size"`
		}
		if err := json.Unmarshal(parameters, &p); err != nil {
			return cwmpxml.Body{}, err
		}
		return cwmpxml.Body{Download: &cwmpxml.Download{
			XMLName:        cwmpxml.RPCName(ns, "Download"),
			CommandKey:     taskUUID,
			FileType:       p.FileType,
			URL:            p.URL,
			Username:       p.Username,
			Password:       p.Password,
			TargetFileName: p.TargetFileName,
			FileSize:       p.FileSize,
		}}, nil

	case domain.TaskTypeUpload:
		var p struct {
			FileType string `json:"file_type"`
			URL      string `json:"url"`
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal(parameters, &p); err != nil {
			return cwmpxml.Body{}, err
		}
		return cwmpxml.Body{Upload: &cwmpxml.Upload{
			XMLName:    cwmpxml.RPCName(ns, "Upload"),
			CommandKey: taskUUID, FileType: p.FileType, URL: p.URL, Username: p.Username, Password: p.Password,
		}}, nil

	case domain.TaskTypeScheduleInform:
		var p struct {
			DelaySeconds int `json:"delay_seconds"`
		}
		if err := json.Unmarshal(parameters, &p); err != nil {
			return cwmpxml.Body{}, err
		}
		return cwmpxml.Body{ScheduleInform: &cwmpxml.ScheduleInform{
			XMLName: cwmpxml.RPCName(ns, "ScheduleInform"), DelaySeconds: p.DelaySeconds, CommandKey: taskUUID,
		}}, nil

	case domain.TaskTypeGetParameterAttributes:
		var p struct {
			Names []string `json:"names"`
		}
		if err := json.Unmarshal(parameters, &p); err != nil {
			return cwmpxml.Body{}, err
		}
		return cwmpxml.Body{GetParameterAttributes: &cwmpxml.GetParameterAttributes{
			XMLName:        cwmpxml.RPCName(ns, "GetParameterAttributes"),
			ParameterNames: cwmpxml.StringList{ArrayType: fmt.Sprintf("xsd:string[%d]", len(p.Names)), Items: p.Names},
		}}, nil

	case domain.TaskTypeSetParameterAttributes:
		var p struct {
			Attributes []struct {
				Name               string   `json:"name"`
				NotificationChange bool     `json:"notification_change"`
				Notification       int      `json:"notification"`
				AccessListChange   bool     `json:"access_list_change"`
				AccessList         []string `json:"access_list"`
			} `json:"attributes"`
		}
		if err := json.Unmarshal(parameters, &p); err != nil {
			return cwmpxml.Body{}, err
		}
		var list cwmpxml.SetParameterAttributesList
		for _, a := range p.Attributes {
			list.Items = append(list.Items, cwmpxml.SetParameterAttributesStruct{
				Name: a.Name, NotificationChange: a.NotificationChange, Notification: a.Notification,
				AccessListChange: a.AccessListChange,
				AccessList:       cwmpxml.StringList{ArrayType: fmt.Sprintf("xsd:string[%d]", len(a.AccessList)), Items: a.AccessList},
			})
		}
		list.ArrayType = fmt.Sprintf("cwmp:SetParameterAttributesStruct[%d]", len(list.Items))
		return cwmpxml.Body{SetParameterAttributes: &cwmpxml.SetParameterAttributes{
			XMLName: cwmpxml.RPCName(ns, "SetParameterAttributes"), ParameterList: list,
		}}, nil

	default:
		return cwmpxml.Body{}, fmt.Errorf("cwmp: builder belum mendukung tipe task %q", taskTypeCode)
	}
}

// xsdTypeFor menerjemahkan hint tipe ("boolean", "xsd:unsignedInt", ...) ke
// atribut xsi:type; tak dikenal/kosong -> xsd:string (perilaku lama).
func xsdTypeFor(hint string) string {
	if code := domain.NormalizeXSDType(hint); code != "" {
		return "xsd:" + code
	}
	return "xsd:string"
}

// ParseResponseValues mengekstrak ParameterValueList generik dari Body
// respons (GetParameterValuesResponse) menjadi pasangan name/value polos.
func ParseResponseValues(pl cwmpxml.ParameterValueList) []NameValue {
	out := make([]NameValue, 0, len(pl.Values))
	for _, v := range pl.Values {
		out = append(out, NameValue{Name: v.Name, Value: v.Value.Value, Type: v.Value.Type})
	}
	return out
}

type NameValue struct {
	Name  string
	Value string
	Type  string // xsi:type dari CPE, mis. "xsd:unsignedInt" (kosong bila tak ada)
}
