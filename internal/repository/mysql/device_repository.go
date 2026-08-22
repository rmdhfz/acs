package mysql

import (
	"context"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// ---- Device ----

type deviceRepository struct{ db *sqlx.DB }

func NewDeviceRepository(db *sqlx.DB) domain.DeviceRepository { return &deviceRepository{db: db} }

func (r *deviceRepository) Create(ctx context.Context, d *domain.Device) error {
	now := time.Now()
	d.CreatedAt, d.UpdatedAt = now, now
	const q = `INSERT INTO devices
		(device_uuid, tenant_id, vendor_id, device_model_id, device_status_id, provisioning_profile_id,
		 oui, serial_number, product_class, mac_address, software_version, hardware_version, ip_address,
		 connection_request_url, connection_request_username, connection_request_password_enc,
		 inform_username, inform_password_enc, last_inform_at, last_boot_event_at, notes, created_at, updated_at, created_by)
		VALUES
		(:device_uuid, :tenant_id, :vendor_id, :device_model_id, :device_status_id, :provisioning_profile_id,
		 :oui, :serial_number, :product_class, :mac_address, :software_version, :hardware_version, :ip_address,
		 :connection_request_url, :connection_request_username, :connection_request_password_enc,
		 :inform_username, :inform_password_enc, :last_inform_at, :last_boot_event_at, :notes, :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, d)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	d.ID = uint64(id)
	return nil
}

func (r *deviceRepository) GetByID(ctx context.Context, id uint64) (*domain.Device, error) {
	var d domain.Device
	if err := r.db.GetContext(ctx, &d, `SELECT * FROM devices WHERE id = ? AND is_deleted = 0`, id); err != nil {
		return nil, translateErr(err)
	}
	return &d, nil
}

func (r *deviceRepository) GetByUUID(ctx context.Context, uuid string) (*domain.Device, error) {
	var d domain.Device
	if err := r.db.GetContext(ctx, &d, `SELECT * FROM devices WHERE device_uuid = ? AND is_deleted = 0`, uuid); err != nil {
		return nil, translateErr(err)
	}
	return &d, nil
}

func (r *deviceRepository) GetByOUISerial(ctx context.Context, oui, serial string) (*domain.Device, error) {
	var d domain.Device
	err := r.db.GetContext(ctx, &d,
		`SELECT * FROM devices WHERE oui = ? AND serial_number = ? AND is_deleted = 0`, oui, serial)
	if err != nil {
		return nil, translateErr(err)
	}
	return &d, nil
}

func (r *deviceRepository) List(ctx context.Context, f domain.DeviceFilter, p domain.Pagination) ([]domain.Device, int, error) {
	where := []string{"is_deleted = 0"}
	args := []interface{}{}
	if f.TenantID != nil {
		where = append(where, "tenant_id = ?")
		args = append(args, *f.TenantID)
	}
	if f.VendorID != nil {
		where = append(where, "vendor_id = ?")
		args = append(args, *f.VendorID)
	}
	if f.DeviceModelID != nil {
		where = append(where, "device_model_id = ?")
		args = append(args, *f.DeviceModelID)
	}
	if f.DeviceStatusID != nil {
		where = append(where, "device_status_id = ?")
		args = append(args, *f.DeviceStatusID)
	}
	if f.Search != "" {
		where = append(where, "(serial_number LIKE ? OR mac_address LIKE ?)")
		like := "%" + f.Search + "%"
		args = append(args, like, like)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM devices WHERE "+whereSQL, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.Device
	args = append(args, p.Limit(), p.Offset())
	err := r.db.SelectContext(ctx, &rows,
		"SELECT * FROM devices WHERE "+whereSQL+" ORDER BY id DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *deviceRepository) CountByStatus(ctx context.Context, tenantID *uint64) ([]domain.DeviceStatusCount, error) {
	where := "is_deleted = 0"
	args := []interface{}{}
	if tenantID != nil {
		where += " AND tenant_id = ?"
		args = append(args, *tenantID)
	}
	var rows []domain.DeviceStatusCount
	err := r.db.SelectContext(ctx, &rows,
		"SELECT device_status_id, COUNT(*) AS cnt FROM devices WHERE "+where+" GROUP BY device_status_id", args...)
	if err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *deviceRepository) CountByVendor(ctx context.Context, tenantID *uint64) ([]domain.DeviceVendorCount, error) {
	where := "is_deleted = 0"
	args := []interface{}{}
	if tenantID != nil {
		where += " AND tenant_id = ?"
		args = append(args, *tenantID)
	}
	var rows []domain.DeviceVendorCount
	err := r.db.SelectContext(ctx, &rows,
		"SELECT vendor_id, COUNT(*) AS cnt FROM devices WHERE "+where+" GROUP BY vendor_id", args...)
	if err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *deviceRepository) Update(ctx context.Context, d *domain.Device) error {
	const q = `UPDATE devices SET
		tenant_id = :tenant_id, vendor_id = :vendor_id, device_model_id = :device_model_id,
		device_status_id = :device_status_id, provisioning_profile_id = :provisioning_profile_id,
		oui = :oui, serial_number = :serial_number, product_class = :product_class, mac_address = :mac_address,
		software_version = :software_version, hardware_version = :hardware_version, ip_address = :ip_address,
		connection_request_url = :connection_request_url, connection_request_username = :connection_request_username,
		connection_request_password_enc = :connection_request_password_enc,
		inform_username = :inform_username, inform_password_enc = :inform_password_enc,
		last_inform_at = :last_inform_at, last_boot_event_at = :last_boot_event_at, notes = :notes,
		updated_by = :updated_by
		WHERE id = :id AND is_deleted = 0`
	_, err := r.db.NamedExecContext(ctx, q, d)
	return translateErr(err)
}

func (r *deviceRepository) UpdateStatus(ctx context.Context, id, statusID uint64, updatedBy *uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE devices SET device_status_id = ?, updated_by = ? WHERE id = ? AND is_deleted = 0`,
		statusID, updatedBy, id)
	return translateErr(err)
}

