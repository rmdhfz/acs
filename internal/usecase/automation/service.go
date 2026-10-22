// Package automation = aturan otomasi kondisi-aksi per tenant: "jika <ekspresi> maka <aksi>" yang dievaluasi saat Inform.
// Jawaban atas provision berskrip GenieACS TANPA skrip — kondisi memakai ekspresi deklaratif (pkg/vexpr) atas parameter,
// atribut perangkat, dan event sesi; aksi dari daftar tertutup. Pasti berhenti, biaya dibatasi, tiap pemicuan tercatat.
//
// Pagar anti-badai (aturan buruk tidak boleh membanjiri perangkat): (1) cooldown per (aturan, perangkat); (2) aturan di-suspend
// untuk perangkat setelah max_consecutive_fires pemicuan berturut-turut tanpa kondisi pernah salah/konvergen di antaranya;
// (3) tidak memicu bila perangkat masih punya task tertunda; (4) set_parameter/add_tag yang nilainya sudah sesuai tidak memicu
// apa pun (konvergen).
package automation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
	"acs/pkg/vexpr"
)

// ContextVars = variabel konteks yang boleh dirujuk {@nama} pada kondisi/nilai aksi.
var ContextVars = []string{"serial_number", "oui", "product_class", "software_version", "hardware_version", "ip_address", "events"}

var triggerEvents = map[string]bool{
	domain.ZtpTriggerEventBootstrapOnly: true, domain.ZtpTriggerEventBootstrapOrBoot: true, domain.ZtpTriggerEventEveryInform: true,
}

// DeviceGetter = device.Service.Get (sekaligus menegakkan scope tenant).
type DeviceGetter interface {
	Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.Device, error)
}

// Tasks = task.Service (antre RPC). Satu antarmuka kecil agar usecase ini tidak bergantung ke paket task.
type Tasks interface {
	domain.TaskEnqueuer
	CreateTask(ctx context.Context, actor domain.Actor, in domain.CreateTaskInput) (*domain.Task, error)
}

type Service struct {
	rules    domain.AutomationRuleRepository
	states   domain.AutomationStateRepository
	runs     domain.AutomationRunRepository
	devices  DeviceGetter
	params   domain.DeviceParameterRepository
	tasks    Tasks
	tags     domain.TagRepository
	webhooks domain.WebhookEnqueuer
	activity domain.ActivityLogRepository
	now      func() time.Time

	cacheMu sync.Mutex
	cache   map[string]*vexpr.Expr // sumber ekspresi -> hasil parse (dibatasi ukurannya)
}

func NewService(rules domain.AutomationRuleRepository, states domain.AutomationStateRepository, runs domain.AutomationRunRepository,
	devices DeviceGetter, params domain.DeviceParameterRepository, tasks Tasks, tags domain.TagRepository,
	webhooks domain.WebhookEnqueuer, activity domain.ActivityLogRepository) *Service {
	return &Service{rules: rules, states: states, runs: runs, devices: devices, params: params, tasks: tasks, tags: tags,
		webhooks: webhooks, activity: activity, now: time.Now, cache: map[string]*vexpr.Expr{}}
}

// parse = vexpr.Parse dengan cache kecil (aturan dievaluasi pada setiap Inform; parse ulang tiap kali boros).
func (s *Service) parse(src string) (*vexpr.Expr, error) {
	s.cacheMu.Lock()
	if e, ok := s.cache[src]; ok {
		s.cacheMu.Unlock()
		return e, nil
	}
	s.cacheMu.Unlock()
	e, err := vexpr.Parse(src)
	if err != nil {
		return nil, err
	}
	s.cacheMu.Lock()
	if len(s.cache) >= 2000 { // batas memori: definisi yang sering diedit tidak boleh menumpuk selamanya
		s.cache = map[string]*vexpr.Expr{}
	}
	s.cache[src] = e
	s.cacheMu.Unlock()
	return e, nil
}

// ---- Validasi ----

func knownContext(name string) bool {
	for _, v := range ContextVars {
		if "@"+v == name {
			return true
		}
	}
	return false
}

