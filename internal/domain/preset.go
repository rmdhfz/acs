package domain

import "time"

type Preset struct {
	ID             uint64    `db:"id" json:"id"`
	TenantID       *uint64   `db:"tenant_id" json:"tenant_id,omitempty"`
	Name           string    `db:"name" json:"name"`
	Weight         int       `db:"weight" json:"weight"`
	Precondition   string    `db:"precondition" json:"precondition"`
	Configurations string    `db:"configurations" json:"configurations"`
	IsActive       bool      `db:"is_active" json:"is_active"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}
