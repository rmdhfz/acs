# Peta, status realtime, dan diagnosis offline — Catatan Desain

> Status: **irisan A diimplementasikan 2026-10-29** (migrasi `0046`, `usecase/troubleshoot`, `GET /devices/map`, `GET /devices/{id}/diagnosis`, event WebSocket
> `DEVICE_OFFLINE`, tampilan Peta di `/devices`, kartu diagnosis di detail perangkat; e2e S38). **Irisan B1 (kerangka data sisi OLT dengan simulator)** diimplementasikan
> 2026-10-29 (migrasi `0047`, e2e S39; lihat bagian terakhir). **Data OLT nyata belum ada** dan terblokir akses OLT lab.

## Batas yang harus dibaca dulu

- Semua yang tampil berasal dari **TR-069**: Inform, event, redaman yang dilaporkan perangkat, fault, task. ACS **belum terhubung ke OLT** (hanya driver simulator),
  jadi tidak ada relasi ONT ↔ OLT/PON/port, tidak ada status port, dan penyebab fisik (fiber putus, LOS, dying gasp) **tidak bisa disimpulkan**. Diagnosis
  menyatakannya sendiri di field `limits`.
- Diagnosis adalah **hipotesis berbukti**, bukan vonis. Tiap temuan membawa `evidence` (fakta terukur) dan `hint` (langkah saran).

## Status online/offline

Perangkat OFFLINE bila diam lebih lama dari `max(ACS_OFFLINE_MIN_MINUTES, 2,5 × interval Periodic Inform yang dilaporkannya)`, dibatasi 7 hari (`domain.OfflineThreshold`;
rumus yang sama ditulis di SQL penyapu `MarkStaleOffline` — ubah keduanya bersamaan; tes integrasi MariaDB memeriksa kesepakatan keduanya di 12 kasus).

- **Mengapa per perangkat:** ambang tetap 15 menit membuat ONT ber-interval 12 jam (bawaan Telkom, 43200 dtk) tampak offline padahal sehat.
- **Dari mana intervalnya:** kolom `devices.inform_interval_seconds` (migrasi 0046). Kebanyakan ONT nyata **tidak** melaporkannya di Inform (terbukti pada ZTE F660/F609 dan
  Optictimes), jadi ACS mengantre satu tugas GetParameterValues `ManagementServer.PeriodicInformInterval` (TR-098 atau TR-181 sesuai model data). Interval belum diketahui =
  batas minimum, dan diagnosis menyatakannya ("status bisa keliru bila perangkat memang ber-interval panjang").
- **Urutan di Inform:** penemuan direncanakan *sebelum* poll optik/PPPoE dan dijalankan *sesudahnya*. Dua urutan sederhana sama-sama salah (ditemukan lewat e2e): poll lebih dulu
  membuat penemuan selalu "sibuk" dan tak pernah jalan; penemuan lebih dulu memakai satu-satunya slot sehingga pembacaan optik pertama tertunda sampai Inform berikutnya (12 jam
  untuk ONT nyata). "Sedang sibuk" tidak mengunci percobaan berikutnya; percobaan dikunci 24 jam per perangkat.
- **Realtime:** `DEVICE_ONLINE` (saat Inform) dan `DEVICE_OFFLINE` (penyapu, tiap menit) disiarkan hanya ke tenant pemilik. Frontend menyegarkan daftar, peta, dan diagnosis.
  Deteksi offline paling lambat ambang + 1 menit; **bukan** deteksi seketika (itu butuh data OLT atau Connection Request berkala).

## Peta

`GET /devices/map` memakai pembangun filter yang sama dengan `GET /devices` (`buildDeviceWhere`) dan aturan scope tenant/proteksi parameter rahasia yang sama. Leaflet + `leaflet.markercluster`
(open source), dimuat lazy. Titik maks 5000 (`truncated` lalu mode viewport memakai `bbox`). Klaster berwarna menurut status **terparah** di dalamnya. Perangkat tanpa koordinat
dinyatakan jumlahnya (tidak digambar). Popup dibangun dengan DOM `textContent` (serial berasal dari CPE, tidak tepercaya).

