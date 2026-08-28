// Package device menangani inventory & status CPE (TECH.md §11, FR-22/FR-23/FR-25).
package device

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
	"acs/pkg/cryptoutil"
)

type Service struct {
	devices        domain.DeviceRepository
	vendorOUIs     domain.VendorOUIRepository
	deviceModels   domain.DeviceModelRepository
	refs           domain.RefRepository
	deviceParams   domain.DeviceParameterRepository
	deviceEvents   domain.DeviceEventRepository
	opticalMetrics domain.DeviceOpticalMetricRepository
	configSnaps    domain.DeviceConfigSnapshotRepository
	enc            *cryptoutil.Encryptor
	activity       domain.ActivityLogRepository
	tasks          domain.TaskCreator
	fileRepo       domain.FileRepository
	storage        domain.ObjectStorage
}

func NewService(
	devices domain.DeviceRepository,
	vendorOUIs domain.VendorOUIRepository,
	deviceModels domain.DeviceModelRepository,
	refs domain.RefRepository,
	deviceParams domain.DeviceParameterRepository,
	deviceEvents domain.DeviceEventRepository,
	opticalMetrics domain.DeviceOpticalMetricRepository,
	configSnaps domain.DeviceConfigSnapshotRepository,
	enc *cryptoutil.Encryptor,
	activity domain.ActivityLogRepository,
	tasks domain.TaskCreator,
	fileRepo domain.FileRepository,
	storage domain.ObjectStorage,
) *Service {
	return &Service{
		devices: devices, vendorOUIs: vendorOUIs, deviceModels: deviceModels, refs: refs,
		deviceParams: deviceParams, deviceEvents: deviceEvents, opticalMetrics: opticalMetrics,
		configSnaps: configSnaps, enc: enc, activity: activity, tasks: tasks,
		fileRepo: fileRepo, storage: storage,
	}
}

func (s *Service) Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.Device, error) {
	d, err := s.devices.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireTenantScope(actor, d.TenantID); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Service) CreateConfigSnapshot(ctx context.Context, deviceID uint64) error {
	params, err := s.deviceParams.ListByDevice(ctx, deviceID, "")
	if err != nil {
		return err
	}

	snapMap := make(map[string]any)
	for _, p := range params {
		if p.ParameterValue != nil {
			snapMap[p.ParameterName] = *p.ParameterValue
		}
	}
	
	// Convert map to JSON
	b, err := json.Marshal(snapMap)
	if err != nil {
		return err
	}

	snap := &domain.DeviceConfigSnapshot{
		DeviceID:     deviceID,
		SnapshotData: domain.JSONRawMessage(b),
		CreatedAt:    time.Now(),
	}
	
	return s.configSnaps.Create(ctx, snap)
}

func (s *Service) ListConfigSnapshots(ctx context.Context, actor domain.Actor, deviceID uint64, p domain.Pagination) ([]domain.DeviceConfigSnapshot, int, error) {
	if _, err := s.Get(ctx, actor, deviceID); err != nil {
		return nil, 0, err
	}
	return s.configSnaps.ListByDevice(ctx, deviceID, p)
}

func (s *Service) List(ctx context.Context, actor domain.Actor, f domain.DeviceFilter, p domain.Pagination) ([]domain.Device, int, error) {
	if !actor.IsSuperadmin() {
		tid, err := auth.ScopedTenantFilter(actor)
		if err != nil {
			return nil, 0, err
		}
		f.TenantID = tid
	}
	return s.devices.List(ctx, f, p)
}

// DeviceStats — agregat untuk dashboard analitik (ROADMAP.md Fase 1).
type DeviceStats struct {
	ByStatus []domain.DeviceStatusCount `json:"by_status"`
	ByVendor []domain.DeviceVendorCount `json:"by_vendor"`
}

