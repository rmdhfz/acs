-- 0019_tags_and_presets
--
-- Tag device (segmentasi) + preset gaya GenieACS (precondition JSON + set
-- konfigurasi JSON). Preset dievaluasi terhadap device saat Inform; berbeda
-- dari provisioning_profiles yang di-apply eksplisit (FR-18).

CREATE TABLE tags (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    tenant_id   BIGINT UNSIGNED NULL,
    name        VARCHAR(255)    NOT NULL,
    color       VARCHAR(7)      NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_tags_name_tenant (name, tenant_id),
    CONSTRAINT fk_tags_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Tag device untuk segmentasi & precondition preset (migrations/0019)';

CREATE TABLE device_tags (
    device_id   BIGINT UNSIGNED NOT NULL,
    tag_id      BIGINT UNSIGNED NOT NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (device_id, tag_id),
    CONSTRAINT fk_device_tags_device FOREIGN KEY (device_id) REFERENCES devices (id) ON DELETE CASCADE,
    CONSTRAINT fk_device_tags_tag    FOREIGN KEY (tag_id)    REFERENCES tags (id)    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Relasi many-to-many device <-> tag (migrations/0019)';

CREATE TABLE presets (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    tenant_id       BIGINT UNSIGNED NULL,
    name            VARCHAR(255)    NOT NULL,
    weight          INT             NOT NULL DEFAULT 0,
    precondition    JSON            NOT NULL COMMENT 'Aturan pencocokan (mis. berdasar model / tag)',
    configurations  JSON            NOT NULL COMMENT 'Aturan set parameter yang diterapkan bila precondition cocok',
    is_active       TINYINT(1)      NOT NULL DEFAULT 1,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_presets_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Preset match+config gaya GenieACS (migrations/0019)';
