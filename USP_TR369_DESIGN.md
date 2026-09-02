# USP_TR369_DESIGN.md — Proposal Desain Dukungan TR-369 / USP

**Status:** PROPOSAL KEPUTUSAN — belum ada implementasi nyata. Butuh jawaban
pemilik produk atas §8 sebelum coding dimulai.
**Dibuat:** 2026-09-02 (sesi `/goal` "lebih baik dari GenieACS?")

Dokumen ini menaruh keputusan arsitektur di tangan pemilik produk — bukan
spesifikasi final. Baca `PRD.md` §4.2 (USP = out-of-scope Fase 1, dipertimbangkan
Fase 3) dan `CLAUDE.md` ("keputusan arsitektur besar → tanyakan dulu").

---

## 1. Kondisi saat ini — jujur

USP di repo ini **stub, bukan implementasi**:

| Berkas | Kondisi nyata |
|---|---|
| `pkg/usp/record.go` | Struct **JSON** buatan sendiri (`Record`, `Msg`, `Get`, `Set`, `Notify`). **BUKAN** format wire USP sungguhan — USP pakai **Protocol Buffers** (`usp-record.proto` + `usp-msg.proto` dari Broadband Forum). Tidak ada agen USP nyata yang bisa bicara dengan struct ini. |
| `internal/delivery/usp/ws_handler.go` | WebSocket handler di `/usp`, subprotokol `v1.usp` benar. Tapi `endpointID` fallback ke `RemoteAddr`, `CheckOrigin` selalu true, dan pesan masuk cuma di-`json.Unmarshal`. |
| `internal/usecase/usp_session/service.go` | `HandleMessage` = `// TODO: Route USP Message` + `slog.Info`. **Tidak melakukan apa-apa.** `conns map[string]Connection` **in-memory** — tidak multi-instance-safe (app server didesain stateless, TECH.md §9). |

**Kesimpulan:** klaim "USP didukung" tidak benar. Untuk jujur, ROADMAP/scorecard
harus bilang "USP belum diimplementasikan".

## 2. Apa itu USP, dan kenapa beda dari CWMP

TR-369 (USP — User Services Platform) adalah penerus TR-069, dari Broadband Forum
yang sama. Perbedaan yang relevan untuk arsitektur kita:

| Aspek | TR-069 / CWMP (yang sudah kita punya) | TR-369 / USP |
|---|---|---|
| Wire format | SOAP/XML | **Protocol Buffers** (2 lapis: Record membungkus Message) |
| Transport (MTP) | HTTP(S), CPE→ACS, sesi per-Inform | **Beberapa MTP**: WebSocket, MQTT 5.0, STOMP, CoAP. Koneksi **long-lived** (agent menahan koneksi). |
| Arah koneksi | CPE meng-*Inform* ACS; ACS "membangunkan" CPE lewat Connection Request | Agent membuka & **menahan** koneksi ke Controller; Controller kirim request kapan saja lewat koneksi itu (tidak perlu Connection Request / STUN) |
| Data model | TR-098 (`InternetGatewayDevice.`) **atau** TR-181 (`Device.`) | **Selalu** Device:2 (`Device.` — TR-181) |
| Operasi | GetParameterValues, SetParameterValues, AddObject, DeleteObject, GetParameterNames, Reboot, Download, … | **Get, Set, Add, Delete, Operate, GetSupportedDM, GetInstances, Notify** (Operate = RPC generik: Reboot, FactoryReset, firmware, diagnostics semua lewat Operate) |
| Identitas | OUI + Serial (+ Basic/Digest auth) | **Endpoint ID** (`proto::` / `os::` / `self::` scheme) + auth per-MTP (TLS client cert, MQTT creds, dst.) |
| Notifikasi | Event codes (`0 BOOTSTRAP`, `4 VALUE CHANGE`, …) di Inform | **Notify** message: `Event`, `ValueChange`, `ObjectCreation`, `ObjectDeletion`, `OperationComplete`, `OnBoardRequest` |
| Registrasi awal | Inform `0 BOOTSTRAP` | `Notify` dengan event `Boot!` + `OnBoardRequest` (agent minta di-adopt) |

**Yang penting:** USP **bukan** "CWMP di atas WebSocket". Model koneksinya
kebalikan — dan itu mengubah bagian tersulit sistem kita (§5).

## 3. Yang BISA dipakai ulang vs yang HARUS baru

### Bisa dipakai ulang (besar — ini alasan kita tidak mulai dari nol)

- **`data_model_versions` + `vendor_parameter_mappings`** — USP agent = TR-181
  (`Device.`), dan layer resolusi logical-key kita sudah menangani `Device.*`.
  `wifi.5g.ssid` → `Device.WiFi.SSID.{i}.SSID` bekerja tanpa perubahan.
