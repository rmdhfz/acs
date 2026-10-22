-- 0039_virtual_parameters
--
-- Virtual parameter: nilai TURUNAN yang dihitung dari parameter perangkat lewat ekspresi deklaratif (pkg/vexpr) -- mis. redaman dBm
-- dari satuan 0,001, uptime dalam hari, atau "parameter pertama yang terisi" lintas TR-098/TR-181. Padanan /virtual_parameters
-- GenieACS, tetapi ekspresi (bukan skrip): tanpa loop/akses luar, biaya dibatasi, dan disimpan sebagai DATA per tenant sehingga
-- menambah turunan baru = operasi data, bukan perubahan kode (CLAUDE.md, domain TR-069).
--
-- Nilai TIDAK disimpan: dihitung saat dibaca dari device_parameters terkini (tidak ada data basi dan tidak ada tulis tambahan di
-- jalur Inform). Ekspresi yang merujuk parameter rahasia ditolak di usecase (tampilan virtual parameter terbuka untuk semua peran).
--
-- Audit 7-kolom penuh; vp_uuid = identifier publik (PK tidak diekspos). tenant_id NULL = global (hanya SUPERADMIN yang membuat/
-- mengubah); virtual parameter tenant dengan nama sama menimpa yang global untuk tenant itu. Tanpa UNIQUE (tenant, name): unik
-- hanya di antara baris aktif dan ditegakkan di usecase supaya baris yang sudah dihapus tidak memblokir nama itu selamanya.

CREATE TABLE virtual_parameters (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    vp_uuid     CHAR(36)        NOT NULL COMMENT 'Identifier publik di API; PK tidak pernah diekspos',
    tenant_id   BIGINT UNSIGNED NULL COMMENT 'NULL = global (semua tenant)',
    name        VARCHAR(64)     NOT NULL COMMENT 'Kunci mesin: huruf kecil, angka, underscore (mis. uptime_hari)',
    label       VARCHAR(255)    NOT NULL COMMENT 'Nama tampilan',
    description VARCHAR(1024)   NULL,
    expression  VARCHAR(1000)   NOT NULL COMMENT 'Ekspresi pkg/vexpr; divalidasi saat disimpan',
    unit        VARCHAR(32)     NULL COMMENT 'Satuan tampilan (mis. dBm, hari)',
    is_active   TINYINT(1)      NOT NULL DEFAULT 1,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by  BIGINT UNSIGNED NULL,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by  BIGINT UNSIGNED NULL,
    deleted_at  DATETIME        NULL,
    deleted_by  BIGINT UNSIGNED NULL,
    is_deleted  TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_virtual_parameters_uuid (vp_uuid),
    KEY idx_virtual_parameters_scope (tenant_id, is_deleted, name),
    CONSTRAINT fk_virtual_parameters_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Definisi virtual parameter (ekspresi pkg/vexpr); nilai dihitung saat dibaca, tidak disimpan';
