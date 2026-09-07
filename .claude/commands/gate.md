---
description: Jalankan gerbang kualitas lokal yang meniru CI (gofmt, vet, build, test -race, frontend, openapi lint)
argument-hint: "[backend|frontend|openapi|all] (default all)"
---

Jalankan gerbang kualitas untuk cakupan: **$ARGUMENTS** (kalau kosong, artinya `all`).

Gunakan agent `ci-gatekeeper` untuk ini. Ingat konteks host: **tidak ada Go toolchain terpasang**, jadi cek Go dijalankan lewat Docker image `golang:1.26`; Node 24 + npx tersedia native.

Laporkan hasil per gerbang secara eksplisit (LULUS / GAGAL / TIDAK DIJALANKAN + alasan), tempelkan output error asli untuk yang gagal, dan jangan menyimpulkan "CI aman" bila ada gerbang yang tidak sempat dijalankan.