// MarkStaleOffline menandai OFFLINE device yang last_inform_at-nya lebih lama
// dari staleBefore dan belum berstatus offline/decommissioned (FR-22).
func (r *deviceRepository) MarkStaleOffline(ctx context.Context, offlineStatusID uint64, staleBefore time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE devices SET device_status_id = ?
		 WHERE is_deleted = 0 AND device_status_id <> ?
		   AND last_inform_at IS NOT NULL AND last_inform_at < ?`,
		offlineStatusID, offlineStatusID, staleBefore)
	if err != nil {
		return 0, translateErr(err)
	}
	return res.RowsAffected()
}

func (r *deviceRepository) SoftDelete(ctx context.Context, id, deletedBy uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE devices SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now(), deletedBy, id)
	return translateErr(err)
}

// ---- DeviceParameter ----

type deviceParameterRepository struct{ db *sqlx.DB }

func NewDeviceParameterRepository(db *sqlx.DB) domain.DeviceParameterRepository {
	return &deviceParameterRepository{db: db}
}

func (r *deviceParameterRepository) Upsert(ctx context.Context, p *domain.DeviceParameter) error {
	const q = `INSERT INTO device_parameters (device_id, parameter_name, parameter_value, parameter_type_id, writable)
		VALUES (:device_id, :parameter_name, :parameter_value, :parameter_type_id, :writable)
		ON DUPLICATE KEY UPDATE parameter_value = VALUES(parameter_value),
			parameter_type_id = VALUES(parameter_type_id), writable = VALUES(writable)`
	_, err := r.db.NamedExecContext(ctx, q, p)
	return translateErr(err)
}

func (r *deviceParameterRepository) UpsertBatch(ctx context.Context, ps []domain.DeviceParameter) error {
	if len(ps) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translateErr(err)
	}
	const q = `INSERT INTO device_parameters (device_id, parameter_name, parameter_value, parameter_type_id, writable)
		VALUES (:device_id, :parameter_name, :parameter_value, :parameter_type_id, :writable)
		ON DUPLICATE KEY UPDATE parameter_value = VALUES(parameter_value),
			parameter_type_id = VALUES(parameter_type_id), writable = VALUES(writable)`
	for i := range ps {
		if _, err := tx.NamedExecContext(ctx, q, &ps[i]); err != nil {
			_ = tx.Rollback()
			return translateErr(err)
		}
	}
	return tx.Commit()
}

func (r *deviceParameterRepository) ListByDevice(ctx context.Context, deviceID uint64, prefix string) ([]domain.DeviceParameter, error) {
	var rows []domain.DeviceParameter
	if prefix == "" {
		err := r.db.SelectContext(ctx, &rows,
			`SELECT * FROM device_parameters WHERE device_id = ? ORDER BY parameter_name`, deviceID)
		return rows, translateErr(err)
	}
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM device_parameters WHERE device_id = ? AND parameter_name LIKE ? ORDER BY parameter_name`,
		deviceID, prefix+"%")
	return rows, translateErr(err)
}

