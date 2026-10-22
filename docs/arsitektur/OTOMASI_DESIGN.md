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

Aturan yang buruk tidak boleh membanjiri perangkat. Empat pagar, semuanya diuji (`service_test.go`, e2e S34):

1. **Cooldown** per (aturan, perangkat), min 300 detik.
2. **Suspend**: setelah `max_consecutive_fires` pemicuan berturut-turut tanpa kondisi pernah salah/konvergen di antaranya, aturan berhenti untuk
   perangkat itu dan tercatat (`AUTOMATION_SUSPENDED`); operator membukanya lewat `POST /automation-rules/{uuid}/reset`. Contoh yang ditangkap:
   "jika versi ≠ X maka reboot" yang tak pernah konvergen.
3. **Task tertunda**: perangkat yang masih punya task pending/queued tidak dipicu (pola yang sama dengan ZTP/preset).
4. **Konvergen = diam**: `set_parameter` yang nilainya sudah sama dan `add_tag` yang sudah terpasang tidak memicu apa pun dan menolkan hitungan.

## Keamanan

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