// checkExpr menolak ekspresi tak valid, variabel konteks tak dikenal, dan rujukan parameter rahasia (tampilan/riwayat aturan
// terbuka bagi peran staf; ekspresi tidak boleh menjadi jalan menyalin password/PSK).
func (s *Service) checkExpr(field, src string) error {
	e, err := s.parse(src)
	if err != nil {
		return fmt.Errorf("%w: %s tidak valid — %s", domain.ErrInvalidInput, field, err.Error())
	}
	for _, ref := range e.Refs() {
		switch {
		case vexpr.IsContextRef(ref):
			if !knownContext(ref) {
				return fmt.Errorf("%w: %s: variabel konteks %s tidak dikenal (tersedia: @%s)", domain.ErrInvalidInput, field, ref, strings.Join(ContextVars, ", @"))
			}
		case domain.IsSensitiveParamName(ref):
			return fmt.Errorf("%w: %s tidak boleh merujuk parameter rahasia (%s)", domain.ErrInvalidInput, field, ref)
		}
	}
	return nil
}

func (s *Service) validateActions(ctx context.Context, tenantID uint64, actions []domain.AutomationAction) error {
	if len(actions) == 0 {
		return fmt.Errorf("%w: minimal satu aksi", domain.ErrInvalidInput)
	}
	if len(actions) > domain.AutomationMaxActions {
		return fmt.Errorf("%w: maksimal %d aksi per aturan", domain.ErrInvalidInput, domain.AutomationMaxActions)
	}
	for i, a := range actions {
		at := fmt.Sprintf("aksi #%d (%s)", i+1, a.Type)
		bare := func() error {
			if a.Name != "" || a.Value != "" || len(a.Names) > 0 || a.TagID != nil {
				return fmt.Errorf("%w: %s tidak memakai field tambahan", domain.ErrInvalidInput, at)
			}
			return nil
		}
		switch a.Type {
		case domain.AutomationActionReboot, domain.AutomationActionWebhook:
			if err := bare(); err != nil {
				return err
			}
		case domain.AutomationActionSetParameter:
			if !domain.ValidParamName(a.Name) {
				return fmt.Errorf("%w: %s: name tidak valid (logical key atau raw path; huruf, angka, '.', '_', '-')", domain.ErrInvalidInput, at)
			}
			if domain.IsSensitiveParamName(a.Name) {
				return fmt.Errorf("%w: %s: parameter rahasia tidak boleh diatur lewat aturan otomasi (nilainya akan tersimpan polos di definisi) — pakai provisioning profile", domain.ErrInvalidInput, at)
			}
			if strings.TrimSpace(a.Value) == "" {
				return fmt.Errorf("%w: %s: value wajib (ekspresi; teks literal ditulis 'dalam kutip')", domain.ErrInvalidInput, at)
			}
			if err := s.checkExpr(at+" value", a.Value); err != nil {
				return err
			}
			if len(a.Names) > 0 || a.TagID != nil {
				return fmt.Errorf("%w: %s tidak memakai names/tag_id", domain.ErrInvalidInput, at)
			}
		case domain.AutomationActionGetParameterValues:
			if len(a.Names) == 0 || len(a.Names) > 20 {
				return fmt.Errorf("%w: %s: names harus 1-20 nama parameter", domain.ErrInvalidInput, at)
			}
			for _, n := range a.Names {
				if !domain.ValidParamName(n) {
					return fmt.Errorf("%w: %s: nama parameter tidak valid: %q", domain.ErrInvalidInput, at, n)
				}
			}
			if a.Name != "" || a.Value != "" || a.TagID != nil {
				return fmt.Errorf("%w: %s hanya memakai names", domain.ErrInvalidInput, at)
			}
		case domain.AutomationActionAddTag:
			if a.TagID == nil {
				return fmt.Errorf("%w: %s: tag_id wajib", domain.ErrInvalidInput, at)
			}
			tag, err := s.tags.GetByID(ctx, *a.TagID)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return fmt.Errorf("%w: %s: tag %d tidak ditemukan", domain.ErrInvalidInput, at, *a.TagID)
				}
				return err
			}
			if tag.TenantID != nil && *tag.TenantID != tenantID {
				return fmt.Errorf("%w: %s: tag milik tenant lain", domain.ErrInvalidInput, at)
			}
			if a.Name != "" || a.Value != "" || len(a.Names) > 0 {
				return fmt.Errorf("%w: %s hanya memakai tag_id", domain.ErrInvalidInput, at)
			}
		default:
			return fmt.Errorf("%w: %s: jenis aksi tidak dikenal (set_parameter, reboot, get_parameter_values, add_tag, webhook)", domain.ErrInvalidInput, at)
		}
	}
	return nil
}

func (s *Service) validateCondition(cond string) error {
	if strings.TrimSpace(cond) == "" {
		return nil
	}
	if len([]rune(cond)) > domain.AutomationConditionMaxLen {
		return fmt.Errorf("%w: condition maksimal %d karakter", domain.ErrInvalidInput, domain.AutomationConditionMaxLen)
	}
	return s.checkExpr("condition", cond)
}

