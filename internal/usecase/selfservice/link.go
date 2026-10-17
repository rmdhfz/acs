package selfservice

import (
	"context"
	"fmt"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
)

// Penautan akun ENDUSER <-> perangkat (tabel user_devices, migrations/0020). Tanpa ini portal
// "WiFi Saya" tidak punya jalur resmi untuk memberi pelanggan akses ke ONT miliknya.

// maxLinkedDevicesPerUser membatasi jumlah perangkat per akun pelanggan (anti-penyalahgunaan/salah klik).
const maxLinkedDevicesPerUser = 20

// WithUsers menyediakan akses ke akun untuk validasi tenant/role saat penautan.
func (s *Service) WithUsers(users domain.UserRepository) *Service {
	s.users = users
	return s
}

// LinkedDevice — ringkasan perangkat yang tertaut ke akun (untuk tampilan admin).
type LinkedDevice struct {
	ID           uint64  `json:"id"`
	SerialNumber string  `json:"serial_number"`
	Model        *string `json:"model"`
	Status       string  `json:"status"`
}

func requireAdmin(actor domain.Actor) error {
	if actor.IsSuperadmin() || actor.HasRole(domain.RoleAdmin) {
		return nil
	}
	return domain.ErrForbidden
}

// loadUser memuat akun target dan memastikan actor berhak atas tenant-nya.
func (s *Service) loadUser(ctx context.Context, actor domain.Actor, userID uint64) (*domain.User, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	if s.users == nil {
		return nil, fmt.Errorf("%w: penautan perangkat tidak dikonfigurasi", domain.ErrInvalidInput)
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireTenantScope(actor, u.TenantID); err != nil {
		return nil, err
	}
	return u, nil
}

// LinkDevice menautkan perangkat ke akun ENDUSER. Syarat: akun ber-role ENDUSER, akun & perangkat
// berada di tenant yang SAMA, dan actor berhak atas tenant itu (ADMIN tenant tsb / SUPERADMIN).
func (s *Service) LinkDevice(ctx context.Context, actor domain.Actor, userID, deviceID uint64) error {
	u, err := s.loadUser(ctx, actor, userID)
	if err != nil {
		return err
	}
	d, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return err
	}
	if err := auth.RequireTenantScope(actor, d.TenantID); err != nil {
		return err
	}
	if u.TenantID == nil || d.TenantID == nil || *u.TenantID != *d.TenantID {
		return fmt.Errorf("%w: akun dan perangkat harus berada di tenant yang sama", domain.ErrInvalidInput)
	}
	roles, err := s.users.RolesByUserID(ctx, userID)
	if err != nil {
		return err
	}
	isEndUser := false
	for _, r := range roles {
		if r == domain.RoleEndUser {
			isEndUser = true
		}
	}
	if !isEndUser {
		return fmt.Errorf("%w: hanya akun ber-role ENDUSER yang dapat ditautkan ke perangkat", domain.ErrInvalidInput)
	}
	mapped, err := s.userDevices.IsMapped(ctx, userID, deviceID)
	if err != nil {
		return err
	}
	if !mapped {
		ids, err := s.userDevices.DeviceIDsForUser(ctx, userID)
		if err != nil {
			return err
		}
		if len(ids) >= maxLinkedDevicesPerUser {
			return fmt.Errorf("%w: maksimum %d perangkat per akun pelanggan", domain.ErrQuotaExceeded, maxLinkedDevicesPerUser)
		}
	}
	if err := s.userDevices.Map(ctx, userID, deviceID, actor.UserIDPtr()); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: u.TenantID, Action: "LINK_USER_DEVICE", EntityType: "user", EntityID: &userID,
		Details: map[string]any{"device_id": deviceID},
	})
	return nil
}

// UnlinkDevice melepas tautan (idempoten). Perangkat boleh sudah dihapus.
func (s *Service) UnlinkDevice(ctx context.Context, actor domain.Actor, userID, deviceID uint64) error {
	u, err := s.loadUser(ctx, actor, userID)
	if err != nil {
		return err
	}
	if err := s.userDevices.Unmap(ctx, userID, deviceID); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: u.TenantID, Action: "UNLINK_USER_DEVICE", EntityType: "user", EntityID: &userID,
		Details: map[string]any{"device_id": deviceID},
	})
	return nil
}

// ListUserDevices — perangkat yang tertaut ke akun (hanya yang masih ada & se-tenant).
func (s *Service) ListUserDevices(ctx context.Context, actor domain.Actor, userID uint64) ([]LinkedDevice, error) {
	u, err := s.loadUser(ctx, actor, userID)
	if err != nil {
		return nil, err
	}
	ids, err := s.userDevices.DeviceIDsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]LinkedDevice, 0, len(ids))
	for _, id := range ids {
		d, err := s.devices.GetByID(ctx, id)
		if err != nil {
			if err == domain.ErrNotFound {
				continue
			}
			return nil, err
		}
		if u.TenantID == nil || d.TenantID == nil || *u.TenantID != *d.TenantID {
			continue
		}
		ld := LinkedDevice{ID: d.ID, SerialNumber: d.SerialNumber, Status: s.statusCode(ctx, d.DeviceStatusID)}
		if d.ProductClass != nil && *d.ProductClass != "" {
			ld.Model = d.ProductClass
		}
		out = append(out, ld)
	}
	return out, nil
}
