-- 0046_device_inform_interval
--
-- Interval Periodic Inform yang DILAPORKAN perangkat (ManagementServer.PeriodicInformInterval, detik). Dipakai penyapu offline:
-- sebelumnya semua perangkat dianggap offline setelah 15 menit tanpa Inform, sehingga ONT ber-interval 12 jam (43200 dtk, bawaan Telkom)
-- tampak offline padahal sehat. Batas offline kini = max(batas minimum, 2,5 x interval), dibatasi 7 hari; NULL (belum diketahui) =
-- batas minimum, perilaku lama. Kolom turunan dari parameter CPE (bukan keputusan ACS); tidak pernah ditulis oleh operator.
-- Penambahan kolom pada tabel yang sudah beraudit 7-kolom: tidak ada pengecualian audit baru.

ALTER TABLE devices
    ADD COLUMN inform_interval_seconds INT UNSIGNED NULL AFTER last_boot_event_at;
