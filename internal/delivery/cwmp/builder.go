package cwmp

import (
	"encoding/json"
	"fmt"

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
			XMLName:        cwmpxml.RPCName(ns, "GetParameterValues"),
			ParameterNames: cwmpxml.StringList{Items: p.Names},
		}}, nil

	case domain.TaskTypeSetParameterValues:
		var p struct {
			Values       map[string]string `json:"values"`
			ParameterKey string            `json:"parameter_key"`
		}
		if err := json.Unmarshal(parameters, &p); err != nil {
			return cwmpxml.Body{}, err
		}
		var pl cwmpxml.ParameterValueList
		for name, val := range p.Values {
			pl.Values = append(pl.Values, cwmpxml.ParameterValue{
				Name:  name,
				Value: cwmpxml.ValueType{Type: "xsd:string", Value: val},
			})
		}
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

	default:
		return cwmpxml.Body{}, fmt.Errorf("cwmp: builder belum mendukung tipe task %q", taskTypeCode)
	}
}

// ParseResponseValues mengekstrak ParameterValueList generik dari Body
// respons (GetParameterValuesResponse) menjadi pasangan name/value polos.
func ParseResponseValues(pl cwmpxml.ParameterValueList) []NameValue {
	out := make([]NameValue, 0, len(pl.Values))
	for _, v := range pl.Values {
		out = append(out, NameValue{Name: v.Name, Value: v.Value.Value})
	}
	return out
}

type NameValue struct {
	Name  string
	Value string
}
