# Aturan Otomasi Kondisi–Aksi — Catatan Desain

> Status: **diimplementasikan 2026-10-22** (migrasi `0040`, `internal/usecase/automation`, `pkg/vexpr`, UI `/automation`, e2e S34).
> Menjawab gap "provision berskrip" vs GenieACS. Keputusan: **engine deklaratif, bukan skrip** (opsi 1 dari tiga opsi yang diajukan).

## Mengapa bukan skrip

GenieACS menjalankan JavaScript (provision) di prosesnya. Untuk ACS SaaS multi-tenant itu berarti kode tenant berjalan di proses yang
sama dengan data tenant lain: loop tak hingga, panggilan ke URL internal (SSRF), dan kebocoran antar-tenant harus dicegah oleh sandbox
yang sempurna. Engine deklaratif menutup seluruh kelas risiko itu dengan konstruksi, dan tetap menutup kasus yang dipakai operator ISP:
"jika X maka atur Y", "tandai perangkat berredaman buruk", "seragamkan SSID saat pertama terhubung".

Yang **tidak** bisa diekspresikan (disengaja): perulangan, variabel antar-langkah, panggilan HTTP bebas dari dalam aturan, dan membaca
data tenant lain. Bila kebutuhan nyata melampaui ini, keputusan berikutnya adalah JavaScript tersandbox (QuickJS/goja) — belum diambil.

## Bentuk aturan

`jika <kondisi> maka <aksi...>`, dievaluasi pada Inform sesuai `trigger_event` (tabel `ref_ztp_trigger_event` yang sama dengan ZTP:
`BOOTSTRAP_ONLY`, `BOOTSTRAP_OR_BOOT`, `EVERY_INFORM`). Aturan **selalu milik satu tenant** (tidak ada aturan global lintas tenant — aksinya
mengubah perangkat).

- **Kondisi** = ekspresi `pkg/vexpr` (kosong = selalu benar) atas parameter `{Nama.Parameter}` dan variabel konteks `{@serial_number}`,
  `{@oui}`, `{@product_class}`, `{@software_version}`, `{@hardware_version}`, `{@ip_address}`, `{@events}`. Bahasa ekspresi sama dengan virtual parameter.
- **Aksi** (daftar tertutup, maks 10): `set_parameter` (nama + *ekspresi* nilai), `reboot`, `get_parameter_values`, `add_tag`, `webhook`
  (event `AUTOMATION_TRIGGERED`). Menambah jenis aksi = perubahan kode yang ditinjau, bukan data.

## Pagar anti-badai

Aturan yang buruk tidak boleh membanjiri perangkat. Lima pagar, semuanya diuji (`service_test.go`, e2e S34); pagar 5 dan beberapa batas lain ditambahkan setelah audit keamanan independen 22 Okt 2026:

1. **Cooldown** per (aturan, perangkat), min 300 detik.
2. **Suspend**: setelah `max_consecutive_fires` pemicuan berturut-turut tanpa kondisi pernah salah/konvergen di antaranya, aturan berhenti untuk
   perangkat itu dan tercatat (`AUTOMATION_SUSPENDED`); operator membukanya lewat `POST /automation-rules/{uuid}/reset`. Contoh yang ditangkap:
   "jika versi ≠ X maka reboot" yang tak pernah konvergen.
3. **Task tertunda**: perangkat yang masih punya task pending/queued tidak dipicu (pola yang sama dengan ZTP/preset).
4. **Konvergen = diam**: `set_parameter` yang nilainya sudah sama dan `add_tag` yang sudah terpasang tidak memicu apa pun dan menolkan hitungan.
5. **Batas keras per 24 jam** (`migrations/0042`): maksimal 12 pemicuan per (aturan, perangkat) per jendela bergulir 24 jam, **tidak bergantung pada kondisi**.
   Pagar 2 bisa dilewati kondisi yang berosilasi akibat aksinya sendiri (mis. `contains({@events},'2 PERIODIC')` + reboot: Inform berikutnya adalah `1 BOOT`,
   kondisi salah, hitungan berturut direset, lalu periodik menembak lagi — selamanya). Terlampaui = suspend.