- **Ubin peta:** bawaan server publik OpenStreetMap yang kebijakannya melarang pemakaian berat. Produksi: set `VITE_MAP_TILE_URL` / `VITE_MAP_ATTRIBUTION` ke penyedia sendiri atau self-host.
- **Koordinat** diisi lewat Edit perangkat atau impor massal; belum ada isian "klik di peta".

## Aturan diagnosis

| Kode | Keparahan | Syarat (ringkas) |
|---|---|---|
| `NEVER_INFORMED` | warning | belum pernah Inform |
| `OPTICAL_BAD` / `OPTICAL_WEAK` | critical / warning | RX terakhir di luar -27..-8 dBm / di bawah -25 dBm |
| `OPTICAL_DROPPING` | warning | RX turun ≥ 3 dB dari pembacaan terbaik 6 sebelumnya (butuh ≥ 3 pembacaan) |
| `REBOOT_BEFORE_SILENCE` | warning | Inform terakhir ≤ 30 menit setelah event BOOT |
| `BOOT_FLAPPING` | warning | ≥ 3 event BOOT dalam 24 jam |
| `ACTIVE_FAULTS` | warning | ada fault aktif |
| `OLT_LOS`, `OLT_DYING_GASP` | critical | OLT melaporkan LOS / dying gasp untuk ONU ini (hanya bila ACS juga tidak menerima Inform) |
| `OLT_ONU_OFFLINE`, `OLT_ONLINE_ACS_SILENT` | warning | OLT bilang offline / **online tetapi ACS senyap atau belum pernah Inform** (lapisan optik dan daya sehat; periksa jalur IP/VLAN ke ACS dan setelan TR-069 ONU) |
| `OLT_STATE_CONFLICT`, `OLT_STATE_UNKNOWN`, `OLT_DATA_STALE`, `OLT_NOT_FOUND` | info | OLT dan ACS tak sepaham (perangkat terjangkau), keadaan tak terpetakan, data OLT > 30 menit, serial tak ada di data OLT |
| `OPTICAL_BOTH_WEAK`, `OPTICAL_UPSTREAM_WEAK`, `OPTICAL_DOWNSTREAM_WEAK` | critical bila salah satu sisi kritis, selain itu warning | **RX dari dua arah**: RX sisi OLT (upstream) dan RX ONT (downstream) sama-sama lemah = dugaan jalur fiber bersama; hanya upstream = dugaan pemancar/konektor sisi ONU; hanya downstream = dugaan penerima ONT atau path RX salah petakan. Hanya bila ONU **online di OLT**, polling OLT dan pembacaan ONT berselisih ≤ 2 jam |
| `OPTICAL_PAIR_OK` (hanya perangkat offline), `OPTICAL_PAIR_NOT_COMPARABLE` (hanya perangkat terjangkau) | info | kedua arah normal (membebaskan jalur optik) / kedua pembacaan berjauhan waktunya sehingga tidak dibandingkan |
| `SILENCE_EXCEEDS_INTERVAL`, `INTERVAL_UNKNOWN`, `OPTICAL_OK`, `OPTICAL_NO_DATA`, `PENDING_TASKS`, `CR_*`, `EVIDENCE_INCOMPLETE`, `WITHIN_TOLERANCE` | info | gejala/keterbatasan/konteks |

`summary` hanya menyebut **penyebab berbukti** (critical/warning) sebagai "Dugaan utama"; gejala dan keterbatasan (info) tidak. Tanpa penyebab berbukti, ringkasannya berkata demikian dan menyebut
batas data OLT. Sumber bukti yang gagal dibaca dinyatakan (`EVIDENCE_INCOMPLETE`), bukan diam-diam dianggap "tidak ada masalah". Aksi yang disarankan memakai endpoint yang sudah ada
(Connection Request, baca interval, buka tab optik); hak aksesnya tetap ditegakkan endpoint itu.

## Pengujian (dan batasnya)

Unit: aturan diagnosis (tabel + kasus batas, mutasi diuji), `OfflineThreshold`, siaran offline, scope tenant peta, penangkapan/penemuan interval, parser `bbox`, komponen UI. Integrasi MariaDB nyata:
penyapu (12 kasus), query peta (bbox, lintas bujur 180, pemotongan), migrasi 0046 up/down/up. e2e S38 (38/38 lulus). Browser nyata (Chrome headless): peta tergambar, klaster merah, kartu diagnosis,
dan **alur realtime utuh** (frame `DEVICE_OFFLINE` tiba lewat WebSocket → peta menyegarkan diri tanpa reload).

