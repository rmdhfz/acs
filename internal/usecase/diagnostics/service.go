// Package diagnostics menangani diagnostik TR-069 standar yang dijalankan CPE (Ping &
// Traceroute): memicu lewat SetParameterValues, menunggu event "8 DIAGNOSTICS COMPLETE",
// mengambil hasil lewat GetParameterValues, lalu menyimpannya sebagai ringkasan terstruktur
// (FR-24). Path parameter TIDAK ditulis di kode: semuanya logical key `diag.*` di
// vendor_parameter_mappings (migrations/0025).
package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"acs/internal/domain"
)

type Service struct {
	diagnostics domain.DeviceDiagnosticRepository
	devices     domain.DeviceRepository
	tasks       domain.TaskCreator
	activity    domain.ActivityLogRepository
}

func NewService(
	diagnostics domain.DeviceDiagnosticRepository,
	devices domain.DeviceRepository,
	tasks domain.TaskCreator,
	activity domain.ActivityLogRepository,
) *Service {
	return &Service{diagnostics: diagnostics, devices: devices, tasks: tasks, activity: activity}
}

// Batas waktu diagnostik yang menggantung (CPE tak pernah mengirim event 8 / hasil).
const StaleAfter = 15 * time.Minute

// Jeda sebelum poll cadangan hasil (sebagian CPE tidak mengirim event 8 dengan andal).
const fallbackPollDelay = 90 * time.Second

var hostRe = regexp.MustCompile(`^[A-Za-z0-9._:\-]{1,255}$`)

// spec — logical key per tipe diagnostik.
type spec struct {
	prefix     string   // awalan logical key
	resultKeys []string // yang dibaca setelah selesai (termasuk state)
}

var specs = map[string]spec{
	domain.DiagnosticTypePing: {prefix: "diag.ping.", resultKeys: []string{
		"diag.ping.state", "diag.ping.success_count", "diag.ping.failure_count",
		"diag.ping.avg_rtt", "diag.ping.min_rtt", "diag.ping.max_rtt"}},
	domain.DiagnosticTypeTraceroute: {prefix: "diag.traceroute.", resultKeys: []string{
		"diag.traceroute.state", "diag.traceroute.response_time", "diag.traceroute.hops"}},
}

// Supported — tipe yang sudah punya alur lengkap.
func Supported(t string) bool { _, ok := specs[t]; return ok }

func intParam(params map[string]string, key string, def, min, max int) (string, error) {
	raw, ok := params[key]
	if !ok || strings.TrimSpace(raw) == "" {
		return strconv.Itoa(def), nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < min || n > max {
		return "", fmt.Errorf("%w: parameter %s harus bilangan bulat %d–%d", domain.ErrInvalidInput, key, min, max)
	}
	return strconv.Itoa(n), nil
}

// BuildTriggerValues memvalidasi masukan & membentuk pasangan logical key → nilai untuk
// SetParameterValues pemicu diagnostik.
func BuildTriggerValues(diagnosticType string, params map[string]string) (map[string]string, error) {
	if !Supported(diagnosticType) {
		return nil, fmt.Errorf("%w: diagnostic_type %q belum didukung (tersedia: PING, TRACEROUTE)", domain.ErrInvalidInput, diagnosticType)
	}
	host := strings.TrimSpace(params["host"])
	if host == "" || !hostRe.MatchString(host) || strings.HasPrefix(host, "-") {
		return nil, fmt.Errorf("%w: parameters.host wajib berupa hostname/IP yang valid (maks. 255 karakter)", domain.ErrInvalidInput)
	}
	timeout, err := intParam(params, "timeout", 5000, 100, 60000)
	if err != nil {
		return nil, err
	}
	block, err := intParam(params, "block_size", 64, 1, 65535)
	if err != nil {
		return nil, err
	}
	switch diagnosticType {
	case domain.DiagnosticTypePing:
		reps, err := intParam(params, "repetitions", 4, 1, 20)
		if err != nil {
			return nil, err
		}
		return map[string]string{
			"diag.ping.state": "Requested", "diag.ping.host": host, "diag.ping.repetitions": reps,
			"diag.ping.timeout": timeout, "diag.ping.block_size": block,
		}, nil
	default: // TRACEROUTE
		tries, err := intParam(params, "tries", 3, 1, 10)
		if err != nil {
			return nil, err
		}
		hops, err := intParam(params, "max_hops", 30, 1, 64)
		if err != nil {
			return nil, err
		}
		return map[string]string{
			"diag.traceroute.state": "Requested", "diag.traceroute.host": host, "diag.traceroute.tries": tries,
			"diag.traceroute.timeout": timeout, "diag.traceroute.block_size": block, "diag.traceroute.max_hops": hops,
		}, nil
	}
}

// Trigger mengantre SetParameterValues pemicu (nilai awal DiagnosticsState=Requested) dan
// mencatat DeviceDiagnostic berstatus PENDING. Hasil final datang asinkron (event 8).
func (s *Service) Trigger(ctx context.Context, actor domain.Actor, deviceID uint64, diagnosticType string, params map[string]string) (*domain.DeviceDiagnostic, error) {
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if !actor.IsSuperadmin() {
		if dev.TenantID == nil || actor.TenantID == nil || *dev.TenantID != *actor.TenantID {
			return nil, domain.ErrForbidden
		}
	}
	values, err := BuildTriggerValues(diagnosticType, params)
	if err != nil {
		return nil, err
	}
	t, err := s.tasks.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID:   deviceID,
		TaskType:   domain.TaskTypeSetParameterValues,
		Priority:   4,
		Parameters: map[string]interface{}{"values": values}, // logical key di-resolve oleh usecase/task
	})
	if err != nil {
		return nil, err
	}
	d := &domain.DeviceDiagnostic{
		DeviceID: deviceID, TaskID: &t.ID, DiagnosticType: diagnosticType, Status: domain.DiagnosticStatusPending,
	}
	if err := s.diagnostics.Create(ctx, d); err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "TRIGGER_DIAGNOSTIC", EntityType: "device_diagnostic", EntityID: &d.ID,
	})
	return d, nil
}

