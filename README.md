# ACS Console — ACS TR-069/CWMP multi-vendor untuk ISP

Auto Configuration Server (TR-069/CWMP) untuk mengelola ribuan ONT/ONU/router lintas merek (ZTE, Huawei, FiberHome, Nokia, C-Data, TP-Link, VSOL, dan 25+ lainnya) dari satu konsol web multi-tenant. Backend Go (Echo v5, sqlx, MariaDB, Redis), frontend React.

**Status:** tahap UAT internal. Seluruh uji otomatis memakai simulator CPE; **belum ada ONT fisik yang diuji** — lihat [`docs/laporan/CHECKLIST_SAAS.md`](docs/laporan/CHECKLIST_SAAS.md) untuk skor dan batasan yang jujur.

## Fitur utama
Provisioning otomatis (Zero-Touch + Preset), firmware canary rollout, diagnostik & redaman optik, Connection Request, multi-tenant dengan 5 peran (RBAC), MFA, **Masuk dengan Google**, notifikasi in-app + email, portal pelanggan, konsol operator SaaS (laporan, masuk-sebagai/impersonate, paket), REST API + webhook, audit trail.

## Mulai cepat (dev)

```bash
cp .env.example .env            # isi rahasia (JANGAN di-commit) — lihat docs/operasional/RUNBOOK.md
docker compose up -d            # MariaDB, Redis, MinIO, migrasi, acsd (API :18080, CWMP :17547)
docker compose exec acsd /app/seed-admin -username admin -password '<kuat>' -email admin@contoh.id -fullname "Admin"
cd frontend && npm ci && npm run dev   # Console di http://localhost:5173
```

Catatan Windows/Git Bash: awali perintah `docker run/exec` yang memakai path `/app/...` dengan `MSYS_NO_PATHCONV=1`.

## Menguji

```bash
go vet ./... && go test ./...                 # unit (race & integrasi MariaDB/Redis: lihat docs/pengujian/PENGUJIAN_E2E.md)
cd frontend && npm test && npm run lint       # tes komponen + lint
cd frontend && npm run audit:ui               # audit aksesibilitas/overflow di browser (butuh stack hidup)
node scripts/smoke-uat.mjs                    # uji asap pasca-deploy
```

## Dokumentasi

Semua dokumen ada di [`docs/`](docs/README.md) — mulai dari indeksnya. Aturan kerja untuk asisten kode ada di [`CLAUDE.md`](CLAUDE.md).

## Struktur repositori

| Direktori | Isi |
|---|---|
| `cmd/` | `acsd` (server), `migrate`, `seed-admin`, `cpesim` (simulator CPE), `e2e`, `loadtest`, `seal-secrets` |
| `internal/` | `domain` → `usecase` → `repository`/`delivery` (Clean Architecture) |
| `pkg/` | pustaka bersama (XML CWMP, kripto, mailer, TOTP, …) |
| `migrations/`, `schema.sql` | skema MariaDB (migrasi terurut + gambaran akhir) |
| `frontend/` | Console web React 19 + Vite + Tailwind v4 |
| `deploy/`, `docker-compose*.yml` | TLS (Caddy), Prometheus/Alertmanager/Grafana |
| `scripts/` | uji asap, data demo, build PDF dokumentasi, audit parity API↔UI |
| `docs/` | dokumentasi |
