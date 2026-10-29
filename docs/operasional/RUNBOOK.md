# Runbook ACS (Auto Configuration Server)

Dokumen ini menjelaskan tata cara menjalankan dan men-deploy sistem ACS ini di lingkungan Development (lokal) dan Production.

## 1. Lingkungan Development (Lokal)

Untuk pengembangan lokal, kami menggunakan Docker Compose untuk menyediakan infrastruktur dasar (MariaDB, Redis, MinIO, Prometheus), sementara backend Go dan frontend React dapat dijalankan baik di dalam Docker maupun langsung di *host* untuk kemudahan proses *hot-reload*.

### Prasyarat Development
- Docker & Docker Compose terinstal
- Go 1.21+ terinstal (opsional jika hanya menjalankan image docker)
- Node.js v18+ terinstal

### A. Menjalankan via Docker Compose Penuh (Backend)
Cara tercepat untuk mendapatkan backend siap pakai tanpa perlu instalasi Go secara lokal:

1. Buat file `.env` di *root* proyek (Anda bisa menyalin/merujuk pada variabel yang digunakan di dalam `docker-compose.yml`):
   ```bash
   ACS_DB_ROOT_PASSWORD=rahasia_root
   ACS_DB_PASSWORD=rahasia
   ACS_JWT_SECRET=rahasia_jwt_super_panjang
   ACS_CREDENTIAL_ENC_KEY=32_byte_kunci_rahasia_untuk_aes_
   ACS_MINIO_ACCESS_KEY=admin
   ACS_MINIO_SECRET_KEY=admin123
   ```
2. Jalankan container:
   ```bash
   docker-compose up -d --build
   ```
   *Catatan: Container `migrate` akan otomatis berjalan untuk mengeksekusi migrasi skema MariaDB.*
3. Akses Layanan Lokal:
   - **REST API (Backend Dashboard)**: Diekspos pada port `18080`.
   - **Endpoint CWMP (TR-069)**: Diekspos pada port `17547` (untuk testing koneksi TR-069 CPE lokal).
   - **MinIO Console (S3 Storage)**: Diekspos di `localhost:19001` (login dengan `ACS_MINIO_ACCESS_KEY` dan `ACS_MINIO_SECRET_KEY`).
   - **Prometheus**: Diekspos di `localhost:19090` (untuk memantau metrik ACS).

### B. Menjalankan Frontend (UI React)
Frontend menggunakan bundler Vite. Selama masa pengembangan, disarankan menjalankan frontend secara native di host Anda (di luar docker):

1. Masuk ke direktori frontend:
   ```bash
   cd frontend
   ```
2. Atur environment variable untuk URL API backend:
   Buat file `.env` di folder `frontend/`:
   ```env
   # Arahkan ke port 18080 dari docker-compose
   VITE_API_URL=http://localhost:18080/api/v1
   ```
3. Install dependencies dan jalankan Dev Server:
   ```bash
   npm install
   npm run dev
   ```
4. Buka Web UI melalui browser, secara default pada tautan `http://localhost:5173`.

---

## 2. Lingkungan Production

Pada environment *Production*, disarankan untuk **memisahkan** *stateful component* (Database, Redis, Object Storage) agar menggunakan *managed services* atau terisolasi, sementara komponen *backend* (*acsd*) dan frontend (*React*) dilayani di balik *Reverse Proxy* (Nginx/HAProxy/Traefik) dengan keamanan (TLS/SSL).

### A. Topologi & Arsitektur
- **MariaDB 10.11+**: Digunakan sebagai *relational database* utama (bisa diganti dengan AWS RDS, Google Cloud SQL, dll).
- **Redis 7**: Digunakan untuk TR-069 Session Caching (bisa diganti dengan ElastiCache, Memorystore, dll).
- **S3 Object Storage**: Gunakan bucket cloud S3 (seperti AWS S3, Cloudflare R2, MinIO) untuk menyimpan *firmware rollout files*.
- **Backend `acsd`**: Berjalan sebagai Stateless Application di mesin VM atau *Pod* Kubernetes (Horizontal Scalable).
- **Frontend**: Hasil *build statis* (HTML/JS/CSS) yang di-*serve* via CDN atau webserver konvensional.

### B. Langkah Deployment Backend

1. **Kompilasi Binari (Go)**:
   Build binary *executeable* di lingkungan CI/CD pipeline:
   ```bash
   # Build binary ACS Daemon
   GOOS=linux GOARCH=amd64 go build -o acsd ./cmd/acsd
   
   # Build binary Migrasi DB
   GOOS=linux GOARCH=amd64 go build -o migrate ./cmd/migrate
   ```

2. **Eksekusi Migrasi Basis Data**:
   Di server atau *pipeline runner*, set ENV `ACS_DB_DSN` dan parameter rahasia, lalu eksekusi *database up*:
   ```bash
   ./migrate up
   ```