func (s *Service) Stats(ctx context.Context, actor domain.Actor) (*DeviceStats, error) {
	tenantID, err := auth.ScopedTenantFilter(actor)
	if err != nil {
		return nil, err
	}
	byStatus, err := s.devices.CountByStatus(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	byVendor, err := s.devices.CountByVendor(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return &DeviceStats{ByStatus: byStatus, ByVendor: byVendor}, nil
}

// PlatformStats — sama seperti Stats tapi SELALU lintas seluruh tenant, tanpa
// perlu domain.Actor sama sekali. Dipakai HANYA oleh internal/metrics
// (endpoint observability /metrics, ROADMAP.md Fase 2) yang memang platform-
// level, bukan endpoint per-tenant milik BSS/portal NOC. Sengaja method
// terpisah, bukan Stats dipanggil dengan actor superadmin palsu — supaya
// domain.Actor/RBAC tetap murni cuma untuk jalur yang benar-benar butuh
// otorisasi (temuan acs-code-reviewer, review fitur observability).
func (s *Service) PlatformStats(ctx context.Context) (*DeviceStats, error) {
	byStatus, err := s.devices.CountByStatus(ctx, nil)
	if err != nil {
		return nil, err
	}
	byVendor, err := s.devices.CountByVendor(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &DeviceStats{ByStatus: byStatus, ByVendor: byVendor}, nil
}

type UpdateDeviceInput struct {
	Notes                     *string
	ConnectionRequestURL      *string
	ConnectionRequestUsername *string
	// ConnectionRequestPassword adalah input plaintext dari caller REST API,
	// dienkripsi sebelum disimpan — tidak pernah ditulis apa adanya ke DB/log
	// (CLAUDE.md: kredensial tidak boleh di-log/disimpan plaintext).
	ConnectionRequestPassword *string
	Latitude                  *float64
	Longitude                 *float64
}

func (s *Service) Update(ctx context.Context, actor domain.Actor, id uint64, in UpdateDeviceInput) (*domain.Device, error) {
	d, err := s.Get(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if in.Notes != nil {
		d.Notes = in.Notes
	}
	if in.Latitude != nil {
		d.Latitude = in.Latitude
	}
	if in.Longitude != nil {
		d.Longitude = in.Longitude
	}
	if in.ConnectionRequestURL != nil {
		d.ConnectionRequestURL = in.ConnectionRequestURL
	}
	if in.ConnectionRequestUsername != nil {
		d.ConnectionRequestUsername = in.ConnectionRequestUsername
	}
	if in.ConnectionRequestPassword != nil {
		enc, err := s.enc.Encrypt(*in.ConnectionRequestPassword)
		if err != nil {
			return nil, err
		}
		d.ConnectionRequestPasswordEnc = enc
	}
	d.UpdatedBy = actor.UserIDPtr()
	if err := s.devices.Update(ctx, d); err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "UPDATE_DEVICE", EntityType: "device", EntityID: &id,
	})
	return d, nil
}

// InformDeviceInfo adalah field DeviceId + info tambahan dari Inform CWMP
// yang dipakai untuk upsert inventory (dipanggil dari usecase/session).
type InformDeviceInfo struct {
	OUI             string
	SerialNumber    string
	ProductClass    string
	SoftwareVersion string
	HardwareVersion string
	RemoteIP        string
	// TenantID hasil resolusi kredensial Inform (usecase/session). Dipakai
	// saat device baru dibuat, DAN untuk "menyembuhkan" device lama yang
	// belum punya tenant_id (mis. dibuat sebelum kredensial Inform wajib) —
	// sekali sembuh, klaim terkunci: authenticateInform menolak tenant lain
	// yang beda begitu tenant_id device tidak lagi nil (lihat usecase/session
	// dan ROADMAP.md soal window transisi untuk device orphan yang sudah ada).
	// Tidak pernah menimpa tenant_id yang SUDAH ter-assign.
	TenantID *uint64
	// ExistingDevice, bila diisi, adalah hasil GetByOUISerial yang sudah
	// dilakukan authenticateInform — menghindari query devices duplikat pada
	// setiap Inform (device_id ini adalah hot path, TECH.md §9).
	ExistingDevice *domain.Device
}

// FindOrCreateFromInform meng-upsert device berdasarkan OUI+SerialNumber
// (unique key devices.uq_devices_oui_serial). Device baru otomatis dicoba
// di-resolve vendor/model-nya dari vendor_ouis/device_models (tetap
// UNREGISTERED bila tidak match — inventory tetap tercatat, lihat FR-15).
func (s *Service) FindOrCreateFromInform(ctx context.Context, info InformDeviceInfo) (dev *domain.Device, isNew bool, err error) {
	now := time.Now()

	existing := info.ExistingDevice
	if existing == nil {
		lookup, lookupErr := s.devices.GetByOUISerial(ctx, info.OUI, info.SerialNumber)
		if lookupErr != nil && !errors.Is(lookupErr, domain.ErrNotFound) {
			return nil, false, lookupErr
		}
		existing = lookup
	}

	if existing != nil {
		existing.SoftwareVersion = &info.SoftwareVersion
		existing.HardwareVersion = &info.HardwareVersion
		existing.IPAddress = &info.RemoteIP
		existing.LastInformAt = &now
		if existing.TenantID == nil && info.TenantID != nil {
			existing.TenantID = info.TenantID
		}
		if err := s.devices.Update(ctx, existing); err != nil {
			return nil, false, err
		}
		return existing, false, nil
	}

	unregisteredStatus, err := s.refs.GetByCode(ctx, domain.RefTableDeviceStatus, domain.DeviceStatusUnregistered)
	if err != nil {
		return nil, false, err
	}

	d := &domain.Device{
		DeviceUUID:      uuid.NewString(),
		TenantID:        info.TenantID,
		DeviceStatusID:  unregisteredStatus.ID,
		OUI:             &info.OUI,
		SerialNumber:    info.SerialNumber,
		ProductClass:    &info.ProductClass,
		SoftwareVersion: &info.SoftwareVersion,
		HardwareVersion: &info.HardwareVersion,
		IPAddress:       &info.RemoteIP,
		LastInformAt:    &now,
	}

	if vOUI, err := s.vendorOUIs.GetByOUI(ctx, info.OUI); err == nil {
		d.VendorID = &vOUI.VendorID
		if dm, err := s.deviceModels.FindByVendorAndProductClass(ctx, vOUI.VendorID, info.ProductClass); err == nil {
			d.DeviceModelID = &dm.ID
		}
	}

	if err := s.devices.Create(ctx, d); err != nil {
		return nil, false, err
	}
	return d, true, nil
}

func (s *Service) MarkOnline(ctx context.Context, deviceID uint64) error {
	onlineStatus, err := s.refs.GetByCode(ctx, domain.RefTableDeviceStatus, domain.DeviceStatusOnline)
	if err != nil {
		return err
	}
	return s.devices.UpdateStatus(ctx, deviceID, onlineStatus.ID, nil)
}

func (s *Service) TouchLastBootEvent(ctx context.Context, d *domain.Device, at time.Time) error {
	d.LastBootEventAt = &at
	return s.devices.Update(ctx, d)
}

// MarkStaleOffline menandai OFFLINE device yang tidak Inform lagi melewati
// threshold (FR-22). Dipanggil periodik dari goroutine di cmd/acsd — lihat
// TECH.md §9 (app server stateless, aman dijalankan dari instance manapun).
func (s *Service) MarkStaleOffline(ctx context.Context, threshold time.Duration) (int64, error) {
	offlineStatus, err := s.refs.GetByCode(ctx, domain.RefTableDeviceStatus, domain.DeviceStatusOffline)
	if err != nil {
		return 0, err
	}
	return s.devices.MarkStaleOffline(ctx, offlineStatus.ID, time.Now().Add(-threshold))
}

func (s *Service) ListParameters(ctx context.Context, actor domain.Actor, deviceID uint64, prefix string) ([]domain.DeviceParameter, error) {
	if _, err := s.Get(ctx, actor, deviceID); err != nil {
		return nil, err
	}
	return s.deviceParams.ListByDevice(ctx, deviceID, prefix)
}

func (s *Service) ListEvents(ctx context.Context, actor domain.Actor, deviceID uint64, p domain.Pagination) ([]domain.DeviceEvent, int, error) {
	if _, err := s.Get(ctx, actor, deviceID); err != nil {
		return nil, 0, err
	}
	return s.deviceEvents.ListByDevice(ctx, deviceID, p)
}

func (s *Service) LatestOpticalMetric(ctx context.Context, actor domain.Actor, deviceID uint64) (*domain.DeviceOpticalMetric, error) {
	if _, err := s.Get(ctx, actor, deviceID); err != nil {
		return nil, err
	}
	return s.opticalMetrics.Latest(ctx, deviceID)
}

func (s *Service) ListOpticalMetrics(ctx context.Context, actor domain.Actor, deviceID uint64, p domain.Pagination) ([]domain.DeviceOpticalMetric, int, error) {
	if _, err := s.Get(ctx, actor, deviceID); err != nil {
		return nil, 0, err
	}
	return s.opticalMetrics.ListByDevice(ctx, deviceID, p)
}

// ListActivity — timeline audit trail per device (ROADMAP.md Fase 1): aksi
// admin/operator yang tercatat langsung terhadap record device ini (update,
// apply provisioning profile, match zero-touch). Histori event CWMP
// (BOOT/PERIODIC/dst) dan task RPC punya tab terpisah (ListEvents/tasks) —
// timeline ini spesifik "siapa mengubah apa", bukan duplikasi keduanya.
func (s *Service) ListActivity(ctx context.Context, actor domain.Actor, deviceID uint64, p domain.Pagination) ([]domain.ActivityLog, int, error) {
	if _, err := s.Get(ctx, actor, deviceID); err != nil {
		return nil, 0, err
	}
	return s.activity.ListByEntity(ctx, "device", deviceID, p)
}

// TriggerConnectionRequest mengirim Connection Request HTTP ke CPE (RFC\
// TR-069 §3.2.2): ACS meng-GET url connection_request_url device dgn Basic\
// Auth — CPE membalas dgn Inform dalam waktu singkat. Dipakai operator NOC\
// untuk memaksa device offline agar segera melaporkan diri tanpa menunggu\
// periodic inform interval (biasanya 15-60 menit).\
//\
// Berbeda dari task (task mengantre di DB, dieksekusi SAAT sesi CWMP aktif):\
// Connection Request justru diperlukan SEBELUM sesi ada, untuk membuka sesi.\
//\
// Error domain.ErrInvalidInput dikembalikan bila device tidak punya\
// connection_request_url terkonfigurasi (device lama / device di balik NAT\
// yang tidak mengekspos URL-nya ke ACS).
func (s *Service) TriggerConnectionRequest(ctx context.Context, actor domain.Actor, deviceID uint64) error {
	d, err := s.Get(ctx, actor, deviceID)
	if err != nil {
		return err
	}
	if d.ConnectionRequestURL == nil || *d.ConnectionRequestURL == "" {
		return fmt.Errorf("%w: device tidak memiliki connection_request_url terkonfigurasi", domain.ErrInvalidInput)
	}

	var username, password string
	if d.ConnectionRequestUsername != nil {
		username = *d.ConnectionRequestUsername
	}
	if len(d.ConnectionRequestPasswordEnc) > 0 {
		plain, err := s.enc.Decrypt(d.ConnectionRequestPasswordEnc)
		if err != nil {
			return fmt.Errorf("triggerConnectionRequest: gagal dekripsi password: %w", err)
		}
		password = plain
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, *d.ConnectionRequestURL, nil)
	if err != nil {
		return fmt.Errorf("%w: URL connection request tidak valid: %v", domain.ErrInvalidInput, err)
	}
	if username != "" {
		req.SetBasicAuth(username, password)
	}

	// Timeout singkat: respons connection request seharusnya sangat cepat
	// (CPE hanya perlu menerima request dan memulai Inform, tidak return body).
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	
	// Jika gagal via TCP (mis. timeout karena NAT), fallback ke STUN UDP (TR-111).
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp != nil {
			resp.Body.Close()
		}
		
		// Fallback ke UDP Connection Request (STUN/NAT Traversal)
		udpParam, paramErr := s.deviceParams.Get(ctx, deviceID, "InternetGatewayDevice.ManagementServer.UDPConnectionRequestAddress")
		if paramErr == nil && udpParam != nil && udpParam.ParameterValue != nil && *udpParam.ParameterValue != "" {
			udpErr := s.sendUDPConnectionRequest(ctx, *udpParam.ParameterValue, username, password)
			if udpErr == nil {
				_ = s.activity.Record(ctx, &domain.ActivityLog{
					UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
					Action: "TRIGGER_CONNECTION_REQUEST_UDP", EntityType: "device", EntityID: &deviceID,
				})
				return nil
			}
			s.log().Warn("cwmp: UDP Connection Request (STUN fallback) gagal", "device_id", deviceID, "error", udpErr)
		}

		if err != nil {
			return fmt.Errorf("triggerConnectionRequest: gagal menghubungi device: %w", err)
		}
		return fmt.Errorf("%w: device menolak connection request (HTTP %d)", domain.ErrInvalidInput, resp.StatusCode)
	}
	defer resp.Body.Close()

	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "TRIGGER_CONNECTION_REQUEST", EntityType: "device", EntityID: &deviceID,
	})
	return nil
}

