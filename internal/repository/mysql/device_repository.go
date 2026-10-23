package mysql

import (
	"context"
	"errors"
	"fmt"
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

	// Revive: uq_devices_oui_serial mencakup baris yang sudah di-soft-delete,
	// sedangkan GetByOUISerial menyaring is_deleted -> CPE yang pernah dihapus
	// lalu Inform lagi dianggap baru dan INSERT di bawah akan kena duplicate
	// key (Inform gagal, CPE tak bisa terhubung). Maka baris terhapus dengan
	// (oui, serial) yang sama dihidupkan lagi sebagai registrasi baru: id &
	// uuid & riwayat (event/parameter) dipertahankan, tetapi kredensial
	// per-device, profil provisioning, dan catatan di-RESET karena itu milik
	// registrasi sebelumnya. Baris AKTIF tidak tersentuh (is_deleted = 1).
	const revive = `UPDATE devices SET
		is_deleted = 0, deleted_at = NULL, deleted_by = NULL,
		tenant_id = :tenant_id, vendor_id = :vendor_id, device_model_id = :device_model_id,
		device_status_id = :device_status_id, provisioning_profile_id = NULL,
		product_class = :product_class, mac_address = :mac_address,
		software_version = :software_version, hardware_version = :hardware_version, ip_address = :ip_address,
		connection_request_url = :connection_request_url, connection_request_username = NULL,
		connection_request_password_enc = NULL, inform_username = NULL, inform_password_enc = NULL,
		last_inform_at = :last_inform_at, last_boot_event_at = NULL, notes = NULL,
		latitude = :latitude, longitude = :longitude,
		updated_at = :updated_at, updated_by = :created_by
		WHERE oui = :oui AND serial_number = :serial_number AND is_deleted = 1
		  AND (tenant_id IS NULL OR tenant_id <=> :tenant_id)`
	// Klausa tenant di atas MENCEGAH pengambilalihan lintas tenant: tanpanya,
	// tenant B (yang punya shared secret CWMP-nya sendiri) bisa me-revive baris
	// device yang dihapus tenant A hanya dengan mengirim Inform memakai OUI+serial
	// yang sama — baris (id, uuid, riwayat parameter/event/task) berpindah ke B.
	// Revive lintas tenant TIDAK terjadi; INSERT di bawah lalu kena duplicate key
	// -> ErrConflict (Inform ditolak 409), dan baris tetap milik tenant asalnya
	// sampai superadmin menanganinya secara sadar.
	const q = `INSERT INTO devices
		(device_uuid, tenant_id, vendor_id, device_model_id, device_status_id, provisioning_profile_id,
		 oui, serial_number, product_class, mac_address, software_version, hardware_version, ip_address,
		 connection_request_url, connection_request_username, connection_request_password_enc,
		 inform_username, inform_password_enc, last_inform_at, last_boot_event_at, notes, latitude, longitude, created_at, updated_at, created_by)
		VALUES
		(:device_uuid, :tenant_id, :vendor_id, :device_model_id, :device_status_id, :provisioning_profile_id,
		 :oui, :serial_number, :product_class, :mac_address, :software_version, :hardware_version, :ip_address,
		 :connection_request_url, :connection_request_username, :connection_request_password_enc,
		 :inform_username, :inform_password_enc, :last_inform_at, :last_boot_event_at, :notes, :latitude, :longitude, :created_at, :updated_at, :created_by)`
	// INSERT lebih dulu: jalur umum (CPE benar-benar baru) tidak membayar UPDATE revive yang pasti tak cocok. Revive hanya dicoba
	// bila INSERT kena duplicate key (baris soft-delete dengan oui+serial yang sama ATAU pendaftaran serentak).
	res, err := r.db.NamedExecContext(ctx, q, d)
	if err == nil {
		id, _ := res.LastInsertId()
		d.ID = uint64(id)
		return nil
	}
	te := translateErr(err)
	if !errors.Is(te, domain.ErrConflict) {
		return te
	}
	rres, rerr := r.db.NamedExecContext(ctx, revive, d)
	if rerr != nil {
		return translateErr(rerr)
	}
	if n, _ := rres.RowsAffected(); n > 0 {
		var row struct {
			ID   uint64 `db:"id"`
			UUID string `db:"device_uuid"`
		}
		if err := r.db.GetContext(ctx, &row,
			`SELECT id, device_uuid FROM devices WHERE oui = ? AND serial_number = ? AND is_deleted = 0`,
			d.OUI, d.SerialNumber); err != nil {
			return translateErr(err)
		}
		d.ID, d.DeviceUUID = row.ID, row.UUID
		return nil
	}

	return te // duplicate key bukan dari baris terhapus yang boleh dihidupkan (aktif/tenant lain) -> ErrConflict apa adanya
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
	if f.SerialNumber != "" {
		where = append(where, "serial_number = ?")
		args = append(args, f.SerialNumber)
	}
	if f.Search != "" {
		like := "%" + likeEscape(f.Search) + "%"
		cond := "serial_number LIKE ? OR mac_address LIKE ? OR product_class LIKE ?"
		args = append(args, like, like, like)
		// Username PPPoE (parameter standar TR-098/TR-181) — hanya untuk kata kunci >= 4 karakter agar tidak memindai terlalu luas.
		if len([]rune(f.Search)) >= 4 {
			cond += " OR EXISTS (SELECT 1 FROM device_parameters p WHERE p.device_id = devices.id AND p.parameter_value LIKE ?" +
				" AND (p.parameter_name LIKE 'InternetGatewayDevice.WANDevice.%.WANPPPConnection.%.Username' OR p.parameter_name LIKE 'Device.PPP.Interface.%.Username'))"
			args = append(args, like)
		}
		where = append(where, "("+cond+")")
	}
	if f.TagID != nil {
		// EXISTS lebih aman dari JOIN di sini: tidak menduplikasi baris device
		// bila (hipotetis) ada >1 relasi, dan tidak mengubah bentuk SELECT *.
		where = append(where, "EXISTS (SELECT 1 FROM device_tags dt WHERE dt.device_id = devices.id AND dt.tag_id = ?)")
		args = append(args, *f.TagID)
	}
	if f.NoVendor {
		where = append(where, "vendor_id IS NULL")
	}
	if f.OpticalLevel != "" {
		// RX dari pembacaan terakhir (urutan sama dengan ringkasan daftar); tanpa pembacaan/RX NULL = unknown.
		const rx = "(SELECT m.rx_power_dbm FROM device_optical_metrics m WHERE m.device_id = devices.id ORDER BY m.recorded_at DESC, m.id DESC LIMIT 1)"
		switch f.OpticalLevel {
		case domain.OpticalUnknown:
			where = append(where, rx+" IS NULL")
		case domain.OpticalBad:
			where = append(where, "("+rx+" < ? OR "+rx+" > ?)")
			args = append(args, domain.OpticalRXBadBelowDBM, domain.OpticalRXOverloadAboveDBM)
		case domain.OpticalWarn:
			where = append(where, "("+rx+" >= ? AND "+rx+" < ?)")
			args = append(args, domain.OpticalRXBadBelowDBM, domain.OpticalRXWarnBelowDBM)
		case domain.OpticalOK:
			where = append(where, "("+rx+" >= ? AND "+rx+" <= ?)")
			args = append(args, domain.OpticalRXWarnBelowDBM, domain.OpticalRXOverloadAboveDBM)
		default:
			return nil, 0, fmt.Errorf("%w: optical_level tidak dikenal", domain.ErrInvalidInput)
		}
	}
	if f.InformBefore != nil {
		where = append(where, "last_inform_at IS NOT NULL AND last_inform_at < ?")
		args = append(args, *f.InformBefore)
	}
	if f.InformAfter != nil {
		where = append(where, "last_inform_at >= ?")
		args = append(args, *f.InformAfter)
	}
	for _, c := range f.Params {
		cond, cargs := paramCondSQL(c)
		where = append(where, cond)
		args = append(args, cargs...)
	}
	orderBy, err := deviceOrderBy(f.Sort, f.SortDesc)
	if err != nil {
		return nil, 0, err
	}
	whereSQL := strings.Join(where, " AND ")

	// Query bebas (param= / sort pada kolom tanpa indeks) bisa mahal pada armada besar: batasi waktu eksekusi di sisi server (MariaDB
	// "SET STATEMENT ... FOR"), supaya satu permintaan tidak menahan DB. Terlewati -> galat 1969 -> 400 "persempit filter" (errors.go).
	pre := ""
	if len(f.Params) > 0 || f.Sort == "ip_address" || f.Sort == "software_version" || f.Sort == "product_class" || f.Sort == "created_at" {
		pre = "SET STATEMENT max_statement_time=10 FOR "
	}
	var total int
	if err := r.db.GetContext(ctx, &total, pre+"SELECT COUNT(*) FROM devices WHERE "+whereSQL, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.Device
	args = append(args, p.Limit(), p.Offset())
	err = r.db.SelectContext(ctx, &rows,
		pre+"SELECT * FROM devices WHERE "+whereSQL+" ORDER BY "+orderBy+" LIMIT ? OFFSET ?", args...)
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

// deviceOrderBy membangun ORDER BY dari whitelist domain.DeviceSortFields (nama kolom TIDAK pernah berasal dari klien
// langsung). id DESC sebagai penentu urutan agar paginasi stabil.
func deviceOrderBy(field string, desc bool) (string, error) {
	if field == "" {
		return "id DESC", nil
	}
	for _, f := range domain.DeviceSortFields {
		if f == field {
			dir := "ASC"
			if desc {
				dir = "DESC"
			}
			if field == "id" {
				return "id " + dir, nil
			}
			return field + " " + dir + ", id DESC", nil
		}
	}
	return "", fmt.Errorf("%w: sort tidak dikenal", domain.ErrInvalidInput)
}

// numericParamValue: hanya nilai berbentuk angka yang ikut perbandingan numerik (CAST nilai non-angka menjadi 0 dan akan
// salah cocok pada "lt 5").
const numericParamValue = "p.parameter_value REGEXP '^-?[0-9]+([.][0-9]+)?$'"

// paramCondSQL menerjemahkan satu kondisi (sudah divalidasi domain.ParseParamCond) menjadi EXISTS terhadap device_parameters.
// Nama dan nilai selalu di-bind. Pencarian memakai unique key (device_id, parameter_name).
func paramCondSQL(c domain.ParamCond) (string, []interface{}) {
	const base = "SELECT 1 FROM device_parameters p WHERE p.device_id = devices.id AND p.parameter_name = ?"
	switch c.Op {
	case domain.ParamOpMissing:
		return "NOT EXISTS (" + base + " AND p.parameter_value IS NOT NULL AND p.parameter_value <> '')", []interface{}{c.Name}
	case domain.ParamOpExists:
		return "EXISTS (" + base + " AND p.parameter_value IS NOT NULL AND p.parameter_value <> '')", []interface{}{c.Name}
	case domain.ParamOpEq:
		return "EXISTS (" + base + " AND p.parameter_value = ?)", []interface{}{c.Name, c.Value}
	case domain.ParamOpNe:
		return "EXISTS (" + base + " AND p.parameter_value IS NOT NULL AND p.parameter_value <> ?)", []interface{}{c.Name, c.Value}
	case domain.ParamOpLike:
		return "EXISTS (" + base + " AND p.parameter_value LIKE ?)", []interface{}{c.Name, "%" + likeEscape(c.Value) + "%"}
	case domain.ParamOpPrefix:
		return "EXISTS (" + base + " AND p.parameter_value LIKE ?)", []interface{}{c.Name, likeEscape(c.Value) + "%"}
	}
	op := map[string]string{domain.ParamOpGt: ">", domain.ParamOpGe: ">=", domain.ParamOpLt: "<", domain.ParamOpLe: "<="}[c.Op]
	if op == "" { // tak terjangkau (ParseParamCond menolak op lain); gagal tertutup, bukan melonggarkan filter
		return "1 = 0", nil
	}
	return "EXISTS (" + base + " AND " + numericParamValue + " AND CAST(p.parameter_value AS DECIMAL(24,6)) " + op + " ?)",
		[]interface{}{c.Name, c.Value}
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
		latitude = :latitude, longitude = :longitude,
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

// SoftDelete menandai device terhapus DAN melepas relasi aksesnya
// (user_devices: pemetaan pelanggan ENDUSER; device_tags). Soft delete tidak
// memicu FK ON DELETE CASCADE, padahal pemetaan itu bukan data historis yang
// perlu dipertahankan — kalau tersisa, pelanggan lama tetap punya akses ke CPE
// yang kelak di-revive untuk pelanggan lain (bisa mengganti SSID/PSK-nya).
// Dalam satu transaksi supaya tidak ada keadaan setengah jalan.
func (r *deviceRepository) SoftDelete(ctx context.Context, id, deletedBy uint64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translateErr(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{
		`DELETE FROM user_devices WHERE device_id = ?`,
		`DELETE FROM device_tags WHERE device_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return translateErr(err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE devices SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now(), deletedBy, id); err != nil {
		return translateErr(err)
	}
	return translateErr(tx.Commit())
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
			parameter_type_id = COALESCE(VALUES(parameter_type_id), parameter_type_id), writable = VALUES(writable)`
	_, err := r.db.NamedExecContext(ctx, q, p)
	return translateErr(err)
}

// deviceParameterUpsertChunk — batas jumlah baris per statement INSERT
// multi-row (dipilih konservatif jauh di bawah batas placeholder MySQL
// 65535, sekaligus menghindari statement SQL raksasa untuk device dgn ribuan
// parameter dari GetParameterValues penuh, mis. full tree dump TR-098/TR-181).
const deviceParameterUpsertChunk = 500

// UpsertBatch — dipanggil usecase/session pada SETIAP respons GetParameterValues/
// SetParameterValues sepanjang sesi CWMP (jalur panas, TECH.md §3), sehingga
// SENGAJA memakai satu statement INSERT multi-row per chunk (bukan
// NamedExecContext per baris dalam loop seperti sebelumnya) -- device dengan
// ratusan parameter sebelumnya berarti ratusan round-trip DB terpisah per
// sesi (ditemukan saat audit performa, ROADMAP.md).
func (r *deviceParameterRepository) UpsertBatch(ctx context.Context, ps []domain.DeviceParameter) error {
	if len(ps) == 0 {
		return nil
	}
	// Satu chunk = satu statement, sudah atomik: lewati BEGIN/COMMIT (2 round trip + 1 commit/fsync tambahan per Inform).
	if len(ps) <= deviceParameterUpsertChunk {
		return translateErr(upsertDeviceParameterChunk(ctx, r.db, ps))
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translateErr(err)
	}
	for start := 0; start < len(ps); start += deviceParameterUpsertChunk {
		end := start + deviceParameterUpsertChunk
		if end > len(ps) {
			end = len(ps)
		}
		if err := upsertDeviceParameterChunk(ctx, tx, ps[start:end]); err != nil {
			_ = tx.Rollback()
			return translateErr(err)
		}
	}
	return tx.Commit()
}

func upsertDeviceParameterChunk(ctx context.Context, tx sqlx.ExecerContext, chunk []domain.DeviceParameter) error {
	placeholders := make([]string, len(chunk))
	args := make([]interface{}, 0, len(chunk)*5)
	for i, p := range chunk {
		placeholders[i] = "(?, ?, ?, ?, ?)"
		args = append(args, p.DeviceID, p.ParameterName, p.ParameterValue, p.ParameterTypeID, p.Writable)
	}
	q := `INSERT INTO device_parameters (device_id, parameter_name, parameter_value, parameter_type_id, writable)
		VALUES ` + strings.Join(placeholders, ",") + `
		ON DUPLICATE KEY UPDATE parameter_value = VALUES(parameter_value),
			parameter_type_id = COALESCE(VALUES(parameter_type_id), parameter_type_id), writable = VALUES(writable)`
	_, err := tx.ExecContext(ctx, q, args...)
	return translateErr(err)
}

func (r *deviceParameterRepository) UpsertNames(ctx context.Context, ps []domain.DeviceParameter) error {
	if len(ps) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translateErr(err)
	}
	for start := 0; start < len(ps); start += deviceParameterUpsertChunk {
		end := start + deviceParameterUpsertChunk
		if end > len(ps) {
			end = len(ps)
		}
		chunk := ps[start:end]
		ph := make([]string, len(chunk))
		args := make([]interface{}, 0, len(chunk)*3)
		for i, p := range chunk {
			ph[i] = "(?, ?, ?)"
			args = append(args, p.DeviceID, p.ParameterName, p.Writable)
		}
		q := `INSERT INTO device_parameters (device_id, parameter_name, writable) VALUES ` + strings.Join(ph, ",") +
			` ON DUPLICATE KEY UPDATE writable = VALUES(writable)`
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			_ = tx.Rollback()
			return translateErr(err)
		}
	}
	return tx.Commit()
}

func (r *deviceParameterRepository) HasPrefix(ctx context.Context, deviceID uint64, prefix string) (bool, error) {
	var n int
	err := r.db.GetContext(ctx, &n,
		`SELECT COUNT(*) FROM (SELECT 1 FROM device_parameters WHERE device_id = ? AND parameter_name LIKE ? LIMIT 1) x`,
		deviceID, likeEscape(prefix)+"%")
	return n > 0, translateErr(err)
}

func (r *deviceParameterRepository) PPPoEUsernames(ctx context.Context, deviceIDs []uint64) (map[uint64]string, error) {
	out := map[uint64]string{}
	if len(deviceIDs) == 0 {
		return out, nil
	}
	q, args, err := sqlx.In(
		`SELECT device_id, parameter_value FROM device_parameters
		 WHERE device_id IN (?) AND parameter_value IS NOT NULL AND parameter_value <> ''
		   AND (parameter_name LIKE 'InternetGatewayDevice.WANDevice.%.WANPPPConnection.%.Username' OR parameter_name LIKE 'Device.PPP.Interface.%.Username')
		 ORDER BY device_id, parameter_name`, deviceIDs)
	if err != nil {
		return nil, translateErr(err)
	}
	var rows []struct {
		DeviceID uint64 `db:"device_id"`
		Value    string `db:"parameter_value"`
	}
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(q), args...); err != nil {
		return nil, translateErr(err)
	}
	for _, row := range rows {
		if _, seen := out[row.DeviceID]; !seen {
			out[row.DeviceID] = row.Value
		}
	}
	return out, nil
}

func (r *deviceParameterRepository) ValuesFor(ctx context.Context, deviceIDs []uint64, names []string) (map[uint64]map[string]string, error) {
	out := map[uint64]map[string]string{}
	if len(deviceIDs) == 0 || len(names) == 0 {
		return out, nil
	}
	q, args, err := sqlx.In(
		`SELECT device_id, parameter_name, parameter_value FROM device_parameters
		 WHERE device_id IN (?) AND parameter_name IN (?) AND parameter_value IS NOT NULL`, deviceIDs, names)
	if err != nil {
		return nil, translateErr(err)
	}
	var rows []struct {
		DeviceID uint64 `db:"device_id"`
		Name     string `db:"parameter_name"`
		Value    string `db:"parameter_value"`
	}
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(q), args...); err != nil {
		return nil, translateErr(err)
	}
	for _, row := range rows {
		m := out[row.DeviceID]
		if m == nil {
			m = map[string]string{}
			out[row.DeviceID] = m
		}
		m[row.Name] = row.Value
	}
	return out, nil
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
		deviceID, likeEscape(prefix)+"%")
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

func (r *deviceParameterRepository) GetBySuffix(ctx context.Context, deviceID uint64, suffix string) (*domain.DeviceParameter, error) {
	var p domain.DeviceParameter
	err := r.db.GetContext(ctx, &p,
		`SELECT * FROM device_parameters WHERE device_id = ? AND parameter_name LIKE ? ORDER BY id DESC LIMIT 1`, deviceID, "%"+likeEscape(suffix))
	if err != nil {
		return nil, translateErr(err)
	}
	return &p, nil
}

// ---- DeviceConfigSnapshot ----

type deviceConfigSnapshotRepository struct{ db *sqlx.DB }

func NewDeviceConfigSnapshotRepository(db *sqlx.DB) domain.DeviceConfigSnapshotRepository {
	return &deviceConfigSnapshotRepository{db: db}
}

func (r *deviceConfigSnapshotRepository) Create(ctx context.Context, s *domain.DeviceConfigSnapshot) error {
	const q = `INSERT INTO device_config_snapshots (device_id, snapshot_data, created_at)
		VALUES (:device_id, :snapshot_data, :created_at)`
	res, err := r.db.NamedExecContext(ctx, q, s)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	s.ID = uint64(id)
	return nil
}

func (r *deviceConfigSnapshotRepository) ListByDevice(ctx context.Context, deviceID uint64, p domain.Pagination) ([]domain.DeviceConfigSnapshot, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM device_config_snapshots WHERE device_id = ?`, deviceID); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.DeviceConfigSnapshot
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM device_config_snapshots WHERE device_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		deviceID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *deviceConfigSnapshotRepository) GetByID(ctx context.Context, id uint64) (*domain.DeviceConfigSnapshot, error) {
	var s domain.DeviceConfigSnapshot
	if err := r.db.GetContext(ctx, &s, `SELECT * FROM device_config_snapshots WHERE id = ?`, id); err != nil {
		return nil, translateErr(err)
	}
	return &s, nil
}

// ---- DeviceSession ----

type deviceSessionRepository struct{ db *sqlx.DB }

func NewDeviceSessionRepository(db *sqlx.DB) domain.DeviceSessionRepository {
	return &deviceSessionRepository{db: db}
}

func (r *deviceSessionRepository) Create(ctx context.Context, s *domain.DeviceSession) error {
	const q = `INSERT INTO device_sessions (device_id, session_token, cwmp_id, status, remote_ip, started_at, cwmp_namespace)
		VALUES (:device_id, :session_token, :cwmp_id, :status, :remote_ip, :started_at, :cwmp_namespace)`
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

func (r *deviceSessionRepository) SetCWMPNamespace(ctx context.Context, id uint64, namespace string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE device_sessions SET cwmp_namespace = ? WHERE id = ?`, namespace, id)
	return translateErr(err)
}