func validateTunables(cooldown *uint32, maxFires *uint8) error {
	if *cooldown == 0 {
		*cooldown = domain.AutomationDefaultCooldown
	}
	if *cooldown < domain.AutomationMinCooldownSec || *cooldown > domain.AutomationMaxCooldownSec {
		return fmt.Errorf("%w: cooldown_seconds harus %d-%d", domain.ErrInvalidInput, domain.AutomationMinCooldownSec, domain.AutomationMaxCooldownSec)
	}
	if *maxFires == 0 {
		*maxFires = domain.AutomationDefaultMaxFires
	}
	if *maxFires > domain.AutomationMaxFiresCeiling {
		return fmt.Errorf("%w: max_consecutive_fires maksimal %d", domain.ErrInvalidInput, domain.AutomationMaxFiresCeiling)
	}
	return nil
}

// ---- CRUD ----

// Input = isi yang boleh diisi pengguna. TenantID wajib bagi SUPERADMIN; peran lain selalu tenant sendiri.
type Input struct {
	TenantID            *uint64
	Name                string
	Description         *string
	TriggerEvent        string
	Condition           string
	Actions             []domain.AutomationAction
	CooldownSeconds     uint32
	MaxConsecutiveFires uint8
	Priority            *uint32
	IsActive            *bool
}

// UpdateInput — partial: field nil = tidak diubah.
type UpdateInput struct {
	Name                *string
	Description         *string
	TriggerEvent        *string
	Condition           *string
	Actions             *[]domain.AutomationAction
	CooldownSeconds     *uint32
	MaxConsecutiveFires *uint8
	Priority            *uint32
	IsActive            *bool
}

func (s *Service) audit(ctx context.Context, actor domain.Actor, tenantID uint64, action string, r *domain.AutomationRule) {
	if s.activity == nil {
		return
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: &tenantID, Action: action, EntityType: "automation_rule",
		Details: map[string]any{"rule_id": r.UUID, "name": r.Name},
	})
}

