// Package task mengorkestrasi antrean RPC CWMP (lihat TECH.md §4) — enqueue,
// dequeue per device (dipanggil dari usecase/session), retry dengan
// max_retries, dan resolusi logical key -> raw TR-069 path (TECH.md §5).
package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
)

type Service struct {
	tasks         domain.TaskRepository
	devices       domain.DeviceRepository
	deviceParams  domain.DeviceParameterRepository // Ditambahkan untuk auto-discovery worker
	deviceModels  domain.DeviceModelRepository
	paramMappings domain.VendorParameterMappingRepository
	refs          domain.RefRepository
	activity      domain.ActivityLogRepository
	// tenants — dipakai HANYA untuk resolve max_pending_tasks tenant pemilik
	// device saat CreateTask (kuota task queue per tenant, ROADMAP.md Fase 2).
	// Boleh nil di test unit yang tidak menyentuh CreateTask (lihat service_test.go).
	tenants domain.TenantRepository
	// webhookEnq — fan-out event TASK_FAILED (migrations/0013). Boleh nil;
	// selalu lewat notifyTaskFailed() yang nil-safe & tidak menggagalkan alur.
	webhookEnq domain.WebhookEnqueuer
	publisher  domain.EventPublisher
}

func NewService(
	tasks domain.TaskRepository,
	devices domain.DeviceRepository,
	deviceParams domain.DeviceParameterRepository,
	deviceModels domain.DeviceModelRepository,
	paramMappings domain.VendorParameterMappingRepository,
	refs domain.RefRepository,
	activity domain.ActivityLogRepository,
	tenants domain.TenantRepository,
	webhookEnq domain.WebhookEnqueuer,
	publisher domain.EventPublisher,
) *Service {
	return &Service{
		tasks: tasks, devices: devices, deviceParams: deviceParams, deviceModels: deviceModels,
		paramMappings: paramMappings, refs: refs, activity: activity,
		tenants: tenants, webhookEnq: webhookEnq, publisher: publisher,
	}
}

// notifyTaskFailed mem-fan-out webhook TASK_FAILED (migrations/0013) untuk
// task yang mencapai status FAILED terminal. Nil-safe & best-effort — tidak
// pernah menggagalkan alur task queue.
func (s *Service) notifyTaskFailed(ctx context.Context, t *domain.Task, errMsg string) {
	if s.webhookEnq == nil {
		return
	}
	var tenantID *uint64
	if s.devices != nil {
		if dev, err := s.devices.GetByID(ctx, t.DeviceID); err == nil {
			tenantID = dev.TenantID
		}
	}
	_ = s.webhookEnq.Enqueue(ctx, domain.WebhookEventTaskFailed, tenantID, map[string]any{
		"task_id":       t.ID,
		"task_uuid":     t.TaskUUID,
		"device_id":     t.DeviceID,
		"task_type_id":  t.TaskTypeID,
		"error_message": errMsg,
	})
}

// ResolveParameterPath menerjemahkan logical key (mis. "wifi.5g.ssid") ke raw
// path TR-069 sesuai vendor/model device. Raw path yang sudah eksplisit
// (diawali root object TR-098/TR-181) diteruskan apa adanya (FR-12).
func (s *Service) ResolveParameterPath(ctx context.Context, deviceID uint64, key string) (string, error) {
	if looksLikeRawPath(key) {
		return key, nil
	}
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return "", err
	}
	if dev.VendorID == nil {
		return "", fmt.Errorf("task: device belum diketahui vendor-nya, tidak bisa resolve logical key %q", key)
	}
	if dev.DeviceModelID == nil {
		return "", fmt.Errorf("task: device belum diketahui model-nya, tidak bisa resolve logical key %q", key)
	}
	dm, err := s.deviceModels.GetByID(ctx, *dev.DeviceModelID)
	if err != nil {
		return "", err
	}
	m, err := s.paramMappings.Resolve(ctx, *dev.VendorID, dm.DataModelVersionID, dev.DeviceModelID, key, dev.SoftwareVersion)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", fmt.Errorf("task: tidak ada vendor_parameter_mappings untuk logical key %q pada device ini (kirim raw TR-069 path bila belum dipetakan, lihat FR-12)", key)
		}
		return "", err
	}
	return m.TR069Path, nil
}

