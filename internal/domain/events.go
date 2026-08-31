package domain

type EventPublisher interface {
	BroadcastEvent(eventType string, payload interface{})
	BroadcastToTenant(tenantID uint64, eventType string, payload interface{})
}