// CountOpen — metrik observability TECH.md §10 (acs_cwmp_sessions_open),
// lintas seluruh tenant, lihat komentar domain.DeviceSessionRepository.
func (r *deviceSessionRepository) CountOpen(ctx context.Context) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM device_sessions WHERE status = ?`, domain.SessionStatusOpen)
	if err != nil {
		return 0, translateErr(err)
	}
	return count, nil
}

// CountOpenForTenant — CountOpen yang dibatasi ke perangkat milik satu tenant.
func (r *deviceSessionRepository) CountOpenForTenant(ctx context.Context, tenantID uint64) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM device_sessions s JOIN devices d ON d.id = s.device_id
		 WHERE s.status = ? AND d.tenant_id = ?`, domain.SessionStatusOpen, tenantID)
	if err != nil {
		return 0, translateErr(err)
	}
	return count, nil
}

// TimeoutStaleOpen — lihat komentar domain.DeviceSessionRepository.
func (r *deviceSessionRepository) TimeoutStaleOpen(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE device_sessions SET status = ?, ended_at = NOW()
		 WHERE status = ? AND started_at < ?`,
		domain.SessionStatusTimeout, domain.SessionStatusOpen, olderThan)
	if err != nil {
		return 0, translateErr(err)
	}
	n, _ := res.RowsAffected()
	return n, nil
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

// ---- Wawasan armada (dashboard) ----

type fleetInsightsRepository struct{ db *sqlx.DB }

func NewFleetInsightsRepository(db *sqlx.DB) domain.FleetInsightsRepository {
	return &fleetInsightsRepository{db: db}
}

// FleetRows: satu baris ringan per perangkat (status, Inform terakhir, RX & suhu terakhir, uptime, penanda GPON/EPON).
// Subquery berkorelasi memakai indeks (device_id, ...) sehingga biaya linear terhadap jumlah perangkat; pengelompokan ke
// keranjang dilakukan di usecase. Untuk armada sangat besar (>100 ribu) pertimbangkan agregat berkala/cache.
func (r *fleetInsightsRepository) FleetRows(ctx context.Context, tenantID *uint64) ([]domain.FleetRow, error) {
	q := `SELECT s.code AS status, d.last_inform_at,
		(SELECT m.rx_power_dbm FROM device_optical_metrics m WHERE m.device_id = d.id ORDER BY m.recorded_at DESC, m.id DESC LIMIT 1) AS rx,
		(SELECT m.temperature_celsius FROM device_optical_metrics m WHERE m.device_id = d.id ORDER BY m.recorded_at DESC, m.id DESC LIMIT 1) AS temp,
		(SELECT p.parameter_value FROM device_parameters p WHERE p.device_id = d.id
		   AND p.parameter_name IN ('InternetGatewayDevice.DeviceInfo.UpTime', 'Device.DeviceInfo.UpTime') LIMIT 1) AS uptime,
		EXISTS (SELECT 1 FROM device_parameters p WHERE p.device_id = d.id AND p.parameter_name LIKE '%Gpon%') AS gpon,
		EXISTS (SELECT 1 FROM device_parameters p WHERE p.device_id = d.id AND p.parameter_name LIKE '%Epon%') AS epon
		FROM devices d JOIN ref_device_status s ON s.id = d.device_status_id
		WHERE d.is_deleted = 0`
	args := []interface{}{}
	if tenantID != nil {
		q += " AND d.tenant_id = ?"
		args = append(args, *tenantID)
	}
	var rows []domain.FleetRow
	if err := r.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
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

func (r *deviceOpticalMetricRepository) LatestForDevices(ctx context.Context, deviceIDs []uint64) (map[uint64]domain.DeviceOpticalMetric, error) {
	out := map[uint64]domain.DeviceOpticalMetric{}
	if len(deviceIDs) == 0 {
		return out, nil
	}
	q, args, err := sqlx.In(
		`SELECT m.* FROM device_optical_metrics m
		 JOIN (SELECT device_id, MAX(id) AS mid FROM device_optical_metrics WHERE device_id IN (?) GROUP BY device_id) x ON x.mid = m.id`, deviceIDs)
	if err != nil {
		return nil, translateErr(err)
	}
	var rows []domain.DeviceOpticalMetric
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(q), args...); err != nil {
		return nil, translateErr(err)
	}
	for _, m := range rows {
		out[m.DeviceID] = m
	}
	return out, nil
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