func cleanDescription(p *string) (*string, error) {
	if p == nil || strings.TrimSpace(*p) == "" {
		return nil, nil
	}
	v, err := domain.CleanText("description", *p, 1, domain.AutomationDescriptionMaxLen, true)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Service) Create(ctx context.Context, actor domain.Actor, in Input) (*domain.AutomationRule, error) {
	var tenantID uint64
	if actor.IsSuperadmin() {
		if in.TenantID == nil {
			return nil, fmt.Errorf("%w: tenant_id wajib (aturan otomasi selalu milik satu tenant)", domain.ErrInvalidInput)
		}
		tenantID = *in.TenantID
	} else {
		if actor.TenantID == nil {
			return nil, domain.ErrForbidden
		}
		tenantID = *actor.TenantID
	}
	name, err := domain.CleanText("name", in.Name, 1, domain.AutomationNameMaxLen, false)
	if err != nil {
		return nil, err
	}
	desc, err := cleanDescription(in.Description)
	if err != nil {
		return nil, err
	}
	if !triggerEvents[in.TriggerEvent] {
		return nil, fmt.Errorf("%w: trigger_event harus BOOTSTRAP_ONLY, BOOTSTRAP_OR_BOOT, atau EVERY_INFORM", domain.ErrInvalidInput)
	}
	cond := strings.TrimSpace(in.Condition)
	if err := s.validateCondition(cond); err != nil {
		return nil, err
	}
	if err := s.validateActions(ctx, tenantID, in.Actions); err != nil {
		return nil, err
	}
	cooldown, maxFires := in.CooldownSeconds, in.MaxConsecutiveFires
	if err := validateTunables(&cooldown, &maxFires); err != nil {
		return nil, err
	}
	if _, err := s.rules.FindByName(ctx, tenantID, name); err == nil {
		return nil, fmt.Errorf("%w: aturan bernama %q sudah ada", domain.ErrConflict, name)
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if n, err := s.rules.CountInTenant(ctx, tenantID); err != nil {
		return nil, err
	} else if n >= domain.AutomationMaxPerTenant {
		return nil, fmt.Errorf("%w: batas %d aturan otomasi per tenant tercapai", domain.ErrInvalidInput, domain.AutomationMaxPerTenant)
	}
	r := &domain.AutomationRule{
		TenantID: tenantID, Name: name, Description: desc, TriggerEvent: in.TriggerEvent, Condition: cond,
		Actions: in.Actions, CooldownSeconds: cooldown, MaxConsecutiveFires: maxFires, Priority: 100, IsActive: true,
	}
	if in.Priority != nil {
		r.Priority = *in.Priority
	}
	if in.IsActive != nil {
		r.IsActive = *in.IsActive
	}
	if err := s.rules.Create(ctx, r, actor.UserIDPtr()); err != nil {
		return nil, err
	}
	s.audit(ctx, actor, tenantID, "CREATE_AUTOMATION_RULE", r)
	return r, nil
}

func (s *Service) scoped(actor domain.Actor, r *domain.AutomationRule) error {
	return auth.RequireTenantScope(actor, &r.TenantID)
}

func (s *Service) Get(ctx context.Context, actor domain.Actor, uuid string) (*domain.AutomationRule, error) {
	r, err := s.rules.GetByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if err := s.scoped(actor, r); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Service) List(ctx context.Context, actor domain.Actor, tenantFilter *uint64, p domain.Pagination) ([]domain.AutomationRule, int, error) {
	tid := tenantFilter
	if !actor.IsSuperadmin() {
		var err error
		if tid, err = auth.ScopedTenantFilter(actor); err != nil {
			return nil, 0, err
		}
	}
	return s.rules.List(ctx, tid, p)
}

func (s *Service) Update(ctx context.Context, actor domain.Actor, uuid string, in UpdateInput) (*domain.AutomationRule, error) {
	r, err := s.Get(ctx, actor, uuid)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		name, err := domain.CleanText("name", *in.Name, 1, domain.AutomationNameMaxLen, false)
		if err != nil {
			return nil, err
		}
		if name != r.Name {
			if other, ferr := s.rules.FindByName(ctx, r.TenantID, name); ferr == nil && other.ID != r.ID {
				return nil, fmt.Errorf("%w: aturan bernama %q sudah ada", domain.ErrConflict, name)
			} else if ferr != nil && !errors.Is(ferr, domain.ErrNotFound) {
				return nil, ferr
			}
			r.Name = name
		}
	}
	if in.Description != nil {
		if r.Description, err = cleanDescription(in.Description); err != nil {
			return nil, err
		}
	}
	if in.TriggerEvent != nil {
		if !triggerEvents[*in.TriggerEvent] {
			return nil, fmt.Errorf("%w: trigger_event harus BOOTSTRAP_ONLY, BOOTSTRAP_OR_BOOT, atau EVERY_INFORM", domain.ErrInvalidInput)
		}
		r.TriggerEvent = *in.TriggerEvent
	}
	if in.Condition != nil {
		cond := strings.TrimSpace(*in.Condition)
		if err := s.validateCondition(cond); err != nil {
			return nil, err
		}
		r.Condition = cond
	}
	if in.Actions != nil {
		if err := s.validateActions(ctx, r.TenantID, *in.Actions); err != nil {
			return nil, err
		}
		r.Actions = *in.Actions
	}
	if in.CooldownSeconds != nil {
		r.CooldownSeconds = *in.CooldownSeconds
	}
	if in.MaxConsecutiveFires != nil {
		r.MaxConsecutiveFires = *in.MaxConsecutiveFires
	}
	if err := validateTunables(&r.CooldownSeconds, &r.MaxConsecutiveFires); err != nil {
		return nil, err
	}
	if in.Priority != nil {
		r.Priority = *in.Priority
	}
	if in.IsActive != nil {
		r.IsActive = *in.IsActive
	}
	if err := s.rules.Update(ctx, r, actor.UserIDPtr()); err != nil {
		return nil, err
	}
	s.audit(ctx, actor, r.TenantID, "UPDATE_AUTOMATION_RULE", r)
	return r, nil
}

func (s *Service) Delete(ctx context.Context, actor domain.Actor, uuid string) error {
	r, err := s.Get(ctx, actor, uuid)
	if err != nil {
		return err
	}
	by := actor.UserIDPtr()
	if by == nil {
		return domain.ErrForbidden
	}
	if err := s.rules.SoftDelete(ctx, r.ID, *by); err != nil {
		return err
	}
	s.audit(ctx, actor, r.TenantID, "DELETE_AUTOMATION_RULE", r)
	return nil
}

