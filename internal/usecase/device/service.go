// Package device menangani inventory & status CPE (TECH.md §11, FR-22/FR-23/FR-25).
package device

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
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
	enc            *cryptoutil.Encryptor
	activity       domain.ActivityLogRepository
}

func NewService(
	devices domain.DeviceRepository,
	vendorOUIs domain.VendorOUIRepository,
	deviceModels domain.DeviceModelRepository,
	refs domain.RefRepository,
	deviceParams domain.DeviceParameterRepository,
	deviceEvents domain.DeviceEventRepository,
	opticalMetrics domain.DeviceOpticalMetricRepository,
	enc *cryptoutil.Encryptor,
	activity domain.ActivityLogRepository,
) *Service {
	return &Service{
		devices: devices, vendorOUIs: vendorOUIs, deviceModels: deviceModels, refs: refs,
		deviceParams: deviceParams, deviceEvents: deviceEvents, opticalMetrics: opticalMetrics,
		enc: enc, activity: activity,
	}
}

func requireTenantScope(actor domain.Actor, resourceTenantID *uint64) error {
	if actor.IsSuperadmin() {
		return nil
	}
	if resourceTenantID == nil || actor.TenantID == nil || *actor.TenantID != *resourceTenantID {
		return domain.ErrForbidden
	}
	return nil
}

func (s *Service) Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.Device, error) {
	d, err := s.devices.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := requireTenantScope(actor, d.TenantID); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Service) List(ctx context.Context, actor domain.Actor, f domain.DeviceFilter, p domain.Pagination) ([]domain.Device, int, error) {
	if !actor.IsSuperadmin() {
		f.TenantID = actor.TenantID
	}
	return s.devices.List(ctx, f, p)
}

type UpdateDeviceInput struct {
	Notes                     *string
	ConnectionRequestURL      *string
	ConnectionRequestUsername *string
	// ConnectionRequestPassword adalah input plaintext dari caller REST API,
	// dienkripsi sebelum disimpan — tidak pernah ditulis apa adanya ke DB/log
	// (CLAUDE.md: kredensial tidak boleh di-log/disimpan plaintext).
	ConnectionRequestPassword *string
}

func (s *Service) Update(ctx context.Context, actor domain.Actor, id uint64, in UpdateDeviceInput) (*domain.Device, error) {
	d, err := s.Get(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if in.Notes != nil {
		d.Notes = in.Notes
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
