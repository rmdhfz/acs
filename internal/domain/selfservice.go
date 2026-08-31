package domain

import "context"

// UserDeviceRepository memetakan akun (biasanya role ENDUSER) ke device yang
// boleh diaksesnya di portal self-service (tabel user_devices, migrations/0020).
type UserDeviceRepository interface {
	// DeviceIDsForUser mengembalikan seluruh device_id yang dipetakan ke user.
	DeviceIDsForUser(ctx context.Context, userID uint64) ([]uint64, error)
	// IsMapped true bila (userID, deviceID) ada di user_devices.
	IsMapped(ctx context.Context, userID, deviceID uint64) (bool, error)
	// Map menautkan device ke user (idempoten).
	Map(ctx context.Context, userID, deviceID uint64, createdBy *uint64) error
	// Unmap melepas tautan (no-op bila tidak ada).
	Unmap(ctx context.Context, userID, deviceID uint64) error
}

// SelfServiceWiFiChange adalah payload perubahan WiFi dari portal pelanggan.
// Band "" berarti terapkan ke 2.4G dan 5G sekaligus.
type SelfServiceWiFiChange struct {
	SSID       string
	Passphrase string
	Band       string // "2g" | "5g" | ""
}