- **`devices`, `device_parameters` (EAV), `device_events`, `device_optical_metrics`** —
  skema device netral-protokol. USP agent jadi baris `devices` biasa (tambah
  kolom `mtp` / `endpoint_id`).
- **`tasks` + task queue** — USP Get/Set/Operate pada dasarnya task RPC. Bisa
  `task_type` baru (`USP_GET`, `USP_SET`, `USP_OPERATE`) atau map ke type yang ada.
- **`usecase/provisioning` (ZTP + engine preset)** — precondition & drift-check
  berlaku sama; cuma jalur eksekusi (kirim ke agent) yang beda.
- **`usecase/device`, `usecase/firmware`, `usecase/diagnostics`** — logika bisnis
  sama; yang beda cuma "RPC apa yang dikirim ke device".
- **RBAC, multi-tenant, audit, webhook** — semua netral-protokol.

### Harus baru

- **Protobuf USP** — vendor `usp-record.proto` + `usp-msg.proto` (BBF, lisensi
  BSD-3), generate Go via `protoc`/`buf` **di build-time** (bukan runtime).
  Tambah ke CI. Ini dependency baru — perlu persetujuan (TECH.md §12: "riset
  lisensi & kematangan sebelum dipakai produksi").
- **MTP layer** (`internal/delivery/usp/`) — decode Record → verifikasi →
  decode Message → route. Encode balik untuk response/request.
- **Routing koneksi lintas-instance** (§5) — bagian tersulit.
- **`usp_agents` / kolom di `devices`** — Endpoint ID, MTP, controller cred,
  status koneksi, `last_seen`.
- **Adapter operasi** — USP Operate → task; USP Notify → event/registrasi.
- **Registrasi/adopsi** — `OnBoardRequest` → buat device, assign tenant (mirip
  `FindOrCreateFromInform`), evaluasi ZTP/preset.

## 4. Pilihan MTP (transport)

| MTP | Plus | Minus | Rekomendasi |
|---|---|---|---|
| **WebSocket** | Agent-initiated (lewat NAT/CGNAT tanpa STUN — masalah `PRD.md` §12 hilang untuk USP!). Tanpa broker. Mirip pola yang sudah kita pakai (`internal/delivery/ws` untuk frontend). TLS = wss. | Koneksi pinned ke 1 instance acsd → butuh routing (§5). Agent harus reachable... tidak, agent yang connect keluar. | **v1: WebSocket saja.** |
| **MQTT 5.0** | Broker menangani fan-out & routing lintas-instance secara alami (acsd subscribe ke topik). Skala besar lebih mudah. Banyak agent CPE dukung MQTT. | **Dependency infra baru** (broker MQTT HA — Mosquitto/EMQX/HiveMQ). Auth & topic ACL per-tenant perlu desain. | v2 — kalau skala/vendor menuntut. |
| **STOMP** | — | Jarang dipakai vendor CPE. | Lewati. |
| **CoAP** | Ringan (IoT). | Bukan use-case ONT/router FTTH kita. | Lewati. |

**Alasan WebSocket dulu:** menghilangkan seluruh kelas masalah "CPE di belakang
CGNAT tidak reachable" (agent yang dial keluar & menahan koneksi), tanpa
menambah komponen infra. Trade-off-nya (routing lintas-instance) harus kita
selesaikan juga untuk MQTT nanti, jadi bukan kerja terbuang.

## 5. Masalah tersulit: koneksi long-lived + app server multi-instance

CWMP kita stateless karena tiap request CPE bisa mendarat di instance mana pun
(state di `device_sessions`/`tasks`). **USB WebSocket tidak begitu** — koneksi
agent X hidup di **satu** proses acsd. Kalau operator klik "Reboot" dan request
REST itu mendarat di instance B sedangkan agent X terhubung ke instance A,
instance B harus meneruskan perintah ke A.

Opsi:

| Opsi | Cara | Plus | Minus |
|---|---|---|---|
| **A. Redis pub/sub** | Tiap acsd daftar `usp:conn:<endpoint_id> → instance_id` di Redis. Kirim task = publish ke channel instance pemilik; instance itu menulis ke socket. Redis **sudah dipakai** (session cache CWMP). | Reuse infra, sederhana | Redis jadi SPOF untuk USP (mitigasi: Redis HA) |
| **B. Sticky LB by Endpoint ID** | LB route WebSocket by hash(endpoint_id). REST tetap stateless tapi tahu instance mana via tabel. | — | Butuh LB pintar; rebalancing saat scale = massal reconnect |
| **C. Broker (MQTT) dari awal** | Skip WebSocket, mulai MQTT — broker yang route. | Tidak ada masalah routing di kode kita | Dependency infra besar dari hari 1 |
| **D. Single-instance untuk USP** | 1 proses khusus handle semua USP WebSocket. | Paling simpel | Bukan HA; tidak sesuai target skala |

**Rekomendasi: Opsi A (Redis pub/sub).** Konsisten dengan keputusan Redis yang
sudah diambil, HA-nya masalah Redis (bukan kode kita), dan tidak menambah
komponen baru. Tulis abstraksi `USPRouter` supaya bisa ganti ke broker nanti.

## 6. Cakupan v1 yang diusulkan (kalau disetujui)

**IN:**
- Protobuf USP (vendored proto + generated Go, di CI).
- MTP WebSocket + TLS, verifikasi Endpoint ID.
- Decode/encode Record + Message.
- Operasi: **Get, Set, Add, Delete, Operate, GetInstances** (cukup untuk parity
  dengan fungsi CWMP yang dipakai).
- **Notify**: `Boot!`, `ValueChange`, `OnBoardRequest`, `OperationComplete`.
- Registrasi/adopsi agent → `devices` + assign tenant + ZTP + engine preset.
- `USPRouter` (Redis pub/sub) untuk kirim request lintas-instance.
- Task queue: `USP_GET`/`USP_SET`/`USP_OPERATE` map ke `tasks`.
- Migrasi: kolom `devices.mtp` + `devices.usp_endpoint_id` (+ `ref_mtp`?), atau
  tabel `usp_agents` — **keputusan db-schema-guardian**.
- Unit test (fake MTP conn, fake router).

**OUT (v1.1+):**
- MQTT / STOMP / CoAP MTP.
- USP Session Context (reliable messaging, retransmit) — v1 pakai sessionless.
- GetSupportedDM (introspeksi data model penuh).
- Bulk Data Collection (`Device.BulkData.`).
- E2E session security (payload encryption di atas TLS).
- Controller-initiated subscription persistence lintas-restart.

## 7. Estimasi & risiko

- **Effort:** besar — protobuf toolchain + MTP + router + adapter + registrasi +
  test. Realistis beberapa sesi, bukan satu.
- **Risiko lisensi/tooling:** proto BBF = BSD-3 (aman), tapi `protoc`/`buf` di
  CI = infra baru. Alternatif: commit generated `.pb.go` (tanpa protoc di CI).
- **Risiko "tidak teruji":** sama seperti CWMP — tidak ada agen USP fisik di
  lab. Bisa diuji dengan **obuspa** (implementasi agen USP referensi BBF,
  open-source) di container. **Ini keunggulan vs CWMP:** ada agen referensi
  resmi yang bisa dipakai CI/lab, tidak perlu perangkat fisik untuk validasi
  dasar.
- **Risiko scope creep:** USP Operate + data model Device:2 sangat luas.
  Disiplin ke §6 IN/OUT.

## 8. Pertanyaan untuk pemilik produk (jawab sebelum coding)

1. **Prioritas:** USP sekarang (Fase 3 dimulai), atau tetap tunda sampai uji
   lapangan CWMP selesai? (Rekomendasi: tunda — kematangan CWMP lapangan =
   blocker yang lebih menentukan untuk klaim "lebih baik dari GenieACS".)
2. **MTP v1:** setuju **WebSocket saja** dulu (§4)? Atau ada vendor CPE target
   yang hanya bicara MQTT?
3. **Routing lintas-instance:** setuju **Redis pub/sub** (§5 Opsi A)? Redis
   sudah jadi dependency — ini memperluas perannya jadi kritikal untuk USP.
4. **Protobuf di build:** commit generated `.pb.go` (tanpa protoc di CI) atau
   tambah `buf`/`protoc` ke CI?
5. **Skema:** kolom di `devices` (`mtp`, `usp_endpoint_id`) atau tabel
   `usp_agents` terpisah? (Serahkan ke `db-schema-guardian` setelah arah jelas.)
6. **Adopsi agent:** auto-adopt semmua `OnBoardRequest` yang lolos TLS (seperti
   `FindOrCreateFromInform` sekarang), atau butuh approval manual di UI?

---

*Bila disetujui, implementasi dipecah: (a) protobuf + MTP WebSocket + echo Record,
(b) adapter operasi + task, (c) Notify + registrasi + ZTP/preset, (d) USPRouter
Redis. Tiap bagian direview `acs-code-reviewer` + (untuk a/d) `acs-security-reviewer`,
skema oleh `db-schema-guardian`.*
