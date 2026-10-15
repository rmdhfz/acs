package domain

import (
	"context"
	"fmt"
	"strings"
)

// UsageCount adalah jumlah baris AKTIF (is_deleted = 0) di satu tabel yang masih
// merujuk sebuah entitas. Dipakai sebagai guard hapus: entitas master yang
// masih dirujuk data hidup tidak boleh di-soft-delete (akan mengorbankan data
// itu jadi yatim), cukup di-nonaktifkan (is_active = 0).
type UsageCount struct {
	Kind  string // nama sumber daya yang merujuk, mis. "device", "device_model"
	Count int
}

// usageConflictError adalah ErrConflict (errors.Is) dengan pesan sendiri.
// ErrConflict polos berbunyi "data sudah ada / konflik unique" — benar untuk
// duplicate key, tetapi MENYESATKAN untuk guard hapus; pesan guard harus
// langsung menjelaskan apa yang masih memakai entitas itu.
type usageConflictError struct{ msg string }

func (e *usageConflictError) Error() string        { return e.msg }
func (e *usageConflictError) Is(target error) bool { return target == ErrConflict }

// NewUsageConflict menyusun ErrConflict (HTTP 409) yang menuntun operator:
// apa yang masih memakai entitas itu. Dipakai bersama oleh semua usecase yang
// punya guard hapus supaya pesannya seragam. Pemanggil boleh menambah saran
// jalan keluar lewat fmt.Errorf("%w — ...", err).
func NewUsageConflict(what string, usage []UsageCount) error {
	parts := make([]string, 0, len(usage))
	for _, u := range usage {
		parts = append(parts, fmt.Sprintf("%d %s", u.Count, u.Kind))
	}
	return &usageConflictError{msg: fmt.Sprintf("%s masih dipakai oleh %s", what, strings.Join(parts, ", "))}
}

// UsageRepository menghitung referensi aktif ke entitas master. Hanya query
// agregat (COUNT) — keputusan "boleh dihapus atau tidak" ada di usecase.
// Hanya mengembalikan Kind dengan Count > 0 (slice kosong = tidak dipakai).
type UsageRepository interface {
	VendorUsage(ctx context.Context, vendorID uint64) ([]UsageCount, error)
	DeviceModelUsage(ctx context.Context, deviceModelID uint64) ([]UsageCount, error)
	// FirmwareUsage: zero-touch rule yang menunjuk firmware ini, rollout batch
	// yang belum terminal, dan job upgrade yang task-nya belum terminal.
	// Job/batch yang sudah selesai (riwayat) TIDAK menghalangi penghapusan.
	FirmwareUsage(ctx context.Context, firmwareID uint64) ([]UsageCount, error)
	// TenantUsage: user dan device aktif milik tenant.
	TenantUsage(ctx context.Context, tenantID uint64) ([]UsageCount, error)
}
