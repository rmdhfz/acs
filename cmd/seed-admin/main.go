// Command seed-admin membuat user superadmin pertama pada instalasi baru.
// Dibutuhkan karena endpoint POST /users mensyaratkan actor yang sudah
// admin/superadmin — tidak ada jalur self-registration (CLAUDE.md: tidak ada
// endpoint mutasi tanpa autentikasi, termasuk pembuatan user pertama).
//
// Usage:
//
//	seed-admin -username admin -password "..." -email admin@example.com
package main

import (
	"context"
	"flag"
	"log"

	"github.com/google/uuid"

	"acs/internal/config"
	"acs/internal/domain"
	"acs/internal/repository/mysql"
	"acs/internal/usecase/auth"
)

func main() {
	username := flag.String("username", "", "username superadmin")
	password := flag.String("password", "", "password superadmin")
	email := flag.String("email", "", "email superadmin")
	fullName := flag.String("fullname", "Superadmin", "nama lengkap")
	flag.Parse()

	if *username == "" || *password == "" || *email == "" {
		log.Fatal("wajib isi -username -password -email")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := mysql.Connect(cfg.DBDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	userRepo := mysql.NewUserRepository(db)
	refRepo := mysql.NewRefRepository(db)
	ctx := context.Background()

	hash, err := auth.HashPassword(*password)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	u := &domain.User{
		UserUUID:     uuid.NewString(),
		Username:     *username,
		Email:        *email,
		PasswordHash: hash,
		FullName:     *fullName,
		IsActive:     true,
	}
	if err := userRepo.Create(ctx, u); err != nil {
		log.Fatalf("create user: %v", err)
	}

	role, err := refRepo.GetByCode(ctx, domain.RefTableRoles, domain.RoleSuperadmin)
	if err != nil {
		log.Fatalf("lookup role superadmin: %v", err)
	}
	if err := userRepo.AssignRole(ctx, u.ID, role.ID, nil); err != nil {
		log.Fatalf("assign role: %v", err)
	}

	log.Printf("superadmin %q dibuat (id=%d, uuid=%s)", *username, u.ID, u.UserUUID)
}