// requestResult mengantre GetParameterValues hasil (opsional tertunda) dan mengarahkan
// diagnostik ke task itu.
func (s *Service) requestResult(ctx context.Context, d *domain.DeviceDiagnostic, at *time.Time) error {
	sp, ok := specs[d.DiagnosticType]
	if !ok {
		return nil
	}
	sys := domain.Actor{Roles: []string{domain.RoleSuperadmin}}
	t, err := s.tasks.CreateTask(ctx, sys, domain.CreateTaskInput{
		DeviceID:    d.DeviceID,
		TaskType:    domain.TaskTypeGetParameterValues,
		Priority:    4,
		Parameters:  map[string]interface{}{"names": sp.resultKeys},
		ScheduledAt: at,
	})
	if err != nil {
		return err
	}
	return s.diagnostics.UpdateTask(ctx, d.ID, t.ID, domain.DiagnosticStatusRunning)
}

// OnDiagnosticsComplete dipanggil saat Inform membawa event "8 DIAGNOSTICS COMPLETE":
// ambil hasil untuk semua diagnostik terbuka milik device.
func (s *Service) OnDiagnosticsComplete(ctx context.Context, deviceID uint64) error {
	open, err := s.diagnostics.ListOpenByDevice(ctx, deviceID)
	if err != nil {
		return err
	}
	for i := range open {
		if err := s.requestResult(ctx, &open[i], nil); err != nil {
			return err
		}
	}
	return nil
}

// HandleResult menerima akhir sebuah task yang terkait diagnostik (bukan GetParameterValues):
//   - sukses pada fase pemicu (PENDING) → RUNNING + poll cadangan hasil (CPE bisa lupa event 8);
//   - gagal → FAILED.
func (s *Service) HandleResult(ctx context.Context, taskID uint64, _ domain.JSONRawMessage, success bool) error {
	d, err := s.diagnostics.GetByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	if !success {
		return s.diagnostics.UpdateResult(ctx, d.ID, domain.DiagnosticStatusFailed,
			domain.JSONRawMessage(`{"error":"task diagnostik gagal dieksekusi perangkat"}`), time.Now())
	}
	if d.Status == domain.DiagnosticStatusPending {
		at := time.Now().Add(fallbackPollDelay)
		if err := s.diagnostics.UpdateStatus(ctx, d.ID, domain.DiagnosticStatusRunning); err != nil {
			return err
		}
		return s.requestResult(ctx, d, &at)
	}
	return nil
}

