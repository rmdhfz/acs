-- Proteksi brute-force login (ROADMAP.md/audit keamanan Fase 3): sebelumnya
-- Login() sama sekali tidak melacak percobaan gagal. failed_login_attempts
-- di-increment tiap password salah; saat mencapai threshold
-- (auth.maxFailedLoginAttempts, lihat internal/usecase/auth/service.go),
-- locked_until diset ke now + auth.lockoutDuration dan login ditolak selama
-- masa itu -- dengan pesan error yang SAMA dgn kredensial salah biasa
-- (menghindari membocorkan validitas username ke penyerang).
ALTER TABLE users
    ADD COLUMN failed_login_attempts INT UNSIGNED NOT NULL DEFAULT 0
        COMMENT 'Jumlah percobaan login gagal berturut-turut sejak reset terakhir' AFTER last_login_at,
    ADD COLUMN locked_until DATETIME NULL
        COMMENT 'Login ditolak selama waktu ini masih di masa depan (proteksi brute-force)' AFTER failed_login_attempts;
