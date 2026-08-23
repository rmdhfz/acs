-- Kuota task queue per tenant (ROADMAP.md Fase 2) — mencegah satu tenant
-- menghabiskan resource task queue bersama (task PENDING lintas device milik
-- tenant tsb). NULL = tidak dibatasi, default aman utk tenant existing yang
-- belum diset. Scope kuota ini HANYA task queue -- BUKAN rate limit koneksi/
-- sesi CWMP itu sendiri (CWMP adalah sesi stateful, perubahan di area itu
-- di luar cakupan migrasi ini, lihat CLAUDE.md).
ALTER TABLE tenants
    ADD COLUMN max_pending_tasks INT UNSIGNED NULL COMMENT 'Batas jumlah task PENDING tenant ini; NULL = tidak dibatasi' AFTER primary_color;
