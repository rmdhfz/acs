-- 0049_olt_onu_state_events
--
-- Riwayat perubahan keadaan ONU terdaftar di OLT (irisan B1). Satu baris per perubahan yang TERAMATI oleh polling ACS:
--   from_state_id NULL = ONU baru terlihat (atau muncul kembali setelah sempat tidak dilaporkan OLT)
--   to_state_id   NULL = ONU tidak lagi dilaporkan OLT (dihapus-lunak di olt_registered_onus)
-- Keduanya NULL tidak pernah ditulis. Resolusi waktu = selang polling: perubahan yang terjadi dan pulih di antara dua polling TIDAK terlihat
-- (jangan dibaca sebagai bukti tidak ada gangguan).
--
-- Kunci = (olt_id, serial_number), BUKAN olt_registered_onus.id: baris ONU dihapus-lunak dan dihidupkan lagi, dan upsert massal tidak
-- mengembalikan id. Serial dicocokkan tanpa peka huruf besar/kecil (collation), sama dengan tautan ke perangkat ACS.
--
-- Tabel LOG volume tinggi: audit MINIMAL (created_at saja) tanpa updated_*/deleted_*/is_deleted — pengecualian yang dinyatakan di sini sesuai
-- CLAUDE.md. Dipangkas FISIK oleh penyapu retensi log (repository/mysql/retention_repository.go) menurut ACS_LOG_RETENTION_DAYS; itu juga batas
-- panjang riwayat yang dapat dilihat. Tidak diekspos dengan PK; API hanya mengembalikan isi baris.

CREATE TABLE olt_onu_state_events (
    id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    olt_id         BIGINT UNSIGNED NOT NULL,
    serial_number  VARCHAR(32)     NOT NULL COMMENT 'Serial ONU apa adanya dari OLT (kunci riwayat bersama olt_id)',
    pon_port       VARCHAR(32)     NOT NULL DEFAULT '' COMMENT 'Port PON pada saat perubahan teramati',
    from_state_id  BIGINT UNSIGNED NULL COMMENT 'NULL = baru terlihat',
    to_state_id    BIGINT UNSIGNED NULL COMMENT 'NULL = tidak lagi dilaporkan OLT',
    rx_power_dbm   DECIMAL(6,2)    NULL COMMENT 'Daya terima sisi OLT pada saat itu (dBm); NULL = tidak dilaporkan',
    occurred_at    DATETIME(3)     NOT NULL COMMENT 'Waktu polling yang mengamati perubahan (bukan waktu perubahan sebenarnya)',
    created_at     DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_olt_onu_events_onu (olt_id, serial_number, id),
    KEY idx_olt_onu_events_time (occurred_at),
    CONSTRAINT fk_olt_onu_events_olt FOREIGN KEY (olt_id) REFERENCES olts (id),
    CONSTRAINT fk_olt_onu_events_from FOREIGN KEY (from_state_id) REFERENCES ref_olt_onu_state (id),
    CONSTRAINT fk_olt_onu_events_to FOREIGN KEY (to_state_id) REFERENCES ref_olt_onu_state (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Riwayat perubahan keadaan ONU terdaftar di OLT (log; audit minimal, dipangkas fisik oleh retensi); lihat header migrations/0049';
