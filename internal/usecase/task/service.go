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
	m, err := s.ResolveMapping(ctx, deviceID, key)
	if err != nil {
		return "", err
	}
	return m.TR069Path, nil
}

// ResolveMapping = ResolveParameterPath tetapi mengembalikan seluruh mapping (path, tipe, skala).
// Raw path menghasilkan mapping sintetis (skala 1).
func (s *Service) ResolveMapping(ctx context.Context, deviceID uint64, key string) (*domain.VendorParameterMapping, error) {
	if looksLikeRawPath(key) {
		return &domain.VendorParameterMapping{TR069Path: key, LogicalKey: key, ValueScale: 1}, nil
	}
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if dev.VendorID == nil {
		return nil, fmt.Errorf("task: device belum diketahui vendor-nya, tidak bisa resolve logical key %q", key)
	}
	// Data model version: dari model perangkat bila ada; bila belum (ONT baru, model
	// belum terdaftar) dideteksi dari root parameter yang pernah dilaporkan CPE.
	var dmvID uint64
	if dev.DeviceModelID != nil {
		dm, err := s.deviceModels.GetByID(ctx, *dev.DeviceModelID)
		if err != nil {
			return nil, err
		}
		dmvID = dm.DataModelVersionID
	} else {
		id, err := s.detectDataModelVersion(ctx, dev.ID)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
		}
		dmvID = id
	}
	m, err := s.paramMappings.Resolve(ctx, *dev.VendorID, dmvID, dev.DeviceModelID, key, dev.SoftwareVersion)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("task: tidak ada vendor_parameter_mappings untuk logical key %q pada device ini (kirim raw TR-069 path bila belum dipetakan, lihat FR-12)", key)
		}
		return nil, err
	}
	if m.ValueScale == 0 {
		m.ValueScale = 1
	}
	return m, nil
}

