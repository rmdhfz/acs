// node --test scripts/field-kit.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  MS_DISCOVERED_MIN_NAMES, compareSerials, crVerdict, describeSerials, hexAsciiDecode, isSecretName, maskOltOutput, maskSerial, normSerial, redactValue, rootOf, stunVerdict, table,
} from './field-kit-lib.mjs'

const P = (name, value = null) => ({ name, value })
const MS = 'InternetGatewayDevice.ManagementServer.'

test('rootOf: TR-098, TR-181, atau belum diketahui', () => {
  assert.equal(rootOf(['InternetGatewayDevice.DeviceInfo.X']), 'InternetGatewayDevice.')
  assert.equal(rootOf(['Device.DeviceInfo.X']), 'Device.')
  assert.equal(rootOf(['Foo.Bar']), null)
  assert.equal(rootOf([]), null)
})

test('nilai bernama rahasia disamarkan, yang lain apa adanya', () => {
  for (const n of ['X.ConnectionRequestPassword', 'X.STUNPassword', 'X.KeyPassphrase', 'X.PreSharedKey', 'X.STUNUsername', 'X.SNMPCommunity', 'X.Token']) {
    assert.ok(isSecretName(n), n)
    assert.equal(redactValue(n, 'abc'), '[disembunyikan]')
  }
  assert.equal(redactValue('X.STUNServerAddress', '203.0.113.5'), '203.0.113.5')
  assert.equal(redactValue('X.Password', ''), '') // kosong tetap kosong
  assert.equal(redactValue('X.Password', null), '')
})

test('stunVerdict: parameter STUN ada = DIDUKUNG, dengan rincian nilai', () => {
  const v = stunVerdict([P(MS + 'STUNEnable', '0'), P(MS + 'STUNServerAddress', ''), P(MS + 'UDPConnectionRequestAddress', '198.51.100.7:50000'), P(MS + 'NATDetected', '1')])
  assert.equal(v.status, 'DIDUKUNG')
  const txt = v.notes.join(' ')
  assert.match(txt, /STUNEnable = 0 \(nonaktif\)/)
  assert.match(txt, /UDPConnectionRequestAddress terisi/)
  assert.match(txt, /NATDetected = 1/)
  assert.equal(stunVerdict([P(MS + 'STUNEnable', 'true')]).notes.join(' ').includes('(aktif)'), true)
  assert.match(stunVerdict([P(MS + 'STUNEnable')]).notes.join(' '), /belum dibaca/)
})

test('stunVerdict: tanpa parameter STUN — hanya "tampak tidak didukung" bila daftar nama cukup lengkap; selain itu BELUM DIKETAHUI', () => {
  const few = [P(MS + 'URL'), P(MS + 'ConnectionRequestURL')]
  assert.equal(stunVerdict(few).status, 'BELUM_DIKETAHUI')
  assert.equal(stunVerdict([]).status, 'BELUM_DIKETAHUI')
  const many = Array.from({ length: MS_DISCOVERED_MIN_NAMES }, (_, i) => P(MS + 'Param' + i))
  assert.equal(stunVerdict(many).status, 'TAMPAK_TIDAK_DIDUKUNG')
  assert.equal(stunVerdict(many.slice(0, MS_DISCOVERED_MIN_NAMES - 1)).status, 'BELUM_DIKETAHUI') // tepat di bawah ambang
  // nama yang kebetulan memuat "NAT" di tengah kata lain tidak dihitung STUN (hanya nama daun yang cocok polanya)
  assert.equal(stunVerdict(many.concat([P(MS + 'Alternative')])).status, 'TAMPAK_TIDAK_DIDUKUNG')
})

test('hexAsciiDecode: heksa dari teks serial; bukan heksa murni atau di luar rentang = null', () => {
  assert.equal(hexAsciiDecode('5A54454743313233'), 'ZTEGC123')
  assert.equal(hexAsciiDecode('5a54454743313233'), 'ZTEGC123') // huruf kecil
  assert.equal(hexAsciiDecode('ZTEGC123'), null)
  assert.equal(hexAsciiDecode('ABCDEF12'), null) // heksa murni tetapi hasilnya bukan teks serial
  assert.equal(hexAsciiDecode('5A5445'), null) // terlalu pendek
  assert.equal(hexAsciiDecode('5A5445474331323'), null) // ganjil
})

test('compareSerials: cocok persis (tak peka huruf), kandidat per jenis transformasi, dan yang benar-benar tak cocok', () => {
  const olt = ['ZTEGC0000001', 'zteg-c000-0002', '5A5445474330303030303033', 'PRE-ZTEGC0000004', 'ZTEGC0000005', 'ABCD']
  const acs = ['ztegc0000001', 'ZTEGC0000002', 'ZTEGC0000003', 'ZTEGC0000004', 'ZTEGC0000099']
  const r = compareSerials(olt, acs)
  assert.deepEqual(r.matched, ['ZTEGC0000001'])
  const by = Object.fromEntries(r.candidates.map((c) => [c.olt, c]))
  assert.match(by['zteg-c000-0002'].how, /pemisah/)
  assert.equal(by['zteg-c000-0002'].acs, 'ZTEGC0000002')
  assert.match(by['5A5445474330303030303033'].how, /heksa dari teks serial ACS/)
  assert.equal(by['5A5445474330303030303033'].acs, 'ZTEGC0000003')
  assert.match(by['PRE-ZTEGC0000004'].how, /awalan tambahan/)
  assert.deepEqual(r.unmatched.sort(), ['ABCD', 'ZTEGC0000005'])
})

