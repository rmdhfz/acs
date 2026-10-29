package mysql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"acs/internal/domain"

	"github.com/jmoiron/sqlx"
)

// Dipenuhi oleh oltRepository yang sama dengan antrean ONU belum terdaftar (satu tipe, dua antarmuka).
var _ domain.OLTInventoryRepository = (*oltRepository)(nil)

// NewOLTInventoryRepository mengembalikan repositori inventaris ONU terdaftar (tabel olt_registered_onus).
func NewOLTInventoryRepository(db *sqlx.DB) domain.OLTInventoryRepository {
	return &oltRepository{db: db}
}

// regCols/regFrom — ONU terdaftar dengan nama OLT, tenant, dan kode keadaan (join). Alias n = ONU, o = OLT, s = keadaan.
const regCols = `n.id, n.onu_uuid, n.olt_id, o.olt_uuid, o.name AS olt_name, o.tenant_id, n.pon_port, n.onu_index, n.serial_number,
	n.onu_type, n.state_id, s.code AS state, n.vendor_state, n.rx_power_dbm, n.first_seen_at, n.last_seen_at,
	n.last_state_change_at, o.last_poll_at AS olt_last_poll_at`
const regFrom = ` FROM olt_registered_onus n JOIN olts o ON o.id = n.olt_id JOIN ref_olt_onu_state s ON s.id = n.state_id `

// regLink — tautan ke perangkat ACS: serial sama (collation tak peka huruf besar/kecil) dan tenant sama. Subquery LIMIT 1 (bukan join biasa)
// karena serial yang sama dapat ada pada dua perangkat berbeda OUI; yang paling baru Inform menang, dan baris ONU tidak pernah tergandakan.
const regLinkCols = `, d.id AS device_id, ds.code AS device_status`
const regLinkFrom = ` LEFT JOIN devices d ON d.id = (
		SELECT d2.id FROM devices d2 WHERE d2.serial_number = n.serial_number AND d2.tenant_id = o.tenant_id AND d2.is_deleted = 0
		ORDER BY d2.last_inform_at IS NULL, d2.last_inform_at DESC, d2.id DESC LIMIT 1)
	LEFT JOIN ref_device_status ds ON ds.id = d.device_status_id `

