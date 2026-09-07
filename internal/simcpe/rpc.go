package simcpe

import (
	"strings"
	"time"

	"acs/pkg/cwmpxml"
)

// rpcOutcome adalah hasil menangani satu RPC dari ACS.
type rpcOutcome struct {
	respBody       cwmpxml.Body
	method         string
	fault          string // "" bila sukses
	deferredXfer   *cwmpxml.TransferComplete
	newSoftwareVer string
}

// handleRPC menerjemahkan Body request ACS -> Body response CPE, memutasi data
// model device. ns dipakai untuk XMLName elemen response (echo namespace ACS).
func handleRPC(d *Device, ns string, b cwmpxml.Body, faultParamSubstr string) rpcOutcome {
	switch {
	case b.GetParameterValues != nil:
		names := b.GetParameterValues.ParameterNames.Items
		vals, err := d.Get(names)
		if err != nil {
			return rpcOutcome{
				method:   "GetParameterValues",
				fault:    "9005 " + err.Error(),
				respBody: cwmpxml.Body{Fault: cwmpFault("9005", "Invalid parameter name")},
			}
		}
		var pl cwmpxml.ParameterValueList
		for _, nv := range vals {
			pl.Values = append(pl.Values, cwmpxml.ParameterValue{
				Name: nv.Name, Value: cwmpxml.ValueType{Type: "xsd:string", Value: nv.Value},
			})
		}
		return rpcOutcome{method: "GetParameterValues", respBody: cwmpxml.Body{
			GetParameterValuesResponse: &cwmpxml.GetParameterValuesResponse{
				XMLName: cwmpxml.RPCName(ns, "GetParameterValuesResponse"), ParameterList: pl,
			}}}

	case b.SetParameterValues != nil:
		values := map[string]string{}
		for _, pv := range b.SetParameterValues.ParameterList.Values {
			values[pv.Name] = pv.Value.Value
		}
		if faultParamSubstr != "" {
			for k := range values {
				if strings.Contains(k, faultParamSubstr) {
					return rpcOutcome{
						method:   "SetParameterValues",
						fault:    "9005 parameter " + k + " tidak didukung",
						respBody: cwmpxml.Body{Fault: cwmpFault("9005", "Invalid parameter name: "+k)},
					}
				}
			}
		}
		d.Set(values)
		return rpcOutcome{method: "SetParameterValues", respBody: cwmpxml.Body{
			SetParameterValuesResponse: &cwmpxml.SetParameterValuesResponse{
				XMLName: cwmpxml.RPCName(ns, "SetParameterValuesResponse"), Status: 0,
			}}}

	case b.GetParameterNames != nil:
		infos := d.ParameterNames(b.GetParameterNames.ParameterPath, b.GetParameterNames.NextLevel)
		var il cwmpxml.ParameterInfoList
		for _, pi := range infos {
			il.Items = append(il.Items, cwmpxml.ParameterInfoStruct{Name: pi.Name, Writable: pi.Writable})
		}
		return rpcOutcome{method: "GetParameterNames", respBody: cwmpxml.Body{
			GetParameterNamesResponse: &cwmpxml.GetParameterNamesResponse{
				XMLName: cwmpxml.RPCName(ns, "GetParameterNamesResponse"), ParameterList: il,
			}}}

	case b.AddObject != nil:
		inst := d.AddObject(b.AddObject.ObjectName)
		return rpcOutcome{method: "AddObject", respBody: cwmpxml.Body{
			AddObjectResponse: &cwmpxml.AddObjectResponse{
				XMLName: cwmpxml.RPCName(ns, "AddObjectResponse"), InstanceNumber: inst, Status: 0,
			}}}

	case b.DeleteObject != nil:
		d.DeleteObject(b.DeleteObject.ObjectName)
		return rpcOutcome{method: "DeleteObject", respBody: cwmpxml.Body{
			DeleteObjectResponse: &cwmpxml.DeleteObjectResponse{
				XMLName: cwmpxml.RPCName(ns, "DeleteObjectResponse"), Status: 0,
			}}}

	case b.Reboot != nil:
		return rpcOutcome{method: "Reboot", respBody: cwmpxml.Body{
			RebootResponse: &cwmpxml.RebootResponse{XMLName: cwmpxml.RPCName(ns, "RebootResponse")}}}

	case b.FactoryReset != nil:
		return rpcOutcome{method: "FactoryReset", respBody: cwmpxml.Body{
			FactoryResetResponse: &cwmpxml.FactoryResetResponse{XMLName: cwmpxml.RPCName(ns, "FactoryResetResponse")}}}

	case b.Download != nil:
		now := time.Now().UTC()
		tc := &cwmpxml.TransferComplete{
			XMLName:      cwmpxml.RPCName(ns, "TransferComplete"),
			CommandKey:   b.Download.CommandKey,
			StartTime:    now.Format(time.RFC3339),
			CompleteTime: now.Add(2 * time.Second).Format(time.RFC3339),
		}
		return rpcOutcome{
			method:         "Download",
			deferredXfer:   tc,
			newSoftwareVer: firmwareVersionFrom(b.Download),
			respBody: cwmpxml.Body{DownloadResponse: &cwmpxml.DownloadResponse{
				XMLName: cwmpxml.RPCName(ns, "DownloadResponse"), Status: 1,
				StartTime: now.Format(time.RFC3339), CompleteTime: now.Format(time.RFC3339),
			}},
		}

	case b.ScheduleInform != nil:
		return rpcOutcome{method: "ScheduleInform", respBody: cwmpxml.Body{
			ScheduleInformResponse: &cwmpxml.ScheduleInformResponse{XMLName: cwmpxml.RPCName(ns, "ScheduleInformResponse")}}}

	case b.SetParameterAttributes != nil:
		return rpcOutcome{method: "SetParameterAttributes", respBody: cwmpxml.Body{
			SetParameterAttributesResponse: &cwmpxml.SetParameterAttributesResponse{
				XMLName: cwmpxml.RPCName(ns, "SetParameterAttributesResponse")}}}

	case b.GetParameterAttributes != nil:
		var al cwmpxml.ParameterAttributeList
		for _, n := range b.GetParameterAttributes.ParameterNames.Items {
			al.Items = append(al.Items, cwmpxml.ParameterAttributeStruct{Name: n, Notification: 0})
		}
		return rpcOutcome{method: "GetParameterAttributes", respBody: cwmpxml.Body{
			GetParameterAttributesResponse: &cwmpxml.GetParameterAttributesResponse{
				XMLName: cwmpxml.RPCName(ns, "GetParameterAttributesResponse"), ParameterList: al,
			}}}

	default:
		return rpcOutcome{
			method:   "UNKNOWN(" + b.Method() + ")",
			fault:    "9000 method tidak didukung simulator",
			respBody: cwmpxml.Body{Fault: cwmpFault("9000", "Method not supported by simulator: "+b.Method())},
		}
	}
}

func firmwareVersionFrom(dl *cwmpxml.Download) string {
	if dl.TargetFileName != "" {
		return "SIM-" + dl.TargetFileName
	}
	return "SIM-UPGRADED"
}

// cwmpFault membangun SOAP Fault pembungkus cwmp:Fault seperti CPE nyata saat
// menolak RPC (dibaca balik ACS di buildRPCResponse -> handleFault).
func cwmpFault(code, msg string) *cwmpxml.Fault {
	return &cwmpxml.Fault{
		FaultCode:   "Client",
		FaultString: "CWMP fault",
		Detail: &cwmpxml.FaultDetail{
			CWMPFault: cwmpxml.CWMPFault{FaultCode: code, FaultString: msg},
		},
	}
}
