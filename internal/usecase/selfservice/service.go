// Package selfservice adalah usecase portal self-service pelanggan (role
// ENDUSER, PRD.md §4.2). Scope akses BUKAN tenant seperti role internal,
// melainkan daftar device yang dipetakan eksplisit ke akun lewat user_devices
// (migrations/0020). Semua operasi memverifikasi kepemilikan device lebih
// dulu (ownership check), tidak mengandalkan RBAC tenant.
package selfservice

import (
	"context"
	"strings"

	"acs/internal/domain"
	"acs/internal/usecase/task"
)

type Service struct {
	userDevices domain.UserDeviceRepository
	devices     domain.DeviceRepository
	refs        domain.RefRepository
	tasks       *task.Service
	activity    domain.ActivityLogRepository
}

func NewService(userDevices domain.UserDeviceRepository, devices domain.DeviceRepository, refs domain.RefRepository, tasks *task.Service, activity domain.ActivityLogRepository) *Service {
	return &Service{userDevices: userDevices, devices: devices, refs: refs, tasks: tasks, activity: activity}
}

// statusCode memetakan device_status_id -> kode ref_device_status.
func (s *Service) statusCode(ctx context.Context, id uint64) string {
	if r, err := s.refs.GetByID(ctx, domain.RefTableDeviceStatus, id); err == nil {
		return r.Code
	}
	return ""
}

// requireOwnership mengembalikan device bila (dan hanya bila) dipetakan ke akun actor.
func (s *Service) requireOwnership(ctx context.Context, actor domain.Actor, deviceID uint64) (*domain.Device, error) {
	if actor.UserID == 0 {
		return nil, domain.ErrForbidden
	}
	ok, err := s.userDevices.IsMapped(ctx, actor.UserID, deviceID)
	if err != nil {
		return nil, err
	}
	if !ok {
		// 404, bukan 403 — jangan bocorkan keberadaan device milik orang lain.
		return nil, domain.ErrNotFound
	}
	return s.devices.GetByID(ctx, deviceID)
}

// SelfServiceDevice adalah tampilan device yang aman untuk pelanggan akhir —
// tanpa kredensial, tanpa detail internal (tenant, profil, dsb).
type SelfServiceDevice struct {
	ID              uint64  `json:"id"`
	SerialNumber    string  `json:"serial_number"`
	Model           *string `json:"model"`
	SoftwareVersion *string `json:"software_version"`
	Status          string  `json:"status"`
	Online          bool    `json:"online"`
}

func (s *Service) toSelfServiceDevice(ctx context.Context, d *domain.Device) SelfServiceDevice {
	status := s.statusCode(ctx, d.DeviceStatusID)
	out := SelfServiceDevice{
		ID:              d.ID,
		SerialNumber:    d.SerialNumber,
		SoftwareVersion: d.SoftwareVersion,
		Status:          status,
		Online:          status == domain.DeviceStatusOnline,
	}
	if d.ProductClass != nil && *d.ProductClass != "" {
		out.Model = d.ProductClass
	}
	return out
}

func (s *Service) ListMyDevices(ctx context.Context, actor domain.Actor) ([]SelfServiceDevice, error) {
	if actor.UserID == 0 {
		return nil, domain.ErrForbidden
	}
	ids, err := s.userDevices.DeviceIDsForUser(ctx, actor.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]SelfServiceDevice, 0, len(ids))
	for _, id := range ids {
		d, err := s.devices.GetByID(ctx, id)
		if err != nil {
			if err == domain.ErrNotFound {
				continue
			}
			return nil, err
		}
		out = append(out, s.toSelfServiceDevice(ctx, d))
	}
	return out, nil
}

func (s *Service) GetMyDevice(ctx context.Context, actor domain.Actor, deviceID uint64) (SelfServiceDevice, error) {
	d, err := s.requireOwnership(ctx, actor, deviceID)
	if err != nil {
		return SelfServiceDevice{}, err
	}
	return s.toSelfServiceDevice(ctx, d), nil
}

// ChangeMyWiFi mengantre satu task SetParameterValues berisi SSID dan/atau
// passphrase WiFi. Logical key di-resolve ke path TR-069 per vendor device
// oleh task.Service (TECH.md §5) — bila band 5G belum dipetakan untuk vendor
// itu, error "belum dipetakan" diteruskan apa adanya (FR-12).
func (s *Service) ChangeMyWiFi(ctx context.Context, actor domain.Actor, deviceID uint64, ch domain.SelfServiceWiFiChange) (*domain.Task, error) {
	if _, err := s.requireOwnership(ctx, actor, deviceID); err != nil {
		return nil, err
	}
	ssid := strings.TrimSpace(ch.SSID)
	pass := strings.TrimSpace(ch.Passphrase)
	if ssid == "" && pass == "" {
		return nil, domain.ErrInvalidInput
	}
	if pass != "" && (len(pass) < 8 || len(pass) > 63) {
		return nil, domain.ErrInvalidInput // batas WPA-PSK
	}

	prefix := "wifi."
	switch strings.ToLower(ch.Band) {
	case "", "2g", "2.4g":
		prefix = "wifi."
	case "5g":
		prefix = "wifi.5g."
	default:
		return nil, domain.ErrInvalidInput
	}

	values := map[string]string{}
	if ssid != "" {
		values[prefix+"ssid"] = ssid
	}
	if pass != "" {
		values[prefix+"wpa_passphrase"] = pass
	}

	t, err := s.tasks.EnqueueSetParameterValues(ctx, actor, deviceID, values, 3)
	if err != nil {
		return nil, err
	}
	s.record(ctx, actor, deviceID, "SELF_SERVICE_WIFI_CHANGE")
	return t, nil
}

func (s *Service) RebootMyDevice(ctx context.Context, actor domain.Actor, deviceID uint64) error {
	if _, err := s.requireOwnership(ctx, actor, deviceID); err != nil {
		return err
	}
	if _, err := s.tasks.EnqueueReboot(ctx, actor, deviceID, 2); err != nil {
		return err
	}
	s.record(ctx, actor, deviceID, "SELF_SERVICE_REBOOT")
	return nil
}

func (s *Service) record(ctx context.Context, actor domain.Actor, deviceID uint64, action string) {
	if s.activity == nil {
		return
	}
	id := deviceID
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID:     actor.UserIDPtr(),
		TenantID:   actor.TenantID,
		Action:     action,
		EntityType: "device",
		EntityID:   &id,
	})
}
