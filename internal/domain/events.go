package domain

import (
	"context"
	"time"
)

type EventPublisher interface {
	BroadcastEvent(eventType string, payload interface{})
	BroadcastToTenant(tenantID uint64, eventType string, payload interface{})
}

// RetentionRepository memangkas tabel LOG volume tinggi yang lebih tua dari
// cutoff (device_events, device_sessions, device_optical_metrics,
// webhook_deliveries). Tidak menyentuh data master maupun activity_logs.
type RetentionRepository interface {
	PruneLogs(ctx context.Context, cutoff time.Time) (map[string]int64, error)
}
