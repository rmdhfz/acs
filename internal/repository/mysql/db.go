// Package mysql berisi implementasi sqlx dari interface repository yang
// didefinisikan di internal/domain. Hanya query di sini — tidak ada logic
// keputusan (lihat CLAUDE.md).
package mysql

import (
	"fmt"
	"strings"
	"time"

	mysqldrv "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
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
	dsn, err := withInterpolateParams(dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql: DSN tidak valid: %w", err)
	}
	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql: gagal konek database: %w", err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	return db, nil
}

// withInterpolateParams menyalakan interpolateParams: driver menyisipkan argumen ke SQL di sisi klien sehingga satu
// query = satu round trip. Tanpa ini setiap query berparameter dikirim sebagai Prepare + Execute + Close (3 round
// trip, plus parse ulang di server) — pada hot path Inform ±20 query per sesi. Aman untuk charset utf8mb4 (DSN kita);
// string dan []byte di-escape driver. Nilai yang sudah diset eksplisit di DSN dihormati.
func withInterpolateParams(dsn string) (string, error) {
	cfg, err := mysqldrv.ParseDSN(dsn)
	if err != nil {
		return "", err
	}
	if !strings.Contains(dsn, "interpolateParams=") {
		cfg.InterpolateParams = true
	}
	return cfg.FormatDSN(), nil
}