**Belum terbukti:** perilaku pada **ONT/armada nyata** (hanya simulator dan data sintetis), ubin peta di bawah beban, ribuan titik di browser, dan diagnosis terhadap gangguan fisik sungguhan.

## Irisan B1 — data sisi OLT dengan simulator (2026-10-29)

Inventaris ONU terdaftar per OLT (`olt_registered_onus`, migrasi 0047), tautan ONU ↔ perangkat ACS lewat serial + tenant, ringkasan per port PON, posisi perangkat di
jaringan OLT (`GET /devices/{id}/olt`), tab "ONU terdaftar" dan "Port PON" di halaman OLT, kartu "Posisi di jaringan" di detail perangkat, dan bukti OLT di diagnosis (tabel
aturan di atas). Rancangan, kewajiban driver, dan batasnya: `OLT_CONTROLLER_DESIGN.md` §10.

**Perubahan perilaku diagnosis yang penting:** dengan bukti OLT, LOS/dying gasp menjadi **kesimpulan fisik** (bukan lagi hipotesis tersirat), dan "online di OLT tetapi senyap di ACS"
memindahkan kecurigaan dari fiber/daya ke jalur IP/TR-069. `limits` berubah menjadi: bukti ACS + data OLT dari polling terakhir. Perangkat yang masih terjangkau ACS tidak pernah
mendapat temuan kritis dari OLT (ACS baru saja menerima Inform, jadi "LOS" pasti basi); selisihnya dinyatakan `OLT_STATE_CONFLICT` berkeparahan info.

**Bug yang ditemukan e2e:** untuk perangkat yang belum pernah Inform, ringkasan berhenti di "belum pernah terhubung" walau OLT sudah melaporkan LOS; kini penyebab berbukti selain fakta itu
yang dijadikan dugaan utama (tes regresi ada).

**Belum terbukti:** semua ini hanya dengan **simulator**. Driver OLT nyata (ZTE), format serial nyata, teks keadaan nyata, dan perilaku polling OLT nyata belum ada. Status administratif port,
RX sisi OLT dari perangkat nyata, dan seluruh aksi **tulis** (authorize ONU, kontrol port) tidak dibuat — lihat butir §8 di `OLT_CONTROLLER_DESIGN.md` untuk bahan yang dibutuhkan.

## Pelengkap tanpa menunggu pihak lain (2026-10-29)

Semua butir di bawah dibangun dan diuji **tanpa OLT/ONT nyata** (unit, MariaDB nyata, e2e 39/39, tes UI); bukti lapangan tetap belum ada.

**Lokasi dari peta.** `GET /devices?has_location=true|false` (koordinat *lengkap* = lintang dan bujur sama-sama terisi; 0,0 sah). Tampilan Peta menampilkan panel "Belum punya lokasi" (sesuai filter aktif,
10 per halaman) dengan tombol "Tetapkan lokasi" untuk ADMIN/NOC, juga di keadaan kosong; kartu Lokasi di detail perangkat punya "Pilih di peta". Pemilih: klik peta, seret penanda, atau **tempel koordinat**
(`-6.2088, 106.8456`; juga titik koma, minus Unicode, koma desimal bergaya Indonesia bila tak ambigu; yang ambigu ditolak, bukan ditebak). Sengaja **tanpa** pencarian alamat pihak ketiga (privasi, kebijakan
pemakaian). Disimpan lewat `PATCH /devices/{id}` yang sudah ada. Untuk ratusan perangkat, impor massal (kolom latitude/longitude) tetap jalur yang tepat.

