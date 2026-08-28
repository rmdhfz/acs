CREATE TABLE files (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    file_uuid CHAR(36) NOT NULL UNIQUE,
    tenant_id BIGINT UNSIGNED NULL, -- NULL = Global
    file_type VARCHAR(100) NOT NULL COMMENT '1 Firmware Upgrade Image, 2 Web Content, 3 Vendor Configuration File, dll',
    vendor_id BIGINT UNSIGNED NULL,
    device_model_id BIGINT UNSIGNED NULL,
    version VARCHAR(100) NULL,
    file_name VARCHAR(255) NOT NULL,
    storage_key VARCHAR(512) NOT NULL,
    file_size_bytes BIGINT UNSIGNED NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by BIGINT UNSIGNED NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by BIGINT UNSIGNED NULL,
    is_deleted TINYINT(1) NOT NULL DEFAULT 0,
    deleted_at TIMESTAMP NULL,
    deleted_by BIGINT UNSIGNED NULL,
    
    CONSTRAINT fk_files_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT,
    CONSTRAINT fk_files_vendor FOREIGN KEY (vendor_id) REFERENCES ref_vendors(id) ON DELETE RESTRICT,
    CONSTRAINT fk_files_model FOREIGN KEY (device_model_id) REFERENCES device_models(id) ON DELETE RESTRICT,
    CONSTRAINT fk_files_cb FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT fk_files_ub FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT fk_files_db FOREIGN KEY (deleted_by) REFERENCES users(id) ON DELETE SET NULL,
    
    INDEX idx_files_tenant_type (tenant_id, file_type),
    INDEX idx_files_vendor_model (vendor_id, device_model_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