func looksLikeRawPath(key string) bool {
	return strings.HasPrefix(key, "InternetGatewayDevice.") || strings.HasPrefix(key, "Device.")
}

// MaxGetParameterValuesNamesPerTask adalah ambang batas jumlah parameter
// name dalam SATU task GET_PARAMETER_VALUES sebelum proaktif dipecah jadi
// beberapa task berurutan saat task tsb DIBUAT (bukan menunggu CPE menolak
// lewat cwmp:Fault 9003 "Invalid Arguments" — lihat penanganan REAKTIF di
// SplitGetParameterValuesOnFault). Banyak implementasi CPE membatasi ukuran
// request SOAP/jumlah parameter per RPC — ini kuirk umum lintas vendor
// (bukan satu vendor spesifik), jadi wajar sbg default di layer task,
// bukan internal/vendor_adapter/ (CLAUDE.md). Nilai 50 dipilih sbg default
// yang aman utk mayoritas implementasi CPE tanpa riset per-vendor spesifik;
// task yang dihasilkan tetap diproses satu-per-satu sesuai priority/
// created_at seperti task lain manapun (TECH.md §3/§4: satu task in-flight
// per device per sesi, BUKAN mekanisme paralel khusus).
const MaxGetParameterValuesNamesPerTask = 50

func (s *Service) CreateTask(ctx context.Context, actor domain.Actor, in domain.CreateTaskInput) (*domain.Task, error) {
	dev, err := s.devices.GetByID(ctx, in.DeviceID)
	if err != nil {
		return nil, err
	}
	if !actor.IsSuperadmin() {
		if dev.TenantID == nil || actor.TenantID == nil || *dev.TenantID != *actor.TenantID {
			return nil, domain.ErrForbidden
		}
	}
	taskType, err := s.refs.GetByCode(ctx, domain.RefTableTaskTypes, in.TaskType)
	if err != nil {
		return nil, fmt.Errorf("task: tipe task tidak dikenal %q: %w", in.TaskType, err)
	}

	// Proactive chunking (lihat komentar MaxGetParameterValuesNamesPerTask) —
	// dicek SEBELUM kuota/status pending & SEBELUM baris Task tunggal
	// dibentuk, supaya satu request oversized otomatis jadi beberapa task
	// berurutan. Tiap chunk melalui CreateTask normal secara rekursif
	// (quota/RBAC/audit tetap ditegakkan per baris task, konsisten dgn task
	// manapun) — trade-off yang disadari: kalau kuota tenant habis di
	// tengah proses chunking, sebagian chunk bisa sudah terlanjur dibuat
	// sebelum error dikembalikan (sama semangatnya dgn catatan TOCTOU di
	// enforceTenantTaskQuota, bukan boundary keamanan, jadi diterima apa
	// adanya alih-alih menambah transaksi lintas-row demi ini).
	if taskType.Code == domain.TaskTypeGetParameterValues {
		if names, ok := extractParameterNames(in.Parameters); ok && len(names) > MaxGetParameterValuesNamesPerTask {
			return s.createChunkedGetParameterValues(ctx, actor, in, names)
		}
	}

	pendingStatus, err := s.refs.GetByCode(ctx, domain.RefTableTaskStatus, domain.TaskStatusPending)
	if err != nil {
		return nil, err
	}
	if err := s.enforceTenantTaskQuota(ctx, dev); err != nil {
		return nil, err
	}
	paramsJSON, err := json.Marshal(in.Parameters)
	if err != nil {
		return nil, fmt.Errorf("task: parameters tidak valid: %w", err)
	}

	maxRetries := in.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}
	priority := in.Priority
	if priority == 0 {
		priority = 5
	}

	t := &domain.Task{
		TaskUUID:     uuid.NewString(),
		DeviceID:     in.DeviceID,
		TaskTypeID:   taskType.ID,
		TaskStatusID: pendingStatus.ID,
		Priority:     priority,
		Parameters:   domain.JSONRawMessage(paramsJSON),
		MaxRetries:   maxRetries,
		ScheduledAt:  in.ScheduledAt,
		ExpiresAt:    in.ExpiresAt,
		Audit:        domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if err := s.tasks.Create(ctx, t); err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "CREATE_TASK", EntityType: "task", EntityID: &t.ID,
	})

	// WS Event
	if s.publisher != nil && dev.TenantID != nil {
		s.publisher.BroadcastToTenant(*dev.TenantID, "TASK_CREATED", map[string]interface{}{
			"task_id":   t.ID,
			"device_id": t.DeviceID,
		})
	}

	return t, nil
}

