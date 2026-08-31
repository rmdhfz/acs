-- 0020_user_devices_mapping
--
-- Portal self-service pelanggan (PRD.md §4.2 — "sistem terpisah yang memanggil
-- REST API ACS", di sini di-embed sbg role ENDUSER + endpoint /selfservice/*).
-- ENDUSER hanya boleh melihat/mengubah device yang dipetakan ke akunnya lewat
-- tabel user_devices.
--
-- ref_roles TIDAK punya kolom `description` (lihat schema.sql) dan `code`
-- NOT NULL + UNIQUE — insert wajib menyertakan `code`. Kode ENDUSER dipakai
-- domain.RoleEndUser (internal/domain/common.go).

INSERT INTO ref_roles (code, name) VALUES ('ENDUSER', 'Pelanggan akhir (portal self-service)');

-- Relasi many-to-many user (ENDUSER) -> devices miliknya.
CREATE TABLE user_devices (
    user_id     BIGINT UNSIGNED NOT NULL,
    device_id   BIGINT UNSIGNED NOT NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by  BIGINT UNSIGNED NULL,
    PRIMARY KEY (user_id, device_id),
    KEY idx_user_devices_device (device_id),
    CONSTRAINT fk_user_devices_user   FOREIGN KEY (user_id)   REFERENCES users (id)   ON DELETE CASCADE,
    CONSTRAINT fk_user_devices_device FOREIGN KEY (device_id) REFERENCES devices (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Mapping akun ENDUSER -> device yang boleh diaksesnya di portal self-service (migrations/0020)';
