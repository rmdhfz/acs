package session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"acs/internal/domain"
	"acs/internal/usecase/diagnostics"
	"acs/internal/usecase/firmware"
	"acs/internal/usecase/task"
)

// Test mesin sesi (NextRequest / HandleRPCResponse / fault / TransferComplete)
// memakai task.Service ASLI di atas repository in-memory — yang diuji adalah
// koordinasi sesi <-> antrean task (termasuk retry & max_retries, CLAUDE.md),
// bukan SQL. Jalur SQL-nya dicakup e2e (cmd/e2e S2-S5).

const (
	stPending, stQueued, stSent, stCompleted, stFailed, stCancelled, stTimeout = 1, 2, 3, 4, 5, 6, 7
	tyGetPV, tyReboot, tySetPV                                                 = 11, 12, 13
)

var statusIDs = map[string]uint64{
	domain.TaskStatusPending: stPending, domain.TaskStatusQueued: stQueued, domain.TaskStatusSent: stSent,
	domain.TaskStatusCompleted: stCompleted, domain.TaskStatusFailed: stFailed,
	domain.TaskStatusCancelled: stCancelled, domain.TaskStatusTimeout: stTimeout,
}

var typeIDs = map[string]uint64{
	domain.TaskTypeGetParameterValues: tyGetPV, domain.TaskTypeReboot: tyReboot, domain.TaskTypeSetParameterValues: tySetPV,
}

// ---- fakes ----

type engRefs struct{ domain.RefRepository }

func (engRefs) GetByCode(_ context.Context, table, code string) (domain.RefLookup, error) {
	switch table {
	case domain.RefTableTaskStatus:
		if id, ok := statusIDs[code]; ok {
			return domain.RefLookup{ID: id, Code: code}, nil
		}
	case domain.RefTableTaskTypes:
		if id, ok := typeIDs[code]; ok {
			return domain.RefLookup{ID: id, Code: code}, nil
		}
	}
	return domain.RefLookup{}, domain.ErrNotFound
}
func (engRefs) GetByID(_ context.Context, table string, id uint64) (domain.RefLookup, error) {
	m := statusIDs
	if table == domain.RefTableTaskTypes {
		m = typeIDs
	}
	for code, v := range m {
		if v == id {
			return domain.RefLookup{ID: id, Code: code}, nil
		}
	}
	return domain.RefLookup{}, domain.ErrNotFound
}

type engSessions struct {
	domain.DeviceSessionRepository
	byToken map[string]*domain.DeviceSession
	updated []string // "id:status"
}

func (r *engSessions) GetByToken(_ context.Context, tok string) (*domain.DeviceSession, error) {
	if s, ok := r.byToken[tok]; ok {
		return s, nil
	}
	return nil, domain.ErrNotFound
}
func (r *engSessions) UpdateStatus(_ context.Context, id uint64, status string, _ *time.Time) error {
	r.updated = append(r.updated, status)
	for _, s := range r.byToken {
		if s.ID == id {
			s.Status = status
		}
	}
	return nil
}

// snapshot meniru baris baru dari DB: mutasi lewat repository (IncrementRetry dst.)
// tidak boleh mengubah struct yang sudah dipegang service — kalau tidak,
// failOrRetry membaca RetryCount yang sudah bertambah dan hitungannya dobel.
func snapshot(t *domain.Task) *domain.Task { cp := *t; return &cp }

type engTasks struct {
	domain.TaskRepository
	byID    map[uint64]*domain.Task
	nextErr error
	retries map[uint64]int // jumlah IncrementRetry per task
}