func (r *deviceParameterRepository) Get(ctx context.Context, deviceID uint64, name string) (*domain.DeviceParameter, error) {
	var p domain.DeviceParameter
	err := r.db.GetContext(ctx, &p,
		`SELECT * FROM device_parameters WHERE device_id = ? AND parameter_name = ?`, deviceID, name)
	if err != nil {
		return nil, translateErr(err)
	}
	return &p, nil
}

// ---- DeviceSession ----

type deviceSessionRepository struct{ db *sqlx.DB }

func NewDeviceSessionRepository(db *sqlx.DB) domain.DeviceSessionRepository {
	return &deviceSessionRepository{db: db}
}

func (r *deviceSessionRepository) Create(ctx context.Context, s *domain.DeviceSession) error {
	const q = `INSERT INTO device_sessions (device_id, session_token, cwmp_id, status, remote_ip, started_at)
		VALUES (:device_id, :session_token, :cwmp_id, :status, :remote_ip, :started_at)`
	res, err := r.db.NamedExecContext(ctx, q, s)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	s.ID = uint64(id)
	return nil
}

func (r *deviceSessionRepository) GetByToken(ctx context.Context, token string) (*domain.DeviceSession, error) {
	var s domain.DeviceSession
	if err := r.db.GetContext(ctx, &s, `SELECT * FROM device_sessions WHERE session_token = ?`, token); err != nil {
		return nil, translateErr(err)
	}
	return &s, nil
}

func (r *deviceSessionRepository) UpdateStatus(ctx context.Context, id uint64, status string, endedAt *time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE device_sessions SET status = ?, ended_at = ? WHERE id = ?`, status, endedAt, id)
	return translateErr(err)
}

func (r *deviceSessionRepository) SetCWMPID(ctx context.Context, id uint64, cwmpID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE device_sessions SET cwmp_id = ? WHERE id = ?`, cwmpID, id)
	return translateErr(err)
}

// ---- DeviceEvent ----

type deviceEventRepository struct{ db *sqlx.DB }

func NewDeviceEventRepository(db *sqlx.DB) domain.DeviceEventRepository {
	return &deviceEventRepository{db: db}
}

func (r *deviceEventRepository) Create(ctx context.Context, e *domain.DeviceEvent) error {
	const q = `INSERT INTO device_events (device_id, session_id, event_code_id, command_key, raw_payload, occurred_at)
		VALUES (:device_id, :session_id, :event_code_id, :command_key, :raw_payload, :occurred_at)`
	res, err := r.db.NamedExecContext(ctx, q, e)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	e.ID = uint64(id)
	return nil
}

func (r *deviceEventRepository) ListByDevice(ctx context.Context, deviceID uint64, p domain.Pagination) ([]domain.DeviceEvent, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM device_events WHERE device_id = ?`, deviceID); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.DeviceEvent
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM device_events WHERE device_id = ? ORDER BY occurred_at DESC LIMIT ? OFFSET ?`,
		deviceID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

// ---- DeviceOpticalMetric ----

type deviceOpticalMetricRepository struct{ db *sqlx.DB }

func NewDeviceOpticalMetricRepository(db *sqlx.DB) domain.DeviceOpticalMetricRepository {
	return &deviceOpticalMetricRepository{db: db}
}

func (r *deviceOpticalMetricRepository) Create(ctx context.Context, m *domain.DeviceOpticalMetric) error {
	const q = `INSERT INTO device_optical_metrics
		(device_id, rx_power_dbm, tx_power_dbm, voltage, bias_current_ma, temperature_celsius, distance_meters, recorded_at)
		VALUES (:device_id, :rx_power_dbm, :tx_power_dbm, :voltage, :bias_current_ma, :temperature_celsius, :distance_meters, :recorded_at)`
	res, err := r.db.NamedExecContext(ctx, q, m)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	m.ID = uint64(id)
	return nil
}

func (r *deviceOpticalMetricRepository) Latest(ctx context.Context, deviceID uint64) (*domain.DeviceOpticalMetric, error) {
	var m domain.DeviceOpticalMetric
	err := r.db.GetContext(ctx, &m,
		`SELECT * FROM device_optical_metrics WHERE device_id = ? ORDER BY recorded_at DESC LIMIT 1`, deviceID)
	if err != nil {
		return nil, translateErr(err)
	}
	return &m, nil
}

func (r *deviceOpticalMetricRepository) ListByDevice(ctx context.Context, deviceID uint64, p domain.Pagination) ([]domain.DeviceOpticalMetric, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM device_optical_metrics WHERE device_id = ?`, deviceID); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.DeviceOpticalMetric
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM device_optical_metrics WHERE device_id = ? ORDER BY recorded_at DESC LIMIT ? OFFSET ?`,
		deviceID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}
