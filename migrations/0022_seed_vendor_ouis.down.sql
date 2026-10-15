-- 0022_seed_vendor_ouis (down)
--
-- Hanya menghapus baris hasil seed (marker di notes); OUI buatan operator tetap.

DELETE FROM vendor_ouis WHERE notes LIKE 'seed-0022 IEEE MA-L:%';
