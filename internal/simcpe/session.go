package simcpe

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"acs/pkg/cwmpxml"
)

// SessionReport merangkum satu sesi CWMP simulasi untuk output & assertion.
type SessionReport struct {
	Vendor       string   `json:"vendor"`
	Serial       string   `json:"serial"`
	Events       []string `json:"events"`
	InformStatus int      `json:"inform_status"`
	RPCsReceived []string `json:"rpcs_received"`
	FaultsSent   []string `json:"faults_sent"`
	Transfers    int      `json:"transfers"`
	Turns        int      `json:"turns"`
	DurationMS   int64    `json:"duration_ms"`
	OK           bool     `json:"ok"`
	ErrMsg       string   `json:"err,omitempty"`
}

func (r SessionReport) String() string {
	status := "OK"
	if !r.OK {
		status = "GAGAL"
	}
	s := fmt.Sprintf("[%s] %-9s %-24s events=%v inform=%d turns=%d rpc=%v faults=%v xfer=%d %dms",
		status, r.Vendor, r.Serial, r.Events, r.InformStatus, r.Turns,
		r.RPCsReceived, r.FaultsSent, r.Transfers, r.DurationMS)
	if r.ErrMsg != "" {
		s += "  err=" + r.ErrMsg
	}
	return s
}

// CountRPC menghitung berapa kali suatu method RPC diterima dalam sesi ini.
func (r SessionReport) CountRPC(method string) int {
	n := 0
	for _, m := range r.RPCsReceived {
		if m == method {
			n++
		}
	}
	return n
}

// SessionOpts mengatur satu jalannya sesi.
type SessionOpts struct {
	ACSURL   string
	Username string
	Password string
	Events   []string // event code; kosong -> ["2 PERIODIC"]

	RetryCount int
	// FaultParamSubstr: bila SetParameterValues dari ACS memuat nama parameter
	// yang mengandung substring ini, simulator membalas Fault 9005.
	FaultParamSubstr string
	MaxTurns         int
	Verbose          bool
}

type pendingXfer struct {
	tc     *cwmpxml.TransferComplete
	newVer string
}

// RunSession menjalankan satu sesi CWMP penuh: Inform -> InformResponse ->
// loop RPC sampai ACS membalas 204. Data model device dimutasi sesuai RPC.
func RunSession(ctx context.Context, hc *http.Client, d *Device, o SessionOpts) SessionReport {
	if o.MaxTurns == 0 {
		o.MaxTurns = 60
	}
	events := o.Events
	if len(events) == 0 {
		events = []string{"2 PERIODIC"}
	}
	rep := SessionReport{Vendor: d.Profile.Key, Serial: d.SerialNumber, Events: events}
	start := time.Now()
	defer func() { rep.DurationMS = time.Since(start).Milliseconds() }()

	ns := d.Profile.Namespace
	evStructs := make([]cwmpxml.EventStruct, 0, len(events))
	for _, e := range events {
		evStructs = append(evStructs, cwmpxml.EventStruct{EventCode: e})
	}

	fail := func(format string, args ...any) SessionReport {
		rep.ErrMsg = fmt.Sprintf(format, args...)
		return rep
	}

	// --- 1. Inform ---
	informXML, err := cwmpxml.Marshal(buildInformEnvelope(d, ns, evStructs, o.RetryCount))
	if err != nil {
		return fail("marshal Inform: %v", err)
	}
	resp, err := doPost(ctx, hc, o, nil, informXML)
	if err != nil {
		return fail("kirim Inform: %v", err)
	}
	rep.InformStatus = resp.status
	if resp.status != http.StatusOK {
		return fail("Inform ditolak: HTTP %d %s", resp.status, snippet(resp.body))
	}
	cookies := resp.cookies
	if _, err := cwmpxml.Unmarshal(resp.body); err != nil {
		return fail("parse InformResponse: %v", err)
	}
	if o.Verbose {
		fmt.Printf("  <- InformResponse (%d cookie)\n", len(cookies))
	}

	// --- 2. Loop RPC ---
	var outBody []byte // nil = POST kosong
	var pending []pendingXfer

	for turn := 0; turn < o.MaxTurns; turn++ {
		rep.Turns = turn + 1
		resp, err := doPost(ctx, hc, o, cookies, outBody)
		if err != nil {
			return fail("turn %d: %v", turn, err)
		}
		if len(resp.cookies) > 0 {
			cookies = resp.cookies
		}

		idle := resp.status == http.StatusNoContent
		var env *cwmpxml.Envelope
		if !idle {
			if resp.status != http.StatusOK {
				return fail("turn %d: HTTP %d %s", turn, resp.status, snippet(resp.body))
			}
			env, err = cwmpxml.Unmarshal(resp.body)
			if err != nil {
				return fail("turn %d: parse envelope: %v", turn, err)
			}
			idle = env.IsEmpty()
		}

		if idle {
			if len(pending) > 0 {
				p := pending[0]
				pending = pending[1:]
				if p.newVer != "" {
					d.SetSoftwareVersion(p.newVer)
				}
				rep.Transfers++
				outBody, err = marshalBody(nextID(), ns, cwmpxml.Body{TransferComplete: p.tc})
				if err != nil {
					return fail("marshal TransferComplete: %v", err)
				}
				if o.Verbose {
					fmt.Printf("  -> TransferComplete (commandKey=%s)\n", p.tc.CommandKey)
				}
				continue
			}
			rep.OK = true
			return rep
		}

		method := env.Body.Method()
		rep.RPCsReceived = append(rep.RPCsReceived, method)
		if o.Verbose {
			fmt.Printf("  <- %s\n", method)
		}

		outcome := handleRPC(d, ns, env.Body, o.FaultParamSubstr)
		if outcome.fault != "" {
			rep.FaultsSent = append(rep.FaultsSent, outcome.fault)
		}
		if outcome.deferredXfer != nil {
			pending = append(pending, pendingXfer{tc: outcome.deferredXfer, newVer: outcome.newSoftwareVer})
		}
		outBody, err = marshalBody(headerIDOf(env), ns, outcome.respBody)
		if err != nil {
			return fail("marshal response %s: %v", method, err)
		}
	}
	return fail("sesi tidak selesai setelah %d turn (ACS terus mengirim RPC — reboot loop?)", o.MaxTurns)
}

