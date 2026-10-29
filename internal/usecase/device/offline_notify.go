package device

import (
	"context"
	"fmt"
	"strings"
	"time"

	"acs/internal/domain"
)

// maxSerialsInOfflineNotice = jumlah serial yang disebut di isi notifikasi ringkasan; selebihnya hanya dihitung.
const maxSerialsInOfflineNotice = 5

// WithWebhooks mengaktifkan webhook DEVICE_OFFLINE untuk BSS/NMS (satu event per perangkat yang ditandai offline).
func (s *Service) WithWebhooks(w domain.WebhookEnqueuer) *Service {
	s.webhooks = w
	return s
}

// announceOffline memberitahukan perangkat yang baru ditandai OFFLINE oleh penyapu, di luar siaran realtime:
//   - webhook DEVICE_OFFLINE: satu per perangkat (BSS membuka tiket per pelanggan). Satu sapuan dibatasi staleOfflineBatch perangkat
//     di repository, jadi jumlah event per tick terbatas; sisanya diselesaikan tick berikutnya.
//   - notifikasi DEVICE_OFFLINE: DIRINGKAS satu per tenant per sapuan, bukan satu per perangkat, supaya pemadaman listrik massal
//     tidak membanjiri kotak masuk ADMIN/NOC. Kegagalan keduanya tidak menggagalkan sapuan (hanya diabaikan, seperti notifikasi lain).
//
// Perangkat tanpa tenant tidak punya penerima dan dilewati (sama seperti siaran WebSocket).
func (s *Service) announceOffline(ctx context.Context, moved []domain.StaleDevice, now time.Time) {
	if s.webhooks == nil && s.notifier == nil {
		return
	}
	var order []uint64
	byTenant := map[uint64][]domain.StaleDevice{}
	for _, d := range moved {
		if d.TenantID == nil {
			continue
		}
		tid := *d.TenantID
		if _, ok := byTenant[tid]; !ok {
			order = append(order, tid)
		}
		byTenant[tid] = append(byTenant[tid], d)
		if s.webhooks != nil {
			var last any
			if d.LastInformAt != nil {
				last = d.LastInformAt.UTC().Format(time.RFC3339)
			}
			t := tid
			_ = s.webhooks.Enqueue(ctx, domain.WebhookEventDeviceOffline, &t, map[string]any{
				"device_uuid":    d.DeviceUUID,
				"serial_number":  d.SerialNumber,
				"last_inform_at": last,
				"detected_at":    now.UTC().Format(time.RFC3339),
			})
		}
	}
	if s.notifier == nil {
		return
	}
	for _, tid := range order {
		tenantID := tid
		_ = s.notifier.Notify(ctx, offlineNotice(&tenantID, byTenant[tid]))
	}
}

// offlineNotice menyusun satu notifikasi ringkasan untuk perangkat yang baru offline pada satu tenant (devs tidak kosong).
func offlineNotice(tenantID *uint64, devs []domain.StaleDevice) domain.NotificationEvent {
	if len(devs) == 1 {
		d := devs[0]
		id := d.ID
		return domain.NotificationEvent{
			TypeCode: domain.NotifDeviceOffline, TenantID: tenantID,
			Title:      "Perangkat offline: " + d.SerialNumber,
			Body:       fmt.Sprintf("Perangkat %s berhenti melapor dan ditandai offline. Buka detail perangkat untuk melihat penyebab yang mungkin.", d.SerialNumber),
			Link:       fmt.Sprintf("/devices/%d", d.ID),
			EntityType: "device", EntityID: &id,
		}
	}
	shown := devs
	if len(shown) > maxSerialsInOfflineNotice {
		shown = shown[:maxSerialsInOfflineNotice]
	}
	names := make([]string, len(shown))
	for i, d := range shown {
		names[i] = d.SerialNumber
	}
	body := fmt.Sprintf("%d perangkat berhenti melapor dan ditandai offline: %s", len(devs), strings.Join(names, ", "))
	if rest := len(devs) - len(shown); rest > 0 {
		body += fmt.Sprintf(" dan %d lainnya", rest)
	}
	body += ". Bila jumlahnya besar, periksa pemadaman listrik atau gangguan jaringan di area yang sama."
	return domain.NotificationEvent{
		TypeCode: domain.NotifDeviceOffline, TenantID: tenantID,
		Title: fmt.Sprintf("%d perangkat offline", len(devs)),
		Body:  body,
		Link:  "/devices",
	}
}