// createChunkedGetParameterValues memecah satu permintaan GET_PARAMETER_VALUES
// bervolume besar (> MaxGetParameterValuesNamesPerTask) jadi beberapa task
// berurutan, masing-masing <= MaxGetParameterValuesNamesPerTask nama.
// Mengembalikan task chunk PERTAMA (yang akan dieksekusi lebih dulu sesuai
// urutan priority/created_at ASC seperti task lain manapun, TECH.md §4) sbg
// representasi ke pemanggil — pemanggil yang butuh SEMUA task hasil pecahan
// (mis. untuk melacak status lengkap) bisa query ulang lewat List dgn
// device_id+task_type yang sama.
func (s *Service) createChunkedGetParameterValues(ctx context.Context, actor domain.Actor, in domain.CreateTaskInput, names []string) (*domain.Task, error) {
	var first *domain.Task
	for start := 0; start < len(names); start += MaxGetParameterValuesNamesPerTask {
		end := start + MaxGetParameterValuesNamesPerTask
		if end > len(names) {
			end = len(names)
		}
		chunkIn := in
		chunkIn.Parameters = map[string]interface{}{"names": names[start:end]}
		t, err := s.CreateTask(ctx, actor, chunkIn)
		if err != nil {
			return nil, err
		}
		if first == nil {
			first = t
		}
	}
	return first, nil
}

