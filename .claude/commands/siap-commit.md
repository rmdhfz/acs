---
description: Pipeline pra-commit lengkap — gerbang CI, review arsitektur, audit keamanan, sinkron API, lalu commit
argument-hint: "<ringkasan perubahan untuk pesan commit> (opsional)"
---

Jalankan pipeline pra-commit untuk perubahan yang ada di working tree sekarang. Konteks perubahan dari saya: **$ARGUMENTS**

Kerjakan berurutan, dan **berhenti serta lapor** kalau ada tahap yang gagal — jangan lanjut ke commit dengan gerbang merah.

1. `git status --short` + `git diff --stat` — tetapkan cakupan nyata perubahan.
2. **Gerbang CI** (`ci-gatekeeper`): gofmt, vet, build, `go test -race` lewat Docker `golang:1.26`; frontend `npm run lint` + `npm run build` bila `frontend/` tersentuh; `redocly lint` bila `openapi.yaml` tersentuh.
3. **Review arsitektur** (`acs-code-reviewer`) bila ada perubahan di `internal/` — pelanggaran layering, percabangan vendor di luar `vendor_adapter/`, konvensi skema.
4. **Audit keamanan** (`acs-security-reviewer`) bila ada perubahan pada route, middleware, IAM, atau kode yang menyentuh kredensial.
5. **Sinkron kontrak API** (`api-contract-sync`) bila `internal/delivery/http/` berubah.
6. Ringkas semua temuan ke saya dan **tanya dulu** sebelum commit apabila ada temuan berisiko tinggi.
7. Kalau bersih: commit ke branch saat ini dengan pesan bergaya conventional commit yang konsisten dengan riwayat repo (`feat(frontend):`, `fix(domain):`, `docs(api):`, `chore(dev):`). Jangan push kecuali saya minta.

Tahap 3–5 hanya dijalankan bila relevan dengan diff — sebutkan tahap mana yang dilewati dan kenapa.