// ResetSuspension membuka kembali aturan yang di-suspend (untuk satu perangkat, atau semua bila deviceID nil).
func (s *Service) ResetSuspension(ctx context.Context, actor domain.Actor, uuid string, deviceID *uint64) (int64, error) {
	r, err := s.Get(ctx, actor, uuid)
	if err != nil {
		return 0, err
	}
	if deviceID != nil {
		if _, err := s.devices.Get(ctx, actor, *deviceID); err != nil {
			return 0, err
		}
	}
	n, err := s.states.ClearSuspension(ctx, r.ID, deviceID)
	if err != nil {
		return 0, err
	}
	if s.activity != nil {
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			UserID: actor.UserIDPtr(), TenantID: &r.TenantID, Action: "RESET_AUTOMATION_RULE", EntityType: "automation_rule",
			Details: map[string]any{"rule_id": r.UUID, "name": r.Name, "devices": n},
		})
	}
	return n, nil
}

// ListRuns = riwayat pemicuan; non-superadmin dibatasi ke tenant sendiri.
func (s *Service) ListRuns(ctx context.Context, actor domain.Actor, f domain.AutomationRunFilter, p domain.Pagination) ([]domain.AutomationRun, int, error) {
	if !actor.IsSuperadmin() {
		tid, err := auth.ScopedTenantFilter(actor)
		if err != nil {
			return nil, 0, err
		}
		f.TenantID = tid
	}
	return s.runs.List(ctx, f, p)
}

// ---- Evaluasi ----

func joinEvents(events []string) string { return strings.Join(events, ",") }

func contextValues(dev *domain.Device, events []string) map[string]string {
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	return map[string]string{
		"@serial_number": dev.SerialNumber, "@oui": deref(dev.OUI), "@product_class": deref(dev.ProductClass),
		"@software_version": deref(dev.SoftwareVersion), "@hardware_version": deref(dev.HardwareVersion),
		"@ip_address": deref(dev.IPAddress), "@events": joinEvents(events),
	}
}

func triggerMatches(trigger string, events []string) bool {
	has := func(code string) bool {
		for _, e := range events {
			if e == code {
				return true
			}
		}
		return false
	}
	switch trigger {
	case domain.ZtpTriggerEventEveryInform:
		return true
	case domain.ZtpTriggerEventBootstrapOnly:
		return has(domain.EventCodeBootstrap)
	case domain.ZtpTriggerEventBootstrapOrBoot:
		return has(domain.EventCodeBootstrap) || has(domain.EventCodeBoot)
	}
	return false
}

// step = satu aksi yang sudah direncanakan terhadap keadaan perangkat saat ini.
type step struct {
	Action  domain.AutomationAction
	Path    string  // set_parameter: raw path hasil resolusi
	Value   string  // set_parameter: nilai hasil evaluasi
	Current *string // set_parameter: nilai perangkat saat ini (nil = belum pernah terlihat)
	Change  bool    // true = perlu dieksekusi
	Skip    string  // alasan dilewati (tak dapat dihitung / tak dapat di-resolve)
}

func (st step) describe() string {
	switch st.Action.Type {
	case domain.AutomationActionSetParameter:
		v := st.Value
		if r := []rune(v); len(r) > 60 {
			v = string(r[:60]) + "…"
		}
		return fmt.Sprintf("set_parameter %s=%s", st.Action.Name, v)
	case domain.AutomationActionGetParameterValues:
		return fmt.Sprintf("get_parameter_values (%d)", len(st.Action.Names))
	case domain.AutomationActionAddTag:
		return fmt.Sprintf("add_tag %d", *st.Action.TagID)
	}
	return st.Action.Type
}

// plan menghitung langkah aksi terhadap keadaan perangkat saat ini TANPA mengeksekusi apa pun (dipakai evaluasi nyata dan uji).
func (s *Service) plan(ctx context.Context, dev *domain.Device, actions []domain.AutomationAction, get func(string) (string, bool)) ([]step, error) {
	var deviceTags map[uint64]bool
	steps := make([]step, 0, len(actions))
	for _, a := range actions {
		st := step{Action: a, Change: true}
		switch a.Type {
		case domain.AutomationActionSetParameter:
			e, err := s.parse(a.Value)
			if err != nil {
				st.Change, st.Skip = false, "ekspresi nilai tidak valid: "+err.Error()
				break
			}
			v := e.Eval(get)
			if v.IsNull() {
				st.Change, st.Skip = false, "nilai belum dapat dihitung (parameter sumber belum dilaporkan)"
				break
			}
			path, err := s.tasks.ResolveParameterPath(ctx, dev.ID, a.Name)
			if err != nil {
				st.Change, st.Skip = false, "parameter tidak dapat di-resolve untuk perangkat ini: "+err.Error()
				break
			}
			st.Path, st.Value = path, v.String()
			cur, err := s.params.Get(ctx, dev.ID, path)
			switch {
			case err == nil && cur.ParameterValue != nil:
				c := *cur.ParameterValue
				st.Current = &c
				st.Change = c != st.Value
			case err != nil && !errors.Is(err, domain.ErrNotFound):
				return nil, err
			}
		case domain.AutomationActionAddTag:
			if deviceTags == nil {
				ts, err := s.tags.ListByDevice(ctx, dev.ID)
				if err != nil {
					return nil, err
				}
				deviceTags = map[uint64]bool{}
				for _, t := range ts {
					deviceTags[t.ID] = true
				}
			}
			st.Change = !deviceTags[*a.TagID]
		}
		steps = append(steps, st)
	}
	return steps, nil
}

