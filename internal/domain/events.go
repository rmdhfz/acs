package domain

type EventPublisher interface {
	BroadcastEvent(eventType string, payload interface{})
	BroadcastToTenant(tenantID int64, eventType string, payload interface{})
}
