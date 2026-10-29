// Menyamarkan keluaran OLT (CLI/SNMP) sebelum ditempel ke laporan atau dibagikan. Node 20+, tanpa dependensi.
//
//   node scripts/mask-olt-output.mjs < keluaran-mentah.txt > keluaran-aman.txt
//   node scripts/mask-olt-output.mjs --mask-serials < mentah.txt > aman.txt     (serial GPON 4 huruf + 8 heksa ikut disamarkan)
//
// Yang disamarkan: nilai setelah kata password/passwd/pwd/secret/community/key/token, alamat IPv4 (dua oktet terakhir), dan MAC address.
// TIDAK disamarkan secara bawaan: serial ONU (butir 8 desain OLT justru butuh format aslinya). Ini pola umum, bukan khusus vendor: BACA hasilnya
// sebelum dibagikan; rahasia dengan bentuk tak lazim (mis. kunci SNMPv3 tanpa kata kunci di depannya) tidak dikenali.
import { readFileSync } from 'node:fs'
import { maskOltOutput } from './field-kit-lib.mjs'

const maskSerials = process.argv.includes('--mask-serials')
const input = readFileSync(0, 'utf8')
process.stdout.write(maskOltOutput(input, { maskSerials }))