func anyChange(steps []step) bool {
	for _, st := range steps {
		if st.Change {
			return true
		}
	}
	return false
}

func summarize(steps []step) string {
	var parts []string
	for _, st := range steps {
		if st.Change {
			parts = append(parts, st.describe())
		}
	}
	out := strings.Join(parts, "; ")
	if r := []rune(out); len(r) > 1000 {
		out = string(r[:1000]) + "…"
	}
	return out
}

// execute mengantre aksi yang berubah. set_parameter digabung jadi satu SetParameterValues; urutan lain mengikuti definisi.
func (s *Service) execute(ctx context.Context, rule *domain.AutomationRule, dev *domain.Device, steps []step) error {
	actor := domain.Actor{TenantID: dev.TenantID} // aktor sistem, seperti ZTP/preset
	set := map[string]string{}
	for _, st := range steps {
		if st.Change && st.Action.Type == domain.AutomationActionSetParameter {
			set[st.Path] = st.Value
		}
	}
	if len(set) > 0 {
		if _, err := s.tasks.EnqueueSetParameterValues(ctx, actor, dev.ID, set, 3); err != nil {
			return fmt.Errorf("set_parameter: %w", err)
		}
	}
	for _, st := range steps {
		if !st.Change {
			continue
		}
		switch st.Action.Type {
		case domain.AutomationActionReboot:
			if _, err := s.tasks.EnqueueReboot(ctx, actor, dev.ID, 3); err != nil {
				return fmt.Errorf("reboot: %w", err)
			}
		case domain.AutomationActionGetParameterValues:
			if _, err := s.tasks.CreateTask(ctx, actor, domain.CreateTaskInput{
				DeviceID: dev.ID, TaskType: domain.TaskTypeGetParameterValues, Priority: 5,
				Parameters: map[string]interface{}{"names": st.Action.Names},
			}); err != nil {
				return fmt.Errorf("get_parameter_values: %w", err)
			}
		case domain.AutomationActionAddTag:
			if err := s.tags.AssignToDevice(ctx, dev.ID, *st.Action.TagID); err != nil {
				return fmt.Errorf("add_tag: %w", err)
			}
		case domain.AutomationActionWebhook:
			if s.webhooks != nil {
				_ = s.webhooks.Enqueue(ctx, domain.WebhookEventAutomationTriggered, dev.TenantID, map[string]any{
					"rule_id": rule.UUID, "rule_name": rule.Name, "device_id": dev.ID, "serial_number": dev.SerialNumber,
					"actions": summarize(steps),
				})
			}
		}
	}
	return nil
}

func (s *Service) record(ctx context.Context, rule *domain.AutomationRule, dev *domain.Device, fired, suspended bool, summary string, runErr error) {
	run := &domain.AutomationRun{RuleID: rule.ID, DeviceID: dev.ID, Fired: fired, Suspended: suspended, Summary: summary}
	if runErr != nil {
		msg := runErr.Error()
		if r := []rune(msg); len(r) > 500 {
			msg = string(r[:500]) + "…"
		}
		run.Error = &msg
	}
	_ = s.runs.Insert(ctx, run)
	if s.activity != nil {
		action := "AUTOMATION_FIRED"
		switch {
		case suspended:
			action = "AUTOMATION_SUSPENDED"
		case !fired:
			action = "AUTOMATION_FAILED"
		}
		desc := fmt.Sprintf("rule=%s %s", rule.Name, summary)
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			TenantID: dev.TenantID, Action: action, EntityType: "device", EntityID: &dev.ID, Description: &desc,
		})
	}
}

