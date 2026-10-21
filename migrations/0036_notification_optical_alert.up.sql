-- 0036_notification_optical_alert
--
-- Jenis notifikasi OPTICAL_ALERT: redaman optik (RX) perangkat melewati ambang waspada (-25 dBm) atau kritis (-27 dBm /
-- overload > -8 dBm). Keparahan per kejadian ditimpa di kode (WARNING/CRITICAL); bawaan jenis = WARNING, hanya in-app
-- (email dimatikan agar pelanggaran massal tidak membanjiri kotak masuk; pengguna dapat menyalakannya di preferensi).
-- Menambah jenis = operasi data, bukan skema (CLAUDE.md). Tidak ada tabel baru.

INSERT INTO ref_notification_types (code, name, description, severity_id, default_in_app, default_email, is_mandatory) VALUES
    ('OPTICAL_ALERT', 'Redaman optik perangkat', 'Redaman optik (RX) perangkat melewati ambang waspada atau kritis.',
        (SELECT id FROM ref_notification_severity WHERE code = 'WARNING'), 1, 0, 0);
