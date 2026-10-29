# Kit Uji Lapangan — OLT ZTE, STUN/TR-111, Connection Request, dan fitur 29 Okt

> Dibuat 2026-10-29. Tujuannya satu: **menghasilkan bukti nyata** yang membuka empat pekerjaan yang sekarang terblokir —
> driver OLT ZTE nyata, Auto Config lewat OLT, keputusan STUN/CGNAT, dan kepastian fitur baru di ONT sungguhan. Saya (asisten pengembang) tidak punya akses ke OLT/ONT,
> jadi dokumen ini dan dua skrip di `scripts/` adalah cara agar **Anda atau teknisi** mengumpulkannya tanpa saya menebak.

## 0. Batas yang harus dibaca

- **Tidak ada perintah vendor di sini.** Sintaks CLI, OID, dan format keluaran OLT tidak boleh ditulis dari ingatan (aturan proyek, `OLT_CONTROLLER_DESIGN.md` §2). Anda menjalankan perintah yang
  **sudah Anda kenal** untuk OLT itu; dokumen ini hanya menyebut *apa yang harus tampil*, bukan *bagaimana memintanya*.
- **Hanya-baca di OLT.** Gunakan akun read-only. Jangan menjalankan perintah yang mengubah konfigurasi, mengautorisasi ONU, atau mengubah port.
- **Skrip `field-kit` bawaannya hanya-baca terhadap ACS.** Tiga aksi (`--queue-discovery`, `--queue-values`, `--try-cr`) harus diminta eksplisit; semuanya hanya membaca/membangunkan perangkat, tidak mengubah konfigurasi.
- Ikuti **Batas aman** dan **Pembersihan** di `PANDUAN_UJI_ONT_FITUR_BARU.md` (§0 dan §6): tenant uji saja, jangan ubah setelan TR-069 ONT dari ACS, catat nilai asli sebelum mulai, dan pulihkan state lab di akhir.
- Hasil yang dikirim kembali **tidak boleh memuat kredensial**. Lihat §5 (penyamaran).

## 1. Apa yang dibuka oleh bukti ini

| Bukti | Membuka | Tanpa bukti ini |
|---|---|---|
| Bahan OLT (§2, butir 1–9 desain OLT) | Driver ZTE baca nyata (daftar ONU, keadaan, RX), lalu pemetaan keadaan dan normalisasi serial | Hanya simulator |
| Format serial OLT vs TR-069 (butir 8) | Tautan ONU ↔ perangkat yang benar; normalisasi per driver | Tautan hanya jalan bila serial persis sama |
| Langkah 0 STUN (§3) | Keputusan: opsi A (Inform pendek), B (server STUN), atau E (USP) | Membangun B tanpa tahu ONT mendukung = kode tak terbukti |
| Bukti Connection Request (§4) | Kepastian ACS dapat membangunkan ONT nyata (jalur tercepat untuk aksi interaktif) | CR belum pernah terbukti di ONT fisik |
| Uji fitur 29 Okt di ONT (§6) | Kepastian lokasi/notifikasi/wildcard/portal/NBI di perangkat sungguhan | Hanya teruji di simulator |

**Auto Config lewat OLT (menulis ke OLT) tidak termasuk kit ini.** Itu baru boleh dirancang setelah baca nyata terbukti (`OLT_CONTROLLER_DESIGN.md` §9 butir 4: lab saja, dengan dry-run, persetujuan eksplisit, dan rollback).

## 2. Bagian A — Bahan OLT ZTE

Kumpulkan **keluaran mentah** (bukan ringkasan) ke berkas teks, satu berkas per butir. Sertakan perintah yang Anda jalankan di baris pertama berkas supaya konteksnya jelas.

| Berkas | Isi yang diminta | Catatan |
|---|---|---|
| `olt-01-model.txt` | Model dan versi perangkat lunak OLT | Mis. keluaran perintah versi/sistem milik OLT Anda |
| `olt-02-unreg-ada.txt` | Daftar ONU **belum terdaftar**, minimal **satu** ONU | Sampel dengan data |
| `olt-02-unreg-kosong.txt` | Perintah yang sama saat **tidak ada** ONU belum terdaftar | Sampel kosong: penting agar parser tidak salah membaca "kosong" |
| `olt-03-sistem.txt` | Keluaran informasi sistem/versi untuk `Probe` | Boleh sama dengan butir 1 bila isinya sama |
| `olt-04-snmp.txt` | Bila SNMP dipakai: community **read-only** dan hasil `snmpwalk` pada cabang yang memuat ONU belum terdaftar | Untuk sekadar memastikan koneksi SNMP: `snmpwalk` pada grup `system` MIB-II (`1.3.6.1.2.1.1`, standar). Cabang ONU = OID dari dokumentasi MIB vendor |
| `olt-05-sesi.txt` | Perilaku sesi SSH/Telnet: bentuk **prompt**, ada tidaknya **paginasi** (mis. `--More--`), **batas waktu** menganggur, **jumlah sesi bersamaan** yang diizinkan | Tulis tangan; bukan keluaran perintah |
| `olt-06-terdaftar.txt` | Daftar ONU **terdaftar** dengan minimal satu ONU **normal**, satu **LOS**, satu **dying gasp**, satu **offline** | Dari sini dibaca port PON, nomor ONU, serial, dan **teks keadaan asli** |
| `olt-07-keadaan.txt` | **Daftar lengkap nilai keadaan** yang dikenal OLT itu beserta artinya (dokumentasi vendor atau keluaran bantuan) | Termasuk keadaan yang tak kita duga: yang tak terpetakan tetap `UNKNOWN`, bukan ditebak |
| `olt-08-serial.txt` | Serial **ONU yang sama** seperti ditampilkan OLT dan seperti dilaporkan ONT lewat TR-069 (minimal 3 ONU) | Contoh: awalan vendor, heksa vs teks. Skrip §3 membantu membandingkannya |
| `olt-09-opsional.txt` | (Opsional) perintah daya terima sisi OLT per ONU dan status administratif port PON | Hanya bila ada keluaran nyata |

