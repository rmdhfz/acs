package mysql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"acs/internal/domain"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// ---- AutomationRule ----

type automationRuleRepository struct{ db *sqlx.DB }

func NewAutomationRuleRepository(db *sqlx.DB) domain.AutomationRuleRepository {
	return &automationRuleRepository{db: db}
}

// arCols — kolom aturan + kode trigger dari ref_ztp_trigger_event (alias r untuk automation_rules, t untuk ref).
const arCols = `r.id, r.rule_uuid, r.tenant_id, r.name, r.description, r.trigger_event_id, t.code AS trigger_event,
	r.condition_expr, r.actions, r.cooldown_seconds, r.max_consecutive_fires, r.priority, r.is_active, r.created_at, r.updated_at`
const arFrom = ` FROM automation_rules r JOIN ref_ztp_trigger_event t ON t.id = r.trigger_event_id `

func (r *automationRuleRepository) triggerID(ctx context.Context, code string) (uint64, error) {
	var id uint64
	if err := r.db.GetContext(ctx, &id, `SELECT id FROM ref_ztp_trigger_event WHERE code = ? AND is_deleted = 0`, code); err != nil {
		if translateErr(err) == domain.ErrNotFound {
			return 0, fmt.Errorf("%w: trigger_event %q tidak dikenal", domain.ErrInvalidInput, code)
		}
		return 0, translateErr(err)
	}
	return id, nil
}

func (r *automationRuleRepository) Create(ctx context.Context, rule *domain.AutomationRule, by *uint64) error {
	tid, err := r.triggerID(ctx, rule.TriggerEvent)
	if err != nil {
		return err
	}
	rule.UUID = uuid.NewString()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO automation_rules (rule_uuid, tenant_id, name, description, trigger_event_id, condition_expr, actions,
			cooldown_seconds, max_consecutive_fires, priority, is_active, created_by, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.UUID, rule.TenantID, rule.Name, rule.Description, tid, rule.Condition, rule.Actions,
		rule.CooldownSeconds, rule.MaxConsecutiveFires, rule.Priority, rule.IsActive, by, by)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	rule.ID = uint64(id)
	return r.db.GetContext(ctx, rule, `SELECT `+arCols+arFrom+`WHERE r.id = ?`, rule.ID)
}

func (r *automationRuleRepository) GetByUUID(ctx context.Context, uuid string) (*domain.AutomationRule, error) {
	var rule domain.AutomationRule
	if err := r.db.GetContext(ctx, &rule, `SELECT `+arCols+arFrom+`WHERE r.rule_uuid = ? AND r.is_deleted = 0`, uuid); err != nil {
		return nil, translateErr(err)
	}
	return &rule, nil
}

func (r *automationRuleRepository) List(ctx context.Context, tenantID *uint64, p domain.Pagination) ([]domain.AutomationRule, int, error) {
	where, args := "r.is_deleted = 0", []interface{}{}
	if tenantID != nil {
		where += " AND r.tenant_id = ?"
		args = append(args, *tenantID)
	}
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM automation_rules r WHERE `+where, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	rows := []domain.AutomationRule{}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT `+arCols+arFrom+`WHERE `+where+` ORDER BY r.priority ASC, r.id ASC LIMIT ? OFFSET ?`,
		append(args, p.Limit(), p.Offset())...); err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *automationRuleRepository) ListActive(ctx context.Context, tenantID uint64) ([]domain.AutomationRule, error) {
	rows := []domain.AutomationRule{}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT `+arCols+arFrom+`WHERE r.tenant_id = ? AND r.is_deleted = 0 AND r.is_active = 1 ORDER BY r.priority ASC, r.id ASC`, tenantID); err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *automationRuleRepository) FindByName(ctx context.Context, tenantID uint64, name string) (*domain.AutomationRule, error) {
	var rule domain.AutomationRule
	if err := r.db.GetContext(ctx, &rule, `SELECT `+arCols+arFrom+`WHERE r.tenant_id = ? AND r.is_deleted = 0 AND r.name = ? ORDER BY r.id LIMIT 1`, tenantID, name); err != nil {
		return nil, translateErr(err)
	}
	return &rule, nil
}

func (r *automationRuleRepository) CountInTenant(ctx context.Context, tenantID uint64) (int, error) {
	var n int
	if err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM automation_rules WHERE tenant_id = ? AND is_deleted = 0`, tenantID); err != nil {
		return 0, translateErr(err)
	}
	return n, nil
}

