-- 0038_device_faults
--
-- Resource "faults": catatan cwmp:Fault yang dikirim CPE sebagai jawaban RPC dari ACS (setara resource /faults GenieACS).
-- Sebelumnya fault hanya mengubah task jadi FAILED dan memicu webhook DEVICE_FAULT -- tidak ada tempat melihat "perangkat mana
-- yang sedang bermasalah dan kenapa". Satu baris = satu masalah AKTIF per (perangkat, kode fault, RPC): kejadian berulang
-- menaikkan occurrence_count dan last_seen_at, bukan menambah baris (tabel tetap kecil walau CPE mengulang fault yang sama
-- tiap sesi). Fault dibersihkan otomatis (soft delete) saat RPC yang sama berhasil pada perangkat itu, atau manual oleh
-- operator (DELETE /faults/:id).
--
-- Audit 7-kolom penuh (data transaksional kecil, bukan log volume tinggi). fault_uuid = identifier publik di API; PK tidak
-- pernah diekspos. Tanpa tenant_id: tenant dihitung dari devices.tenant_id (pola yang sama dengan tasks).
-- fault_code BUKAN ref_*: nilainya string standar dari spec Broadband Forum / kuirk vendor yang diterima apa adanya dari CPE,
-- bukan enum yang dikelola ACS.

CREATE TABLE device_faults (
    id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    fault_uuid       CHAR(36)        NOT NULL COMMENT 'Identifier publik di API; PK tidak pernah diekspos',
    device_id        BIGINT UNSIGNED NOT NULL,
    task_id          BIGINT UNSIGNED NULL COMMENT 'Task yang memicu fault terakhir (NULL bila fault datang tanpa task yang cocok)',
    rpc_method       VARCHAR(64)     NOT NULL DEFAULT '' COMMENT 'Kode ref_task_types RPC yang ditolak CPE; kosong bila tak diketahui',
    fault_code       VARCHAR(16)     NOT NULL COMMENT 'Kode fault CWMP apa adanya dari CPE (mis. 9005); string, jangan diubah penulisannya',
    fault_string     VARCHAR(1024)   NOT NULL DEFAULT '' COMMENT 'FaultString dari CPE (dipotong bila lebih panjang)',
    occurrence_count INT UNSIGNED    NOT NULL DEFAULT 1,
    first_seen_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    last_seen_at     DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    created_at       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by       BIGINT UNSIGNED NULL,
    updated_at       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by       BIGINT UNSIGNED NULL,
    deleted_at       DATETIME        NULL,
    deleted_by       BIGINT UNSIGNED NULL,
    is_deleted       TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_device_faults_uuid (fault_uuid),
    KEY idx_device_faults_device_active (device_id, is_deleted, rpc_method, fault_code),
    KEY idx_device_faults_last_seen (is_deleted, last_seen_at),
    CONSTRAINT fk_device_faults_device FOREIGN KEY (device_id) REFERENCES devices (id),
    CONSTRAINT fk_device_faults_task FOREIGN KEY (task_id) REFERENCES tasks (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='cwmp:Fault aktif per (perangkat, kode, RPC); kejadian berulang menaikkan occurrence_count';