Lalu samarkan (§5) sebelum dibagikan.

## 3. Bagian B — Langkah 0: apakah ONT mendukung STUN/TR-111?

Ini pertanyaan yang menentukan apakah server STUN layak dibangun (`CONNECTION_REQUEST_NAT_DESIGN.md` §4). Dua tahap karena ONT hanya bisa ditanya saat Inform:

```bash
export ACS_KIT_API=https://<acs>/api/v1  ACS_KIT_USER=<email-atau-username>  ACS_KIT_PASS=<password>   # atau ACS_KIT_TOKEN=<JWT>
# Tahap 1: antre penelusuran nama parameter ManagementServer.* (satu RPC baca), lalu tunggu ONT Inform (atau picu Connection Request)
node scripts/field-kit.mjs --serial <SN-ONT> --queue-discovery --out tahap1.md
# Tahap 2 (setelah Inform): baca laporan; bila ada parameter STUN, antre pembacaan NILAI-nya
node scripts/field-kit.mjs --serial <SN-ONT> --queue-values --out tahap2.md
# Tahap 3 (setelah Inform berikutnya): laporan akhir
node scripts/field-kit.mjs --serial <SN-ONT> --out tahap3.md
```

Ulangi untuk **tiap model ONT** (ZTE F660, ZTE F609, Optictimes, dan lainnya di armada). Cara membaca hasil:

| Hasil | Arti | Keputusan |
|---|---|---|
| `DIDUKUNG` + `STUNEnable` dapat ditulis + `UDPConnectionRequestAddress` terisi bila dinyalakan | ONT model ini mampu STUN | B layak dipertimbangkan, **dan** butuh CPE di balik NAT nyata untuk membuktikannya (desain §6 butir 3) |
| `DIDUKUNG` tetapi tak pernah mengisi `UDPConnectionRequestAddress` | Parameter ada, fiturnya mungkin tidak berfungsi | Jangan bangun B untuk model ini |
| `TAMPAK_TIDAK_DIDUKUNG` | Daftar nama `ManagementServer.*` cukup lengkap dan tak ada STUN/NAT | A (Inform pendek lewat aturan otomasi) atau E (USP) |
| `BELUM_DIKETAHUI` | Penelusuran belum berjalan atau ONT belum Inform | Ulangi tahap 1 dan tunggu Inform |

**Batas hasil:** vonis `TAMPAK_TIDAK_DIDUKUNG` memakai ambang heuristik (≥ 12 nama `ManagementServer.*` dikenal ACS tanpa satu pun STUN/NAT). Daftar nama mentah selalu ikut laporan: **baca sendiri** untuk memastikan daftarnya memang lengkap.
Skrip tidak mengubah setelan STUN pada ONT.

Sekalian ukur skala masalahnya: berapa perangkat produksi yang CR HTTP-nya tak terjangkau (hasil `TRIGGER_CONNECTION_REQUEST` gagal vs berhasil di Audit Trail). Tanpa angka ini keputusan A/B/E hanya tebakan.

## 4. Bagian C — Bukti Connection Request nyata

Prasyarat: Connection Request URL dan **kredensial CR** pada perangkat sudah terisi (laporan menampilkan "kredensial CR terisi/BELUM terisi"). Kredensial CR diisi lewat Edit perangkat di Console; **jangan**
mengubah setelan TR-069 ONT.

```bash
node scripts/field-kit.mjs --serial <SN-ONT> --try-cr --wait 90 --out cr.md
```

| Hasil | Arti |
|---|---|
| `TERBUKTI` | ONT melakukan Inform dengan event `6 CONNECTION REQUEST` setelah CR dikirim (latensi tercantum). CR HTTP nyata berfungsi untuk ONT/jaringan ini |
| `TIDAK_TERBUKTI` | Tidak ada event CR dalam jendela tunggu. **Belum berarti ONT tak mendukung**: bisa ONT tak terjangkau dari ACS, kredensial salah, atau jendela terlalu pendek. Coba `--wait 180` dan periksa Capture sesi |
| `GAGAL_DIKIRIM` | ACS menolak mengirim (mis. perangkat tak punya CR URL). Pesan galat dicantumkan |