// parseGetParameterValuesNames mengekstrak "names" ([]string) dari payload
// JSON mentah task.Parameters bertipe GET_PARAMETER_VALUES (lihat kontrak
// bentuk JSON di delivery/cwmp/builder.go#BuildRequestBody).
func parseGetParameterValuesNames(raw []byte) ([]string, bool) {
	var p struct {
		Names []string `json:"names"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, false
	}
	return p.Names, len(p.Names) > 0
}

// extractParameterNames sama seperti parseGetParameterValuesNames tapi
// menerima in.Parameters (map[string]interface{}) SEBELUM di-marshal jadi
// JSON tersimpan — dipakai CreateTask utk deteksi proactive chunking.
// Round-trip lewat json.Marshal/Unmarshal SENGAJA dipakai (bukan type
// assertion manual) krn representasi runtime "names" bisa []string
// (pemanggil internal, mis. SplitGetParameterValuesOnFault) MAUPUN
// []interface{} (hasil decode JSON body REST API mentah lewat
// encoding/json ke map[string]interface{}, lihat
// delivery/http/task_handler.go createTaskRequest.Parameters) — round-trip
// ini menormalkan keduanya jadi satu bentuk yang sama tanpa type-switch
// manual utk semua kemungkinan tipe yang bisa dihasilkan decoder JSON.
func extractParameterNames(params map[string]interface{}) ([]string, bool) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, false
	}
	return parseGetParameterValuesNames(raw)
}

// enforceTenantTaskQuota menolak pembuatan task baru bila tenant pemilik
// device sudah punya task PENDING >= max_pending_tasks tenant tsb
// (ROADMAP.md Fase 2 — kuota task queue per tenant, mencegah satu tenant
// menghabiskan resource task queue bersama). Scope kuota ini SENGAJA HANYA
// task queue -- BUKAN rate limit koneksi/sesi CWMP itu sendiri (di luar
// cakupan, CLAUDE.md: sesi CWMP stateful butuh perubahan lebih invasif).
// Device tanpa tenant (belum tercatat/superadmin-managed) atau tenant tanpa
// kuota (max_pending_tasks NULL) tidak dibatasi sama sekali.
func (s *Service) enforceTenantTaskQuota(ctx context.Context, dev *domain.Device) error {
	if dev.TenantID == nil || s.tenants == nil {
		return nil
	}
	tenant, err := s.tenants.GetByID(ctx, *dev.TenantID)
	if err != nil {
		return err
	}
	if tenant.MaxPendingTasks == nil {
		return nil
	}
	// CountPendingForTenant, BUKAN List(): query COUNT murni (satu query, tanpa
	// ikut fetch baris task lengkap termasuk kolom JSON parameters yang tidak
	// dipakai di sini) -- ini jalur panas, dipanggil di setiap CreateTask utk
	// tenant yang punya kuota. Cakupan status PENDING+QUEUED disamakan dengan
	// HasPendingForDevice, bukan cuma PENDING, supaya konsisten kalau status
	// QUEUED mulai dipakai di masa depan.
	//
	// Catatan jujur soal race (TOCTOU): count dan insert task baru di bawah
	// bukan operasi atomik (tanpa SELECT...FOR UPDATE/transaksi) -- beberapa
	// request paralel utk tenant yang sama bisa membuat total pending
	// melewati batas sebesar jumlah request yang lolos di window race
	// tsb. Diterima apa adanya (reviewer acs-security-reviewer +
	// acs-code-reviewer: severity rendah/sedang, ini kuota lunak utk cegah
	// satu tenant memonopoli resource bersama, bukan boundary keamanan) --
	// tidak diperbaiki dgn locking supaya tidak overengineer fitur ini.
	total, err := s.tasks.CountPendingForTenant(ctx, *dev.TenantID)
	if err != nil {
		return err
	}
	if uint32(total) >= *tenant.MaxPendingTasks {
		return fmt.Errorf("%w: tenant sudah punya %d task pending (batas %d)", domain.ErrQuotaExceeded, total, *tenant.MaxPendingTasks)
	}
	return nil
}

// EnqueueSetParameterValues mengimplementasikan domain.TaskEnqueuer — dipakai
// usecase/provisioning untuk menerapkan profile/ZTP tanpa usecase/task perlu
// bergantung balik pada usecase/provisioning (lihat domain/task.go).
func (s *Service) EnqueueSetParameterValues(ctx context.Context, actor domain.Actor, deviceID uint64, params map[string]string, priority uint8) (*domain.Task, error) {
	resolved := make(map[string]string, len(params))
	for k, v := range params {
		path, err := s.ResolveParameterPath(ctx, deviceID, k)
		if err != nil {
			return nil, err
		}
		resolved[path] = v
	}
	return s.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID:   deviceID,
		TaskType:   domain.TaskTypeSetParameterValues,
		Priority:   priority,
		Parameters: map[string]interface{}{"values": resolved},
	})
}

// EnqueueGetParameterNames mengantre task GetParameterNames — dipakai
// auto-discovery parameter tree saat device baru dgn vendor tak dikenal
// BOOTSTRAP (usecase/session). path "" + nextLevel true = ambil level teratas.
func (s *Service) EnqueueGetParameterNames(ctx context.Context, actor domain.Actor, deviceID uint64, path string, nextLevel bool, priority uint8) (*domain.Task, error) {
	return s.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID:   deviceID,
		TaskType:   domain.TaskTypeGetParameterNames,
		Priority:   priority,
		Parameters: map[string]interface{}{"path": path, "next_level": nextLevel},
	})
}

// EnqueueReboot mengimplementasikan domain.TaskEnqueuer — dipakai aksi
// PostApplyReboot pada ZeroTouchRule (migrations/0009, usecase/provisioning).
// REBOOT tidak butuh resolusi logical key/parameter apa pun (beda dari
// EnqueueSetParameterValues), jadi cukup CreateTask langsung.
func (s *Service) EnqueueReboot(ctx context.Context, actor domain.Actor, deviceID uint64, priority uint8) (*domain.Task, error) {
	return s.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID: deviceID,
		TaskType: domain.TaskTypeReboot,
		Priority: priority,
	})
}

// requireTaskTenantScope memastikan actor non-superadmin hanya mengakses
// task milik device yang tenant-nya sama dengan tenant actor (RBAC scope
// tenant, CLAUDE.md) — pola sama seperti device.Service.requireTenantScope,
// diduplikasi kecil di sini karena task->tenant harus di-resolve lewat
// device pemiliknya dulu (tasks tidak punya tenant_id langsung).
func (s *Service) requireTaskTenantScope(ctx context.Context, actor domain.Actor, t *domain.Task) error {
	if actor.IsSuperadmin() {
		return nil
	}
	dev, err := s.devices.GetByID(ctx, t.DeviceID)
	if err != nil {
		return err
	}
	if dev.TenantID == nil || actor.TenantID == nil || *actor.TenantID != *dev.TenantID {
		return domain.ErrForbidden
	}
	return nil
}

// redactSensitiveParams menutupi field kredensial/URL sensitif di level atas
// JSON task.Parameters (dipakai task tipe Download/Upload: "url" presigned
// MinIO bertindak sbg bearer token -- siapa pun yang pegang bisa download
// tanpa auth lain sampai kedaluwarsa -- plus opsional "username"/"password"
// milik CPE) sebelum dikembalikan ke role yang tidak seharusnya melihatnya.
// GET /tasks/GET /tasks/:id SENGAJA tidak digating role (NOC/VIEWER boleh
// lihat status task utk monitoring operasional), tapi isi kredensial/URL
// cuma boleh terlihat role yg memang bisa menjadwalkan task jenis ini sendiri
// (ADMIN/SUPERADMIN, lihat RequireRoles(admin...) di POST /devices/:id/
// firmware-upgrade) -- NOC/VIEWER cukup tahu statusnya, bukan isi
// kredensialnya (temuan acs-security-reviewer, review fitur MinIO object
// storage: presigned URL bocor ke role rendah lewat endpoint task generik).
func redactSensitiveParams(params *domain.JSONRawMessage, actor domain.Actor) {
	if actor.IsSuperadmin() || actor.HasRole(domain.RoleAdmin) {
		return
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(*params, &m); err != nil {
		return
	}
	redacted := false
	for _, key := range []string{"url", "username", "password"} {
		if _, ok := m[key]; ok {
			m[key] = json.RawMessage(`"[redacted]"`)
			redacted = true
		}
	}
	if !redacted {
		return
	}
	if out, err := json.Marshal(m); err == nil {
		*params = out
	}
}

func (s *Service) Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.Task, error) {
	t, err := s.tasks.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireTaskTenantScope(ctx, actor, t); err != nil {
		return nil, err
	}
	redactSensitiveParams(&t.Parameters, actor)
	return t, nil
}

func (s *Service) List(ctx context.Context, actor domain.Actor, f domain.TaskFilter, p domain.Pagination) ([]domain.Task, int, error) {
	if !actor.IsSuperadmin() {
		tid, err := auth.ScopedTenantFilter(actor)
		if err != nil {
			return nil, 0, err
		}
		f.TenantID = tid
	}
	tasks, total, err := s.tasks.List(ctx, f, p)
	if err != nil {
		return nil, 0, err
	}
	for i := range tasks {
		redactSensitiveParams(&tasks[i].Parameters, actor)
	}
	return tasks, total, nil
}

// Stats — agregat untuk dashboard analitik (ROADMAP.md Fase 1), tenant-scoped
// sama seperti List/Get di atas.
func (s *Service) Stats(ctx context.Context, actor domain.Actor) ([]domain.TaskStatusCount, error) {
	tenantID, err := auth.ScopedTenantFilter(actor)
	if err != nil {
		return nil, err
	}
	return s.tasks.CountByStatus(ctx, tenantID)
}

// PlatformStats — sama seperti Stats tapi SELALU lintas seluruh tenant, tanpa
// domain.Actor. Dipakai HANYA oleh internal/metrics (endpoint /metrics,
// ROADMAP.md Fase 2) — lihat komentar PlatformStats di device.Service utk
// alasan lengkap kenapa ini method terpisah, bukan Stats dgn actor palsu.
func (s *Service) PlatformStats(ctx context.Context) ([]domain.TaskStatusCount, error) {
	return s.tasks.CountByStatus(ctx, nil)
}

// AvgCompletionSeconds — rata-rata waktu penyelesaian task COMPLETED dalam
// `window` terakhir, lintas seluruh tenant (metrik observability TECH.md §10,
// dipakai HANYA internal/metrics — tidak ada endpoint REST per-tenant untuk
// ini, jadi sengaja tanpa domain.Actor/tenant-scope sama sekali, bukan
// dipanggil dgn actor superadmin palsu). nil berarti tidak ada task selesai
// dalam window.
func (s *Service) AvgCompletionSeconds(ctx context.Context, window time.Duration) (*float64, error) {
	return s.tasks.AvgCompletionSeconds(ctx, nil, time.Now().Add(-window))
}

// ErrorCountsByVendor — jumlah task FAILED saat ini per vendor, lintas
// seluruh tenant (metrik observability TECH.md §10, indikasi masalah
// kompatibilitas parameter mapping vendor tsb) — dipakai HANYA
// internal/metrics, pola sama seperti AvgCompletionSeconds di atas.
func (s *Service) ErrorCountsByVendor(ctx context.Context) ([]domain.TaskVendorErrorCount, error) {
	return s.tasks.CountFailedByVendor(ctx, nil)
}

func (s *Service) Cancel(ctx context.Context, actor domain.Actor, id uint64) error {
	t, err := s.tasks.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.requireTaskTenantScope(ctx, actor, t); err != nil {
		return err
	}
	return s.tasks.Cancel(ctx, id, actor.UserIDPtr())
}

// ---- Dipanggil dari usecase/session selama sesi CWMP berlangsung ----

func (s *Service) NextForDevice(ctx context.Context, deviceID uint64) (*domain.Task, error) {
	return s.tasks.NextForDevice(ctx, deviceID)
}

func (s *Service) HasPendingForDevice(ctx context.Context, deviceID uint64) (bool, error) {
	return s.tasks.HasPendingForDevice(ctx, deviceID)
}

func (s *Service) GetSentForDevice(ctx context.Context, deviceID uint64) (*domain.Task, error) {
	return s.tasks.GetSentForDevice(ctx, deviceID)
}

func (s *Service) GetByUUID(ctx context.Context, uuid string) (*domain.Task, error) {
	return s.tasks.GetByUUID(ctx, uuid)
}

func (s *Service) MarkSent(ctx context.Context, taskID uint64) error {
	err := s.tasks.MarkSent(ctx, taskID, time.Now())
	if err == nil {
		s.publishTaskStatusEvent(ctx, taskID, domain.TaskStatusSent)
	}
	return err
}

func (s *Service) Complete(ctx context.Context, taskID uint64, response domain.JSONRawMessage) error {
	err := s.tasks.MarkCompleted(ctx, taskID, response, time.Now())
	if err == nil {
		s.publishTaskStatusEvent(ctx, taskID, domain.TaskStatusCompleted)

		// Hook Auto-Discovery
		if t, err := s.tasks.GetByID(ctx, taskID); err == nil && t.TaskTypeCode == domain.TaskTypeGetParameterNames {
			go s.handleAutoDiscoveryResponse(context.Background(), t, response)
		}
	}
	return err
}

// Fail menandai task gagal (dari cwmp:Fault CPE) dengan retry hingga max_retries.
func (s *Service) Fail(ctx context.Context, t *domain.Task, errMsg string) error {
	return s.failOrRetry(ctx, t, domain.TaskStatusFailed, errMsg)
}

// FailPermanently menandai task FAILED SEGERA, TANPA melalui siklus retry
// max_retries biasa (failOrRetry) — dipakai utk kasus yang pasti akan gagal
// identik pada percobaan berikutnya (mis. cwmp:Fault domain.
// FaultCodeInvalidParameterName "9005": parameter memang tidak ada di device
// ini, retry tidak akan mengubah hasil), atau task yang sudah digantikan
// task pecahan lain (lihat SplitGetParameterValuesOnFault). retry_count
// tetap di-increment (mencatat bahwa 1 percobaan terjadi, konsisten dgn
// failOrRetry), tapi TIDAK dibandingkan ke max_retries sama sekali —
// langsung MarkFailed.
// SENGAJA tidak memicu webhook TASK_FAILED di sini: pemakainya adalah kasus
// "gagal terduga" (9005 param-not-found — sudah memicu DEVICE_FAULT dari
// usecase/session, jadi TASK_FAILED akan duplikat) atau task yang digantikan
// task pecahan (bukan kegagalan operasional). TASK_FAILED hanya dari
// failOrRetry (retry/timeout habis).
func (s *Service) FailPermanently(ctx context.Context, t *domain.Task, errMsg string) error {
	if err := s.tasks.IncrementRetry(ctx, t.ID); err != nil {
		return err
	}
	status, err := s.refs.GetByCode(ctx, domain.RefTableTaskStatus, domain.TaskStatusFailed)
	if err != nil {
		return err
	}
	err = s.tasks.MarkFailed(ctx, t.ID, status.ID, errMsg)
	if err == nil {
		s.publishTaskStatusEvent(ctx, t.ID, domain.TaskStatusFailed)
	}
	return err
}

// SplitGetParameterValuesOnFault menangani cwmp:Fault domain.
// FaultCodeInvalidArguments "9003" pada task GET_PARAMETER_VALUES yang SUDAH
// TERKIRIM (reaktif) — beda dari proactive chunking
// (createChunkedGetParameterValues, dicek saat task DIBUAT) yang mencegah
// task oversized terbentuk sejak awal. Banyak implementasi CPE membalas 9003
// generik saat menolak request krn ParameterNames-nya kepanjangan (kuirk
// umum lintas vendor, bukan satu vendor spesifik — CLAUDE.md soal
// vendor_adapter). Alih-alih retry request identik yg pasti gagal lagi lewat
// failOrRetry, daftar nama dipecah dua & masing2 jadi task GET_PARAMETER_VALUES
// baru; task asli ditandai FAILED (lewat FailPermanently, BUKAN balik ke
// PENDING) krn sudah digantikan task pecahan tsb.
//
// Return (false, nil) bila task ini TIDAK BISA dipecah lagi (<= 1 parameter
// name) — pemanggil (usecase/session.Service.handleFault) HARUS fallback ke
// Fail/failOrRetry normal pada kasus ini, task ini TIDAK disentuh sama sekali.
func (s *Service) SplitGetParameterValuesOnFault(ctx context.Context, t *domain.Task, faultMsg string) (bool, error) {
	names, ok := parseGetParameterValuesNames(t.Parameters)
	if !ok || len(names) <= 1 {
		return false, nil
	}

	dev, err := s.devices.GetByID(ctx, t.DeviceID)
	if err != nil {
		return false, err
	}
	// Aktor sistem (bukan user login) — dipicu cwmp:Fault dari CPE, bukan
	// tindakan operator (pola sama seperti systemActor di
	// usecase/session/service.go#HandleInform utk aksi ZTP otomatis).
	actor := domain.Actor{TenantID: dev.TenantID}

	mid := len(names) / 2
	chunks := [][]string{names[:mid], names[mid:]}
	for _, chunk := range chunks {
		if _, err := s.CreateTask(ctx, actor, domain.CreateTaskInput{
			DeviceID:   t.DeviceID,
			TaskType:   domain.TaskTypeGetParameterValues,
			Priority:   t.Priority,
			MaxRetries: t.MaxRetries,
			Parameters: map[string]interface{}{"names": chunk},
		}); err != nil {
			return false, err
		}
	}

	msg := fmt.Sprintf("%s (dipecah jadi %d task GetParameterValues baru krn parameter list ditolak CPE)", faultMsg, len(chunks))
	if err := s.FailPermanently(ctx, t, msg); err != nil {
		return false, err
	}
	return true, nil
}

// Timeout menandai task time-out menunggu respons CPE, dengan retry hingga max_retries.
func (s *Service) Timeout(ctx context.Context, t *domain.Task) error {
	return s.failOrRetry(ctx, t, domain.TaskStatusTimeout, "timeout menunggu respons CPE")
}

// TimeoutStaleSent men-timeout-kan task SENT yang sudah lebih lama dari
// threshold tanpa respons CPE (mis. koneksi CPE putus di tengah sesi).
// Dipanggil periodik dari goroutine di cmd/acsd — aman dijalankan dari
// instance manapun karena app server stateless (TECH.md §9).
func (s *Service) TimeoutStaleSent(ctx context.Context, threshold time.Duration) (int, error) {
	stale, err := s.tasks.ListStaleSent(ctx, time.Now().Add(-threshold))
	if err != nil {
		return 0, err
	}
	count := 0
	for i := range stale {
		if err := s.Timeout(ctx, &stale[i]); err == nil {
			count++
		}
	}
	return count, nil
}

func (s *Service) publishTaskStatusEvent(ctx context.Context, taskID uint64, status string) {
	if s.publisher == nil {
		return
	}
	t, err := s.tasks.GetByID(ctx, taskID)
	if err != nil {
		return
	}
	dev, err := s.devices.GetByID(ctx, t.DeviceID)
	if err != nil || dev.TenantID == nil {
		return
	}
	s.publisher.BroadcastToTenant(*dev.TenantID, "TASK_STATUS_CHANGED", map[string]interface{}{
		"task_id":   t.ID,
		"device_id": t.DeviceID,
		"status":    status,
	})
}

// failOrRetry: retry_count bertambah 1; jika sudah mencapai/melewati
// max_retries task ditandai gagal permanen (finalStatusCode), selain itu
// dikembalikan ke PENDING agar dicoba lagi pada sesi berikutnya (TECH.md §4).
func (s *Service) failOrRetry(ctx context.Context, t *domain.Task, finalStatusCode, errMsg string) error {
	if err := s.tasks.IncrementRetry(ctx, t.ID); err != nil {
		return err
	}
	if err := s.tasks.SetErrorMessage(ctx, t.ID, errMsg); err != nil {
		return err
	}
	statusCode := domain.TaskStatusPending
	if t.RetryCount+1 >= t.MaxRetries {
		statusCode = finalStatusCode
	}
	status, err := s.refs.GetByCode(ctx, domain.RefTableTaskStatus, statusCode)
	if err != nil {
		return err
	}
	if err := s.tasks.UpdateStatus(ctx, t.ID, status.ID, nil); err != nil {
		return err
	}

	s.publishTaskStatusEvent(ctx, t.ID, statusCode)

	if statusCode == domain.TaskStatusFailed {
		s.notifyTaskFailed(ctx, t, errMsg)
	}
	return nil
}

func (s *Service) handleAutoDiscoveryResponse(ctx context.Context, t *domain.Task, response domain.JSONRawMessage) {
	// Parse GetParameterNamesResponse
	var b struct {
		ParameterList struct {
			Items []struct {
				Name     string `json:"Name"`
				Writable bool   `json:"Writable"`
			} `json:"Items"`
		} `json:"ParameterList"`
	}
	if err := json.Unmarshal(response, &b); err != nil {
		return
	}

	params := make([]domain.DeviceParameter, 0, len(b.ParameterList.Items))
	var namesToGet []string
	for _, item := range b.ParameterList.Items {
		// Simpan nama parameter dengan nilai kosong sebagai placeholder
		val := ""
		params = append(params, domain.DeviceParameter{
			DeviceID:       t.DeviceID,
			ParameterName:  item.Name,
			ParameterValue: &val, // Harus pointer ke string sesuai skema
		})

		// Kumpulkan leaf nodes (yang bukan parent object) untuk GetParameterValues
		// Biasanya leaf nodes tidak berakhiran dengan "."
		if !strings.HasSuffix(item.Name, ".") {
			namesToGet = append(namesToGet, item.Name)
		}
	}

	if len(params) > 0 {
		_ = s.deviceParams.UpsertBatch(ctx, params)
	}

	// Queue task GetParameterValues untuk leaf nodes yang ditemukan (untuk mengisi nilainya)
	if len(namesToGet) > 0 {
		// Chunking sudah ditangani oleh CreateTask jika lebih dari MaxGetParameterValuesNamesPerTask
		systemActor := domain.Actor{Roles: []string{domain.RoleSuperadmin}} // Bypass RBAC untuk operasi internal sistem
		_, _ = s.CreateTask(ctx, systemActor, domain.CreateTaskInput{
			DeviceID:   t.DeviceID,
			TaskType:   domain.TaskTypeGetParameterValues,
			Priority:   5, // Prioritas rendah (background discovery)
			Parameters: map[string]interface{}{"names": namesToGet},
		})
	}
}