func (r *automationRuleRepository) Update(ctx context.Context, rule *domain.AutomationRule, by *uint64) error {
	tid, err := r.triggerID(ctx, rule.TriggerEvent)
	if err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE automation_rules SET name = ?, description = ?, trigger_event_id = ?, condition_expr = ?, actions = ?,
			cooldown_seconds = ?, max_consecutive_fires = ?, priority = ?, is_active = ?, updated_by = ?
		 WHERE id = ? AND is_deleted = 0`,
		rule.Name, rule.Description, tid, rule.Condition, rule.Actions,
		rule.CooldownSeconds, rule.MaxConsecutiveFires, rule.Priority, rule.IsActive, by, rule.ID); err != nil {
		return translateErr(err)
	}
	return nil
}

func (r *automationRuleRepository) SoftDelete(ctx context.Context, id, by uint64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE automation_rules SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ? AND is_deleted = 0`, time.Now(), by, id)
	if err != nil {
		return translateErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ---- AutomationState ----

type automationStateRepository struct{ db *sqlx.DB }

func NewAutomationStateRepository(db *sqlx.DB) domain.AutomationStateRepository {
	return &automationStateRepository{db: db}
}

func (r *automationStateRepository) ListByDevice(ctx context.Context, deviceID uint64) (map[uint64]domain.AutomationState, error) {
	var rows []domain.AutomationState
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT rule_id, device_id, last_fired_at, consecutive_fires, suspended, window_started_at, window_fires FROM automation_states WHERE device_id = ?`, deviceID); err != nil {
		return nil, translateErr(err)
	}
	out := make(map[uint64]domain.AutomationState, len(rows))
	for _, s := range rows {
		out[s.RuleID] = s
	}
	return out, nil
}

func (r *automationStateRepository) Save(ctx context.Context, s *domain.AutomationState) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO automation_states (rule_id, device_id, last_fired_at, consecutive_fires, suspended, window_started_at, window_fires) VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE last_fired_at = VALUES(last_fired_at), consecutive_fires = VALUES(consecutive_fires), suspended = VALUES(suspended),
			window_started_at = VALUES(window_started_at), window_fires = VALUES(window_fires)`,
		s.RuleID, s.DeviceID, s.LastFiredAt, s.ConsecutiveFires, s.Suspended, s.WindowStartedAt, s.WindowFires)
	return translateErr(err)
}

func (r *automationStateRepository) ResetConsecutive(ctx context.Context, ruleID, deviceID uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE automation_states SET consecutive_fires = 0 WHERE rule_id = ? AND device_id = ? AND consecutive_fires <> 0`, ruleID, deviceID)
	return translateErr(err)
}

func (r *automationStateRepository) ClearSuspension(ctx context.Context, ruleID uint64, deviceID *uint64) (int64, error) {
	q := `UPDATE automation_states SET suspended = 0, consecutive_fires = 0, window_started_at = NULL, window_fires = 0 WHERE rule_id = ? AND suspended = 1`
	args := []interface{}{ruleID}
	if deviceID != nil {
		q += ` AND device_id = ?`
		args = append(args, *deviceID)
	}
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, translateErr(err)
	}
	return res.RowsAffected()
}

// ---- AutomationRun ----

type automationRunRepository struct{ db *sqlx.DB }

func NewAutomationRunRepository(db *sqlx.DB) domain.AutomationRunRepository {
	return &automationRunRepository{db: db}
}

func (r *automationRunRepository) Insert(ctx context.Context, run *domain.AutomationRun) error {
	run.UUID = uuid.NewString()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO automation_runs (run_uuid, rule_id, device_id, fired, suspended, summary, error) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		run.UUID, run.RuleID, run.DeviceID, run.Fired, run.Suspended, run.Summary, run.Error)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	// Retensi eksplisit (CLAUDE.md §3): pangkas fisik yang lebih tua dari 30 hari, tiap ~50 insert agar murah.
	if id%50 == 0 {
		_, _ = r.db.ExecContext(ctx, `DELETE FROM automation_runs WHERE created_at < ? LIMIT 500`, time.Now().Add(-domain.AutomationRunRetention))
	}
	return nil
}

func (r *automationRunRepository) List(ctx context.Context, f domain.AutomationRunFilter, p domain.Pagination) ([]domain.AutomationRun, int, error) {
	where := []string{"1 = 1"}
	args := []interface{}{}
	if f.TenantID != nil {
		where = append(where, "ar.tenant_id = ?")
		args = append(args, *f.TenantID)
	}
	if f.RuleUUID != "" {
		where = append(where, "ar.rule_uuid = ?")
		args = append(args, f.RuleUUID)
	}
	if f.DeviceID != nil {
		where = append(where, "x.device_id = ?")
		args = append(args, *f.DeviceID)
	}
	from := ` FROM automation_runs x JOIN automation_rules ar ON ar.id = x.rule_id JOIN devices d ON d.id = x.device_id WHERE ` + strings.Join(where, " AND ")
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*)`+from, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	rows := []domain.AutomationRun{}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT x.id, x.run_uuid, x.rule_id, ar.rule_uuid, ar.name AS rule_name, x.device_id, d.serial_number AS device_serial,
			x.fired, x.suspended, x.summary, x.error, x.created_at`+from+` ORDER BY x.id DESC LIMIT ? OFFSET ?`,
		append(args, p.Limit(), p.Offset())...); err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}
