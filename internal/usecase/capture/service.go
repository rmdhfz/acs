// Package capture = capture sesi CWMP per perangkat untuk diagnosis kuirk firmware. Operator (ADMIN/SUPERADMIN) menyalakan
// capture untuk SATU perangkat selama maksimal 60 menit; selama itu pesan SOAP kedua arah disimpan dengan nilai rahasia
// DISAMARKAN sebelum menyentuh database (pkg/cwmpxml.Redact). Header HTTP tidak pernah disimpan.
package capture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
	"unicode/utf8"

	"acs/internal/domain"
	"acs/pkg/cwmpxml"
)

// DeviceGetter = device.Service.Get (sekaligus menegakkan scope tenant: lintas tenant -> ErrForbidden / ErrNotFound).
type DeviceGetter interface {
	Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.Device, error)
}

// cacheTTL — selisih paling lama antara menyalakan capture di replika lain dan replika ini mulai merekam.
const cacheTTL = 5 * time.Second

// ExportLimit — jumlah pesan maksimum satu ekspor.
const ExportLimit = domain.CaptureKeepPerDevice

type Service struct {
	repo     domain.CaptureRepository
	devices  DeviceGetter
	sessions domain.DeviceSessionRepository
	activity domain.ActivityLogRepository
	now      func() time.Time

	mu        sync.RWMutex
	active    map[uint64]time.Time // device id -> expires_at
	refreshed time.Time
}

func NewService(repo domain.CaptureRepository, devices DeviceGetter, sessions domain.DeviceSessionRepository, activity domain.ActivityLogRepository) *Service {
	return &Service{repo: repo, devices: devices, sessions: sessions, activity: activity, now: time.Now, active: map[uint64]time.Time{}}
}

func canManage(actor domain.Actor) bool {
	return actor.IsSuperadmin() || actor.HasRole(domain.RoleAdmin)
}

// device memverifikasi hak (ADMIN/SUPERADMIN) dan scope tenant perangkat.
func (s *Service) device(ctx context.Context, actor domain.Actor, id uint64) (*domain.Device, error) {
	if !canManage(actor) {
		return nil, domain.ErrForbidden
	}
	return s.devices.Get(ctx, actor, id)
}

func (s *Service) audit(ctx context.Context, actor domain.Actor, d *domain.Device, action string, details map[string]any) {
	if s.activity == nil {
		return
	}
	id := d.ID
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: d.TenantID, Action: action, EntityType: "device", EntityID: &id, Details: details,
	})
}

// Enable menyalakan (atau memperpanjang) capture selama `minutes` (1..60).
func (s *Service) Enable(ctx context.Context, actor domain.Actor, deviceID uint64, minutes int) (*domain.CaptureStatus, error) {
	d, err := s.device(ctx, actor, deviceID)
	if err != nil {
		return nil, err
	}
	if minutes < 1 || minutes > domain.CaptureMaxMinutes {
		return nil, fmt.Errorf("%w: minutes harus 1-%d", domain.ErrInvalidInput, domain.CaptureMaxMinutes)
	}
	until := s.now().Add(time.Duration(minutes) * time.Minute)
	if err := s.repo.EnableUntil(ctx, deviceID, until, actor.UserIDPtr()); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.active[deviceID] = until
	s.mu.Unlock()
	s.audit(ctx, actor, d, "CAPTURE_ENABLED", map[string]any{"minutes": minutes})
	return &domain.CaptureStatus{Active: true, Until: &until}, nil
}

func (s *Service) Disable(ctx context.Context, actor domain.Actor, deviceID uint64) error {
	d, err := s.device(ctx, actor, deviceID)
	if err != nil {
		return err
	}
	if err := s.repo.Disable(ctx, deviceID, s.now()); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.active, deviceID)
	s.mu.Unlock()
	s.audit(ctx, actor, d, "CAPTURE_DISABLED", nil)
	return nil
}

func (s *Service) Status(ctx context.Context, actor domain.Actor, deviceID uint64) (*domain.CaptureStatus, error) {
	if _, err := s.device(ctx, actor, deviceID); err != nil {
		return nil, err
	}
	return s.repo.Status(ctx, deviceID, s.now())
}

func (s *Service) List(ctx context.Context, actor domain.Actor, deviceID uint64, p domain.Pagination) ([]domain.SessionCapture, int, error) {
	if _, err := s.device(ctx, actor, deviceID); err != nil {
		return nil, 0, err
	}
	return s.repo.List(ctx, deviceID, p)
}