**Pemberitahuan offline dan LOS (migrasi 0048, hanya data ref).** Webhook baru `DEVICE_OFFLINE` (satu per perangkat yang ditandai offline penyapu; memuat `device_uuid`, serial, `last_inform_at`, `detected_at`)
dan `ONU_STATE_CHANGED` (satu per polling OLT yang menemukan perubahan; jumlah total + maks 50 rincian + `truncated`). Notifikasi in-app `DEVICE_OFFLINE` dan `ONU_STATE_ALERT` **diringkas** (satu per tenant per
sapuan, satu per polling OLT), bukan satu per perangkat/ONU, supaya pemadaman listrik massal tidak membanjiri ADMIN/NOC. Email mati secara bawaan (pengguna dapat menyalakannya). `ONU_STATE_ALERT` hanya untuk ONU
yang *baru masuk* LOS atau dying gasp; keparahan CRITICAL bila >= 5 ONU LOS pada satu polling (dugaan kabel feeder/distribusi), selain itu WARNING; dying gasp massal tetap WARNING (listrik padam). Kegagalan webhook/notifikasi
tidak menggagalkan sapuan maupun polling. Perangkat tanpa tenant tidak diberitahukan. **Belum teruji:** pengiriman webhook ke endpoint BSS nyata (worker yang ada dipakai apa adanya) dan volume pada armada besar.

**Wildcard instance pada aturan otomasi.** Lihat `OTOMASI_DESIGN.md` ("Wildcard instance pada `set_parameter`"). Instance dikenali dari parameter yang sudah dilaporkan perangkat (bukan RPC tambahan), maks 8 instance
per aksi, dilewati seluruhnya bila lebih; kondisi tidak mendukung wildcard.

**Portal pelanggan "Internet saya bermasalah?".** `GET /self-service/devices/{id}/diagnosis`. Hasil dibangun **dari nol dari daftar putih** (kategori `CONNECTED`, `NOT_YET_CONNECTED`, `POWER`, `FIBER`,
`NO_RECENT_CONTACT` -> teks awam tetap), bukan dengan menyaring diagnosis staf: tak ada string, angka, nama OLT/port, serial, ambang, atau temuan staf yang diteruskan; hanya *verdict* dan *kode* temuan dibaca. Bentuk
respons tertutup (tepat 7 field) dan dijaga tes bocoran (sentinel di setiap field diagnosis staf + mutasi diuji) serta e2e. Kepemilikan akun-perangkat dicek sebelum diagnosis dijalankan. Selalu disertai
`disclaimer`: ini perkiraan, bukan kepastian. Pemetaan kode-ke-penyebab bersifat heuristik dan belum divalidasi terhadap gangguan sungguhan.

**NBI tahap 2.** `POST /nbi/devices/{device_uuid}/tasks` (`setParameterValues`, `getParameterValues`, `reboot`), `POST /nbi/tasks/{task_uuid}/retry`, `DELETE /nbi/tasks/{task_uuid}` (= membatalkan). Penerjemah tipis ke
`task.Service` (validasi, kuota, scope tenant, penyegelan rahasia tetap di sana; RBAC = `POST /tasks`). Beda yang disengaja dari GenieACS: ACS **tidak menunggu** perangkat, respons selalu `202` berstatus antre
(`?timeout` tanpa efek, `?connection_request` memicu Connection Request best-effort); `factoryReset`/`download`/`refreshObject` belum didukung (400 yang menyebut yang didukung); nilai parameter rahasia pada dokumen
respons selalu disamarkan; UUID tugas/perangkat tenant lain memberi 404 (tak dapat dibedakan dari yang tak ada). Belum ada `GET /nbi/tasks`. Diuji unit + e2e (S37 NBI-06); **belum** diuji dengan klien GenieACS sungguhan.