// Evaluate dipanggil usecase/session setelah ZTP & preset pada setiap Inform. events = kode event Inform ("0 BOOTSTRAP", ...).
// Tanpa aturan aktif untuk tenant perangkat: nol query (daftar aturan di-cache). Error TIDAK boleh menggagalkan Inform —
// pemanggil mencatat dan lanjut (pola yang sama dengan ZTP/preset, FR-15).
func (s *Service) Evaluate(ctx context.Context, dev *domain.Device, events []string) error {
	if dev.TenantID == nil {
		return nil
	}
	all, err := s.rules.ListActive(ctx, *dev.TenantID)
	if err != nil {
		return err
	}
	var rules []domain.AutomationRule
	for _, r := range all {
		if triggerMatches(r.TriggerEvent, events) {
			rules = append(rules, r)
		}
	}
	if len(rules) == 0 {
		return nil
	}
	// Pagar 3: perangkat dengan task tertunda tidak dipicu (task sebelumnya belum selesai = belum bisa menilai hasilnya).
	if pending, err := s.tasks.HasPendingForDevice(ctx, dev.ID); err != nil {
		return err
	} else if pending {
		return nil
	}

	// Satu query untuk semua parameter yang dirujuk kondisi aturan-aturan ini.
	type compiledRule struct {
		rule *domain.AutomationRule
		cond *vexpr.Expr // nil = selalu benar
	}
	var cs []compiledRule
	seen := map[string]bool{}
	var refs []string
	for i := range rules {
		cr := compiledRule{rule: &rules[i]}
		if strings.TrimSpace(rules[i].Condition) != "" {
			e, err := s.parse(rules[i].Condition)
			if err != nil {
				s.record(ctx, &rules[i], dev, false, false, "", fmt.Errorf("kondisi tidak valid: %w", err))
				continue
			}
			cr.cond = e
			for _, ref := range e.Refs() {
				if !vexpr.IsContextRef(ref) && !seen[ref] {
					seen[ref] = true
					refs = append(refs, ref)
				}
			}
		}
		cs = append(cs, cr)
	}
	ctxVars := contextValues(dev, events)
	values := map[string]string{}
	if len(refs) > 0 {
		m, err := s.params.ValuesFor(ctx, []uint64{dev.ID}, refs)
		if err != nil {
			return err
		}
		values = m[dev.ID]
	}
	get := func(name string) (string, bool) {
		if vexpr.IsContextRef(name) {
			v, ok := ctxVars[name]
			return v, ok
		}
		v, ok := values[name]
		return v, ok
	}
	states, err := s.states.ListByDevice(ctx, dev.ID)
	if err != nil {
		return err
	}

	now := s.now()
	for _, cr := range cs {
		rule := cr.rule
		st := states[rule.ID]
		matched := cr.cond == nil || cr.cond.Eval(get).Truthy()
		if !matched {
			if st.ConsecutiveFires > 0 {
				_ = s.states.ResetConsecutive(ctx, rule.ID, dev.ID)
			}
			continue
		}
		if st.Suspended {
			continue
		}
		if st.LastFiredAt != nil && now.Sub(*st.LastFiredAt) < time.Duration(rule.CooldownSeconds)*time.Second {
			continue
		}
		steps, err := s.plan(ctx, dev, rule.Actions, get)
		if err != nil {
			return err
		}
		if !anyChange(steps) {
			// Kondisi benar tetapi perangkat sudah sesuai target (konvergen): tidak ada yang dikerjakan; hitungan berturut direset.
			if st.ConsecutiveFires > 0 {
				_ = s.states.ResetConsecutive(ctx, rule.ID, dev.ID)
			}
			continue
		}
		next := st
		next.RuleID, next.DeviceID = rule.ID, dev.ID
		// Pagar 2: kondisi tak pernah salah di antara pemicuan -> aturan tidak konvergen; berhenti sebelum membanjiri perangkat.
		if st.ConsecutiveFires >= uint32(rule.MaxConsecutiveFires) {
			next.Suspended = true
			_ = s.states.Save(ctx, &next)
			s.record(ctx, rule, dev, false, true, summarize(steps),
				fmt.Errorf("di-suspend: %d pemicuan berturut-turut tanpa kondisi pernah salah/konvergen (reset lewat API/UI setelah diperiksa)", st.ConsecutiveFires))
			continue
		}
		execErr := s.execute(ctx, rule, dev, steps)
		fired := now // pemicuan (berhasil atau gagal) memulai cooldown agar kegagalan tidak diulang tiap Inform
		next.LastFiredAt = &fired
		next.ConsecutiveFires = st.ConsecutiveFires + 1
		_ = s.states.Save(ctx, &next)
		s.record(ctx, rule, dev, execErr == nil, false, summarize(steps), execErr)
	}
	return nil
}

