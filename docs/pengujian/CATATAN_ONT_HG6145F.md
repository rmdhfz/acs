# Catatan uji lapangan: ONT FiberHome HG6145F (2026-10-18)

Hasil pemeriksaan **read-only** pada ONT milik pengguna, untuk menyiapkan UAT perangkat fisik. Tidak ada pengaturan ONT yang diubah
(semua permintaan selain GET diblokir oleh alat pemeriksa; tercatat 0 percobaan tulis). Kredensial web dan kunci WiFi **sengaja tidak
dicatat** di dokumen ini.

## Identitas yang terbaca (halaman web ONT, akun tingkat pengguna)
| Item | Nilai |
|---|---|
| Merek / model | FiberHome / HG6145F (GPON) |
| Firmware / hardware | RP2951 / WKE2.094.373A01 |
| Serial | FHTT99B203B0 |
| MAC / OUI | 58:AE:F1:B2:03:B0 → OUI **58AEF1** (ada di katalog `vendor_ouis` → FIBERHOME, seed IEEE) |
| Status ONU | O5 (Operation), registrasi OK |
| Web | `http://192.168.1.1`, port 80 |

## Pembacaan optik (sehat) — pembanding bila ACS kelak membaca nilai yang sama
TX **+2,74 dBm**, RX **−18,21 dBm**, suhu 44,4 °C, tegangan 3,17 V, bias 8,43 mA.

## Batasan yang ditemukan
Akun tingkat pengguna hanya punya: Status (info perangkat, WiFi, WAN, LAN, optik, VoIP, USB), Network → pengaturan WLAN, dan Management →
ganti password akun + reboot. **Tidak ada menu TR-069 / Remote Management**, jadi URL ACS, username, dan password TR-069 tidak dapat diubah
dengan akun ini. Mengarahkan ONT ke ACS ini butuh akun admin/operator yang sah (dari ISP/teknisi atau dokumentasi perangkat milik sendiri).

## Yang SUDAH terbukti tanpa mengubah ONT (e2e S27)
Simulator memakai identitas nyata (`simcpe.Profiles["hg6145f"]`): OUI 58AEF1 dikenali katalog **tanpa menyuntik OUI palsu**, vendor ter-resolve
ke FIBERHOME, firmware RP2951 tersimpan, logical key `wifi.ssid` ter-resolve ke path TR-098 lewat mapping vendor dan diterima CPE tanpa fault.

## Yang BELUM teramati (butuh akses TR-069 ke ONT fisik)
- Namespace CWMP yang sebenarnya dipakai firmware RP2951 (simulator memakai `cwmp-1-0` sebagai asumsi).
- Pohon parameter asli (`GetParameterNames`) dan **path optik TR-098 FiberHome** — saat ini tidak dipetakan; jangan menebak.
- Perilaku Connection Request (HTTP/UDP), nilai `PeriodicInformInterval`, Inform setelah reboot.

## Prosedur bila akun admin tersedia (tahap baca-saja dulu)
1. Catat/foto pengaturan TR-069 yang ada sekarang (URL ACS, username, interval) — dasar untuk mengembalikan.
2. Buka firewall Windows untuk TCP 17547 di jaringan rumah (profil jaringan saat ini "Public"), dan pastikan laptop ber-IP tetap.
3. Buat tenant uji + kredensial Inform di ACS; isi URL `http://<IP-laptop>:17547/cwmp` + kredensial itu pada ONT.
4. Amati Inform pertama (event `0 BOOTSTRAP`/`1 BOOT`), lalu `GetParameterNames` (refresh pohon) dan `GetParameterValues` saja.
   Jangan memakai Set/Reboot/FactoryReset/Download pada ONT yang melayani internet rumah.
5. Dari pohon yang terbaca, tambahkan mapping optik FiberHome sebagai **data** (`vendor_parameter_mappings`), lalu bandingkan dengan nilai di atas.
6. Kembalikan URL/kredensial TR-069 semula dan pastikan internet normal. Siapkan hotspot HP sebagai cadangan selama uji.
