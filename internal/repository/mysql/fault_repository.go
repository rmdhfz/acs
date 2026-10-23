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

type faultRepository struct{ db *sqlx.DB }

func NewFaultRepository(db *sqlx.DB) domain.FaultRepository { return &faultRepository{db: db} }

// faultCols — kolom SELECT dengan join devices (serial & tenant untuk scope). Alias d dipakai di semua query.
const faultCols = `f.id, f.fault_uuid, f.device_id, d.serial_number AS device_serial, d.tenant_id AS device_tenant_id,
	f.task_id, f.rpc_method, f.fault_code, f.fault_string, f.occurrence_count, f.first_seen_at, f.last_seen_at`

func (r *faultRepository) Upsert(ctx context.Context, f *domain.DeviceFault) error {
	now := time.Now()
	// Dedup per (device, rpc_method, fault_code) aktif. Dua sesi yang bersamaan bisa sama-sama jatuh ke INSERT dan
	// menghasilkan dua baris; dampaknya hanya hitungan terpecah (tidak ada unique key agar baris yang sudah ditutup
	// tidak memblokir fault yang sama muncul lagi).
	res, err := r.db.ExecContext(ctx,
		`UPDATE device_faults SET occurrence_count = occurrence_count + 1, last_seen_at = ?, fault_string = ?, task_id = ?
		 WHERE device_id = ? AND rpc_method = ? AND fault_code = ? AND is_deleted = 0`,
		now, f.FaultString, f.TaskID, f.DeviceID, f.RPCMethod, f.FaultCode)
	if err != nil {
		return translateErr(err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	// Jalur INSERT (jarang): batasi baris aktif per perangkat. fault_code dari CPE tidak tepercaya; tanpa batas tabel bisa tumbuh tanpa henti.
	var active int
	if err := r.db.GetContext(ctx, &active, `SELECT COUNT(*) FROM device_faults WHERE device_id = ? AND is_deleted = 0`, f.DeviceID); err != nil {
		return translateErr(err)
	}
	if active >= domain.FaultMaxActivePerDevice {
		return fmt.Errorf("%w: perangkat sudah memiliki %d fault aktif (batas)", domain.ErrQuotaExceeded, active)
	}
	f.UUID = uuid.NewString()
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO device_faults (fault_uuid, device_id, task_id, rpc_method, fault_code, fault_string, first_seen_at, last_seen_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		f.UUID, f.DeviceID, f.TaskID, f.RPCMethod, f.FaultCode, f.FaultString, now, now)
	return translateErr(err)
}

func (r *faultRepository) ResolveActive(ctx context.Context, deviceID uint64, rpcMethod string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE device_faults SET is_deleted = 1, deleted_at = ? WHERE device_id = ? AND rpc_method = ? AND is_deleted = 0`,
		time.Now(), deviceID, rpcMethod)
	if err != nil {
		return 0, translateErr(err)
	}
	return res.RowsAffected()
}

func (r *faultRepository) List(ctx context.Context, f domain.FaultFilter, p domain.Pagination) ([]domain.DeviceFault, int, error) {
	where := []string{"f.is_deleted = 0", "d.is_deleted = 0"}
	args := []interface{}{}
	if f.TenantID != nil {
		where = append(where, "d.tenant_id = ?")
		args = append(args, *f.TenantID)
	}
	if f.DeviceID != nil {
		where = append(where, "f.device_id = ?")
		args = append(args, *f.DeviceID)
	}
	if f.FaultCode != "" {
		where = append(where, "f.fault_code = ?")
		args = append(args, f.FaultCode)
	}
	if f.RPCMethod != "" {
		where = append(where, "f.rpc_method = ?")
		args = append(args, f.RPCMethod)
	}
	if f.Search != "" {
		like := "%" + likeEscape(f.Search) + "%"
		where = append(where, "(d.serial_number LIKE ? OR f.fault_string LIKE ?)")
		args = append(args, like, like)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := r.db.GetContext(ctx, &total,
		`SELECT COUNT(*) FROM device_faults f JOIN devices d ON d.id = f.device_id WHERE `+whereSQL, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	rows := []domain.DeviceFault{}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT `+faultCols+` FROM device_faults f JOIN devices d ON d.id = f.device_id WHERE `+whereSQL+
			` ORDER BY f.last_seen_at DESC, f.id DESC LIMIT ? OFFSET ?`,
		append(args, p.Limit(), p.Offset())...); err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *faultRepository) GetByUUID(ctx context.Context, uuid string) (*domain.DeviceFault, error) {
	var f domain.DeviceFault
	if err := r.db.GetContext(ctx, &f,
		`SELECT `+faultCols+` FROM device_faults f JOIN devices d ON d.id = f.device_id
		 WHERE f.fault_uuid = ? AND f.is_deleted = 0 AND d.is_deleted = 0`, uuid); err != nil {
		return nil, translateErr(err)
	}
	return &f, nil
}

func (r *faultRepository) Dismiss(ctx context.Context, id, by uint64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE device_faults SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ? AND is_deleted = 0`,
		time.Now(), by, id)
	if err != nil {
		return translateErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