// HandleParameterValues memproses hasil GetParameterValues diagnostik.
func (s *Service) HandleParameterValues(ctx context.Context, taskID uint64, values map[string]string) error {
	d, err := s.diagnostics.GetByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	status, result := Summarize(d.DiagnosticType, values)
	if status == domain.DiagnosticStatusRunning {
		return nil // CPE belum selesai (state Requested/None); tunggu event 8 / sweeper
	}
	b, _ := json.Marshal(result)
	return s.diagnostics.UpdateResult(ctx, d.ID, status, domain.JSONRawMessage(b), time.Now())
}

var hopRe = regexp.MustCompile(`\.RouteHops\.(\d+)\.(\w+)$`)

// Summarize mengubah nilai parameter mentah menjadi ringkasan terstruktur. Mengembalikan
// status (COMPLETED/FAILED/RUNNING bila belum selesai) dan hasil JSON-able.
func Summarize(diagnosticType string, values map[string]string) (string, map[string]any) {
	last := func(name string) string {
		if i := strings.LastIndex(name, "."); i >= 0 {
			return name[i+1:]
		}
		return name
	}
	state := ""
	summary := map[string]any{}
	hops := map[int]map[string]string{}
	for name, v := range values {
		if m := hopRe.FindStringSubmatch(name); m != nil {
			n, _ := strconv.Atoi(m[1])
			if hops[n] == nil {
				hops[n] = map[string]string{}
			}
			hops[n][m[2]] = v
			continue
		}
		switch last(name) {
		case "DiagnosticsState":
			state = v
		case "SuccessCount":
			summary["success_count"] = atoiOr(v)
		case "FailureCount":
			summary["failure_count"] = atoiOr(v)
		case "AverageResponseTime":
			summary["avg_rtt_ms"] = atoiOr(v)
		case "MinimumResponseTime":
			summary["min_rtt_ms"] = atoiOr(v)
		case "MaximumResponseTime":
			summary["max_rtt_ms"] = atoiOr(v)
		case "ResponseTime":
			summary["response_time_ms"] = atoiOr(v)
		}
	}
	result := map[string]any{"state": state, "summary": summary}
	if len(hops) > 0 {
		idx := make([]int, 0, len(hops))
		for n := range hops {
			idx = append(idx, n)
		}
		sort.Ints(idx)
		list := make([]map[string]string, 0, len(idx))
		for _, n := range idx {
			h := hops[n]
			list = append(list, map[string]string{"hop": strconv.Itoa(n), "host": h["Host"], "address": h["HostAddress"], "rtt": h["RTTimes"], "error_code": h["ErrorCode"]})
		}
		result["hops"] = list
	}
	switch {
	case state == "Complete":
		return domain.DiagnosticStatusCompleted, result
	case strings.HasPrefix(state, "Error"):
		return domain.DiagnosticStatusFailed, result
	default:
		return domain.DiagnosticStatusRunning, result
	}
}

func atoiOr(s string) any {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return s
}

// ExpireStale menandai diagnostik PENDING/RUNNING yang menggantung lebih dari StaleAfter.
func (s *Service) ExpireStale(ctx context.Context) (int64, error) {
	return s.diagnostics.TimeoutStale(ctx, time.Now().Add(-StaleAfter))
}

func (s *Service) Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.DeviceDiagnostic, error) {
	d, err := s.diagnostics.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	dev, err := s.devices.GetByID(ctx, d.DeviceID)
	if err != nil {
		return nil, err
	}
	if !actor.IsSuperadmin() {
		if dev.TenantID == nil || actor.TenantID == nil || *dev.TenantID != *actor.TenantID {
			return nil, domain.ErrForbidden
		}
	}
	return d, nil
}

func (s *Service) ListByDevice(ctx context.Context, actor domain.Actor, deviceID uint64, p domain.Pagination) ([]domain.DeviceDiagnostic, int, error) {
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return nil, 0, err
	}
	if !actor.IsSuperadmin() {
		if dev.TenantID == nil || actor.TenantID == nil || *dev.TenantID != *actor.TenantID {
			return nil, 0, domain.ErrForbidden
		}
	}
	return s.diagnostics.ListByDevice(ctx, deviceID, p)
}
