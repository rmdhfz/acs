-- 0042_automation_daily_ceiling
--
-- Batas keras jumlah pemicuan per (aturan, perangkat) per 24 jam, TIDAK bergantung pada kondisi aturan. Pagar "berturut-turut tanpa
-- kondisi pernah salah" (migrations/0040) bisa dilewati kondisi yang berosilasi akibat aksinya sendiri — mis. kondisi
-- contains({@events},'2 PERIODIC') + aksi reboot: Inform berikutnya adalah '1 BOOT' (kondisi salah, hitungan berturut direset), lalu Inform
-- periodik menembak lagi, selamanya (ditemukan audit keamanan independen 22 Okt 2026). Jendela bergulir 24 jam menutup celah itu.
--
-- Kolom pada automation_states (state operasional; pengecualian audit 7-kolom sudah dinyatakan di header 0040).

ALTER TABLE automation_states
    ADD COLUMN window_started_at DATETIME(3) NULL COMMENT 'Awal jendela 24 jam penghitung pemicuan' AFTER suspended,
    ADD COLUMN window_fires INT UNSIGNED NOT NULL DEFAULT 0 COMMENT 'Pemicuan dalam jendela 24 jam berjalan' AFTER window_started_at;