3. **Menjalankan ACS Daemon (`acsd`)**:
   Konfigurasi environment production di file tersendiri (misal: `/etc/acs/.env`), dan jalankan `acsd` lewat manajer layanan *Systemd*:
   
   Buat file: `/etc/systemd/system/acsd.service`
   ```ini
   [Unit]
   Description=ACS Backend & CWMP Engine
   After=network.target mysql.service

   [Service]
   Type=simple
   User=acsuser
   EnvironmentFile=/etc/acs/.env
   ExecStart=/usr/local/bin/acsd
   Restart=on-failure
   RestartSec=5
   
   # Batas ulimit untuk koneksi tinggi (Ribuan CPE websocket)
   LimitNOFILE=65535

   [Install]
   WantedBy=multi-user.target
   ```
   Aktifkan *service*:
   ```bash
   systemctl daemon-reload
   systemctl enable --now acsd
   ```

### C. Langkah Deployment Frontend

1. **Build Frontend**:
   Di mesin *build*:
   ```bash
   cd frontend
   npm ci
   npm run build
   ```
   Tindakan ini akan menghasilkan folder statis `dist/`.
   
2. **Distribusi File**:
   Salin isi folder `dist/` ke root direktori webserver (misal: `/var/www/acs/`).

### D. Konfigurasi Reverse Proxy (Contoh: NGINX)
Karena frontend React menggunakan *Client Side Routing*, Nginx harus me-*rewrite* 404 URL menuju `index.html`. Websocket untuk frontend dan komunikasi CPE CWMP memerlukan proksi khusus.

```nginx
server {
    listen 443 ssl http2;
    server_name acs.ispanda.com;

    # SSL Certificates Here (Let's Encrypt / Custom)
    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    # 1. Routing Web UI (Frontend React)
    location / {
        root /var/www/acs;
        index index.html;
        try_files $uri $uri/ /index.html;
    }

    # 2. Routing REST API dan Websocket Internal
    location /api/ {
        proxy_pass http://localhost:8080/api/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;      # Penting untuk Websocket
        proxy_set_header Connection "upgrade";       # Penting untuk Websocket
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # 3. Routing Trafik CWMP (Dari Device Router)
    # Direkomendasikan memisahkan domain untuk manajemen CPE jika skala besar, misal cpe.ispanda.com
    location /cwmp {
        proxy_pass http://localhost:7547/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Parameter keamanan untuk trafik Router TR069
        proxy_buffering off;
        proxy_read_timeout 3600;
        proxy_connect_timeout 60;
    }
}
```

### E. Maintenance dan Observabilitas Terusan
Sistem ACS mengekspos rute Prometheus di `GET /api/v1/metrics`. Tambahkan url server `acsd` ke *target scrape* `prometheus.yml` di infrastruktur *monitoring* Anda (Alertmanager/Grafana) agar bisa mendapatkan peringatan jika *latency inform TR-069* melesat atau penggunaan koneksi membengkak.


### F. Reverse proxy TLS siap pakai (Caddy, HTTPS otomatis)
Alternatif contoh NGINX di atas: overlay Compose yang sudah diuji fungsional (API, WebSocket `/api/v1/ws`, `/cwmp`, Console statis, HSTS, aset 404 sungguhan, health).

1. Bangun Console untuk dilayani di origin yang sama:
   `cd frontend && VITE_API_BASE_URL=/api/v1 VITE_SITE_URL=https://$ACS_DOMAIN npm ci && npm run build`