test('compareSerials: akhiran tambahan dan sisi mana yang lebih panjang', () => {
  const r = compareSerials(['ZTEGC0000001-A', 'C0000002', 'ZTEGC0000003'], ['ZTEGC0000001', 'XXZTEGC0000002', 'ZTEGC0000003'])
  const by = Object.fromEntries(r.candidates.map((c) => [c.olt, c.how]))
  assert.equal(by['ZTEGC0000001-A'], 'serial OLT memiliki akhiran tambahan')
  assert.equal(by['C0000002'], 'serial ACS memiliki awalan tambahan')
  assert.deepEqual(r.matched, ['ZTEGC0000003'])
})

test('compareSerials: kandidat awalan memerlukan >= 8 karakter (serial pendek tidak ditebak)', () => {
  const r = compareSerials(['XX1234'], ['1234'])
  assert.equal(r.candidates.length, 0)
  assert.deepEqual(r.unmatched, ['XX1234'])
  assert.deepEqual(compareSerials([], []), { matched: [], candidates: [], unmatched: [] })
})

test('describeSerials meringkas panjang, heksa, huruf kecil, pemisah, dan awalan', () => {
  const d = describeSerials(['ZTEGC0000001', 'ZTEGC0000002', 'abc-12'])
  assert.equal(d.total, 3)
  assert.deepEqual(d.lengths[0], [12, 2])
  assert.equal(d.withLowercase, 1)
  assert.equal(d.withSeparator, 1)
  assert.equal(d.allHex, false)
  assert.equal(describeSerials(['0A1B2C3D']).allHex, true)
  assert.equal(describeSerials([]).allHex, false)
  assert.equal(normSerial('zt-eg c1'), 'ZTEGC1')
})

test('maskSerial: panjang dan awalan dipertahankan; utuh hanya bila diminta', () => {
  assert.equal(maskSerial('ZTEGC0000001'), 'ZTE*********')
  assert.equal(maskSerial('ZTEGC0000001', true), 'ZTEGC0000001')
  assert.equal(maskSerial('AB'), '**')
})

test('crVerdict: terbukti hanya bila event 6 CONNECTION REQUEST muncul SETELAH CR dikirim', () => {
  const t0 = Date.parse('2026-10-29T10:00:00Z')
  const ev = (code, iso) => ({ code, occurred_at: iso })
  assert.equal(crVerdict([], t0, false).status, 'GAGAL_DIKIRIM')
  const ok = crVerdict([ev('6 CONNECTION REQUEST', '2026-10-29T10:00:07Z'), ev('2 PERIODIC', '2026-10-29T09:00:00Z')], t0, true)
  assert.equal(ok.status, 'TERBUKTI')
  assert.equal(ok.latencySeconds, 7)
  // event CR LAMA (sebelum dikirim) bukan bukti
  assert.equal(crVerdict([ev('6 CONNECTION REQUEST', '2026-10-29T09:30:00Z')], t0, true).status, 'TIDAK_TERBUKTI')
  // event lain setelah dikirim bukan bukti CR
  assert.equal(crVerdict([ev('2 PERIODIC', '2026-10-29T10:00:05Z')], t0, true).status, 'TIDAK_TERBUKTI')
  // toleransi 1 dtk untuk selisih jam
  assert.equal(crVerdict([ev('6 CONNECTION REQUEST', '2026-10-29T09:59:59.500Z')], t0, true).status, 'TERBUKTI')
})

test('maskOltOutput: rahasia, IP, dan MAC disamarkan; serial dipertahankan kecuali diminta', () => {
  const raw = [
    'snmp-server community Rahasia123 RO',
    'username admin password S3cr3t!',
    'passwd: abc123',
    'secret=zzz',
    'mgmt ip 10.20.30.40 gw 192.168.1.1',
    'mac 00:1A:2B:3C:4D:5E and 00-1a-2b-3c-4d-5f',
    'ONU ZTEGC1234ABC state working',
  ].join('\n')
  const out = maskOltOutput(raw)
  for (const leak of ['Rahasia123', 'S3cr3t', 'abc123', 'zzz', '10.20.30.40', '192.168.1.1', '3C:4D:5E', '3c-4d-5f']) assert.ok(!out.includes(leak), `bocor: ${leak}`)
  assert.match(out, /community \[disamarkan\]/)
  assert.match(out, /10\.20\.x\.x/)
  assert.match(out, /00:xx:xx:xx:xx:xx/)
  assert.ok(out.includes('ZTEGC1234ABC'), 'serial dipertahankan secara bawaan')
  assert.ok(!maskOltOutput(raw, { maskSerials: true }).includes('ZTEGC1234ABC'))
  assert.equal(maskOltOutput('tanpa rahasia'), 'tanpa rahasia')
})

test('table meloloskan pipa dan baris baru', () => {
  assert.equal(table(['a', 'b'], [['x|y', 'p\nq']]), '| a | b |\n|---|---|\n| x\\|y | p q |')
})
