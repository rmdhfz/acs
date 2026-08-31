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