// sendUDPConnectionRequest mengirim TR-111 UDP Connection Request
func (s *Service) sendUDPConnectionRequest(ctx context.Context, udpURL, username, password string) error {
	u, err := url.Parse(udpURL)
	if err != nil {
		return err
	}
	host := u.Host
	if host == "" {
		host = udpURL // Sometimes stored as raw IP:Port
	}
	
	addr, err := net.ResolveUDPAddr("udp", host)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	ts := time.Now().Unix()
	id := uuid.New().String()
	
	// Signature: SHA1(id + ts + password)
	h := sha1.New()
	h.Write([]byte(fmt.Sprintf("%s%d%s", id, ts, password)))
	sig := hex.EncodeToString(h.Sum(nil))

	// Format HTTP GET request di atas UDP
	reqLine := fmt.Sprintf("GET ?ts=%d&id=%s&un=%s&sig=%s HTTP/1.1\r\nHost: %s\r\n\r\n", ts, id, username, sig, host)
	_, err = conn.Write([]byte(reqLine))
	return err
}

func (s *Service) Reboot(ctx context.Context, actor domain.Actor, deviceID uint64) error {
	d, err := s.Get(ctx, actor, deviceID)
	if err != nil {
		return err
	}
	_, err = s.tasks.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID: d.ID,
		TaskType: domain.TaskTypeReboot,
		Priority: 10,
	})
	if err == nil {
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
			Action: "REBOOT_DEVICE", EntityType: "device", EntityID: &deviceID,
		})
	}
	return err
}