**Perbaikan tampilan ONU terdaftar (2026-10-29).** (1) "Keadaan sejak" tidak lagi menyesatkan: baris yang baru ditemukan memakai waktu penemuan sebagai waktu perubahan, jadi kini tampil
"terlihat sejak ..." (waktu mulai sebenarnya tidak diketahui) dan "sejak ..." hanya untuk perubahan yang benar-benar teramati ACS. (2) `GET /olts/{uuid}/registered-onus?sort=severity` (LOS, dying gasp, offline,
tak diketahui, online; lalu port) dipakai Console sebagai urutan bawaan; tanpa `sort` urutan lama (port, nomor ONU, serial) tetap. (3) Ringkasan menyebut jumlah offline. (4) Filter `problem=true` =
OFFLINE/LOS/DYING_GASP (UNKNOWN tidak dihitung masalah) dan chip "Bermasalah saja". (5) Dugaan per port PON: >= 3 ONU **dan** >= 50% ONU port itu LOS => "kemungkinan feeder/distribusi/splitter"; dying gasp
dengan ambang sama => "kemungkinan listrik padam di area". Ini **heuristik konservatif, bukan kepastian** (ambang belum dikalibrasi dengan data OLT nyata) dan selalu diberi catatan itu di banner. Dihitung di
frontend dari ringkasan port (`lib/oltInventory.ts`); belum ada versi sisi server, jadi notifikasi tidak memakainya. (6) Notifikasi `ONU_STATE_ALERT` kini menautkan ke
`/olts?olt=<uuid>&problem=1`: halaman OLT membuka OLT itu dengan filter bermasalah menyala; OLT yang tak dikenal pada tautan jatuh ke OLT pertama. Riwayat per ONU: lihat di bawah.

**Riwayat perubahan keadaan per ONU (migrasi 0049).** Tabel log `olt_onu_state_events` (audit minimal, dipangkas fisik oleh retensi log `ACS_LOG_RETENTION_DAYS`, yang sekaligus menjadi batas panjang riwayat) mencatat tiap perubahan yang
**teramati polling**: ONU baru terlihat (`from_state` kosong), berganti keadaan, atau tidak lagi dilaporkan OLT (`to_state` kosong; muncul lagi = peristiwa "baru" lagi). Kunci = (OLT, serial), bukan id baris ONU (baris
dihapus-lunak dan dihidupkan lagi). `GET /olts/{uuid}/registered-onus/{onu}/history` mengembalikan riwayat terbaru dulu, `changes_24h`, dan `flapping` (>= 4 perubahan sejati dalam 1 jam; heuristik, belum dikalibrasi dengan OLT
nyata). Di Console: tombol "Riwayat" pada tiap baris ONU membuka garis waktu. **Batas yang harus dibaca:** waktu = saat polling mengamati (bukan saat terjadi); gangguan yang pulih di antara dua polling tidak terlihat, jadi selang polling
panjang menyembunyikan flapping cepat; kegagalan menyimpan riwayat tidak menggagalkan polling (hanya tercatat di log, riwayat bolong). Baris pertama OLT baru memuat satu peristiwa "baru" per ONU (OLT besar = puluhan ribu baris sekali saja).

**Perbandingan RX OLT dan ONT (2026-10-29).** Kartu "Posisi di jaringan" menampilkan RX sisi OLT (**upstream**: cahaya dari ONU diterima OLT) dan RX sisi ONT (**downstream**: cahaya dari OLT diterima ONT), masing-masing dengan tingkat (normal/lemah/kritis) dan usia
pembacaan. Keduanya **bukan besaran yang sama** (panjang gelombang dan daya pemancar berbeda), jadi sengaja **tidak dikurangkan** menjadi "redaman"; yang dibandingkan hanya tingkatnya terhadap ambang yang sama. Diagnosis memakai pola dua arah (tabel di atas).
**Batas yang harus dibaca:** (1) ambang sisi OLT memakai angka yang sama dengan sisi ONT (-25/-27/-8 dBm); sensitivitas penerima OLT tertentu bisa berbeda, dan belum dikalibrasi dengan OLT nyata; (2) RX sisi OLT hanya bermakna bila ONU online; (3) kedua pembacaan harus berdekatan waktunya (<= 2 jam),
kalau tidak tidak dibandingkan; (4) pola penyebabnya heuristik fisika sederhana (mis. tikungan makro memengaruhi panjang gelombang berbeda secara berbeda), bukan kepastian; (5) hanya diuji unit (tabel kombinasi + mutasi) dan tes komponen: **belum ada e2e** karena simulator
tidak membawa pasangan RX ONT dan OLT yang tertaut, dan belum pernah dicoba pada OLT/ONT nyata. Catatan terkait: teks diagnosis "terpantau sejak ..." pada temuan OLT masih memakai waktu perubahan yang sama dengan kolom "Keadaan sejak" lama (menyesatkan untuk ONU yang baru ditemukan); belum diperbaiki di sisi diagnosis.
