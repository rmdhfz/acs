package mysql

import (
	"context"
	"strings"
	"time"

	"acs/internal/domain"

	"github.com/jmoiron/sqlx"
)

type oltRepository struct{ db *sqlx.DB }

func NewOLTRepository(db *sqlx.DB) domain.OLTRepository { return &oltRepository{db: db} }

// oltCols/oltFrom — OLT dengan kode status poll (join). Alias o dipakai di semua query.
const oltCols = `o.id, o.olt_uuid, o.tenant_id, o.name, o.driver_code, o.host, o.snmp_port, o.ssh_port, o.snmp_community_enc,
	o.ssh_username, o.ssh_password_enc, o.notes, o.is_active, o.poll_status_id, rs.code AS poll_status, o.last_poll_at,
	o.last_poll_error, o.reported_name, o.reported_version, o.created_at, o.created_by, o.updated_at, o.updated_by,
	o.deleted_at, o.deleted_by, o.is_deleted`
const oltFrom = ` FROM olts o JOIN ref_olt_poll_status rs ON rs.id = o.poll_status_id `

func (r *oltRepository) Create(ctx context.Context, o *domain.OLT) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO olts (olt_uuid, tenant_id, name, driver_code, host, snmp_port, ssh_port, snmp_community_enc, ssh_username,
		   ssh_password_enc, notes, is_active, poll_status_id, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.UUID, o.TenantID, o.Name, o.DriverCode, o.Host, o.SNMPPort, o.SSHPort, o.SNMPCommunityEnc, o.SSHUsername,
		o.SSHPasswordEnc, o.Notes, o.IsActive, o.PollStatusID, o.CreatedBy)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	o.ID = uint64(id)
	return nil
}

func (r *oltRepository) GetByUUID(ctx context.Context, uuid string) (*domain.OLT, error) {
	var o domain.OLT
	if err := r.db.GetContext(ctx, &o,
		`SELECT `+oltCols+oltFrom+`WHERE o.olt_uuid = ? AND o.is_deleted = 0`, uuid); err != nil {
		return nil, translateErr(err)
	}
	return &o, nil
}

func (r *oltRepository) List(ctx context.Context, f domain.OLTFilter, p domain.Pagination) ([]domain.OLT, int, error) {
	where := []string{"o.is_deleted = 0"}
	args := []interface{}{}
	if f.TenantID != nil {
		where = append(where, "o.tenant_id = ?")
		args = append(args, *f.TenantID)
	}
	if f.Search != "" {
		like := "%" + likeEscape(f.Search) + "%"
		where = append(where, "(o.name LIKE ? OR o.host LIKE ?)")
		args = append(args, like, like)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*)`+oltFrom+`WHERE `+whereSQL, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	rows := []domain.OLT{}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT `+oltCols+oltFrom+`WHERE `+whereSQL+` ORDER BY o.name ASC, o.id ASC LIMIT ? OFFSET ?`,
		append(args, p.Limit(), p.Offset())...); err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *oltRepository) Update(ctx context.Context, o *domain.OLT) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE olts SET name = ?, driver_code = ?, host = ?, snmp_port = ?, ssh_port = ?, snmp_community_enc = ?, ssh_username = ?,
		   ssh_password_enc = ?, notes = ?, is_active = ?, updated_by = ?
		 WHERE id = ? AND is_deleted = 0`,
		o.Name, o.DriverCode, o.Host, o.SNMPPort, o.SSHPort, o.SNMPCommunityEnc, o.SSHUsername,
		o.SSHPasswordEnc, o.Notes, o.IsActive, o.UpdatedBy, o.ID)
	if err != nil {
		return translateErr(err)
	}
	// RowsAffected tidak menghitung baris yang nilainya tak berubah: nol belum tentu "tidak ada".
	if n, _ := res.RowsAffected(); n == 0 {
		var exists bool
		if err := r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM olts WHERE id = ? AND is_deleted = 0)`, o.ID); err != nil {
			return translateErr(err)
		}
		if !exists {
			return domain.ErrNotFound
		}
	}
	return nil
}

