// Package vparam = virtual parameter: nilai turunan yang dihitung dari parameter perangkat lewat ekspresi deklaratif (pkg/vexpr).
// Definisi disimpan sebagai data per tenant (migrations/0039); nilai dihitung saat dibaca dari device_parameters terkini dan
// tidak pernah disimpan, sehingga tidak ada data basi dan tidak ada tulis tambahan di jalur Inform.
package vparam

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
	"acs/pkg/vexpr"
)

// DeviceGetter = device.Service.Get (sekaligus menegakkan scope tenant).
type DeviceGetter interface {
	Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.Device, error)
}

type Service struct {
	repo     domain.VirtualParameterRepository
	devices  DeviceGetter
	params   domain.DeviceParameterRepository
	activity domain.ActivityLogRepository
}

func NewService(repo domain.VirtualParameterRepository, devices DeviceGetter, params domain.DeviceParameterRepository, activity domain.ActivityLogRepository) *Service {
	return &Service{repo: repo, devices: devices, params: params, activity: activity}
}

// Input = isi yang boleh diubah pengguna. TenantID hanya dipakai SUPERADMIN (nil = global); peran lain selalu tenant sendiri.
type Input struct {
	TenantID    *uint64
	Name        string
	Label       string
	Description *string
	Expression  string
	Unit        *string
	IsActive    *bool
}

// UpdateInput — partial: field nil = tidak diubah. Description/Unit "" = kosongkan.
type UpdateInput struct {
	Name        *string
	Label       *string
	Description *string
	Expression  *string
	Unit        *string
	IsActive    *bool
}

func (s *Service) audit(ctx context.Context, actor domain.Actor, tenantID *uint64, action string, v *domain.VirtualParameter) {
	if s.activity == nil {
		return
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: tenantID, Action: action, EntityType: "virtual_parameter",
		Details: map[string]any{"vparam_id": v.UUID, "name": v.Name},
	})
}

// validateExpression mengurai ekspresi dan menolak rujukan ke parameter rahasia: tampilan virtual parameter terbuka untuk semua
// peran staf, jadi ekspresi tidak boleh menjadi jalan membaca password/PSK yang disamarkan di tempat lain.
func validateExpression(expr string) (*vexpr.Expr, error) {
	e, err := vexpr.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("%w: ekspresi tidak valid — %s", domain.ErrInvalidInput, err.Error())
	}
	for _, ref := range e.Refs() {
		if domain.IsSensitiveParamName(ref) {
			return nil, fmt.Errorf("%w: ekspresi tidak boleh merujuk parameter rahasia (%s)", domain.ErrInvalidInput, ref)
		}
	}
	return e, nil
}