func (r *engTasks) NextForDevice(_ context.Context, dev uint64) (*domain.Task, error) {
	if r.nextErr != nil {
		return nil, r.nextErr
	}
	for _, t := range r.byID {
		if t.DeviceID == dev && t.TaskStatusID == stPending {
			return snapshot(t), nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *engTasks) GetSentForDevice(_ context.Context, dev uint64) (*domain.Task, error) {
	for _, t := range r.byID {
		if t.DeviceID == dev && t.TaskStatusID == stSent {
			return snapshot(t), nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *engTasks) GetByUUID(_ context.Context, uuid string) (*domain.Task, error) {
	for _, t := range r.byID {
		if t.TaskUUID == uuid {
			return snapshot(t), nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *engTasks) GetByID(_ context.Context, id uint64) (*domain.Task, error) {
	if t, ok := r.byID[id]; ok {
		return snapshot(t), nil
	}
	return nil, domain.ErrNotFound
}
func (r *engTasks) MarkSent(_ context.Context, id uint64, _ time.Time) error {
	r.byID[id].TaskStatusID = stSent
	return nil
}
func (r *engTasks) MarkCompleted(_ context.Context, id uint64, resp domain.JSONRawMessage, _ time.Time) error {
	r.byID[id].TaskStatusID = stCompleted
	r.byID[id].Response = resp
	return nil
}
func (r *engTasks) MarkFailed(_ context.Context, id, statusID uint64, msg string) error {
	r.byID[id].TaskStatusID = statusID
	r.byID[id].ErrorMessage = &msg
	return nil
}
func (r *engTasks) Reschedule(_ context.Context, id uint64, at time.Time) error {
	r.byID[id].ScheduledAt = &at
	return nil
}
func (r *engTasks) IncrementRetry(_ context.Context, id uint64) error {
	r.byID[id].RetryCount++
	r.retries[id]++
	return nil
}
func (r *engTasks) SetErrorMessage(_ context.Context, id uint64, msg string) error {
	r.byID[id].ErrorMessage = &msg
	return nil
}
func (r *engTasks) UpdateStatus(_ context.Context, id, statusID uint64, _ *uint64) error {
	r.byID[id].TaskStatusID = statusID
	return nil
}

type engParams struct {
	domain.DeviceParameterRepository
	upserted []domain.DeviceParameter
}

func (r *engParams) UpsertBatch(_ context.Context, ps []domain.DeviceParameter) error {
	r.upserted = append(r.upserted, ps...)
	return nil
}

type engDiag struct {
	domain.DeviceDiagnosticRepository
}

func (engDiag) GetByTaskID(context.Context, uint64) (*domain.DeviceDiagnostic, error) {
	return nil, domain.ErrNotFound
}

type engJobs struct {
	domain.FirmwareUpgradeJobRepository
}

func (engJobs) GetByTaskID(context.Context, uint64) (*domain.FirmwareUpgradeJob, error) {
	return nil, domain.ErrNotFound
}

type engRig struct {
	svc      *Service
	sessions *engSessions
	tasks    *engTasks
	params   *engParams
}

const (
	devID   = uint64(77)
	sessTok = "sess-token-1"
)

func newEngRig() *engRig {
	ns := "urn:dslforum-org:cwmp-1-0"
	sessions := &engSessions{byToken: map[string]*domain.DeviceSession{
		sessTok: {ID: 1, DeviceID: devID, SessionToken: sessTok, Status: domain.SessionStatusOpen, CWMPNamespace: &ns},
	}}
	tasks := &engTasks{byID: map[uint64]*domain.Task{}, retries: map[uint64]int{}}
	params := &engParams{}
	refs := engRefs{}
	taskSvc := task.NewService(tasks, nil, params, nil, nil, refs, nil, nil, nil, nil)
	fwSvc := firmware.NewService(nil, engJobs{}, nil, nil, nil, refs, nil, nil, nil)
	diagSvc := diagnostics.NewService(engDiag{}, nil, nil, nil)
	return &engRig{
		svc: &Service{sessions: sessions, taskSvc: taskSvc, refs: refs, deviceParams: params,
			firmwareSvc: fwSvc, diagnosticsSvc: diagSvc},
		sessions: sessions, tasks: tasks, params: params,
	}
}

func (r *engRig) addTask(id uint64, typeID, statusID uint64, maxRetries uint32) *domain.Task {
	t := &domain.Task{ID: id, TaskUUID: "uuid-" + string(rune('a'+id)), DeviceID: devID, TaskTypeID: typeID,
		TaskStatusID: statusID, MaxRetries: maxRetries, Parameters: domain.JSONRawMessage(`{"x":1}`)}
	r.tasks.byID[id] = t
	return t
}

// ================== NextRequest ==================

func TestNextRequest(t *testing.T) {
	ctx := context.Background()

	t.Run("token tak dikenal -> error & tutup", func(t *testing.T) {
		r := newEngRig()
		rpc, closeS, err := r.svc.NextRequest(ctx, "tak-ada")
		if !errors.Is(err, domain.ErrNotFound) || !closeS || rpc != nil {
			t.Fatalf("rpc=%v close=%v err=%v", rpc, closeS, err)
		}
	})
	t.Run("sesi tidak OPEN -> tutup tanpa error", func(t *testing.T) {
		r := newEngRig()
		r.sessions.byToken[sessTok].Status = domain.SessionStatusClosed
		r.addTask(1, tyReboot, stPending, 3)
		rpc, closeS, err := r.svc.NextRequest(ctx, sessTok)
		if err != nil || !closeS || rpc != nil {
			t.Fatalf("rpc=%v close=%v err=%v", rpc, closeS, err)
		}
		if r.tasks.byID[1].TaskStatusID != stPending {
			t.Fatal("task tidak boleh disentuh pada sesi tertutup")
		}
	})
	t.Run("tanpa task -> sesi ditutup", func(t *testing.T) {
		r := newEngRig()
		rpc, closeS, err := r.svc.NextRequest(ctx, sessTok)
		if err != nil || !closeS || rpc != nil {
			t.Fatalf("rpc=%v close=%v err=%v", rpc, closeS, err)
		}
		if r.sessions.byToken[sessTok].Status != domain.SessionStatusClosed {
			t.Fatalf("status sesi = %q, ingin CLOSED", r.sessions.byToken[sessTok].Status)
		}
	})
	t.Run("task pending -> OutboundRPC lengkap & task jadi SENT", func(t *testing.T) {
		r := newEngRig()
		task1 := r.addTask(5, tyGetPV, stPending, 3)
		rpc, closeS, err := r.svc.NextRequest(ctx, sessTok)
		if err != nil || closeS || rpc == nil {
			t.Fatalf("rpc=%v close=%v err=%v", rpc, closeS, err)
		}
		if rpc.TaskID != 5 || rpc.TaskUUID != task1.TaskUUID || rpc.TaskType != domain.TaskTypeGetParameterValues ||
			rpc.Namespace != "urn:dslforum-org:cwmp-1-0" || string(rpc.Parameters) != `{"x":1}` {
			t.Fatalf("OutboundRPC salah: %+v", rpc)
		}
		if task1.TaskStatusID != stSent {
			t.Fatal("task harus SENT setelah dikirim (korelasi respons bergantung padanya)")
		}
		if r.sessions.byToken[sessTok].Status != domain.SessionStatusOpen {
			t.Fatal("sesi harus tetap OPEN selama masih ada task")
		}
	})
	t.Run("error antrean selain NotFound -> diteruskan", func(t *testing.T) {
		r := newEngRig()
		r.tasks.nextErr = errors.New("db down")
		_, closeS, err := r.svc.NextRequest(ctx, sessTok)
		if err == nil || !closeS {
			t.Fatalf("close=%v err=%v", closeS, err)
		}
	})
	t.Run("sesi lama tanpa namespace -> Namespace kosong (delivery yang fallback)", func(t *testing.T) {
		r := newEngRig()
		r.sessions.byToken[sessTok].CWMPNamespace = nil
		r.addTask(1, tyReboot, stPending, 3)
		rpc, _, err := r.svc.NextRequest(ctx, sessTok)
		if err != nil || rpc.Namespace != "" {
			t.Fatalf("rpc=%+v err=%v", rpc, err)
		}
	})
}

// ================== HandleRPCResponse ==================

func TestHandleRPCResponse_Normal(t *testing.T) {
	ctx := context.Background()

	t.Run("sesi tak dikenal -> error", func(t *testing.T) {
		r := newEngRig()
		if err := r.svc.HandleRPCResponse(ctx, RPCResponse{SessionToken: "x"}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("%v", err)
		}
	})
	t.Run("tanpa task SENT (mis. GetRPCMethods) diabaikan", func(t *testing.T) {
		r := newEngRig()
		r.addTask(1, tyReboot, stPending, 3)
		if err := r.svc.HandleRPCResponse(ctx, RPCResponse{SessionToken: sessTok, RawResponse: []byte(`{}`)}); err != nil {
			t.Fatal(err)
		}
		if r.tasks.byID[1].TaskStatusID != stPending {
			t.Fatal("task PENDING tidak boleh ikut selesai")
		}
	})
	t.Run("respons normal menyelesaikan task SENT & menyimpan respons", func(t *testing.T) {
		r := newEngRig()
		r.addTask(1, tySetPV, stSent, 3)
		if err := r.svc.HandleRPCResponse(ctx, RPCResponse{SessionToken: sessTok, RawResponse: []byte(`{"Status":0}`)}); err != nil {
			t.Fatal(err)
		}
		if tk := r.tasks.byID[1]; tk.TaskStatusID != stCompleted || string(tk.Response) != `{"Status":0}` {
			t.Fatalf("%+v", tk)
		}
	})
	t.Run("SetParameterValues sukses mencatat nilai yang diterapkan ke device_parameters", func(t *testing.T) {
		r := newEngRig()
		tk := r.addTask(1, tySetPV, stSent, 3)
		tk.Parameters = domain.JSONRawMessage(`{"values":{"A.B":"baru","C.D":"2"}}`)
		if err := r.svc.HandleRPCResponse(ctx, RPCResponse{SessionToken: sessTok, RawResponse: []byte(`{"Status":0}`)}); err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, p := range r.params.upserted {
			if p.ParameterValue != nil {
				got[p.ParameterName] = *p.ParameterValue
			}
		}
		if len(got) != 2 || got["A.B"] != "baru" || got["C.D"] != "2" {
			t.Fatalf("nilai yang diterapkan tidak tercatat: %v", got)
		}
	})
	t.Run("nilai parameter disimpan ke device_parameters", func(t *testing.T) {
		r := newEngRig()
		r.addTask(1, tyGetPV, stSent, 3)
		err := r.svc.HandleRPCResponse(ctx, RPCResponse{SessionToken: sessTok, RawResponse: []byte(`{}`),
			ParameterValues: []InformParameter{{Name: "A.B", Value: "1"}, {Name: "C.D", Value: ""}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.params.upserted) != 2 {
			t.Fatalf("%d parameter tersimpan", len(r.params.upserted))
		}
		p := r.params.upserted[0]
		if p.DeviceID != devID || p.ParameterName != "A.B" || p.ParameterValue == nil || *p.ParameterValue != "1" {
			t.Fatalf("%+v", p)
		}
	})
}

// ================== Fault & retry ==================

func TestHandleFault_9005GagalPermanenTanpaRetry(t *testing.T) {
	r := newEngRig()
	tk := r.addTask(1, tySetPV, stSent, 5) // max_retries 5 — tetap tidak boleh di-retry
	err := r.svc.HandleRPCResponse(context.Background(), RPCResponse{SessionToken: sessTok,
		Fault: &FaultInfo{Code: "9005", Message: "Invalid parameter name"}})
	if err != nil {
		t.Fatal(err)
	}
	if tk.TaskStatusID != stFailed {
		t.Fatalf("9005 harus FAILED permanen, status=%d", tk.TaskStatusID)
	}
	if r.tasks.retries[1] != 1 {
		t.Fatalf("hanya satu percobaan yang dicatat, dapat %d", r.tasks.retries[1])
	}
	if tk.ErrorMessage == nil || !strings.Contains(*tk.ErrorMessage, "9005") || !strings.Contains(*tk.ErrorMessage, "tidak didukung") {
		t.Fatalf("pesan galat harus memuat kode & penjelasan: %v", tk.ErrorMessage)
	}
}

// Skenario retry + max_retries tercapai (CLAUDE.md: wajib diuji, bukan hanya happy path).
func TestHandleFault_RetryLaluGagalSetelahMaxRetries(t *testing.T) {
	r := newEngRig()
	tk := r.addTask(1, tySetPV, stSent, 3)
	fault := RPCResponse{SessionToken: sessTok, Fault: &FaultInfo{Code: "9002", Message: "Internal error"}}

	wantAfter := []uint64{stPending, stPending, stFailed} // percobaan 1,2 -> retry; 3 -> max tercapai
	for i, want := range wantAfter {
		tk.TaskStatusID = stSent // sesi berikutnya: task dikirim lagi
		if err := r.svc.HandleRPCResponse(context.Background(), fault); err != nil {
			t.Fatalf("percobaan %d: %v", i+1, err)
		}
		if tk.TaskStatusID != want {
			t.Fatalf("setelah fault ke-%d: status=%d, ingin %d (retry_count=%d, max=%d)", i+1, tk.TaskStatusID, want, tk.RetryCount, tk.MaxRetries)
		}
	}
	if tk.RetryCount != 3 {
		t.Fatalf("retry_count = %d, ingin 3", tk.RetryCount)
	}
	if tk.ErrorMessage == nil || !strings.Contains(*tk.ErrorMessage, "[9002] Internal error") {
		t.Fatalf("pesan galat: %v", tk.ErrorMessage)
	}
}

func TestHandleFault_9003BukanGetParameterValuesTetapRetryBiasa(t *testing.T) {
	r := newEngRig()
	tk := r.addTask(1, tyReboot, stSent, 3)
	if err := r.svc.HandleRPCResponse(context.Background(), RPCResponse{SessionToken: sessTok,
		Fault: &FaultInfo{Code: "9003", Message: "Invalid arguments"}}); err != nil {
		t.Fatal(err)
	}
	if tk.TaskStatusID != stPending || tk.RetryCount != 1 {
		t.Fatalf("9003 pada task selain GetParameterValues harus retry biasa: status=%d retry=%d", tk.TaskStatusID, tk.RetryCount)
	}
}

func TestHandleFault_9003GetParameterValuesSatuNamaTidakDipecah(t *testing.T) {
	r := newEngRig()
	tk := r.addTask(1, tyGetPV, stSent, 3)
	tk.Parameters = domain.JSONRawMessage(`{"names":["A.B"]}`) // <=1 nama: tak bisa dipecah
	if err := r.svc.HandleRPCResponse(context.Background(), RPCResponse{SessionToken: sessTok,
		Fault: &FaultInfo{Code: "9003", Message: "Invalid arguments"}}); err != nil {
		t.Fatal(err)
	}
	if tk.TaskStatusID != stPending || tk.RetryCount != 1 {
		t.Fatalf("jatuh ke retry generik: status=%d retry=%d", tk.TaskStatusID, tk.RetryCount)
	}
	if len(r.tasks.byID) != 1 {
		t.Fatalf("tidak boleh membuat task baru, total=%d", len(r.tasks.byID))
	}
}

func TestHandleFault_TanpaTaskSentDiabaikan(t *testing.T) {
	r := newEngRig()
	if err := r.svc.HandleRPCResponse(context.Background(), RPCResponse{SessionToken: sessTok,
		Fault: &FaultInfo{Code: "9002", Message: "x"}}); err != nil {
		t.Fatalf("fault tanpa task SENT bukan error: %v", err)
	}
}

func TestFaultTanpaKodeMemakaiPesanApaAdanya(t *testing.T) {
	r := newEngRig()
	tk := r.addTask(1, tySetPV, stSent, 1) // max 1 -> langsung FAILED
	if err := r.svc.HandleRPCResponse(context.Background(), RPCResponse{SessionToken: sessTok,
		Fault: &FaultInfo{Code: "", Message: "Internal"}}); err != nil {
		t.Fatal(err)
	}
	if tk.TaskStatusID != stFailed || tk.ErrorMessage == nil || *tk.ErrorMessage != "Internal" {
		t.Fatalf("%+v msg=%v", tk, tk.ErrorMessage)
	}
}

// ================== TransferComplete ==================

func TestHandleTransferComplete(t *testing.T) {
	ctx := context.Background()

	t.Run("sukses (korelasi lewat CommandKey = task uuid) menyelesaikan task", func(t *testing.T) {
		r := newEngRig()
		tk := r.addTask(3, tySetPV, stSent, 3)
		err := r.svc.HandleRPCResponse(ctx, RPCResponse{SessionToken: sessTok,
			TransferComplete: &TransferCompleteInfo{CommandKey: tk.TaskUUID, Success: true, StartTime: "s", CompleteTime: "c"}})
		if err != nil {
			t.Fatal(err)
		}
		if tk.TaskStatusID != stCompleted || !strings.Contains(string(tk.Response), `"complete_time":"c"`) {
			t.Fatalf("status=%d resp=%s", tk.TaskStatusID, tk.Response)
		}
	})
	t.Run("sukses tanpa CommandKey memakai task SENT milik device", func(t *testing.T) {
		r := newEngRig()
		tk := r.addTask(3, tySetPV, stSent, 3)
		if err := r.svc.HandleRPCResponse(ctx, RPCResponse{SessionToken: sessTok,
			TransferComplete: &TransferCompleteInfo{Success: true}}); err != nil {
			t.Fatal(err)
		}
		if tk.TaskStatusID != stCompleted {
			t.Fatalf("status=%d", tk.TaskStatusID)
		}
	})
	t.Run("gagal masuk alur retry/max_retries", func(t *testing.T) {
		r := newEngRig()
		tk := r.addTask(3, tySetPV, stSent, 1)
		if err := r.svc.HandleRPCResponse(ctx, RPCResponse{SessionToken: sessTok,
			TransferComplete: &TransferCompleteInfo{CommandKey: tk.TaskUUID, Success: false, ErrorMessage: "Download failure"}}); err != nil {
			t.Fatal(err)
		}
		if tk.TaskStatusID != stFailed || tk.ErrorMessage == nil || *tk.ErrorMessage != "Download failure" {
			t.Fatalf("status=%d msg=%v", tk.TaskStatusID, tk.ErrorMessage)
		}
	})
	t.Run("tidak ada task yang cocok diabaikan", func(t *testing.T) {
		r := newEngRig()
		if err := r.svc.HandleRPCResponse(ctx, RPCResponse{SessionToken: sessTok,
			TransferComplete: &TransferCompleteInfo{CommandKey: "tak-ada", Success: true}}); err != nil {
			t.Fatalf("TransferComplete tanpa task bukan error: %v", err)
		}
	})
}