func (r *oltRepository) ListRegisteredByOLT(ctx context.Context, oltID uint64) ([]domain.RegisteredONU, error) {
	rows := []domain.RegisteredONU{}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT `+regCols+regFrom+`WHERE n.olt_id = ? AND n.is_deleted = 0 AND o.is_deleted = 0 LIMIT ?`,
		oltID, domain.OLTMaxRegisteredONUs+1); err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *oltRepository) UpsertRegisteredONUs(ctx context.Context, oltID uint64, rows []domain.RegisteredONU) error {
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
		sb.WriteString(`INSERT INTO olt_registered_onus (onu_uuid, olt_id, pon_port, onu_index, serial_number, onu_type, state_id, vendor_state,
			rx_power_dbm, first_seen_at, last_seen_at, last_state_change_at) VALUES `)
		args := make([]interface{}, 0, len(part)*12)
		for i, o := range part {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString("(?,?,?,?,?,?,?,?,?,?,?,?)")
			args = append(args, o.UUID, oltID, o.PonPort, o.ONUIndex, o.SerialNumber, o.ONUType, o.StateID, o.VendorState,
				o.RxPowerDBM, o.FirstSeenAt, o.LastSeenAt, o.LastStateChangeAt)
		}
		// Urutan penugasan MySQL/MariaDB dari kiri ke kanan: first_seen_at dan last_state_change_at HARUS dievaluasi sebelum
		// is_deleted dan state_id diubah, karena keduanya membandingkan dengan nilai LAMA. Baris yang dihapus-lunak lalu muncul lagi
		// dihitung sebagai penemuan baru; last_state_change_at hanya maju bila keadaan benar-benar berbeda. onu_uuid tidak diubah.
		sb.WriteString(` ON DUPLICATE KEY UPDATE
			first_seen_at = IF(is_deleted = 1, VALUES(first_seen_at), first_seen_at),
			last_state_change_at = IF(is_deleted = 1 OR state_id <> VALUES(state_id), VALUES(last_state_change_at), last_state_change_at),
			pon_port = VALUES(pon_port), onu_index = VALUES(onu_index), onu_type = VALUES(onu_type), state_id = VALUES(state_id),
			vendor_state = VALUES(vendor_state), rx_power_dbm = VALUES(rx_power_dbm), last_seen_at = VALUES(last_seen_at),
			is_deleted = 0, deleted_at = NULL, deleted_by = NULL`)
		if _, err := tx.ExecContext(ctx, sb.String(), args...); err != nil {
			return translateErr(err)
		}
	}
	return translateErr(tx.Commit())
}

func (r *oltRepository) RemoveRegisteredONUs(ctx context.Context, ids []uint64, by *uint64) error {
	const chunk = 500
	now := time.Now()
	for start := 0; start < len(ids); start += chunk {
		end := start + chunk
		if end > len(ids) {
			end = len(ids)
		}
		q, args, err := sqlx.In(`UPDATE olt_registered_onus SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id IN (?) AND is_deleted = 0`,
			now, by, ids[start:end])
		if err != nil {
			return translateErr(err)
		}
		if _, err := r.db.ExecContext(ctx, r.db.Rebind(q), args...); err != nil {
			return translateErr(err)
		}
	}
	return nil
}

func (r *oltRepository) ListRegisteredONUs(ctx context.Context, f domain.RegisteredONUFilter, p domain.Pagination) ([]domain.RegisteredONU, int, error) {
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
	if f.PonPort != "" {
		where = append(where, "n.pon_port = ?")
		args = append(args, f.PonPort)
	}
	if f.StateID != nil {
		where = append(where, "n.state_id = ?")
		args = append(args, *f.StateID)
	}
	if f.ProblemOnly {
		where = append(where, "s.code IN (?, ?, ?)")
		for _, c := range domain.ONUProblemStates {
			args = append(args, c)
		}
	}
	if f.Search != "" {
		like := "%" + likeEscape(f.Search) + "%"
		where = append(where, "(n.serial_number LIKE ? OR n.pon_port LIKE ?)")
		args = append(args, like, like)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*)`+regFrom+`WHERE `+whereSQL, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	rows := []domain.RegisteredONU{}
	// Urutan stabil untuk paginasi: (opsional keparahan keadaan dulu,) port, nomor ONU (numerik bila memungkinkan), serial.
	order := ` ORDER BY n.pon_port, CAST(n.onu_index AS UNSIGNED), n.serial_number`
	if f.SortSeverity {
		sev := "CASE s.code"
		for i, c := range domain.ONUSeverityOrder {
			sev += fmt.Sprintf(" WHEN '%s' THEN %d", c, i) // konstanta kode dari domain (bukan masukan pengguna)
		}
		sev += " ELSE 99 END"
		order = ` ORDER BY ` + sev + `, n.pon_port, CAST(n.onu_index AS UNSIGNED), n.serial_number`
	}
	args = append(args, p.Limit(), p.Offset())
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT `+regCols+regLinkCols+regFrom+regLinkFrom+`WHERE `+whereSQL+order+` LIMIT ? OFFSET ?`, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *oltRepository) PortSummaries(ctx context.Context, oltID uint64) ([]domain.PONPortSummary, error) {
	rows := []domain.PONPortSummary{}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT n.pon_port, COUNT(*) AS total,
			COALESCE(SUM(s.code = 'ONLINE'), 0) AS n_online, COALESCE(SUM(s.code = 'OFFLINE'), 0) AS n_offline,
			COALESCE(SUM(s.code = 'LOS'), 0) AS n_los, COALESCE(SUM(s.code = 'DYING_GASP'), 0) AS n_dying_gasp,
			COALESCE(SUM(s.code = 'UNKNOWN'), 0) AS n_unknown,
			COALESCE(SUM(EXISTS (SELECT 1 FROM devices d WHERE d.serial_number = n.serial_number AND d.tenant_id = o.tenant_id AND d.is_deleted = 0)), 0) AS linked,
			MIN(n.rx_power_dbm) AS min_rx_dbm, AVG(n.rx_power_dbm) AS avg_rx_dbm, MAX(n.last_seen_at) AS last_seen_at`+
			regFrom+`WHERE n.olt_id = ? AND n.is_deleted = 0 AND o.is_deleted = 0 GROUP BY n.pon_port ORDER BY n.pon_port`, oltID); err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *oltRepository) FindRegisteredBySerial(ctx context.Context, tenantID uint64, serial string) (*domain.RegisteredONU, error) {
	var row domain.RegisteredONU
	err := r.db.GetContext(ctx, &row,
		`SELECT `+regCols+regLinkCols+regFrom+regLinkFrom+
			`WHERE n.serial_number = ? AND o.tenant_id = ? AND n.is_deleted = 0 AND o.is_deleted = 0 AND o.is_active = 1
			 ORDER BY n.last_seen_at DESC, n.id DESC LIMIT 1`, serial, tenantID)
	if err != nil {
		return nil, translateErr(err)
	}
	return &row, nil
}

func (r *oltRepository) CountActiveOLTs(ctx context.Context, tenantID uint64) (int, error) {
	var n int
	if err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM olts WHERE tenant_id = ? AND is_active = 1 AND is_deleted = 0`, tenantID); err != nil {
		return 0, translateErr(err)
	}
	return n, nil
}

func (r *oltRepository) ListOLTsDueForPoll(ctx context.Context, before time.Time, limit int) ([]domain.OLT, error) {
	rows := []domain.OLT{}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT `+oltCols+oltFrom+`WHERE o.is_active = 1 AND o.is_deleted = 0 AND (o.last_poll_at IS NULL OR o.last_poll_at < ?)
		 ORDER BY o.last_poll_at IS NOT NULL, o.last_poll_at, o.id LIMIT ?`, before, limit); err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}