func (s *Service) FactoryReset(ctx context.Context, actor domain.Actor, deviceID uint64) error {
	d, err := s.Get(ctx, actor, deviceID)
	if err != nil {
		return err
	}
	_, err = s.tasks.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID: d.ID,
		TaskType: domain.TaskTypeFactoryReset,
		Priority: 10,
	})
	if err == nil {
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
			Action: "FACTORY_RESET_DEVICE", EntityType: "device", EntityID: &deviceID,
		})
	}
	return err
}

func (s *Service) PushFile(ctx context.Context, actor domain.Actor, deviceID uint64, fileID uint64) error {
	d, err := s.Get(ctx, actor, deviceID)
	if err != nil {
		return err
	}
	f, err := s.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return err
	}
	if f.TenantID != nil && d.TenantID != nil && *f.TenantID != *d.TenantID {
		return domain.ErrForbidden
	}

	downloadURL, err := s.storage.PresignedGetURL(ctx, f.StorageKey, 1*time.Hour)
	if err != nil {
		return fmt.Errorf("pushFile: gagal membuat presigned URL: %w", err)
	}

	tr069FileType := "3 Vendor Configuration File"
	if f.FileType == domain.FileTypeFirmware {
		tr069FileType = "1 Firmware Upgrade Image"
	} else if f.FileType == domain.FileTypeWebContent {
		tr069FileType = "2 Web Content"
	}

	_, err = s.tasks.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID: d.ID,
		TaskType: domain.TaskTypeDownload,
		Priority: 5,
		Parameters: map[string]interface{}{
			"file_type":        tr069FileType,
			"url":              downloadURL,
			"target_file_name": f.FileName,
			"file_size":        f.FileSizeBytes,
		},
	})
	if err == nil {
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
			Action: "PUSH_FILE", EntityType: "device", EntityID: &deviceID,
		})
	}
	return err
}

