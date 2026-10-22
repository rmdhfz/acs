-- 0040_automation_rules
--
-- Aturan otomasi kondisi-aksi: "jika <ekspresi> maka <aksi>" yang dievaluasi saat Inform. Ini jawaban atas provision berskrip
-- GenieACS TANPA skrip: kondisi memakai ekspresi deklaratif (pkg/vexpr) atas parameter + atribut perangkat + event sesi; aksi
-- dari daftar tertutup (set_parameter, reboot, get_parameter_values, add_tag, webhook). Pasti berhenti, biaya dibatasi, dan tiap
-- pemicuan tercatat. Disimpan sebagai DATA per tenant (CLAUDE.md: perilaku per tenant = data, bukan kode).
--
-- Tiga pagar anti-badai (aturan buruk tidak boleh membanjiri perangkat): (1) cooldown per (aturan, perangkat); (2) berhenti
-- (suspended) setelah max_consecutive_fires pemicuan berturut-turut tanpa kondisi pernah salah di antaranya (mis. "jika versi
-- != X maka reboot" yang tak pernah konvergen); (3) dilewati bila perangkat masih punya task tertunda.
--
--  - automation_rules : definisi (audit 7-kolom penuh, rule_uuid publik). WAJIB punya tenant: tanpa aturan global lintas-tenant
--                       karena aksinya mengubah perangkat. Trigger memakai ref_ztp_trigger_event yang sama dengan ZTP
--                       (BOOTSTRAP_ONLY, BOOTSTRAP_OR_BOOT, EVERY_INFORM).
--  - automation_states: state per (aturan, perangkat) untuk cooldown & pagar.
--  - automation_runs  : riwayat pemicuan (untuk UI & audit).
--
-- PENGECUALIAN audit 7-kolom (CLAUDE.md §2) -- disengaja, bukan default diam-diam:
--   * automation_states : tabel state operasional (bukan data master) -> hanya updated_at.
--   * automation_runs   : tabel LOG -> audit MINIMAL, hanya created_at. Tanpa soft delete; dipangkas FISIK setelah 30 hari
--                         (kebijakan retensi eksplisit, CLAUDE.md §3).

CREATE TABLE automation_rules (
    id                    BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    rule_uuid             CHAR(36)         NOT NULL COMMENT 'Identifier publik di API; PK tidak pernah diekspos',
    tenant_id             BIGINT UNSIGNED  NOT NULL,
    name                  VARCHAR(128)     NOT NULL,
    description           VARCHAR(1024)    NULL,
    trigger_event_id      BIGINT UNSIGNED  NOT NULL COMMENT 'FK ref_ztp_trigger_event: kapan aturan dievaluasi relatif thd event Inform',
    condition_expr        VARCHAR(1000)    NOT NULL DEFAULT '' COMMENT 'Ekspresi pkg/vexpr; kosong = selalu benar',
    actions               TEXT             NOT NULL COMMENT 'JSON array aksi (divalidasi usecase; maks 10)',
    cooldown_seconds      INT UNSIGNED     NOT NULL DEFAULT 3600 COMMENT 'Jeda minimum antar-pemicuan aturan yang sama pada perangkat yang sama (min 300 di usecase)',
    max_consecutive_fires TINYINT UNSIGNED NOT NULL DEFAULT 3 COMMENT 'Pemicuan berturut-turut tanpa kondisi pernah salah sebelum aturan di-suspend untuk perangkat itu',
    priority              INT UNSIGNED     NOT NULL DEFAULT 100 COMMENT 'Angka lebih kecil dievaluasi lebih dulu',
    is_active             TINYINT(1)       NOT NULL DEFAULT 1,
    created_at            DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by            BIGINT UNSIGNED  NULL,
    updated_at            DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by            BIGINT UNSIGNED  NULL,
    deleted_at            DATETIME         NULL,
    deleted_by            BIGINT UNSIGNED  NULL,
    is_deleted            TINYINT(1)       NOT NULL DEFAULT 0,
    UNIQUE KEY uq_automation_rules_uuid (rule_uuid),
    KEY idx_automation_rules_scope (tenant_id, is_deleted, is_active, priority),
    CONSTRAINT fk_automation_rules_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_automation_rules_trigger FOREIGN KEY (trigger_event_id) REFERENCES ref_ztp_trigger_event (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Aturan otomasi kondisi-aksi per tenant (ekspresi pkg/vexpr); lihat header migrations/0040';

CREATE TABLE automation_states (
    rule_id           BIGINT UNSIGNED NOT NULL,
    device_id         BIGINT UNSIGNED NOT NULL,
    last_fired_at     DATETIME(3)     NULL,
    consecutive_fires INT UNSIGNED    NOT NULL DEFAULT 0 COMMENT 'Pemicuan berturut-turut tanpa kondisi pernah salah; direset saat kondisi salah / sudah konvergen',
    suspended         TINYINT(1)      NOT NULL DEFAULT 0 COMMENT '1 = berhenti memicu untuk perangkat ini sampai operator mereset',
    updated_at        DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (rule_id, device_id),
    KEY idx_automation_states_device (device_id),
    CONSTRAINT fk_automation_states_rule FOREIGN KEY (rule_id) REFERENCES automation_rules (id),
    CONSTRAINT fk_automation_states_device FOREIGN KEY (device_id) REFERENCES devices (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='State pagar aturan otomasi per (aturan, perangkat). Pengecualian audit 7-kolom (state operasional): lihat header migrations/0040';

CREATE TABLE automation_runs (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    run_uuid    CHAR(36)        NOT NULL COMMENT 'Identifier publik di API; PK tidak pernah diekspos',
    rule_id     BIGINT UNSIGNED NOT NULL,
    device_id   BIGINT UNSIGNED NOT NULL,
    fired       TINYINT(1)      NOT NULL COMMENT '1 = aksi diantre; 0 = gagal atau di-suspend',
    suspended   TINYINT(1)      NOT NULL DEFAULT 0 COMMENT '1 = pemicuan ini membuat aturan di-suspend untuk perangkat',
    summary     VARCHAR(1024)   NOT NULL DEFAULT '' COMMENT 'Ringkasan aksi (nilai rahasia tidak pernah dimasukkan)',
    error       VARCHAR(512)    NULL,
    created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uq_automation_runs_uuid (run_uuid),
    KEY idx_automation_runs_rule (rule_id, id),
    KEY idx_automation_runs_device (device_id, id),
    KEY idx_automation_runs_created (created_at),
    CONSTRAINT fk_automation_runs_rule FOREIGN KEY (rule_id) REFERENCES automation_rules (id),
    CONSTRAINT fk_automation_runs_device FOREIGN KEY (device_id) REFERENCES devices (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Riwayat pemicuan aturan otomasi. Pengecualian audit 7-kolom (log): lihat header migrations/0040';

INSERT INTO ref_webhook_event_types (code, name, description) VALUES
    ('AUTOMATION_TRIGGERED', 'Automation Triggered',
        'Aturan otomasi memicu aksi pada sebuah perangkat (aksi webhook pada aturan itu)');
