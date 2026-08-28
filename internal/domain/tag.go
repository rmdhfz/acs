package domain

import "time"

type Tag struct {
	ID        uint64    `db:"id" json:"id"`
	TenantID  *uint64   `db:"tenant_id" json:"tenant_id,omitempty"`
	Name      string    `db:"name" json:"name"`
	Color     *string   `db:"color" json:"color,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