// ---- Uji (dry-run) ----

// TestInput = aturan yang dicoba pada satu perangkat (tidak disimpan). Events opsional: mensimulasikan @events.
type TestInput struct {
	Condition string
	Actions   []domain.AutomationAction
	Events    []string
}

type TestRef struct {
	Name  string  `json:"name"`
	Value *string `json:"value"`
}

type TestStep struct {
	Type    string  `json:"type"`
	Detail  string  `json:"detail"`
	Path    string  `json:"path,omitempty"`
	Value   *string `json:"value,omitempty"`
	Current *string `json:"current,omitempty"`
	Change  bool    `json:"change"`
	Skip    string  `json:"skip,omitempty"`
}

type TestResult struct {
	OK      bool       `json:"ok"`
	Matched bool       `json:"matched"`
	Refs    []TestRef  `json:"refs"`
	Steps   []TestStep `json:"steps"`
	// WouldFire: kondisi benar DAN ada aksi yang benar-benar akan dieksekusi (belum menghitung cooldown/pagar perangkat).
	WouldFire bool   `json:"would_fire"`
	Error     string `json:"error,omitempty"`
}

// Test menjalankan kondisi + perencanaan aksi terhadap keadaan perangkat saat ini, tanpa mengeksekusi atau menyimpan apa pun.
// Kesalahan definisi dikembalikan sebagai hasil (OK=false) agar editor menampilkannya di tempat.
func (s *Service) Test(ctx context.Context, actor domain.Actor, in TestInput, deviceID uint64) (*TestResult, error) {
	dev, err := s.devices.Get(ctx, actor, deviceID)
	if err != nil {
		return nil, err
	}
	if dev.TenantID == nil {
		return nil, domain.ErrForbidden
	}
	if err := s.validateCondition(strings.TrimSpace(in.Condition)); err != nil {
		return &TestResult{Error: strings.TrimPrefix(err.Error(), domain.ErrInvalidInput.Error()+": ")}, nil
	}
	if err := s.validateActions(ctx, *dev.TenantID, in.Actions); err != nil {
		return &TestResult{Error: strings.TrimPrefix(err.Error(), domain.ErrInvalidInput.Error()+": ")}, nil
	}

	var cond *vexpr.Expr
	var refs []string
	if strings.TrimSpace(in.Condition) != "" {
		cond, _ = s.parse(strings.TrimSpace(in.Condition))
		refs = append(refs, cond.Refs()...)
	}
	// rujukan dari nilai aksi juga ditampilkan agar penulis aturan melihat sumber nilai
	for _, a := range in.Actions {
		if a.Type == domain.AutomationActionSetParameter {
			if e, perr := s.parse(a.Value); perr == nil {
				refs = append(refs, e.Refs()...)
			}
		}
	}
	paramRefs := map[string]bool{}
	var names []string
	for _, r := range refs {
		if !vexpr.IsContextRef(r) && !paramRefs[r] {
			paramRefs[r] = true
			names = append(names, r)
		}
	}
	values := map[string]string{}
	if len(names) > 0 {
		m, err := s.params.ValuesFor(ctx, []uint64{dev.ID}, names)
		if err != nil {
			return nil, err
		}
		values = m[dev.ID]
	}
	ctxVars := contextValues(dev, in.Events)
	get := func(name string) (string, bool) {
		if vexpr.IsContextRef(name) {
			v, ok := ctxVars[name]
			return v, ok
		}
		v, ok := values[name]
		return v, ok
	}

	res := &TestResult{OK: true, Refs: []TestRef{}, Steps: []TestStep{}}
	sort.Strings(names)
	for _, n := range names {
		r := TestRef{Name: n}
		if v, ok := values[n]; ok {
			v := v
			r.Value = &v
		}
		res.Refs = append(res.Refs, r)
	}
	res.Matched = cond == nil || cond.Eval(get).Truthy()
	steps, err := s.plan(ctx, dev, in.Actions, get)
	if err != nil {
		return nil, err
	}
	for _, st := range steps {
		ts := TestStep{Type: st.Action.Type, Detail: st.describe(), Path: st.Path, Current: st.Current, Change: st.Change, Skip: st.Skip}
		if st.Action.Type == domain.AutomationActionSetParameter && st.Path != "" {
			v := st.Value
			ts.Value = &v
		}
		res.Steps = append(res.Steps, ts)
	}
	res.WouldFire = res.Matched && anyChange(steps)
	return res, nil
}
