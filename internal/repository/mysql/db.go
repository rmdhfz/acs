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

// defaultMaxOpenConns/defaultMaxIdleConns/defaultConnMaxLifetime — nilai
// bawaan yang dipertahankan sama seperti sebelum konfigurasi ada, dipakai
// Connect() (tool pendek seperti seed-admin/seal-secrets/test yang tidak
// butuh tuning pool) dan sebagai fallback dokumentasi untuk ConnectWithPool.
const (
	defaultMaxOpenConns    = 50
	defaultMaxIdleConns    = 10
	defaultConnMaxLifetime = 30 * time.Minute
)

func Connect(dsn string) (*sqlx.DB, error) {
	return ConnectWithPool(dsn, defaultMaxOpenConns, defaultMaxIdleConns, defaultConnMaxLifetime)
}

// ConnectWithPool — seperti Connect, tapi ukuran connection pool bisa diatur
// (acsd meneruskannya dari config.Config, lihat ACS_DB_MAX_OPEN_CONNS dkk).
// Load test 2026-10-18 menemukan throughput CWMP plateau krn 50 koneksi
// hardcoded jadi bottleneck sebelum CPU acsd/MariaDB jenuh — lihat
// docs/laporan/PROGRESS.md LANJUTAN 14.
func ConnectWithPool(dsn string, maxOpenConns, maxIdleConns int, connMaxLifetime time.Duration) (*sqlx.DB, error) {
	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql: gagal konek database: %w", err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	return db, nil
}
