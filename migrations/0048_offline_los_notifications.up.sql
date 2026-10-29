-- 0048_offline_los_notifications
--
-- Pemberitahuan perangkat offline dan ONU LOS/dying gasp:
--  * ref_webhook_event_types: DEVICE_OFFLINE (satu event per perangkat yang baru ditandai OFFLINE oleh penyapu) dan ONU_STATE_CHANGED
--    (satu event per polling OLT, memuat daftar perubahan keadaan ONU, dibatasi 50 rincian + jumlah total) untuk BSS/NMS.
--  * ref_notification_types: DEVICE_OFFLINE dan ONU_STATE_ALERT. Keduanya diringkas per tenant/OLT per sapuan, BUKAN satu notifikasi per
--    perangkat. Bawaan hanya in-app (email mati: pemadaman massal tidak membanjiri kotak masuk; pengguna dapat menyalakannya di
--    preferensi). Audiens bawaan ADMIN+NOC tenant. Keparahan per kejadian ditimpa di kode.
-- Menambah jenis = operasi data, bukan skema (CLAUDE.md). Tidak ada tabel baru.

INSERT INTO ref_webhook_event_types (code, name, description) VALUES
    ('DEVICE_OFFLINE', 'Device Offline',
        'Perangkat berpindah ke OFFLINE karena tidak Inform melewati ambang offline per perangkat (satu event per perangkat)'),
    ('ONU_STATE_CHANGED', 'ONU State Changed',
        'Keadaan ONU terdaftar di OLT berubah di antara dua polling (satu event per polling OLT, berisi daftar perubahan)');

INSERT INTO ref_notification_types (code, name, description, severity_id, default_in_app, default_email, is_mandatory) VALUES
    ('DEVICE_OFFLINE', 'Perangkat offline', 'Perangkat baru saja ditandai offline karena berhenti melapor (diringkas per sapuan).',
        (SELECT id FROM ref_notification_severity WHERE code = 'WARNING'), 1, 0, 0),
    ('ONU_STATE_ALERT', 'ONU LOS / dying gasp', 'ONU di OLT masuk keadaan LOS (kehilangan sinyal optik) atau dying gasp (kehilangan daya), diringkas per polling OLT.',
        (SELECT id FROM ref_notification_severity WHERE code = 'WARNING'), 1, 0, 0);
