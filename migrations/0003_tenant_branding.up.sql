-- White-labeling per tenant (ROADMAP.md Fase 2) — logo/nama/warna aksen
-- yang ditampilkan di frontend saat user tenant tsb login. LogoURL berupa
-- URL eksternal (bukan upload file lewat ACS) untuk fase ini; upload logo
-- lewat object storage bisa menyusul kalau strategi S3 (firmware) sudah
-- berjalan dan mau dipakai bersama.
ALTER TABLE tenants
    ADD COLUMN brand_name    VARCHAR(128) NULL COMMENT 'Nama produk custom, override "ACS Console" default' AFTER name,
    ADD COLUMN logo_url      VARCHAR(512) NULL COMMENT 'URL logo eksternal, bukan upload' AFTER brand_name,
    ADD COLUMN primary_color CHAR(7)      NULL COMMENT 'Hex warna aksen, mis. #0f172a' AFTER logo_url;