func (s *Service) AddObject(ctx context.Context, actor domain.Actor, deviceID uint64, objectName string) error {
	d, err := s.Get(ctx, actor, deviceID)
	if err != nil {
		return err
	}
	_, err = s.tasks.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID: d.ID,
		TaskType: domain.TaskTypeAddObject,
		Priority: 5,
		Parameters: map[string]interface{}{
			"object_name": objectName,
		},
	})
	if err == nil {
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
			Action: "ADD_OBJECT", EntityType: "device", EntityID: &deviceID,
			Details: map[string]interface{}{"object_name": objectName},
		})
	}
	return err
}

func (s *Service) DeleteObject(ctx context.Context, actor domain.Actor, deviceID uint64, objectName string) error {
	d, err := s.Get(ctx, actor, deviceID)
	if err != nil {
		return err
	}
	_, err = s.tasks.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID: d.ID,
		TaskType: domain.TaskTypeDeleteObject,
		Priority: 5,
		Parameters: map[string]interface{}{
			"object_name": objectName,
		},
	})
	if err == nil {
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
			Action: "DELETE_OBJECT", EntityType: "device", EntityID: &deviceID,
			Details: map[string]interface{}{"object_name": objectName},
		})
	}
	return err
}

func (s *Service) GetParameterNames(ctx context.Context, actor domain.Actor, deviceID uint64, path string, nextLevel bool) error {
	d, err := s.Get(ctx, actor, deviceID)
	if err != nil {
		return err
	}
	_, err = s.tasks.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID: d.ID,
		TaskType: domain.TaskTypeGetParameterNames,
		Priority: 5,
		Parameters: map[string]interface{}{
			"path":       path,
			"next_level": nextLevel,
		},
	})
	if err == nil {
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
			Action: "GET_PARAMETER_NAMES", EntityType: "device", EntityID: &deviceID,
			Details: map[string]interface{}{"path": path, "next_level": nextLevel},
		})
	}
	return err
}

