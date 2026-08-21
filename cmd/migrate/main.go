// Command migrate menjalankan migrasi database terurut dari folder
// migrations/ menggunakan golang-migrate (TECH.md §1).
//
// Usage:
//
//	migrate up
//	migrate down
//	migrate version
package main

import (
	"errors"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"acs/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate <up|down|version>")
	}
	cmd := os.Args[1]

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	m, err := migrate.New("file://migrations", "mysql://"+cfg.DBDSN)
	if err != nil {
		log.Fatalf("migrate: gagal inisialisasi: %v", err)
	}
	defer m.Close()

	switch cmd {
	case "up":
		err = m.Up()
	case "down":
		err = m.Down()
	case "version":
		v, dirty, verr := m.Version()
		log.Printf("version=%d dirty=%v", v, dirty)
		err = verr
	default:
		log.Fatalf("perintah tidak dikenal: %s", cmd)
	}
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("migrate %s: %v", cmd, err)
	}
	log.Printf("migrate %s: selesai", cmd)
}