func buildInformEnvelope(d *Device, ns string, events []cwmpxml.EventStruct, retryCount int) *cwmpxml.Envelope {
	var pl cwmpxml.ParameterValueList
	for _, nv := range d.InformParams() {
		pl.Values = append(pl.Values, cwmpxml.ParameterValue{
			Name: nv.Name, Value: cwmpxml.ValueType{Type: "xsd:string", Value: nv.Value},
		})
	}
	return cwmpxml.NewEnvelope(nextID(), ns, cwmpxml.Body{
		Inform: &cwmpxml.Inform{
			XMLName: cwmpxml.RPCName(ns, "Inform"),
			DeviceId: cwmpxml.DeviceIDStruct{
				Manufacturer: d.Profile.Manufacturer,
				OUI:          strings.ToUpper(d.Profile.OUI),
				ProductClass: d.Profile.ProductClass,
				SerialNumber: d.SerialNumber,
			},
			Event:         cwmpxml.EventList{Items: events},
			MaxEnvelopes:  1,
			CurrentTime:   time.Now().UTC().Format(time.RFC3339),
			RetryCount:    retryCount,
			ParameterList: pl,
		},
	})
}

func marshalBody(id, ns string, b cwmpxml.Body) ([]byte, error) {
	return cwmpxml.Marshal(cwmpxml.NewEnvelope(id, ns, b))
}

func headerIDOf(env *cwmpxml.Envelope) string {
	if env != nil && env.Header != nil && env.Header.ID != nil {
		return env.Header.ID.Value
	}
	return nextID()
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

// --- HTTP ---

type postResult struct {
	status  int
	body    []byte
	cookies []*http.Cookie
}

func doPost(ctx context.Context, hc *http.Client, o SessionOpts, cookies []*http.Cookie, body []byte) (postResult, error) {
	// Retry pada 429 dengan backoff — CPE nyata pun mundur & mencoba lagi saat
	// ACS menolak (rate limiter /cwmp 5 req/s per IP; di lapangan tiap CPE
	// punya IP sendiri, di e2e semua sesi dari satu IP).
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return postResult{}, ctx.Err()
			case <-time.After(time.Duration(attempt) * 700 * time.Millisecond):
			}
		}
		var r io.Reader
		if body != nil {
			r = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.ACSURL, r)
		if err != nil {
			return postResult{}, err
		}
		req.SetBasicAuth(o.Username, o.Password)
		if body != nil {
			req.Header.Set("Content-Type", "text/xml; charset=utf-8")
		}
		// Cookie sesi acs_session diteruskan manual: server men-set-nya dengan
		// Secure=true sehingga cookie jar standar tidak mengirimnya balik lewat
		// http:// (dev). CPE nyata di belakang TLS terminator tidak kena ini.
		for _, ck := range cookies {
			req.AddCookie(ck)
		}
		resp, err := hc.Do(req)
		if err != nil {
			return postResult{}, err
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 5 {
			continue
		}
		return postResult{status: resp.StatusCode, body: b, cookies: resp.Cookies()}, nil
	}
}