func (s *Service) Get(ctx context.Context, actor domain.Actor, deviceID uint64, uuid string) (*domain.SessionCapture, error) {
	if _, err := s.device(ctx, actor, deviceID); err != nil {
		return nil, err
	}
	return s.repo.GetByUUID(ctx, deviceID, uuid)
}

// Export — pesan perangkat urut waktu naik, lengkap dengan body (sudah tersamarkan), untuk dijadikan fixture uji.
func (s *Service) Export(ctx context.Context, actor domain.Actor, deviceID uint64) ([]domain.SessionCapture, error) {
	d, err := s.device(ctx, actor, deviceID)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListAll(ctx, deviceID, ExportLimit)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor, d, "CAPTURE_EXPORTED", map[string]any{"messages": len(rows)})
	return rows, nil
}

// ---- jalur runtime (dipanggil handler CWMP; tanpa actor) ----

// refresh memuat ulang perangkat yang sedang di-capture; paling sering sekali per cacheTTL.
func (s *Service) refresh(ctx context.Context) {
	now := s.now()
	s.mu.RLock()
	fresh := now.Sub(s.refreshed) < cacheTTL
	s.mu.RUnlock()
	if fresh {
		return
	}
	ids, err := s.repo.ActiveDeviceIDs(ctx, now)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshed = now
	if err != nil {
		return // pertahankan cache lama; gagal baca tidak boleh mengganggu jalur CWMP
	}
	next := make(map[uint64]time.Time, len(ids))
	for _, id := range ids {
		// Baris di DB masih berlaku: anggap aktif sampai refresh berikutnya (diperpanjang di replika lain pun terbaca);
		// batas pasti dari Enable lokal dipakai bila lebih lama.
		next[id] = now.Add(cacheTTL)
		if exp, ok := s.active[id]; ok && exp.After(next[id]) {
			next[id] = exp
		}
	}
	s.active = next
}

// AnyActive — jalur cepat: false bila tidak ada satu pun perangkat yang di-capture (kasus umum; tanpa query berat).
func (s *Service) AnyActive(ctx context.Context) bool {
	s.refresh(ctx)
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := s.now()
	for _, exp := range s.active {
		if exp.After(now) {
			return true
		}
	}
	return false
}

// Active — apakah perangkat ini sedang di-capture.
func (s *Service) Active(ctx context.Context, deviceID uint64) bool {
	s.refresh(ctx)
	s.mu.RLock()
	defer s.mu.RUnlock()
	exp, ok := s.active[deviceID]
	return ok && exp.After(s.now())
}

// DeviceIDForToken memetakan token sesi CWMP ke perangkatnya (sesi dibuat saat Inform).
func (s *Service) DeviceIDForToken(ctx context.Context, token string) (uint64, bool) {
	if token == "" || s.sessions == nil {
		return 0, false
	}
	sess, err := s.sessions.GetByToken(ctx, token)
	if err != nil || sess == nil {
		return 0, false
	}
	return sess.DeviceID, true
}

// Record menyimpan satu pesan: rahasia disamarkan LEBIH DULU, lalu dipotong ke batas. Kegagalan hanya dikembalikan sebagai
// error untuk dicatat pemanggil — tidak boleh menggagalkan sesi CWMP.
func (s *Service) Record(ctx context.Context, deviceID uint64, token string, fromCPE bool, raw []byte, httpStatus int) error {
	redacted := cwmpxml.Redact(raw)
	c := &domain.SessionCapture{
		DeviceID: deviceID, FromCPE: fromCPE, RPCName: cwmpxml.BodyRPCName(raw), BodyBytes: uint32(len(redacted)),
	}
	if c.RPCName == "" {
		c.RPCName = "(kosong)"
	}
	if token != "" {
		h := sha256.Sum256([]byte(token))
		ref := hex.EncodeToString(h[:])[:8]
		c.SessionRef = &ref
	}
	if httpStatus > 0 && !fromCPE {
		st := uint16(httpStatus)
		c.HTTPStatus = &st
	}
	body := redacted
	if len(body) > domain.CaptureMaxBodyBytes {
		body = body[:domain.CaptureMaxBodyBytes]
		for len(body) > 0 && !utf8.Valid(body) { // jangan memotong di tengah karakter multibyte
			body = body[:len(body)-1]
		}
		c.BodyTruncated = true
	}
	c.Body = string(body)
	return s.repo.Insert(ctx, c, domain.CaptureKeepPerDevice, domain.CaptureMaxAge)
}
