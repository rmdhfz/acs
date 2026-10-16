package diagnostics

import (
	"errors"
	"testing"

	"acs/internal/domain"
)

func TestBuildTriggerValues(t *testing.T) {
	v, err := BuildTriggerValues(domain.DiagnosticTypePing, map[string]string{"host": "8.8.8.8"})
	if err != nil || v["diag.ping.state"] != "Requested" || v["diag.ping.host"] != "8.8.8.8" || v["diag.ping.repetitions"] != "4" {
		t.Fatalf("default ping: %v %v", v, err)
	}
	v, err = BuildTriggerValues(domain.DiagnosticTypeTraceroute, map[string]string{"host": "example.com", "max_hops": "10", "tries": "2"})
	if err != nil || v["diag.traceroute.max_hops"] != "10" || v["diag.traceroute.tries"] != "2" {
		t.Fatalf("traceroute: %v %v", v, err)
	}
	for _, bad := range []map[string]string{
		{}, {"host": ""}, {"host": "bad host"}, {"host": "-oProxy=x"}, {"host": "a;b"},
		{"host": "1.1.1.1", "repetitions": "0"}, {"host": "1.1.1.1", "repetitions": "999"}, {"host": "1.1.1.1", "timeout": "abc"},
	} {
		if _, err := BuildTriggerValues(domain.DiagnosticTypePing, bad); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("harus ditolak: %v → %v", bad, err)
		}
	}
	if _, err := BuildTriggerValues(domain.DiagnosticTypeWiFiScan, map[string]string{"host": "1.1.1.1"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("tipe belum didukung harus ErrInvalidInput: %v", err)
	}
}

func TestSummarize(t *testing.T) {
	st, res := Summarize(domain.DiagnosticTypePing, map[string]string{
		"Device.IP.Diagnostics.IPPing.DiagnosticsState":    "Complete",
		"Device.IP.Diagnostics.IPPing.SuccessCount":        "4",
		"Device.IP.Diagnostics.IPPing.FailureCount":        "0",
		"Device.IP.Diagnostics.IPPing.AverageResponseTime": "12",
	})
	if st != domain.DiagnosticStatusCompleted || res["state"] != "Complete" || res["summary"].(map[string]any)["success_count"] != 4 || res["summary"].(map[string]any)["avg_rtt_ms"] != 12 {
		t.Fatalf("ping: %s %v", st, res)
	}
	st, res = Summarize(domain.DiagnosticTypeTraceroute, map[string]string{
		"InternetGatewayDevice.TraceRouteDiagnostics.DiagnosticsState":        "Complete",
		"InternetGatewayDevice.TraceRouteDiagnostics.ResponseTime":            "15",
		"InternetGatewayDevice.TraceRouteDiagnostics.RouteHops.2.HostAddress": "203.0.113.50",
		"InternetGatewayDevice.TraceRouteDiagnostics.RouteHops.1.HostAddress": "10.0.0.1",
		"InternetGatewayDevice.TraceRouteDiagnostics.RouteHops.1.Host":        "gw",
	})
	hops := res["hops"].([]map[string]string)
	if st != domain.DiagnosticStatusCompleted || len(hops) != 2 || hops[0]["address"] != "10.0.0.1" || hops[1]["address"] != "203.0.113.50" {
		t.Fatalf("traceroute: %s %v", st, res)
	}
	if st, _ := Summarize(domain.DiagnosticTypePing, map[string]string{"Device.IP.Diagnostics.IPPing.DiagnosticsState": "Requested"}); st != domain.DiagnosticStatusRunning {
		t.Errorf("Requested harus tetap RUNNING, got %s", st)
	}
	if st, _ := Summarize(domain.DiagnosticTypePing, map[string]string{"Device.IP.Diagnostics.IPPing.DiagnosticsState": "Error_CannotResolveHostName"}); st != domain.DiagnosticStatusFailed {
		t.Errorf("Error_* harus FAILED, got %s", st)
	}
}