Catatan perilaku: kegagalan karena **kuota task tenant penuh** (`ErrQuotaExceeded`) memulai cooldown tetapi tidak dihitung sebagai pemicuan berturut-turut (agar aturan yang benar
tidak ter-suspend hanya karena antrean penuh). Pagar adalah pengaman salah-konfigurasi, **bukan batas keamanan terhadap ADMIN**: menghapus lalu membuat ulang aturan
memulai state bersih. Batas keamanan sesungguhnya adalah kuota task tenant dan batas jumlah aturan/aksi di bawah.

## Keamanan

- **`ManagementServer.*` tidak boleh diatur** (URL, kredensial, interval Inform, Connection Request; ditolak saat simpan dan saat rencana, termasuk path hasil resolusi logical key):
  aturan EVERY_INFORM tanpa kondisi dapat memutus seluruh armada tenant dari ACS secara otomatis dan persisten.
- `set_parameter` **dilewati bila parameter sumber pada nilainya belum dilaporkan** — `concat('ACS-', {X})` dengan `{X}` null sebelumnya menulis `ACS-` ke perangkat.
- Total aksi `set_parameter` pada aturan aktif satu tenant dibatasi 30; target di-resolusi dan nilai saat ini dibaca **sekali per Inform** (satu query batch), resolusi di-memo.
- Mutasi aturan dan virtual parameter ditolak pada sesi **impersonasi** (`denyImpersonation`): aksinya otomatis dan persisten.
- Pesan galat tag seragam untuk "tidak ada" dan "milik tenant lain" (tidak ada oracle penyisir id).
- Daftar nama parameter rahasia (`domain.IsSensitiveParamName`) diperluas (Pwd, UserPass, AuthPasscode, WPSPin, Community, Credential, WPAKey, KeyValue); tetap berbasis **nama**,
  jadi parameter rahasia dengan nama tak lazim yang lolos tetap mungkin — daftar ini perlu diperluas bila uji lapangan menemukan nama baru.
- Parameter **rahasia** (`domain.IsSensitiveParamName`) ditolak sebagai rujukan ekspresi **dan** sebagai target `set_parameter`: ekspresi/riwayat
  terlihat oleh peran staf, dan nilai target tersimpan polos di definisi. Parameter rahasia tetap diatur lewat provisioning profile (nilai tersegel).
- Scope tenant di semua jalur; `tag_id` harus milik tenant aturan; uji (dry-run) hanya pada perangkat tenant pemanggil.
- Biaya ekspresi dibatasi di `pkg/vexpr` (1000 karakter, 200 node, kedalaman 20, hasil 1024 rune) — pasti berhenti.
- Mutasi/uji/reset: ADMIN; baca aturan & riwayat: ADMIN/NOC.

## Jalur panas (Inform)

`Evaluate` dipanggil setelah ZTP & preset. Daftar aturan aktif per tenant di-cache 5 detik (`repository/hotcache`, dibuang saat aturan diubah di instance
yang sama). **Tenant tanpa aturan = nol query tambahan.** Dengan aturan: +1 `HasPendingForDevice`, +1 pembacaan parameter yang dirujuk kondisi
(satu query batch), +1 pembacaan state; tulis hanya saat memicu/meng-suspend/mereset hitungan. Dampak pada benchmark 1.500 perangkat (tanpa aturan)
tidak terukur (dalam rentang kebisingan antar-run).

## Data

`automation_rules` (audit 7-kolom, `rule_uuid`), `automation_states` (state pagar per aturan×perangkat), `automation_runs` (riwayat; dipangkas fisik
setelah 30 hari). Dua tabel terakhir memakai audit minimal — pengecualian yang dinyatakan di header migrasi.

## Yang belum

- Aksi `apply_profile` / `download firmware` / `notify` (bisa ditambah sebagai jenis aksi baru bila diminta).
- Pemicu `VALUE_CHANGE` dan jadwal berkala (sekarang hanya mengikuti Inform; kondisi dapat memeriksa `{@events}`).
- Menguji pagar "suspend" end-to-end di e2e (cooldown minimum 300 detik membuatnya tak praktis; diuji di unit test dengan jam palsu).
- Ukuran biaya dengan banyak aturan per tenant pada skala ratusan ribu perangkat.

## Keputusan berikutnya: JavaScript tersandbox? (usulan 2026-10-29, belum disetujui, belum ada kode)