## 5. Bagian D — Fitur 29 Okt pada ONT sungguhan

Ikuti batas aman di §0. Lembar hasil: salin tabel ini ke catatan uji.

| # | Uji | Cara | Lulus bila |
|---|---|---|---|
| 1 | Lokasi dari peta | Detail perangkat → Lokasi → Pilih di peta; tempel koordinat uji | Titik muncul di tab Peta di posisi benar; "belum punya lokasi" berkurang |
| 2 | Notifikasi offline | **Pada ONT yang sedang tak dipakai**, cabut dayanya; tunggu melewati ambang offline (≥ 15 menit atau 2,5× interval Inform) | Notifikasi "Perangkat offline" muncul (ringkasan), webhook `DEVICE_OFFLINE` terkirim bila dilanggan; diagnosis menyebut sebabnya |
| 3 | Wildcard otomasi | Aturan `set_parameter` ke `...WLANConfiguration.*.SSID` pada **satu ONT kelinci**: **Uji (dry-run) dulu**, lihat instance yang diekspansi, baru aktifkan | Hanya instance yang ada diubah; SSID asli dipulihkan di akhir |
| 4 | Portal pelanggan | Tautkan akun ENDUSER ke ONT uji; buka "Internet saya bermasalah?" saat ONT online, lalu saat offline | Teks awam sesuai keadaan; tidak ada angka RX/serial/nama OLT |
| 5 | NBI tugas | `POST /nbi/devices/{uuid}/tasks` dengan `getParameterValues` pada satu parameter aman | Respons 202 berstatus `PENDING`; tugas selesai pada Inform berikutnya |
| 6 | RX dua arah | Hanya bila OLT sungguhan sudah terhubung dan ONU tertaut | Kartu "Posisi di jaringan" menampilkan RX OLT dan ONT; diagnosis memakai pola dua arah |

Setiap uji yang gagal: aktifkan **Capture sesi** (≤ 60 menit) dan simpan; itu bukti terbaik.

## 6. Penyamaran sebelum dibagikan (WAJIB)

Keluaran OLT mentah sering memuat community, password, alamat manajemen, dan MAC:

```bash
node scripts/mask-olt-output.mjs < olt-02-unreg-ada.txt > olt-02-unreg-ada.aman.txt
node scripts/mask-olt-output.mjs --mask-serials < olt-06-terdaftar.txt > olt-06.aman.txt   # hanya bila serial tidak boleh keluar
```

Disamarkan: nilai setelah `password/passwd/pwd/secret/community/key/token`, dua oktet terakhir IPv4, dan MAC. **Serial ONU tidak disamarkan secara bawaan** karena format aslinya dibutuhkan (butir 8). Pola ini umum, bukan khusus vendor:
**baca hasilnya** sebelum dibagikan; rahasia berbentuk tak lazim tidak dikenali. Laporan `field-kit` menyamarkan nilai bernama kredensial dan serial (kecuali `--show-serials`); sama, baca dulu.

## 7. Apa yang dikirim kembali

1. Berkas `olt-0X.aman.txt` (§2) dan, bila ada, `olt-serial.txt` untuk `--olt-serials` (satu serial OLT per baris).
2. Laporan `tahap3.md` (STUN) dan `cr.md` (CR) per **model** ONT.
3. Hasil lembar §5 (lulus/gagal, dan Capture bila gagal).
4. Jalankan sekali dengan data OLT untuk memeriksa format serial:
   ```bash
   node scripts/field-kit.mjs --all --limit 200 --olt-serials olt-serial.txt --out serial.md
   ```
   Bagian "OLT: format serial" melaporkan berapa serial cocok menurut aturan tautan sistem, dan **kandidat** yang tak tertaut tetapi tampak sama setelah transformasi (awalan/akhiran tambahan, pemisah, heksa↔teks). Itu petunjuk normalisasi, bukan kepastian.

## 8. Pengujian skrip (dan batasnya)

- `node --test scripts/field-kit.test.mjs` — 13 tes pada fungsi murni (vonis STUN dengan batas ambang, perbandingan serial, vonis CR, penyamaran). Mutasi diuji.
- `scripts/field-kit.mjs` dicoba terhadap stack e2e sungguhan: login, laporan hanya-baca, antre penelusuran nama, `--try-cr` pada perangkat tanpa CR URL (galat ditampilkan jujur), dan perbandingan serial dari berkas. Bug yang ditemukan lewat percobaan itu (nama field token login) sudah diperbaiki.
- **Belum teruji:** terhadap ONT/OLT nyata. Vonis STUN dan CR adalah logika, bukan bukti; bukti sebenarnya adalah hasil yang Anda kumpulkan.
