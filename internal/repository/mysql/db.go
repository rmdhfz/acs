// Package mysql berisi implementasi sqlx dari interface repository yang
// didefinisikan di internal/domain. Hanya query di sini — tidak ada logic
// keputusan (lihat CLAUDE.md).
package mysql

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	_ "github.com/go-sql-driver/mysql"
)

func Connect(dsn string) (*sqlx.DB, error) {
	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql: gagal konek database: %w", err)
	}
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}