func (r *oltRepository) SoftDelete(ctx context.Context, id, by uint64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translateErr(err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now()
	res, err := tx.ExecContext(ctx, `UPDATE olts SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ? AND is_deleted = 0`, now, by, id)
	if err != nil {
		return translateErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE olt_unregistered_onus SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE olt_id = ? AND is_deleted = 0`, now, by, id); err != nil {
		return translateErr(err)
	}
	return translateErr(tx.Commit())
}

func (r *oltRepository) RecordPoll(ctx context.Context, id uint64, res domain.OLTPollResult) error {
	var name, version *string
	if res.Info != nil {
		name, version = &res.Info.Name, &res.Info.Version
	}
	// COALESCE: identitas hanya diganti bila Probe/poll berhasil membawanya; galat tidak menghapus nilai terakhir yang diketahui.
	_, err := r.db.ExecContext(ctx,
		`UPDATE olts SET poll_status_id = ?, last_poll_at = ?, last_poll_error = ?,
		   reported_name = COALESCE(?, reported_name), reported_version = COALESCE(?, reported_version)
		 WHERE id = ? AND is_deleted = 0`,
		res.StatusID, time.Now(), res.Error, name, version, id)
	return translateErr(err)
}

// ---- antrean ONU ----

const onuCols = `n.id, n.onu_uuid, n.olt_id, o.olt_uuid, o.name AS olt_name, o.tenant_id, n.pon_port, n.serial_number, n.onu_type,
	n.status_id, s.code AS status, n.first_seen_at, n.last_seen_at, n.resolved_at`
const onuFrom = ` FROM olt_unregistered_onus n JOIN olts o ON o.id = n.olt_id JOIN ref_onu_discovery_status s ON s.id = n.status_id `

func (r *oltRepository) ListONUsByOLT(ctx context.Context, oltID uint64) ([]domain.DiscoveredONU, error) {
	rows := []domain.DiscoveredONU{}
	// Batas pengaman: antrean aktif + riwayat RESOLVED yang dipangkas tidak melebihi jumlah ini dalam keadaan normal.
	limit := domain.OLTMaxUnregisteredONUs + domain.OLTKeepResolvedONUs + 1
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT `+onuCols+onuFrom+`WHERE n.olt_id = ? AND n.is_deleted = 0 AND o.is_deleted = 0 LIMIT ?`, oltID, limit); err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *oltRepository) UpsertONUs(ctx context.Context, oltID uint64, rows []domain.DiscoveredONU) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translateErr(err)
	}
	defer func() { _ = tx.Rollback() }()
	const chunk = 200
	for start := 0; start < len(rows); start += chunk {
		end := start + chunk
		if end > len(rows) {
			end = len(rows)
		}
		part := rows[start:end]
		var sb strings.Builder
		sb.WriteString(`INSERT INTO olt_unregistered_onus (onu_uuid, olt_id, pon_port, serial_number, onu_type, status_id, first_seen_at, last_seen_at, resolved_at) VALUES `)
		args := make([]interface{}, 0, len(part)*9)
		for i, o := range part {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString("(?,?,?,?,?,?,?,?,?)")
			args = append(args, o.UUID, oltID, o.PonPort, o.SerialNumber, o.ONUType, o.StatusID, o.FirstSeenAt, o.LastSeenAt, o.ResolvedAt)
		}
		// first_seen_at dievaluasi lebih dulu (memakai is_deleted lama): baris yang dihapus-lunak lalu muncul lagi dihitung sebagai
		// penemuan baru. Urutan penugasan MySQL/MariaDB dari kiri ke kanan; onu_uuid sengaja tidak diubah.
		sb.WriteString(` ON DUPLICATE KEY UPDATE
			first_seen_at = IF(is_deleted = 1, VALUES(first_seen_at), first_seen_at),
			pon_port = VALUES(pon_port), onu_type = VALUES(onu_type), status_id = VALUES(status_id),
			last_seen_at = VALUES(last_seen_at), resolved_at = VALUES(resolved_at),
			is_deleted = 0, deleted_at = NULL, deleted_by = NULL`)
		if _, err := tx.ExecContext(ctx, sb.String(), args...); err != nil {
			return translateErr(err)
		}
	}
	return translateErr(tx.Commit())
}

func (r *oltRepository) SetONUsStatus(ctx context.Context, ids []uint64, statusID uint64, resolvedAt *time.Time, by *uint64) error {
	const chunk = 500
	for start := 0; start < len(ids); start += chunk {
		end := start + chunk
		if end > len(ids) {
			end = len(ids)
		}
		q, args, err := sqlx.In(`UPDATE olt_unregistered_onus SET status_id = ?, resolved_at = ?, updated_by = ? WHERE id IN (?) AND is_deleted = 0`,
			statusID, resolvedAt, by, ids[start:end])
		if err != nil {
			return translateErr(err)
		}
		if _, err := r.db.ExecContext(ctx, r.db.Rebind(q), args...); err != nil {
			return translateErr(err)
		}
	}
	return nil
}

func (r *oltRepository) PruneResolvedONUs(ctx context.Context, oltID, resolvedStatusID uint64, keep int) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE olt_unregistered_onus SET is_deleted = 1, deleted_at = ?
		 WHERE olt_id = ? AND is_deleted = 0 AND status_id = ?
		   AND id NOT IN (SELECT id FROM (
		     SELECT id FROM olt_unregistered_onus WHERE olt_id = ? AND is_deleted = 0 AND status_id = ?
		     ORDER BY resolved_at DESC, id DESC LIMIT ?) keepers)`,
		time.Now(), oltID, resolvedStatusID, oltID, resolvedStatusID, keep)
	if err != nil {
		return 0, translateErr(err)
	}
	return res.RowsAffected()
}

func (r *oltRepository) ListONUs(ctx context.Context, f domain.ONUFilter, p domain.Pagination) ([]domain.DiscoveredONU, int, error) {
	where := []string{"n.is_deleted = 0", "o.is_deleted = 0"}
	args := []interface{}{}
	if f.TenantID != nil {
		where = append(where, "o.tenant_id = ?")
		args = append(args, *f.TenantID)
	}
	if f.OLTID != nil {
		where = append(where, "n.olt_id = ?")
		args = append(args, *f.OLTID)
	}
	if f.StatusID != nil {
		where = append(where, "n.status_id = ?")
		args = append(args, *f.StatusID)
	}
	if f.Search != "" {
		like := "%" + likeEscape(f.Search) + "%"
		where = append(where, "(n.serial_number LIKE ? OR n.pon_port LIKE ?)")
		args = append(args, like, like)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*)`+onuFrom+`WHERE `+whereSQL, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	rows := []domain.DiscoveredONU{}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT `+onuCols+onuFrom+`WHERE `+whereSQL+` ORDER BY n.last_seen_at DESC, n.id DESC LIMIT ? OFFSET ?`,
		append(args, p.Limit(), p.Offset())...); err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *oltRepository) GetONUByUUID(ctx context.Context, uuid string) (*domain.DiscoveredONU, error) {
	var n domain.DiscoveredONU
	if err := r.db.GetContext(ctx, &n,
		`SELECT `+onuCols+onuFrom+`WHERE n.onu_uuid = ? AND n.is_deleted = 0 AND o.is_deleted = 0`, uuid); err != nil {
		return nil, translateErr(err)
	}
	return &n, nil
}
