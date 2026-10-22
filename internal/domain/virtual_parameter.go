package domain

import (
	"context"
	"regexp"
	"time"
)

// ---- Virtual parameter: nilai turunan dari parameter perangkat (migrations/0039, pkg/vexpr) ----

// Batas kolom virtual_parameters (satu sumber untuk usecase & dokumentasi).
const (
	VParamNameMaxLen        = 64
	VParamLabelMaxLen       = 255
	VParamDescriptionMaxLen = 1024
	VParamUnitMaxLen        = 32
	// VParamMaxPerScope membatasi jumlah definisi aktif per tenant (biaya evaluasi per perangkat berbanding lurus).
	VParamMaxPerScope = 50
)

var vparamNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// ValidVParamName: kunci mesin huruf kecil/angka/underscore, diawali huruf, maks 64.
func ValidVParamName(s string) bool { return vparamNameRe.MatchString(s) }

// VirtualParameter = definisi; nilainya tidak disimpan (dihitung saat dibaca). ID numerik tidak diekspos (UUID publik).
type VirtualParameter struct {
	ID          uint64    `db:"id" json:"-"`
	UUID        string    `db:"vp_uuid" json:"id"`
	TenantID    *uint64   `db:"tenant_id" json:"tenant_id"` // nil = global
	Name        string    `db:"name" json:"name"`
	Label       string    `db:"label" json:"label"`
	Description *string   `db:"description" json:"description"`
	Expression  string    `db:"expression" json:"expression"`
	Unit        *string   `db:"unit" json:"unit"`
	IsActive    bool      `db:"is_active" json:"is_active"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// VirtualValue = hasil evaluasi satu virtual parameter pada satu perangkat. Value nil = belum dapat dihitung (parameter sumber
// belum dilaporkan, pembagian nol, dst.), bukan string kosong.
type VirtualValue struct {
	Name  string  `json:"name"`
	Label string  `json:"label"`
	Unit  *string `json:"unit"`
	Value *string `json:"value"`
}

// VirtualParameterRepository — hanya query; keputusan (validasi, scope, batas) ada di usecase/vparam.
type VirtualParameterRepository interface {
	Create(ctx context.Context, v *VirtualParameter, by *uint64) error
	GetByUUID(ctx context.Context, uuid string) (*VirtualParameter, error)
	// List: tenantID nil = semua tenant (superadmin); selain itu definisi tenant itu DAN global. activeOnly menyaring is_active.
	List(ctx context.Context, tenantID *uint64, activeOnly bool, p Pagination) ([]VirtualParameter, int, error)
	// ListForTenant: seluruh definisi aktif yang berlaku untuk tenant (tenant + global); definisi tenant menimpa global bernama sama
	// (dikembalikan sudah dipilah). tenantID nil = hanya global.
	ListForTenant(ctx context.Context, tenantID *uint64) ([]VirtualParameter, error)
	// FindByName: definisi (aktif atau nonaktif) belum terhapus pada scope PERSIS ini (nil = global); ErrNotFound bila tak ada.
	FindByName(ctx context.Context, tenantID *uint64, name string) (*VirtualParameter, error)
	// CountInScope: jumlah definisi belum terhapus pada scope persis ini.
	CountInScope(ctx context.Context, tenantID *uint64) (int, error)
	Update(ctx context.Context, v *VirtualParameter, by *uint64) error
	SoftDelete(ctx context.Context, id, by uint64) error
}
