package mysql

import (
	"database/sql"
	"errors"
	"strings"

	"acs/internal/domain"
)

// translateErr memetakan error driver database ke error domain yang stabil,
// agar usecase/delivery tidak perlu tahu detail driver SQL.
func translateErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	if strings.Contains(err.Error(), "Duplicate entry") {
		return domain.ErrConflict
	}
	return err
}
