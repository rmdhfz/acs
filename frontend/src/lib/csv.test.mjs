// Test CSV — dijalankan dengan `npm test` (node --test, Node >= 22 mendukung
// import .ts langsung lewat type-stripping).
import test from 'node:test'
import assert from 'node:assert/strict'
import { parseCSV, toCSV, detectDelimiter, neutralize, unneutralize } from './csv.ts'

test('parse dasar, CRLF/LF, BOM, baris kosong diabaikan', () => {
  assert.deepEqual(parseCSV('﻿a,b\r\n1,2\n\n3,4\r\n'), [['a', 'b'], ['1', '2'], ['3', '4']])
  assert.deepEqual(parseCSV(''), [])
})

test('sel ber-kutip: koma, baris baru, kutip ganda', () => {
  const rows = parseCSV('a,b\n"x, y","li\nne ""q"""\n')
  assert.deepEqual(rows, [['a', 'b'], ['x, y', 'li\nne "q"']])
})

test('sel kosong dipertahankan, spasi di luar kutip dipangkas, di dalam kutip dipertahankan', () => {
  assert.deepEqual(parseCSV('a,b,c\n, x ," y "\n'), [['a', 'b', 'c'], ['', 'x', ' y ']])
})

test('deteksi pemisah ; dan tab (Excel regional)', () => {
  assert.equal(detectDelimiter('a;b;c\n1;2;3'), ';')
  assert.equal(detectDelimiter('a\tb\n1\t2'), '\t')
  assert.equal(detectDelimiter('a,b'), ',')
  assert.equal(detectDelimiter('"a;b",c\n'), ',') // ; di dalam kutip tidak dihitung
  assert.deepEqual(parseCSV('a;b\n1,5;2\n'), [['a', 'b'], ['1,5', '2']])
})

test('baris terakhir tanpa newline tetap terbaca', () => {
  assert.deepEqual(parseCSV('a,b\n1,2'), [['a', 'b'], ['1', '2']])
  assert.deepEqual(parseCSV('a,b\n1,"2"'), [['a', 'b'], ['1', '2']])
})

test('toCSV: BOM, CRLF, escape, boolean/null/number', () => {
  const out = toCSV(['a', 'b'], [['x,y', 'q"r'], [1, null], [true, undefined]])
  assert.ok(out.startsWith('﻿'))
  assert.equal(out.slice(1), 'a,b\r\n"x,y","q""r"\r\n1,\r\ntrue,\r\n')
})

test('penetralan formula injection', () => {
  assert.equal(neutralize('=HYPERLINK("x")'), `'=HYPERLINK("x")`)
  assert.equal(neutralize('+1+1'), `'+1+1`)
  assert.equal(neutralize('@SUM(A1)'), `'@SUM(A1)`)
  assert.equal(neutralize('-cmd'), `'-cmd`)
  assert.equal(neutralize('-6.2088'), '-6.2088') // angka murni tidak diubah
  assert.equal(neutralize('106.8456'), '106.8456')
  assert.equal(neutralize('aman'), 'aman')
  assert.equal(unneutralize(`'=1+1`), '=1+1')
  assert.equal(unneutralize(`'biasa`), `'biasa`) // apostrof biasa tidak dibuang
})

test('round trip: ekspor lalu parse mengembalikan data semula', () => {
  const data = [['=cmd|calc', 'a,b', 'baris\nbaru', ' spasi ', '-6.2', '']]
  const rows = parseCSV(toCSV(['c1', 'c2', 'c3', 'c4', 'c5', 'c6'], data))
  assert.equal(rows.length, 2)
  assert.deepEqual(rows[1].map(unneutralize), data[0])
})