Bagian ini membuka kembali keputusan di atas ("deklaratif, bukan skrip"). **Rekomendasi: jangan bangun sekarang.** Alasannya bukan teknis semata: belum ada satu pun
kebutuhan konkret yang terbukti tak bisa diekspresikan aturan deklaratif, dan celah paling mungkin (lihat bawah) bisa ditutup jauh lebih murah dan aman tanpa skrip.

**Celah yang terverifikasi di kode (bukan dugaan):** rujukan parameter pada ekspresi/aksi adalah **nama persis** (`{Nama.Parameter}`); tidak ada wildcard atau iterasi instance
(`WLANConfiguration.*`, `WANConnectionDevice.*`), tidak ada variabel antar-langkah, aksi tertutup (`set_parameter`, `reboot`, `get_parameter_values`, `add_tag`, `webhook`).
GenieACS menutup kasus "semua instance WLAN" dengan perulangan di skrip. **Hipotesis yang harus dikonfirmasi pengguna:** itulah kebutuhan nyata, bukan skrip bebas.

**Urutan yang disarankan:**
1. Kumpulkan ≥2 kebutuhan nyata yang gagal secara deklaratif (contoh provision GenieACS yang dipakai tim NOC). Tanpa itu, skrip = fitur tanpa pengguna dengan permukaan serangan besar.
2. Bila celahnya wildcard/instance: tambahkan **rujukan berwildcard + aksi "untuk setiap instance"** ke `pkg/vexpr`/aksi (tertutup, berbatas, nol sandbox).
3. Hanya bila 1–2 tidak cukup: spike JavaScript tersandbox dengan syarat di bawah.

**Bila JavaScript jadi dibangun — syarat desain (tidak dinegosiasikan):**
- **Skrip menghitung RENCANA, tidak melakukan efek.** Fungsi murni: masukan = snapshot parameter yang dirujuk + konteks perangkat milik tenant itu sendiri; keluaran = daftar aksi
  yang sama dengan aksi aturan. Pelaksanaannya lewat **eksekutor aksi yang sudah ada**, sehingga semua pagar (larangan `ManagementServer.*`, parameter rahasia, cooldown/suspend,
  batas 24 jam, kuota task) berlaku tanpa diubah. Tanpa I/O, jaringan, berkas, timer, atau panggilan balik ke ACS.
- **VM baru per eksekusi, tanpa state bersama:** data tenant lain tidak pernah ada di memori VM — isolasi lintas-tenant by construction, bukan by sandbox sempurna.
- **Batas keras:** waktu CPU (mis. 50 ms) dengan interupsi, memori VM terbatas, ukuran skrip (mis. 16 KiB), maks aksi per eksekusi (30), kapasitas pekerja global + per-tenant.
  Melampaui batas = eksekusi dibatalkan, dicatat, dan skrip di-suspend (pola yang sama dengan aturan). **Inform tidak pernah ditahan/gagal karena skrip** (gagal-terbuka: lewati dan catat).
- **Pra-saring** dengan kondisi `vexpr` yang ada agar skrip hanya jalan saat kondisi cocok; bytecode di-cache; metrik durasi/timeouts/ditolak.
- **Mesin:** pilihan dengan jaminan memori terkuat adalah QuickJS yang dijalankan di dalam wazero (batas halaman memori + pembatalan lewat context). goja (Go murni) punya
  interupsi tetapi **tidak punya batas memori bawaan** — risiko `new Array(1e9)`. Kandidat binding QuickJS-di-wazero **wajib melewati spike** (kematangan, pemeliharaan, performa) sebelum dipilih; tidak diasumsikan.
- **Pengujian keamanan sebelum diaktifkan:** loop tak hingga, bom memori/string, ReDoS, pencemaran prototipe, upaya keluar sandbox, uji dua tenant tak saling melihat, fuzz masukan; tinjauan
  keamanan independen. Saklar `ACS_SCRIPTS_ENABLED` bawaan **MATI**; mutasi/uji hanya ADMIN, ditolak pada sesi impersonasi; dry-run seperti `/automation-rules/test`.

**Keputusan untuk pemilik produk:** (a) apakah ada kebutuhan konkret yang gagal secara deklaratif, (b) setuju mencoba wildcard/instance dulu (langkah 2), (c) bila JS tetap diinginkan, setuju spike QuickJS-di-wazero sebagai syarat.
