import { describe, expect, it } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'

// Gerbang validasi form: setiap input teks wajib memberi batas panjang (maxLength) dan setiap input angka
// wajib memberi batas atas (max). Tanpa itu pengguna bisa mengetik/menempel nilai tak terbatas yang baru
// ditolak backend (atau, dulu, lolos ke MariaDB sebagai 500). Backend tetap sumber kebenaran; ini lapisan UX.
// Tambahkan batas baru di src/lib/limits.ts bila memang mencerminkan lebar kolom.

const SRC = path.resolve(__dirname, '..')
const NON_TEXT = new Set(['checkbox', 'radio', 'file', 'hidden', 'range', 'color'])
const TEXTUAL = new Set(['text', 'email', 'password', 'search', 'tel', 'url', 'textarea'])

function walk(dir: string, out: string[] = []): string[] {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name)
    if (e.isDirectory()) walk(p, out)
    else if (/\.tsx$/.test(e.name) && !/\.test\./.test(e.name)) out.push(p)
  }
  return out
}

// Ambil satu tag JSX utuh: abaikan '>' di dalam {...} dan di dalam string.
function tagAt(s: string, i: number): string {
  let depth = 0
  let q: string | null = null
  for (let j = i; j < s.length; j++) {
    const c = s[j]
    if (q) {
      if (c === q) q = null
      continue
    }
    if (c === '"' || c === "'" || c === '`') {
      q = c
      continue
    }
    if (c === '{') depth++
    else if (c === '}') depth--
    else if (c === '>' && depth === 0 && s[j - 1] !== '=') return s.slice(i, j + 1)
  }
  return s.slice(i, i + 300)
}

function findViolations(): string[] {
  const bad: string[] = []
  for (const file of walk(SRC)) {
    const s = fs.readFileSync(file, 'utf8')
    const re = /<(input|textarea)\b/g
    let m: RegExpExecArray | null
    while ((m = re.exec(s))) {
      const tag = tagAt(s, m.index)
      const type = (tag.match(/\btype=["{']+([a-z]+)/) || [])[1] || (m[1] === 'textarea' ? 'textarea' : 'text')
      if (NON_TEXT.has(type)) continue
      if (/\bdisabled\b/.test(tag) && !/onChange/.test(tag)) continue
      const textual = TEXTUAL.has(type)
      // password BARU wajib punya minLength (server menolak < 8; klien memberi umpan balik sebelum request)
      const weakNew = /autoComplete="new-password"/.test(tag) && !/minLength/.test(tag)
      // angka wajib punya batas bawah juga (negatif/NaN tak boleh lolos sebagai nilai wajar)
      const noMin = type === 'number' && !/\bmin=/.test(tag)
      // field WAJIB berteks harus punya minLength/pattern (atau type email/url yang divalidasi browser): spasi/1 huruf tidak lolos
      const weakRequired = textual && /\brequired\b/.test(tag) && !['email', 'url', 'search'].includes(type) && !/minLength|pattern=/.test(tag) && !/autoComplete="current-password"/.test(tag)
      if (weakRequired) {
        const line = s.slice(0, m.index).split('\n').length
        bad.push(`${path.relative(SRC, file).split(path.sep).join('/')}:${line} <${m[1]} ${type}> required tanpa minLength/pattern`)
      }
      if ((textual && !/maxLength/.test(tag)) || (type === 'number' && !/\bmax=/.test(tag)) || weakNew || noMin) {
        const line = s.slice(0, m.index).split('\n').length
        bad.push(`${path.relative(SRC, file).split(path.sep).join('/')}:${line} <${m[1]} ${type}>`)
      }
    }
  }
  return bad
}

describe('batas input form', () => {
  it('input teks punya maxLength, angka punya min & max, password baru punya minLength', () => {
    expect(findViolations()).toEqual([])
  })
})
