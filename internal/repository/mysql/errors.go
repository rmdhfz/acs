package mysql

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	driver "github.com/go-sql-driver/mysql"

	"acs/internal/domain"
)

// translateErr memetakan error driver database ke error domain yang stabil,
// agar usecase/delivery tidak perlu tahu detail driver SQL.
//
// Kegagalan validasi yang lolos ke lapisan DB (FK tidak ada, data terlalu
// panjang, di luar rentang) dipetakan ke ErrInvalidInput (HTTP 400) — bukan
// dibiarkan jadi 500 yang menyesatkan klien dan memicu alarm.
func translateErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	var me *driver.MySQLError
	if errors.As(err, &me) {
		switch me.Number {
		case 1062: // duplicate entry
			return domain.ErrConflict
		case 1451: // tidak bisa menghapus/mengubah: masih direferensikan
			return fmt.Errorf("%w: data masih direferensikan oleh data lain", domain.ErrConflict)
		case 1452: // referensi (FK) tidak ditemukan
			return fmt.Errorf("%w: data yang direferensikan tidak ditemukan (periksa id vendor/model/tenant/profil, dst.)", domain.ErrInvalidInput)
		case 1406: // data too long
			return fmt.Errorf("%w: ada nilai yang melebihi panjang maksimum kolom", domain.ErrInvalidInput)
		case 1264, 1265, 1366, 1292: // di luar rentang / format salah
			return fmt.Errorf("%w: ada nilai di luar rentang atau berformat salah", domain.ErrInvalidInput)
		case 1048: // kolom wajib NULL
			return fmt.Errorf("%w: ada field wajib yang kosong", domain.ErrInvalidInput)
		}
	}
	if strings.Contains(err.Error(), "Duplicate entry") {
		return domain.ErrConflict
	}
	return err
}

// likeEscape menetralkan wildcard LIKE (% _ \) pada input agar dicocokkan literal.
func likeEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
