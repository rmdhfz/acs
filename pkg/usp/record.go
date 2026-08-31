package usp

import "encoding/json"

// Record merepresentasikan TR-369 USP Record
type Record struct {
	Version         string `json:"version"`
	ToID            string `json:"to_id"`
	FromID          string `json:"from_id"`
	PayloadSecurity string `json:"payload_security"`
	MacSignature    string `json:"mac_signature,omitempty"`
	SenderCert      string `json:"sender_cert,omitempty"`
	NoMAC           *NoMAC `json:"no_mac_signature,omitempty"`
}

type NoMAC struct {
	Payload json.RawMessage `json:"payload"`
}

// Msg merepresentasikan USP Message (isi dari NoMAC.Payload)
type Msg struct {
	Header *Header `json:"header"`
	Body   *Body   `json:"body"`
}

type Header struct {
	MsgID   string `json:"msg_id"`
	MsgType string `json:"msg_type"` // e.g., "CONNECT", "GET", "SET", "NOTIFY"
}

type Body struct {
	Request  *Request  `json:"request,omitempty"`
	Response *Response `json:"response,omitempty"`
}

type Request struct {
	Get    *Get    `json:"get,omitempty"`
	Set    *Set    `json:"set,omitempty"`
	Notify *Notify `json:"notify,omitempty"`
}

type Response struct {
	GetResp *GetResp `json:"get_resp,omitempty"`
	SetResp *SetResp `json:"set_resp,omitempty"`
}

// Structs untuk MsgType spesifik
type Get struct {
	ParamPaths []string `json:"param_paths"`
}

type GetResp struct {
	ReqParamPathResults []Result `json:"req_path_results"`
}

type Set struct {
	AllowPartial bool     `json:"allow_partial"`
	UpdatePBs    []Update `json:"update_pbs"`
}

type SetResp struct {
	// Disederhanakan untuk mockup
}

type Notify struct {
	SubscriptionID string `json:"subscription_id"`
	SendResp       bool   `json:"send_resp"`
}

type Result struct {
	RequestedPath string            `json:"requested_path"`
	ErrCode       uint32            `json:"err_code"`
	ErrMsg        string            `json:"err_msg"`
	ResolvedPaths map[string]string `json:"resolved_path_results"`
}

type Update struct {
	ObjPath string `json:"obj_path"`
	// dll
}
