-- 0045_notification_platform_alert
--
-- Jenis notifikasi PLATFORM_ALERT: alert infrastruktur dari Prometheus/Alertmanager (backlog task, kegagalan task per vendor, dst.)
-- yang diterima lewat POST /api/v1/alertmanager-webhook. Audiens bawaan = SUPERADMIN (operator platform): metriknya agregat
-- LINTAS tenant, jadi tidak pernah dikirim ke admin tenant. Keparahan per alert ditimpa di kode dari label severity; bawaan
-- jenis = WARNING. In-app dan email aktif bawaan (email dibatasi 1/jam per pengguna oleh evaluator notifikasi); pengguna dapat
-- mengubahnya di preferensi. Bukan jenis wajib.
-- Menambah jenis = operasi data, bukan skema (CLAUDE.md). Tidak ada tabel baru.

INSERT INTO ref_notification_types (code, name, description, severity_id, default_in_app, default_email, is_mandatory) VALUES
    ('PLATFORM_ALERT', 'Alert infrastruktur ACS', 'Alert dari Prometheus/Alertmanager (backlog task, kegagalan task per vendor, dst.). Hanya operator platform.',
        (SELECT id FROM ref_notification_severity WHERE code = 'WARNING'), 1, 1, 0);
