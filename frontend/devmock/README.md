# dev mock API

Server REST `/api/v1` **palsu** untuk melihat UI/UX frontend **tanpa** backend
Go, MariaDB, Redis, MinIO, atau Docker. Berguna saat laptop tidak kuat menjalankan
stack penuh.

```bash
# 1. jalankan mock (butuh Node 18+, TANPA npm install)
node frontend/devmock/server.mjs        # -> http://localhost:18080/api/v1

# 2. di terminal lain, jalankan frontend
cd frontend && npm run dev              # -> http://localhost:5173
```

Buka <http://localhost:5173/login>.

**Login:** username menentukan peran — `superadmin`, `admin`, `noc`, `viewer`,
`enduser` (default `superadmin`). Password apa saja.

## Yang BISA dilihat

- Semua halaman & navigasi (role-aware), dark mode, command palette (Ctrl/Cmd+K)
- Dashboard analitik (data statis), daftar device, parameter tree, tasks, dsb.
- Toast & dialog konfirmasi baru (aksi tetap "berhasil" walau tidak nyata)
- Modal ganti password, editor preset/tag/webhook

## Yang TIDAK bisa

- Tidak ada TR-069 / CWMP nyata, tidak ada DB — sebagian besar perubahan
  (create/hapus) hanya bertahan di memori proses mock, hilang saat di-restart.
- Bukan pengganti pengujian fungsional. Untuk itu tetap butuh backend asli
  (lihat `RUNBOOK.md`).

File ini sengaja di-commit sebagai alat bantu dev, bukan bagian dari produk.