2. Isi `.env`: `ACS_DOMAIN=acs.contoh.id`, `ACS_TLS_EMAIL=noc@contoh.id` (notifikasi kedaluwarsa Let's Encrypt, wajib), `ACS_PUBLIC_URL=https://acs.contoh.id`, `ACS_CORS_ALLOW_ORIGINS=https://acs.contoh.id`.
3. Prasyarat: DNS A/AAAA `ACS_DOMAIN` → server; port 80, 443/tcp, 443/udp terbuka dari internet.
4. `docker compose -f docker-compose.yml -f docker-compose.tls.yml up -d`

Hasil: Console di `https://<domain>/`, REST di `https://<domain>/api/v1`, **ACS URL untuk CPE: `https://<domain>/cwmp`**. Port `acsd` tidak lagi dipublikasikan ke host. `ACS_TRUSTED_PROXIES` otomatis diisi subnet privat Docker agar IP klien/CPE asli terbaca (rate limit, audit). Catatan: sertifikat Let's Encrypt butuh domain publik; untuk LAN tertutup tanpa domain, sertifikat internal Caddy tidak dipercaya CPE → gunakan HTTP. Belum diuji dengan CPE fisik.

### G. Health & readiness
- `GET /healthz` — liveness (selalu 200 selama proses hidup; tanpa dependensi, jangan dipakai untuk memutuskan restart karena DB).
- `GET /readyz` — readiness: `200 {"status":"ready","checks":{"database":"ok","redis":"ok"}}`; **503** bila DB mati (kritis). Redis hanya dilaporkan (cache opsional). Tanpa teks error (aman dipublikasikan). Dipakai healthcheck Compose (`acsd` → *healthy*) dan layak untuk probe Kubernetes/load balancer: `livenessProbe: /healthz`, `readinessProbe: /readyz`.

### H. Beberapa instance `acsd` (HA)
Set `ACS_HA=true` pada **semua** instance (butuh Redis bersama; tanpa Redis HA otomatis mati + peringatan di log). Efek: (1) event WebSocket di-fan-out via Redis pub/sub — klien tersambung ke instance mana pun menerima event dari instance mana pun (tepat satu salinan); (2) rate limit REST/CWMP/login dihitung bersama lintas instance; (3) sweeper berkala (stale offline, timeout task, kedaluwarsa diagnostik, wave firmware, retensi) berjalan di satu instance per tick via kunci Redis ber-TTL (tahan crash); dispatcher webhook sudah berbasis klaim DB. Semua instance wajib memakai `ACS_JWT_SECRET` dan `ACS_CREDENTIAL_ENC_KEY` **yang sama**. Load balancer: sticky session **tidak diperlukan** — state sesi CWMP ada di DB (cookie `acs_session` hanya token). Terbukti dengan proxy round-robin per-request di depan dua `acsd`: satu sesi dibuka (Inform) di instance A dan diselesaikan (respons RPC Reboot + penutupan) di instance B tanpa galat. Redis down ≠ ACS berhenti: semuanya fail-open ke perilaku per-instance.
Diuji dengan dua `acsd` nyata berbagi MariaDB+Redis: event dari A sampai ke klien WS di B dan sebaliknya; 12 login bergantian A/B hanya meloloskan batas bersama (2).

### I. Enkripsi nilai rahasia at-rest (Preset/Profil/Task)
Nilai parameter bernama sensitif (`password|passphrase|psk|secret|token|key`) di `provisioning_profile_parameters`, `presets.configurations`, dan `tasks.parameters` (task SET_PARAMETER_VALUES) disimpan AES-GCM (`enc:v1:…`, kunci `ACS_CREDENTIAL_ENC_KEY`); API/engine melihat plaintext untuk profil/preset, sedangkan nilai pada task tidak pernah dikembalikan API (selalu `[redacted]`) dan baru dibuka saat dikirim ke CPE. Setelah upgrade, enkripsi data lama sekali jalan (idempoten; `updated_at` task tidak berubah). Task yang dibuat sebelum penyegelan task ada (mis. password WiFi dari uji lapangan) ikut tercakup:
`docker compose run --rm --no-deps --entrypoint /app/seal-secrets acsd`
**Backup DB dulu.** Setelah itu jangan menjalankan `acsd` versi lama pada DB yang sama (akan mengirim ciphertext ke CPE), dan **jangan kehilangan/ganti `ACS_CREDENTIAL_ENC_KEY`** (nilai tak terbaca; acsd gagal-tertutup, bukan mengirim sampah). Nilai yang dilaporkan CPE (`device_parameters`) tidak dienkripsi tetapi disamarkan di API untuk non-admin.

### I2. Kuota penyimpanan berkas per tenant
`POST /files` (ADMIN tenant) kini dibatasi total ukuran berkas aktif per tenant: `tenants.max_storage_bytes` (NULL = tak dibatasi, bawaan semua tenant lama). SUPERADMIN mengaturnya lewat `PATCH /tenants/{id}/storage-quota` atau modal **Kuota & Paket** di Administrasi (input dalam MB). Unggahan yang akan melampaui batas dijawab **429**; berkas yang sudah ada tidak dihapus bila batas diturunkan. Katalog firmware global (`firmware_files`) tidak dihitung. Cek-lalu-tulis tanpa kunci: unggahan serentak dapat melampaui batas sebesar beberapa berkas. `DELETE /files/{id}` kini juga menghapus objeknya dari MinIO (sebelumnya objek yatim tertinggal, sehingga siklus unggah-hapus bisa menghabiskan bucket tanpa tersentuh kuota).
**Urutan deploy migrasi 0043:** kolom baru di `tenants` mematahkan `acsd` lama (struct tanpa kolom itu gagal membaca `SELECT *`). Bangun ulang `acsd` lalu migrasi, atau lakukan serentak.

### I3. OLT Controller (irisan 1, read-only terhadap OLT)
Halaman **OLT** di Console dan endpoint `/olts`, `/olt-onus`, `/olt-drivers` (rancangan: [`../arsitektur/OLT_CONTROLLER_DESIGN.md`](../arsitektur/OLT_CONTROLLER_DESIGN.md)). ADMIN menambah OLT (driver, host, kredensial SNMP/SSH yang disimpan terenkripsi AES-GCM dan tidak pernah ditampilkan kembali), menguji koneksi, dan NOC/ADMIN mem-poll **ONU yang dilaporkan OLT sebagai belum terdaftar** ke antrean (PENDING / IGNORED / RESOLVED). ACS hanya **membaca**: belum ada pengesahan ONU atau penulisan konfigurasi ke OLT.
- **Driver vendor nyata belum ada** di irisan ini; menunggu keluaran OLT lab (daftar kebutuhan: desain §8). Sampai itu, satu-satunya driver adalah simulator `sim` yang **nonaktif bawaan**: setel `ACS_OLT_SIM_ENABLED=true` hanya untuk pengembangan/e2e/demo (ONU palsu, tanpa koneksi jaringan; jangan di produksi). Perilaku simulator dipilih lewat host: `10.255.0.2` tak terjangkau, `10.255.0.3` autentikasi ditolak, `10.255.0.4` antrean berputar, `10.255.0.5` kosong, lainnya A,B,C.
- **Host OLT** divalidasi saat disimpan (loopback, link-local/metadata cloud, dan multicast ditolak; RFC1918 diizinkan). `ACS_WEBHOOK_BLOCK_PRIVATE=true` juga menolak jaringan privat untuk host OLT (SaaS publik).
- **Polling manual**: tidak ada polling periodik di irisan ini. Satu polling per OLT dalam satu waktu (409 bila sedang berjalan); galat koneksi dijawab 502 tanpa teks dari perangkat dan tidak mengubah antrean. Riwayat RESOLVED dipangkas ke 200 entri terbaru per OLT.
- **Migrasi `0044`** menambah `ref_olt_poll_status`, `ref_onu_discovery_status`, `olts`, `olt_unregistered_onus` (tabel baru, tidak mengubah tabel lama; urutan deploy bebas).

### I4. Alert infrastruktur (Prometheus → Alertmanager → notifikasi SUPERADMIN)
Alur: Prometheus mengevaluasi `deploy/prometheus_alerts.yml` → mengirim ke Alertmanager (blok `alerting:` di `deploy/prometheus.yml`) → Alertmanager memanggil `POST /api/v1/alertmanager-webhook` di `acsd` → notifikasi **`PLATFORM_ALERT`** untuk semua **SUPERADMIN** (in-app, push WebSocket, dan email bila SMTP diatur; dibatasi 1 email/jam per pengguna). Admin tenant tidak menerimanya: metriknya agregat lintas tenant. Sebelumnya rantai ini putus di dua tempat (Prometheus tidak dikonfigurasi menunjuk Alertmanager, dan endpoint penerima tidak ada), jadi alert tidak pernah sampai ke siapa pun.
- **Aktifkan:** isi `ACS_ALERTMANAGER_WEBHOOK_TOKEN` di `.env` (min. 24 karakter acak, mis. `openssl rand -hex 32`), lalu `docker compose up -d --no-deps acsd alertmanager prometheus` (urutan bebas; migrasi `0045` menambah jenis notifikasi, jalankan `migrate` lebih dulu seperti biasa). Token **sama** dipakai `acsd` (memasang endpoint) dan service `alertmanager` (menuliskannya ke berkas sementara di dalam container; tidak ada rahasia di `deploy/alertmanager/config.yml`). Kosong = endpoint tidak dipasang (404) dan Alertmanager gagal mengirim (401/404 di lognya); alert tetap terlihat di UI Prometheus (`:19090/alerts`) dan Alertmanager (`:19093`).
- **Keparahan** dari label `severity` aturan: `critical` → CRITICAL, `info` → INFO, selain itu WARNING; alert pulih → `[PULIH]` berkeparahan INFO. **Dedupe:** alert yang sama (alertname + label) tidak dibuat ulang dalam 6 jam (firing) / 1 jam (resolved), jadi `repeat_interval` Alertmanager (4 jam) tidak membanjiri. Maks. 100 alert per permintaan.
- **Keamanan:** token Bearer dibandingkan waktu-konstan (hash), pembatas laju 5/detik per IP, body maks. 512 KiB, teks alert dibersihkan (karakter kontrol/baris baru dibuang, dipangkas). Mengubah ambang atau menambah aturan = sunting `deploy/prometheus_alerts.yml` lalu restart/reload `prometheus`; tidak perlu mengubah kode.
- **Diuji:** `amtool check-config` dan `promtool check config` pada berkas repo; e2e S36 (autentikasi, firing/pulih, dedupe, isolasi tenant, payload buruk); dan rantai penuh dengan Prometheus + Alertmanager sungguhan memakai definisi service compose ini (aturan uji yang pasti firing → notifikasi CRITICAL untuk SUPERADMIN dalam 20 detik).

### J. Variabel lingkungan operasional (ringkas)
| Variabel | Bawaan | Fungsi |
|---|---|---|
| `ACS_CWMP_RATE_LIMIT` | 5 | request/detik **per IP** di `/cwmp`; naikkan bila banyak CPE berbagi IP (CGNAT) |
| `ACS_REST_RATE_LIMIT` | 100 | request/detik **per IP** di REST API (Console/BSS). Satu halaman Console memuat 15–35 permintaan dan satu kantor biasanya berbagi satu IP (NAT); batas lama 30/dtk memicu 429 / toast "tidak dapat menjangkau server". Login, MFA, lupa/reset password, dan signup punya pembatas **sendiri yang jauh lebih ketat** (tidak ikut berubah) |
| `ACS_OIDC_ISSUER` / `_CLIENT_ID` / `_CLIENT_SECRET` / `_REDIRECT_URL` / `_ALLOWED_DOMAINS` | kosong | Masuk dengan Google / SSO OIDC (§R); `ALLOWED_DOMAINS` membatasi domain email |
| `ACS_NOTIFY_OFFLINE_RATIO` / `ACS_NOTIFY_FAILED_TASKS` | 0,30 / 5 | ambang notifikasi proaktif (§S) |
| `ACS_SELF_SIGNUP` | false | pendaftaran tenant mandiri (§N). **Wajib** SMTP (`ACS_SMTP_HOST`) bila true; aplikasi menolak start tanpa itu |
| `ACS_SIGNUP_DAILY_CAP` | 50 | plafon pendaftaran baru per hari untuk seluruh platform (503 bila terlampaui) |
| `ACS_SIGNUP_MAX_DEVICES` | 25 | batas perangkat awal tenant hasil self-signup (0 = tak dibatasi) |
| `ACS_SIGNUP_MAX_PENDING_TASKS` | 50 | kuota task menunggu awal tenant hasil self-signup (0 = tak dibatasi) |
| `ACS_TRUSTED_PROXIES` | kosong | CIDR proxy yang boleh menentukan IP klien lewat `X-Forwarded-For` (wajib di belakang proxy) |
| `ACS_OFFLINE_MIN_MINUTES` | 15 | Batas **minimum** diam sebelum perangkat ditandai OFFLINE (1–1440). Batas sebenarnya per perangkat = `max(ini, 2,5 × interval Periodic Inform yang dilaporkannya)`, maks 7 hari; interval belum diketahui = batas minimum. ONT ber-interval 12 jam baru offline setelah ±30 jam, bukan 15 menit. Penyapu berjalan tiap menit dan menyiarkan `DEVICE_OFFLINE` (lihat `docs/arsitektur/PETA_STATUS_DIAGNOSIS.md`) |
| `ACS_OLT_POLL_MINUTES` | 0 | Selang polling OLT berkala (menit, 0 = nonaktif). Memperbarui antrean ONU belum terdaftar dan inventaris ONU terdaftar (status LOS/dying gasp/port) dan menyiarkan `ONU_STATE_CHANGED`. Polling OLT nyata membebani perangkat dan membatasi sesi, jadi dinyalakan sengaja; maks 20 OLT per menit, goroutine terpisah dengan kunci sendiri |
| `ACS_CR_TRUSTED_CIDRS` | kosong | CIDR yang boleh menjadi host `ConnectionRequestURL` walau bukan IP sumber Inform (ACS di balik NAT/Docker Desktop, mis. `10.0.0.0/8`). Tetap hanya literal IP & lolos netguard; kosong = ketat |
| `ACS_WEBHOOK_BLOCK_PRIVATE` | false | SaaS publik: tolak target webhook di RFC1918/ULA/CGNAT (validasi **dan** saat menyambung — menutup DNS rebinding) |
| `ACS_ALERTMANAGER_WEBHOOK_TOKEN` | kosong | Token Bearer penerima webhook Alertmanager (min. 24 karakter acak, tanpa spasi, bukan placeholder). Dipakai `acsd` dan service `alertmanager`. Kosong = endpoint tidak dipasang dan alert tidak diteruskan (lihat §I4). Token lemah/pendek = `acsd` gagal start |
| `ACS_OLT_SIM_ENABLED` | false | Daftarkan driver OLT simulator `sim` (ONU palsu, tanpa koneksi jaringan) untuk pengembangan/e2e/demo. Jangan true di produksi (lihat §I3) |
| `ACS_TRANSFER_ALLOWED_HOSTS` | kosong | Host yang boleh menjadi `url` task Download/Upload buatan API (nama host, `*.subdomain` — tidak mencakup apex —, IP, atau CIDR; pisahkan koma; port diabaikan). Kosong = tak dibatasi. Unduhan dari katalog firmware/berkas ACS (presigned) tidak terkena. Entri tidak valid = `acsd` gagal start |
| `ACS_HA` | false | mode multi-instance (§H) |
| `ACS_OPTICAL_POLL_MINUTES` | 360 | interval polling redaman optik per perangkat (0 = mati, minimum 15) |
| `ACS_PPPOE_POLL_MINUTES` | 1440 | interval penyegaran username PPPoE per perangkat (0 = mati, minimum 60) |
| `ACS_LOG_RETENTION_DAYS` | 90 | pangkas tabel log lama (audit `activity_logs` tidak dipangkas); 0 = mati |
| `ACS_OIDC_*` | kosong | SSO OIDC (issuer, client id/secret, redirect URL) — akun dipetakan lewat **email** |
| `ACS_DB_MAX_OPEN_CONNS` / `_MAX_IDLE_CONNS` / `_CONN_MAX_LIFETIME_MINUTES` | 50 / 10 / 30 | ukuran connection pool MariaDB (`db.SetMaxOpenConns` dkk). Load test 2026-10-18 menemukan throughput CWMP plateau ~450–480 sesi/dtk dgn bawaan 50, terlepas dari concurrency, sebelum CPU acsd/MariaDB jenuh (LANJUTAN 14) — naikkan di produksi/HA bila profil beban nyata menunjukkan pola serupa; idle tidak boleh melebihi open |

### K. MFA (TOTP) & pemulihan akun
Pengguna mengaktifkan sendiri di Profil → Keamanan (QR + 10 kode pemulihan sekali pakai). Kehilangan perangkat **dan** kode pemulihan: ADMIN tenant yang sama atau SUPERADMIN memakai tombol **Lepas MFA** (Administrasi → Users) atau `DELETE /api/v1/users/{id}/mfa` — rahasia & kode dihapus, sesi akun dicabut, tercatat `MFA_RESET`. Kode TOTP salah ikut lockout akun (5× → 15 menit). Rahasia TOTP terenkripsi dengan `ACS_CREDENTIAL_ENC_KEY`. SSO tidak menambah MFA lokal.

### L. Hasil uji beban (lab, bukan jaminan produksi)
`cmd/loadtest`, instance + DB terpisah, `ACS_CWMP_RATE_LIMIT=100000`, Inform minimal tanpa RPC lanjutan, Docker Desktop di laptop, 5.000 sesi:

| concurrency | sesi/dtk | p50 | p95 | p99 | galat |
|---:|---:|---:|---:|---:|---:|
| 50 | 554 | 64 ms | 99 ms | 129 ms | 0 |
| 200 | 580 | 256 ms | 406 ms | 504 ms | 0 |
| 500 | 623 | 609 ms | 926 ms | 1,09 s | 0 |

Throughput jenuh ≈ 600 sesi/dtk (latensi naik linear = antrean). 15.000 perangkat unik terbentuk, seluruh sesi tertutup, tanpa error di log. Inform nyata (puluhan parameter, RPC lanjutan) jauh lebih berat — ulangi dengan profil Inform realistis & CPE fisik sebelum menjanjikan kapasitas.


### M. Operator SaaS: laporan, impersonate, paket (SUPERADMIN)
- **Laporan SaaS** (menu *Laporan SaaS*, `GET /reports/saas`): KPI platform, tabel per tenant (perangkat, online, pengguna, task 30 hari, kesehatan), tren 30 hari, unduh CSV (`/reports/saas/tenants.csv`; sel diawali `= + - @` dinetralkan agar aman dibuka di Excel), cetak/PDF dari peramban. Hanya SUPERADMIN; ADMIN tenant mendapat 403.
- **Impersonate** (Administrasi → Users → *Masuk sebagai*): membuat token **30 menit** berperan sebagai pengguna target (bukan SUPERADMIN, tenant aktif). Diblokir saat impersonate: endpoint platform, impersonate ulang, ganti password, MFA, token API, dan mutasi pengguna. Setiap aksi tercatat di audit dengan `impersonated_by`; awal/akhir sesi sebagai `IMPERSONATE_START/END`. Banner di Console menampilkan sesi impersonate + tombol **Keluar dari impersonate**.
- **Paket/batas perangkat** (Administrasi → Tenant → *Kuota & Paket*, `PATCH /tenants/:id/device-limit`): kosong = tak dibatasi. Setelah tercapai: Inform perangkat **baru** dijawab **503 + `Retry-After`** (tidak ada baris dibuat), pra-registrasi/impor ditolak **429**; perangkat yang sudah ada tidak terganggu. Pengecekan cek-lalu-tulis (pendaftaran serentak bisa melampaui sedikit). Ini batas teknis, **bukan** penagihan.

### N. Pendaftaran tenant mandiri (bawaan MATI)
Aktifkan hanya bila Anda memang membuka layanan ke pihak luar: `ACS_SELF_SIGNUP=true` + SMTP berfungsi (`ACS_SMTP_*`, `ACS_PUBLIC_URL` benar karena tautan verifikasi memakainya). Alur: formulir `/signup` → email tautan (24 jam) → klik → tenant + admin pertama dibuat (atomik, token sekali pakai) dengan batas awal §J. Perlindungan: pembatas laju ketat per IP, batas per email/hari, plafon harian platform, honeypot, respons seragam (tidak membocorkan email terdaftar). **Halaman Syarat & Privasi di Console hanyalah draf yang belum ditinjau penasihat hukum** — ganti sebelum dipakai publik.

### O. Menambah dukungan vendor/ONT baru (operasi data) & fallback standar
- Migrasi 0029 menambahkan vendor **`GENERIC`** ("Standar TR-069") berisi mapping standar TR-098/TR-181 (device.*, wan.pppoe.*, wifi.ssid/wpa_passphrase, diag.*, optical.* TR-181) + 27 vendor tambahan dengan OUI IEEE (TP-Link, Tenda, VSOL, Syrotech, Skyworth, Sagemcom, Zyxel, Dasan, Raisecom, KAON, Netis, TOTOLINK, D-Link, Askey, Sercomm, Arcadyan, MitraStar, UTStarcom, Genexis, Optilink, Sumavision, Ubiquoss, Calix, ADTRAN, Gemtek, Mercusys, Ruijie).
- **Urutan resolusi logical key**: mapping khusus model → mapping vendor → mapping `GENERIC`. Perangkat dengan OUI belum dikenal (vendor kosong) langsung memakai `GENERIC`; data model (TR-098/TR-181) dideteksi dari parameter yang dilaporkan CPE. Jangan menghapus vendor `GENERIC`.
- Perangkat dengan struktur nonstandar (index `WLANConfiguration` berbeda, ekstensi `X_…`): tambahkan mapping vendor/model lewat **Katalog Vendor** (tanpa mengubah kode); mapping tersebut otomatis menang atas `GENERIC`. Path optik TR-098 per vendor sengaja **belum** dipetakan (butuh dokumentasi vendor/dump `GetParameterNames`).
- Daftar OUI adalah data awal dari registry IEEE (bukan hasil uji perangkat). OUI di DeviceId CPE belum tentu sama dengan OUI MAC unit: bila Inform pertama muncul "vendor belum diketahui", tambahkan OUI-nya di Katalog.

### P. Catatan operator Windows (Git Bash/Docker Desktop)
Di Git Bash, argumen berawalan `/` diubah menjadi path Windows: set `MSYS_NO_PATHCONV=1` untuk `docker run/compose run` (mis. `/app/migrate`) dan saat membangun Console dengan `VITE_API_BASE_URL=/api/v1`. `docker compose up -d` penuh dapat gagal menarik image MinIO bila tidak ada internet: gunakan `--no-deps` per layanan.

### Q. Skrip bantu: uji asap pasca-deploy & data demo
- **`scripts/smoke-uat.mjs`** — uji asap satu perintah untuk server UAT/produksi (Node 20+, tanpa dependensi): `/healthz`, `/readyz`, Console, HSTS (bila https), metrik tidak terbuka publik, login, katalog (≥ 33 vendor + `GENERIC`), laporan SaaS, CWMP menolak Inform tanpa kredensial, dan — bila secret tenant diberikan — Inform nyata (device uji `SMOKE-…` dibuat lalu dihapus). Kredensial hanya dari environment (`ACS_SMOKE_API`, `ACS_SMOKE_CWMP`, `ACS_SMOKE_USER/PASS`, opsional `ACS_SMOKE_CWMP_USER/PASS`, `ACS_SMOKE_APP`). Exit code 1 bila ada yang gagal. Peringatan "metrik terbuka" pada HTTP langsung ke `acsd` (dev/LAN) wajar; di URL https publik harus tertutup (lihat §F).
- **`scripts/demo-seed.mjs`** — data demo untuk tinjauan/demo: 2 tenant fiktif (Nusa Fiber, Cakrawala Net), pengguna tiap peran, ±53 perangkat multi-vendor yang benar-benar Inform lewat simulator (OUI asli dari katalog, jadi vendor ter-resolve termasuk TP-Link/VSOL), tag, profil provisioning, task berhasil/gagal, penautan portal pelanggan. Lewat REST + CWMP (RBAC nyata); idempoten; **data simulasi, bukan perangkat nyata**. Butuh `bin/cpesim` linux. `--keepalive` meng-Inform ulang tiap 5 menit (sebagian perangkat sengaja dibiarkan offline). Kredensial demo ditulis ke `.demo-seed.local.json` (git-ignored) — jangan dipakai di server nyata.
- **Membersihkan data uji e2e dari DB dev**: e2e meninggalkan puluhan tenant/ratusan perangkat. Backup dulu (`mariadb-dump`), lalu hapus per tenant lewat API (soft delete) atau—khusus DB dev yang boleh dikosongkan—jalankan skrip SQL terpisah dengan sengaja. Jangan pernah pada DB UAT/produksi.

### R. Masuk dengan Google (OpenID Connect)
Login Google memakai alur OIDC yang sama dengan SSO lain (authorization code + `state` + `nonce`, token dikirim lewat fragment). **Akun tidak dibuat otomatis**: email Google harus sudah terdaftar sebagai pengguna ACS (dibuat admin) dan **terverifikasi di Google**; pemetaan lewat email, lalu terikat ke ID Google (`sub`) pada login pertama sehingga pengambilalihan lewat email yang berpindah tangan tertolak.
1. Google Cloud Console → *APIs & Services* → *Credentials* → **Create OAuth client ID** (jenis *Web application*). *Authorized redirect URI*: `https://<domain-api>/api/v1/auth/oidc/callback` (harus **persis** sama dengan `ACS_OIDC_REDIRECT_URL`; di belakang overlay TLS Caddy: `https://<ACS_DOMAIN>/api/v1/auth/oidc/callback`). Isi *OAuth consent screen* (nama aplikasi, email dukungan; untuk dipakai di luar organisasi, ajukan verifikasi — scope `openid email profile` bukan scope sensitif).
2. Isi `.env` (secret **hanya** di `.env`, jangan di-commit):
   `ACS_OIDC_ISSUER=https://accounts.google.com`, `ACS_OIDC_CLIENT_ID=…apps.googleusercontent.com`, `ACS_OIDC_CLIENT_SECRET=…`, `ACS_OIDC_REDIRECT_URL=https://<domain>/api/v1/auth/oidc/callback`, `ACS_PUBLIC_URL=https://<domain>`.
   Opsional: `ACS_OIDC_ALLOWED_DOMAINS=perusahaan.co.id,anak.co.id` — hanya email berdomain itu yang boleh masuk lewat SSO (disarankan untuk Google Workspace).
3. Restart `acsd`. Halaman login otomatis menampilkan tombol resmi **Masuk dengan Google** (backend melaporkan `sso_provider=google` di `GET /auth/config`).
- Pengguna ber-MFA lokal: login Google **tidak** meminta TOTP lokal (verifikasi dua langkah Google berlaku); jika kebijakan Anda mewajibkan MFA lokal, jangan aktifkan SSO.
- Gagal masuk (`/login?sso=error`): email belum terdaftar/tidak terverifikasi/domain tidak diizinkan/akun nonaktif/ID Google tak cocok. Penyebab rinci hanya ada di log `acsd` (pengguna sengaja tidak diberi tahu agar email terdaftar tidak bisa dienumerasi).
- **Belum diuji dengan Google nyata** di lab ini (butuh OAuth client milik Anda); alur penuh diuji dengan IdP tiruan RS256.

### S. Notifikasi (in-app + email)
- **Di Console**: ikon lonceng (jumlah belum dibaca, tab Semua/Belum dibaca, klik = buka halaman terkait), halaman `/notifications` (riwayat, **preferensi** per jenis: Console/email), dan toast realtime via WebSocket; judul tab menampilkan `(n)`.
- **Jenis**: peringatan keamanan akun (wajib; akun terkunci, password diganti/direset, MFA, login SSO, token API, impersonate), lonjakan perangkat offline, task gagal beruntun, rollout firmware dijeda, batas perangkat tenant tercapai, pendaftaran tenant baru (SUPERADMIN), dan pengumuman platform (SUPERADMIN → *Notifikasi → Pengumuman*).
- **Email**: memakai SMTP yang sama dengan reset password (`ACS_SMTP_*`; tanpa SMTP, hanya in-app). Maks. 1 email per pengguna per jenis per jam; notifikasi/email tidak pernah memuat password, token, atau secret.
- **Evaluator** berjalan tiap 5 menit di satu instance (kunci leader bila `ACS_HA=true`): offline ≥ 30% dari ≥ 10 perangkat (dedupe 6 jam), ≥ 5 task gagal dalam 30 menit (dedupe 1 jam).
- Retensi: notifikasi dipangkas bersama log lain (`ACS_LOG_RETENTION_DAYS`).

### T. Langganan & pembayaran (Xendit) — bawaan MATI
1. Buat akun Xendit, ambil **secret API key** (mode test dulu) dan tentukan **callback token** acak ≥ 24 karakter. Simpan hanya di `.env`.
2. Set `ACS_BILLING_ENABLED=true`, `ACS_XENDIT_SECRET_KEY`, `ACS_XENDIT_CALLBACK_TOKEN`, `ACS_PUBLIC_URL` (https publik; dipakai untuk URL kembali `/billing?status=success|failed`). `ACS_XENDIT_BASE_URL` bawaan `https://api.xendit.co`; `ACS_BILLING_INVOICE_HOURS` 1–168 (bawaan 24). Config menolak start bila enabled tanpa kunci/token.
3. Di dashboard Xendit daftarkan **Invoice paid/expired callback** ke `<ACS_PUBLIC_URL>/api/v1/billing/xendit/webhook` dengan token yang sama.
4. Urutan rilis: **rebuild `acsd` lalu migrasi 0032** (atau serentak) — `acsd` lama gagal membaca `tenants` setelah kolom baru ada.
5. Paket contoh (STARTER/PRO/ENTERPRISE) berharga placeholder: ubah di Console → Langganan (SUPERADMIN) sebelum dipakai. Harga hanya dibaca dari DB; klien tidak pernah mengirim nominal.
- Tidak ada pemutusan layanan otomatis saat masa aktif habis — hanya notifikasi + email; keputusan tindakan ada pada operator.
- Callback tidak dipercaya mentah: status/nominal selalu diambil ulang dari Xendit; event idempoten; token salah 401; billing mati 404.
- **Belum diuji dengan Xendit nyata/sandbox**. Uji dulu di mode test: buat invoice, bayar lewat simulator Xendit, pastikan callback menandai LUNAS dan masa aktif bertambah; tombol "Periksa status" sebagai cadangan bila callback gagal.

### U. Struktur dokumen
Semua dokumen ada di `docs/` (lihat `docs/README.md`). Perbarui PDF dengan `node scripts/build-docs-pdf.mjs` setelah mengubah HTML di `docs/panduan` atau `docs/pengujian`.

### V. Menu, laporan bug, riwayat impor/ekspor
- **Atur menu** (sidebar bawah): susunan per pengguna, tersimpan di server; "Kembalikan bawaan" menghapusnya. Menu baru yang kelak ditambahkan otomatis muncul di akhir bagi pengguna yang sudah punya susunan.
- **Laporan bug**: ikon bug di bilah atas. Email dikirim ke `ACS_BUG_REPORT_EMAIL` (bawaan hfzrmd@gmail.com; set kosong untuk mematikan — laporan tetap tersimpan). Butuh SMTP (`ACS_SMTP_*`); tanpa SMTP kolom `emailed_at` tetap kosong. Kelola laporan di **Laporan bug** (SUPERADMIN).
- **Riwayat impor/ekspor** (`/data-transfers`): tiap tenant melihat riwayat tenant-nya; SUPERADMIN semua. Email ringkasan ke pelaku (maks 20/jam/pengguna; tidak dikirim saat sesi impersonate). Tabel `data_transfer_logs` tidak ikut pembersihan retensi — pangkas manual bila perlu.
- **Migrasi 0033**: jalankan `docker compose build migrate` lalu `docker compose run --rm --no-deps migrate` (image migrate terpisah dari acsd).
- **Laporan bug**: dua isian terpisah (terjadi/diharapkan) + tangkapan layar opsional (PNG/JPEG ≤ 2 MB, tersimpan di DB, terlampir di email operator). Pelapor menerima notifikasi + email saat status berubah (butuh SMTP untuk emailnya). Migrasi 0034: `docker compose build migrate` lalu `docker compose run --rm --no-deps migrate`.