func (s *Service) GetParameterValues(ctx context.Context, actor domain.Actor, deviceID uint64, parameterNames []string) error {
	d, err := s.Get(ctx, actor, deviceID)
	if err != nil {
		return err
	}
	_, err = s.tasks.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID: d.ID,
		TaskType: domain.TaskTypeGetParameterValues,
		Priority: 5,
		Parameters: map[string]interface{}{
			"names": parameterNames,
		},
	})
	if err == nil {
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
			Action: "GET_PARAMETER_VALUES", EntityType: "device", EntityID: &deviceID,
			Details: map[string]interface{}{"count": len(parameterNames)},
		})
	}
	return err
}

func (s *Service) SetParameterValues(ctx context.Context, actor domain.Actor, deviceID uint64, values map[string]string) error {
	d, err := s.Get(ctx, actor, deviceID)
	if err != nil {
		return err
	}
	_, err = s.tasks.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID: d.ID,
		TaskType: domain.TaskTypeSetParameterValues,
		Priority: 5,
		Parameters: map[string]interface{}{
			"values": values,
		},
	})
	if err == nil {
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
			Action: "SET_PARAMETER_VALUES", EntityType: "device", EntityID: &deviceID,
			Details: map[string]interface{}{"count": len(values)},
		})
	}
	return err
}
