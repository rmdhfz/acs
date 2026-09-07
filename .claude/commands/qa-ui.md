---
description: Uji frontend di browser sungguhan lewat MCP agent-browser — alur, RBAC per role, console error, dark mode, mobile
argument-hint: "<halaman/alur, mis. 'login + devices sebagai NOC'> (kosong = smoke test penuh)"
---

Uji UI di browser sungguhan untuk: **$ARGUMENTS** (kosong = smoke test penuh).

Gunakan agent `ui-qa-browser` dengan MCP `agent-browser`.

Sebelum mulai, pastikan:
- Tool `agent_browser_*` tersedia (butuh restart sesi + persetujuan server MCP dari `.mcp.json`). Kalau tidak tersedia, **berhenti dan bilang** — jangan mengarang hasil.
- UI menyala: `npm run dev` di `frontend/` (→ `http://localhost:5173`), dengan backend `docker compose up -d` (API `http://localhost:18080/api/v1`) **atau** mock di `frontend/devmock/`. Sebutkan mode mana yang dipakai.

Alur kerja: `agent_browser_open` → `agent_browser_snapshot` (ambil `@ref`) → klik/isi → baca error console → screenshot ke direktori scratchpad.

Yang saya harapkan di laporan: langkah reproduksi, **diharapkan vs terjadi**, error console apa adanya, path screenshot, lalu daftar alur yang lulus dan yang **tidak sempat diuji**. Temuan RBAC (halaman/aksi yang bocor ke role yang tidak berhak) diperlakukan sebagai temuan keamanan.
