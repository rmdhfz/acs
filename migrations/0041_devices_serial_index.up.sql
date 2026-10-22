-- 0041_devices_serial_index
--
-- Pencarian perangkat PERSIS berdasarkan serial (GET /devices?serial_number=...) — pola integrasi BSS/NOC: serial dikenal dari data
-- pelanggan, id internal tidak. Satu-satunya indeks serial sebelumnya adalah uq_devices_oui_serial (oui, serial_number), yang tidak
-- bisa dipakai bila OUI tidak diketahui (serial bukan awalan indeks) sehingga pencarian memindai seluruh tabel devices.
--
-- Catatan operasional: ADD KEY pada InnoDB berjalan online (tanpa mengunci tulis) tetapi memakan waktu sebanding ukuran tabel;
-- jalankan di luar jam sibuk pada armada ratusan ribu perangkat.

ALTER TABLE devices
    ADD KEY idx_devices_serial (serial_number);