func cleanOptional(field string, p *string, max int) (*string, error) {
	if p == nil {
		return nil, nil
	}
	if strings.TrimSpace(*p) == "" {
		return nil, nil
	}
	v, err := domain.CleanText(field, *p, 1, max, false)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Service) Create(ctx context.Context, actor domain.Actor, in Input) (*domain.VirtualParameter, error) {
	scope := actor.TenantID
	if actor.IsSuperadmin() {
		scope = in.TenantID
	} else if actor.TenantID == nil {
		return nil, domain.ErrForbidden
	}
	name := strings.TrimSpace(in.Name)
	if !domain.ValidVParamName(name) {
		return nil, fmt.Errorf("%w: name harus huruf kecil/angka/underscore, diawali huruf, maks %d karakter", domain.ErrInvalidInput, domain.VParamNameMaxLen)
	}
	label, err := domain.CleanText("label", in.Label, 1, domain.VParamLabelMaxLen, false)
	if err != nil {
		return nil, err
	}
	desc, err := cleanOptional("description", in.Description, domain.VParamDescriptionMaxLen)
	if err != nil {
		return nil, err
	}
	unit, err := cleanOptional("unit", in.Unit, domain.VParamUnitMaxLen)
	if err != nil {
		return nil, err
	}
	expr := strings.TrimSpace(in.Expression)
	if _, err := validateExpression(expr); err != nil {
		return nil, err
	}
	if _, err := s.repo.FindByName(ctx, scope, name); err == nil {
		return nil, fmt.Errorf("%w: virtual parameter bernama %q sudah ada", domain.ErrConflict, name)
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if n, err := s.repo.CountInScope(ctx, scope); err != nil {
		return nil, err
	} else if n >= domain.VParamMaxPerScope {
		return nil, fmt.Errorf("%w: batas %d virtual parameter per tenant tercapai", domain.ErrInvalidInput, domain.VParamMaxPerScope)
	}
	v := &domain.VirtualParameter{TenantID: scope, Name: name, Label: label, Description: desc, Expression: expr, Unit: unit, IsActive: true}
	if in.IsActive != nil {
		v.IsActive = *in.IsActive
	}
	if err := s.repo.Create(ctx, v, actor.UserIDPtr()); err != nil {
		return nil, err
	}
	s.audit(ctx, actor, scope, "CREATE_VIRTUAL_PARAMETER", v)
	return v, nil
}

// readable: definisi tenant sendiri atau global (semua peran staf boleh membaca); selain itu ErrForbidden.
func readable(actor domain.Actor, v *domain.VirtualParameter) error {
	if actor.IsSuperadmin() || v.TenantID == nil {
		return nil
	}
	return auth.RequireTenantScope(actor, v.TenantID)
}

// writable: definisi tenant hanya oleh tenant itu; definisi GLOBAL (dipakai semua tenant) hanya SUPERADMIN — kalau tidak, ADMIN
// satu tenant bisa mengubah tampilan semua tenant.
func writable(actor domain.Actor, v *domain.VirtualParameter) error {
	if actor.IsSuperadmin() {
		return nil
	}
	if v.TenantID == nil {
		return domain.ErrForbidden
	}
	return auth.RequireTenantScope(actor, v.TenantID)
}

func (s *Service) Get(ctx context.Context, actor domain.Actor, uuid string) (*domain.VirtualParameter, error) {
	v, err := s.repo.GetByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if err := readable(actor, v); err != nil {
		return nil, err
	}
	return v, nil
}

// List: non-superadmin = tenant sendiri + global; tenantFilter hanya efektif untuk SUPERADMIN.
func (s *Service) List(ctx context.Context, actor domain.Actor, tenantFilter *uint64, p domain.Pagination) ([]domain.VirtualParameter, int, error) {
	tid := tenantFilter
	if !actor.IsSuperadmin() {
		var err error
		if tid, err = auth.ScopedTenantFilter(actor); err != nil {
			return nil, 0, err
		}
	}
	return s.repo.List(ctx, tid, false, p)
}

func (s *Service) Update(ctx context.Context, actor domain.Actor, uuid string, in UpdateInput) (*domain.VirtualParameter, error) {
	v, err := s.repo.GetByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if err := writable(actor, v); err != nil {
		return nil, err
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if !domain.ValidVParamName(name) {
			return nil, fmt.Errorf("%w: name harus huruf kecil/angka/underscore, diawali huruf, maks %d karakter", domain.ErrInvalidInput, domain.VParamNameMaxLen)
		}
		if name != v.Name {
			if other, ferr := s.repo.FindByName(ctx, v.TenantID, name); ferr == nil && other.ID != v.ID {
				return nil, fmt.Errorf("%w: virtual parameter bernama %q sudah ada", domain.ErrConflict, name)
			} else if ferr != nil && !errors.Is(ferr, domain.ErrNotFound) {
				return nil, ferr
			}
			v.Name = name
		}
	}
	if in.Label != nil {
		if v.Label, err = domain.CleanText("label", *in.Label, 1, domain.VParamLabelMaxLen, false); err != nil {
			return nil, err
		}
	}
	if in.Description != nil {
		if v.Description, err = cleanOptional("description", in.Description, domain.VParamDescriptionMaxLen); err != nil {
			return nil, err
		}
	}
	if in.Unit != nil {
		if v.Unit, err = cleanOptional("unit", in.Unit, domain.VParamUnitMaxLen); err != nil {
			return nil, err
		}
	}
	if in.Expression != nil {
		expr := strings.TrimSpace(*in.Expression)
		if _, err := validateExpression(expr); err != nil {
			return nil, err
		}
		v.Expression = expr
	}
	if in.IsActive != nil {
		v.IsActive = *in.IsActive
	}
	if err := s.repo.Update(ctx, v, actor.UserIDPtr()); err != nil {
		return nil, err
	}
	s.audit(ctx, actor, v.TenantID, "UPDATE_VIRTUAL_PARAMETER", v)
	return v, nil
}

func (s *Service) Delete(ctx context.Context, actor domain.Actor, uuid string) error {
	v, err := s.repo.GetByUUID(ctx, uuid)
	if err != nil {
		return err
	}
	if err := writable(actor, v); err != nil {
		return err
	}
	by := actor.UserIDPtr()
	if by == nil {
		return domain.ErrForbidden
	}
	if err := s.repo.SoftDelete(ctx, v.ID, *by); err != nil {
		return err
	}
	s.audit(ctx, actor, v.TenantID, "DELETE_VIRTUAL_PARAMETER", v)
	return nil
}

// compiled = definisi + ekspresi yang sudah di-parse. Definisi yang rusak (mis. dibuat sebelum aturan baru) dilewati, bukan
// menggagalkan seluruh tampilan.
type compiled struct {
	def  domain.VirtualParameter
	expr *vexpr.Expr
}

func compile(defs []domain.VirtualParameter) []compiled {
	out := make([]compiled, 0, len(defs))
	for _, d := range defs {
		if e, err := validateExpression(d.Expression); err == nil {
			out = append(out, compiled{def: d, expr: e})
		}
	}
	return out
}

func refUnion(cs []compiled) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range cs {
		for _, r := range c.expr.Refs() {
			if !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	return out
}

func evalValue(c compiled, vals map[string]string) *string {
	v := c.expr.Eval(func(name string) (string, bool) { s, ok := vals[name]; return s, ok })
	if v.IsNull() {
		return nil
	}
	s := v.String()
	return &s
}

// ForDevice menghitung semua virtual parameter aktif yang berlaku untuk perangkat (definisi tenant menimpa global bernama sama).
func (s *Service) ForDevice(ctx context.Context, actor domain.Actor, deviceID uint64) ([]domain.VirtualValue, error) {
	dev, err := s.devices.Get(ctx, actor, deviceID)
	if err != nil {
		return nil, err
	}
	defs, err := s.repo.ListForTenant(ctx, dev.TenantID)
	if err != nil {
		return nil, err
	}
	cs := compile(defs)
	vals := map[string]string{}
	if refs := refUnion(cs); len(refs) > 0 {
		m, err := s.params.ValuesFor(ctx, []uint64{dev.ID}, refs)
		if err != nil {
			return nil, err
		}
		vals = m[dev.ID]
	}
	out := make([]domain.VirtualValue, 0, len(cs))
	for _, c := range cs {
		out = append(out, domain.VirtualValue{Name: c.def.Name, Label: c.def.Label, Unit: c.def.Unit, Value: evalValue(c, vals)})
	}
	return out, nil
}

// Project mengisi items[i].Virtual dengan nilai virtual parameter bernama `names` (satu query batch untuk satu halaman; tanpa N+1).
// Perangkat boleh milik tenant berbeda (superadmin): definisi dipilih per tenant perangkat. Kegagalan membaca parameter tidak
// menggagalkan daftar (data tambahan), seperti ringkasan optik di ListWithSummary.
func (s *Service) Project(ctx context.Context, items []domain.DeviceListItem, names []string) {
	if len(items) == 0 || len(names) == 0 {
		return
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	perTenant := map[uint64][]compiled{} // kunci 0 = tanpa tenant
	pick := func(t *uint64) []compiled {
		key := uint64(0)
		if t != nil {
			key = *t
		}
		if cs, ok := perTenant[key]; ok {
			return cs
		}
		var cs []compiled
		if defs, err := s.repo.ListForTenant(ctx, t); err == nil {
			var sel []domain.VirtualParameter
			for _, d := range defs {
				if want[d.Name] {
					sel = append(sel, d)
				}
			}
			cs = compile(sel)
		}
		perTenant[key] = cs
		return cs
	}
	var ids []uint64
	seenRef := map[string]bool{}
	var refs []string
	for i := range items {
		ids = append(ids, items[i].ID)
		for _, r := range refUnion(pick(items[i].TenantID)) {
			if !seenRef[r] {
				seenRef[r] = true
				refs = append(refs, r)
			}
		}
	}
	vals := map[uint64]map[string]string{}
	if len(refs) > 0 {
		if m, err := s.params.ValuesFor(ctx, ids, refs); err == nil {
			vals = m
		}
	}
	for i := range items {
		out := map[string]string{}
		for _, c := range pick(items[i].TenantID) {
			if v := evalValue(c, vals[items[i].ID]); v != nil {
				out[c.def.Name] = *v
			}
		}
		if len(out) > 0 {
			items[i].Virtual = out
		}
	}
}

// RefValue = nilai satu parameter yang dirujuk ekspresi uji (nil = belum dilaporkan perangkat).
type RefValue struct {
	Name  string  `json:"name"`
	Value *string `json:"value"`
}

// TestError = kesalahan sintaks ekspresi uji (Pos = rune ke-N, mulai 0).
type TestError struct {
	Pos     int    `json:"pos"`
	Message string `json:"message"`
}

// TestResult = hasil mencoba ekspresi pada satu perangkat tanpa menyimpan apa pun.
type TestResult struct {
	OK    bool       `json:"ok"`
	Value *string    `json:"value"`
	Refs  []RefValue `json:"refs"`
	Error *TestError `json:"error,omitempty"`
}

// Test mengevaluasi ekspresi pada perangkat (scope tenant ditegakkan lewat devices.Get). Kesalahan ekspresi dikembalikan sebagai
// hasil (OK=false), bukan error HTTP, agar editor dapat menampilkannya di tempat.
func (s *Service) Test(ctx context.Context, actor domain.Actor, expression string, deviceID uint64) (*TestResult, error) {
	dev, err := s.devices.Get(ctx, actor, deviceID)
	if err != nil {
		return nil, err
	}
	e, perr := vexpr.Parse(strings.TrimSpace(expression))
	if perr != nil {
		var pe *vexpr.ParseError
		if errors.As(perr, &pe) {
			return &TestResult{Error: &TestError{Pos: pe.Pos, Message: pe.Msg}}, nil
		}
		return &TestResult{Error: &TestError{Message: perr.Error()}}, nil
	}
	for _, r := range e.Refs() {
		if domain.IsSensitiveParamName(r) {
			return &TestResult{Error: &TestError{Message: "ekspresi tidak boleh merujuk parameter rahasia (" + r + ")"}}, nil
		}
	}
	refs := e.Refs()
	vals := map[string]string{}
	if len(refs) > 0 {
		m, err := s.params.ValuesFor(ctx, []uint64{dev.ID}, refs)
		if err != nil {
			return nil, err
		}
		vals = m[dev.ID]
	}
	res := &TestResult{OK: true, Refs: make([]RefValue, 0, len(refs))}
	for _, r := range refs {
		rv := RefValue{Name: r}
		if v, ok := vals[r]; ok {
			v := v
			rv.Value = &v
		}
		res.Refs = append(res.Refs, rv)
	}
	res.Value = evalValue(compiled{expr: e}, vals)
	return res, nil
}
