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