// detectDataModelVersion menentukan TR-098 vs TR-181 dari parameter device (root object
// pada ref_data_model_versions), tanpa mengasumsikan salah satunya.
func (s *Service) detectDataModelVersion(ctx context.Context, deviceID uint64) (uint64, error) {
	if s.deviceParams == nil {
		return 0, errors.New("data model perangkat belum diketahui")
	}
	for _, code := range []string{"TR098", "TR181"} {
		dmv, err := s.refs.GetByCode(ctx, domain.RefTableDataModelVersions, code)
		if err != nil {
			continue
		}
		root := "InternetGatewayDevice."
		if code == "TR181" {
			root = "Device."
		}
		if ok, err := s.deviceParams.HasPrefix(ctx, deviceID, root); err == nil && ok {
			return dmv.ID, nil
		}
	}
	return 0, errors.New("model & data model perangkat belum diketahui (belum ada parameter dari CPE); gunakan raw TR-069 path atau daftarkan model")
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

	if err := ValidateTaskInput(taskType.Code, in.Parameters); err != nil {
		return nil, err
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

	if resolved, rerr := s.resolveLogicalKeys(ctx, in.DeviceID, taskType.Code, in.Parameters); rerr != nil {
		return nil, rerr
	} else {
		in.Parameters = resolved
	}
	if taskType.Code == domain.TaskTypeSetParameterValues {
		in.Parameters = s.withTypeHints(ctx, in.DeviceID, in.Parameters)
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

// resolveLogicalKeys mengganti logical key (mis. wifi.ssid) pada parameters task dengan
// raw path TR-069 sesuai vendor/data model perangkat; raw path dibiarkan. Key yang tak
// dapat di-resolve = 400 (bukan 500 / dikirim mentah ke CPE lalu Fault 9005).
func (s *Service) resolveLogicalKeys(ctx context.Context, deviceID uint64, taskType string, params map[string]interface{}) (map[string]interface{}, error) {
	if params == nil || s.paramMappings == nil {
		return params, nil
	}
	resolveOne := func(k string) (string, error) {
		p, err := s.ResolveParameterPath(ctx, deviceID, k)
		if err != nil && !errors.Is(err, domain.ErrInvalidInput) && !errors.Is(err, domain.ErrNotFound) {
			if strings.HasPrefix(err.Error(), "task: ") {
				return "", fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
		}
		return p, err
	}
	out := make(map[string]interface{}, len(params))
	for k, v := range params {
		out[k] = v
	}
	switch taskType {
	case domain.TaskTypeSetParameterValues:
		raw, _ := json.Marshal(params["values"])
		var vals map[string]string
		if json.Unmarshal(raw, &vals) != nil || len(vals) == 0 {
			return params, nil
		}
		nv := make(map[string]string, len(vals))
		for k, v := range vals {
			p, err := resolveOne(k)
			if err != nil {
				return nil, err
			}
			nv[p] = v
		}
		out["values"] = nv
		if rawT, ok := params["types"]; ok {
			var types map[string]string
			if b, _ := json.Marshal(rawT); json.Unmarshal(b, &types) == nil {
				nt := make(map[string]string, len(types))
				for k, v := range types {
					p, err := resolveOne(k)
					if err != nil {
						return nil, err
					}
					nt[p] = v
				}
				out["types"] = nt
			}
		}
	case domain.TaskTypeGetParameterValues, domain.TaskTypeGetParameterAttributes:
		raw, _ := json.Marshal(params["names"])
		var names []string
		if json.Unmarshal(raw, &names) != nil || len(names) == 0 {
			return params, nil
		}
		for i, n := range names {
			p, err := resolveOne(n)
			if err != nil {
				return nil, err
			}
			names[i] = p
		}
		out["names"] = names
	case domain.TaskTypeSetParameterAttributes:
		raw, _ := json.Marshal(params["attributes"])
		var attrs []map[string]interface{}
		if json.Unmarshal(raw, &attrs) != nil || len(attrs) == 0 {
			return params, nil
		}
		for _, a := range attrs {
			if n, ok := a["name"].(string); ok {
				p, err := resolveOne(n)
				if err != nil {
					return nil, err
				}
				a["name"] = p
			}
		}
		out["attributes"] = attrs
	}
	return out, nil
}

// withTypeHints melengkapi parameters SET_PARAMETER_VALUES dengan "types"
// (path -> xsd:type) dari tipe yang pernah dilaporkan CPE (device_parameters,
// terisi dari Inform/GetParameterValues). Tanpa ini semua nilai dikirim sebagai
// xsd:string dan firmware yang ketat menolak boolean/angka dengan fault 9006.
// Tipe yang diberikan pemanggil dihormati; tipe tak diketahui/"string" dibiarkan
// (default xsd:string). Best-effort: kegagalan lookup tidak menggagalkan task.
func (s *Service) withTypeHints(ctx context.Context, deviceID uint64, params map[string]interface{}) map[string]interface{} {
	if s.deviceParams == nil || s.refs == nil {
		return params
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return params
	}
	var p struct {
		Values map[string]string `json:"values"`
		Types  map[string]string `json:"types"`
	}
	if json.Unmarshal(raw, &p) != nil || len(p.Values) == 0 {
		return params
	}
	types := make(map[string]string, len(p.Types))
	for k, v := range p.Types {
		types[k] = v
	}
	codeByID := map[uint64]string{}
	for name := range p.Values {
		if _, given := types[name]; given {
			continue
		}
		dp, err := s.deviceParams.Get(ctx, deviceID, name)
		if err != nil || dp == nil || dp.ParameterTypeID == nil {
			continue
		}
		code, seen := codeByID[*dp.ParameterTypeID]
		if !seen {
			if ref, err := s.refs.GetByID(ctx, domain.RefTableParameterTypes, *dp.ParameterTypeID); err == nil {
				code = ref.Code
			}
			codeByID[*dp.ParameterTypeID] = code
		}
		if code != "" && code != "string" {
			types[name] = "xsd:" + code
		}
	}
	if len(types) == 0 {
		return params
	}
	out := make(map[string]interface{}, len(params)+1)
	for k, v := range params {
		out[k] = v
	}
	out["types"] = types
	return out
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
	// Nilai SET_PARAMETER_VALUES yang kuncinya rahasia (passphrase WiFi, password
	// PPPoE, ...) juga disamarkan untuk NOC/VIEWER.
	if raw, ok := m["values"]; ok {
		var vals map[string]json.RawMessage
		if json.Unmarshal(raw, &vals) == nil {
			changed := false
			for k := range vals {
				if domain.IsSensitiveParamName(k) {
					vals[k] = json.RawMessage(`"[redacted]"`)
					changed = true
				}
			}
			if changed {
				if b, err := json.Marshal(vals); err == nil {
					m["values"] = b
					redacted = true
				}
			}
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
	// Task yang sudah berakhir tidak boleh "dibatalkan": riwayat COMPLETED/FAILED
	// tidak boleh tertimpa CANCELLED.
	if st, serr := s.refs.GetByID(ctx, domain.RefTableTaskStatus, t.TaskStatusID); serr == nil {
		switch st.Code {
		case domain.TaskStatusCompleted, domain.TaskStatusFailed, domain.TaskStatusCancelled, domain.TaskStatusTimeout:
			return fmt.Errorf("%w: task sudah berstatus %s dan tidak dapat dibatalkan", domain.ErrConflict, st.Code)
		}
	}
	if err := s.tasks.Cancel(ctx, id, actor.UserIDPtr()); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "CANCEL_TASK", EntityType: "task", EntityID: &id,
	})
	return nil
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
	if statusCode == domain.TaskStatusPending {
		// Backoff: tanpa ini task diambil lagi di SESI YANG SAMA (CPE masih
		// terhubung) dan menghabiskan max_retries dalam hitungan detik.
		if err := s.tasks.Reschedule(ctx, t.ID, time.Now().Add(RetryBackoff(t.RetryCount+1))); err != nil {
			return err
		}
	}

	s.publishTaskStatusEvent(ctx, t.ID, statusCode)

	if statusCode == domain.TaskStatusFailed {
		s.notifyTaskFailed(ctx, t, errMsg)
	}
	return nil
}

// RetryBackoff = jeda sebelum percobaan ke-n (n >= 1): 1m, 2m, 4m, ... maks 30m.
func RetryBackoff(n uint32) time.Duration {
	d := time.Minute
	for i := uint32(1); i < n && d < 30*time.Minute; i++ {
		d *= 2
	}
	if d > 30*time.Minute {
		d = 30 * time.Minute
	}
	return d
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

	// Konfigurasi penelusuran rekursif (diisi endpoint refresh): kedalaman & anggaran task.
	var dp struct {
		Discover *struct {
			Depth  int `json:"depth"`
			Budget int `json:"budget"`
		} `json:"discover"`
	}
	_ = json.Unmarshal(t.Parameters, &dp)

	params := make([]domain.DeviceParameter, 0, len(b.ParameterList.Items))
	var namesToGet []string
	var childObjects []string
	for _, item := range b.ParameterList.Items {
		// Hanya nama + writable; NILAI yang sudah ada tidak ditimpa (sebelumnya placeholder
		// kosong menimpa nilai nyata sampai GetParameterValues berikutnya selesai).
		params = append(params, domain.DeviceParameter{
			DeviceID:      t.DeviceID,
			ParameterName: item.Name,
			Writable:      item.Writable,
		})
		if strings.HasSuffix(item.Name, ".") && item.Name != "" && dp.Discover != nil {
			childObjects = append(childObjects, item.Name)
		}

		// Kumpulkan leaf nodes (yang bukan parent object) untuk GetParameterValues
		// Biasanya leaf nodes tidak berakhiran dengan "."
		if !strings.HasSuffix(item.Name, ".") {
			namesToGet = append(namesToGet, item.Name)
		}
	}

	if len(params) > 0 {
		_ = s.deviceParams.UpsertNames(ctx, params)
	}

	// Telusuri anak-anak objek (maks. depth & budget) sebagai task GetParameterNames berantai.
	if dp.Discover != nil && dp.Discover.Depth > 1 && dp.Discover.Budget > 0 && len(childObjects) > 0 {
		perChild := (dp.Discover.Budget - len(childObjects)) / len(childObjects)
		if perChild >= 0 {
			sys := domain.Actor{Roles: []string{domain.RoleSuperadmin}}
			for _, obj := range childObjects {
				_, _ = s.CreateTask(ctx, sys, domain.CreateTaskInput{
					DeviceID: t.DeviceID,
					TaskType: domain.TaskTypeGetParameterNames,
					Priority: 8,
					Parameters: map[string]interface{}{
						"path": obj, "next_level": true,
						"discover": map[string]int{"depth": dp.Discover.Depth - 1, "budget": perChild},
					},
				})
			}
		}
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

// EnqueueOpticalPoll mengantre GetParameterValues untuk kunci optical.* yang TERPETAKAN pada
// perangkat ini (rx/tx/voltage/bias/temperature). Mengembalikan false bila tak ada satu pun
// mapping (vendor/model belum punya path optik) — bukan error. Skala per-path dibawa di task
// agar hasil dikonversi ke satuan kanonik tanpa percabangan vendor.
func (s *Service) EnqueueOpticalPoll(ctx context.Context, deviceID uint64) (bool, error) {
	keys := map[string]string{
		"optical.rx_power": "rx_dbm", "optical.tx_power": "tx_dbm", "optical.voltage": "voltage",
		"optical.bias_current": "bias_ma", "optical.temperature": "temperature_c",
	}
	fields := map[string]string{}
	scales := map[string]float64{}
	var names []string
	for _, k := range []string{"optical.rx_power", "optical.tx_power", "optical.voltage", "optical.bias_current", "optical.temperature"} {
		m, err := s.ResolveMapping(ctx, deviceID, k)
		if err != nil {
			if errors.Is(err, domain.ErrInvalidInput) || errors.Is(err, domain.ErrNotFound) || strings.HasPrefix(err.Error(), "task: ") {
				continue
			}
			return false, err
		}
		fields[keys[k]] = m.TR069Path
		scales[m.TR069Path] = m.ValueScale
		names = append(names, m.TR069Path)
	}
	if len(names) == 0 {
		return false, nil
	}
	sys := domain.Actor{Roles: []string{domain.RoleSuperadmin}}
	_, err := s.CreateTask(ctx, sys, domain.CreateTaskInput{
		DeviceID: deviceID, TaskType: domain.TaskTypeGetParameterValues, Priority: 8,
		Parameters: map[string]interface{}{"names": names, "optical": map[string]interface{}{"fields": fields, "scales": scales}},
	})
	return err == nil, err
}
