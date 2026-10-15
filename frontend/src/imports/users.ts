import { api } from '../lib/api'
import { clientAdapter } from '../lib/clientAdapter'
import { fetchAllPages, type ExportColumn } from '../lib/exporter'
import { RowError, parseBool, type ImportAdapter, type ImportColumn, type ImportMode, type RawRecord } from '../lib/importer'
import { LIMITS } from '../lib/limits'
import type { RefLookup, User } from '../lib/types'

// Impor/ekspor USER (modul Administrasi). Memakai API yang sudah ada per baris
// (POST /users, PATCH /users/:id, PATCH /users/:id/roles) sehingga RBAC dan
// validasi backend tetap berlaku; pratinjau memeriksa data eksisting dan
// aturan RBAC lebih dulu agar kegagalan tidak baru muncul saat eksekusi.
//
// Keputusan desain:
//  - Kunci pencocokan: email (tak peka huruf besar) — unik global di backend.
//  - Password TIDAK pernah diekspor. Kolom `password` (opsional, rahasia) hanya
//    dipakai saat membuat user baru. Bila kosong, klien membuat password acak
//    kuat (24 karakter) yang TIDAK ditampilkan/disimpan di mana pun: pemilik akun
//    membuat password sendiri lewat tautan di email selamat datang / "Lupa
//    password" (alamat *.local/*.test/example.* tidak dikirimi email — pakai
//    "Reset Password" manual untuk akun seperti itu).
//  - Baris yang sudah ada: hanya full_name, is_active, dan roles yang bisa
//    diperbarui. username dan tenant tidak dapat diubah lewat API, jadi bila
//    berbeda dilaporkan sebagai error (bukan diabaikan diam-diam).
//  - Sel kosong pada baris yang sudah ada = "tidak diubah".

export interface UserImportEnv {
  isSuperadmin: boolean
  /** id akun yang sedang login (untuk guard self-lockout). */
  actorId: number
  /** tenant_id akun yang sedang login (ADMIN dikunci ke tenant ini). */
  actorTenantId: number | null
  /** Superadmin: semua tenant. ADMIN: hanya tenant sendiri. */
  tenants: { id: number; code: string; name: string }[]
  /** ref_roles — dipakai memvalidasi & mengkanonkan kode role. */
  roles: RefLookup[]
}

export const userImportColumns: ImportColumn[] = [
  { key: 'email', label: 'Email', required: true, example: 'budi@ispjakarta.co.id', hint: 'Kunci pencocokan (tak peka huruf besar). Alamat polos yang valid.' },
  { key: 'username', label: 'Username', example: 'budi.jkt', hint: `Wajib untuk user baru (maks. ${LIMITS.user.username}). Tidak bisa diubah pada user yang sudah ada.` },
  { key: 'full_name', label: 'Nama Lengkap', example: 'Budi Santoso', aliases: ['nama', 'name'], hint: `Maks. ${LIMITS.user.fullName}. Kosong pada user yang sudah ada = tidak diubah.` },
  { key: 'roles', label: 'Role', example: 'ADMIN|NOC', aliases: ['role', 'role_codes'], hint: 'Kode role dipisah "|". ADMIN tidak boleh memberi SUPERADMIN. Kosong pada user yang sudah ada = tidak diubah.' },
  { key: 'tenant', label: 'Tenant', example: 'acme', aliases: ['tenant_code'], hint: 'Kode tenant. Superadmin: kosong = user global (tanpa tenant). ADMIN: otomatis tenant sendiri. Tidak bisa diubah pada user yang sudah ada.' },
  { key: 'is_active', label: 'Aktif', example: 'true', aliases: ['aktif', 'active'], hint: 'true/false (atau ya/tidak). Baru: default true. Kosong pada user yang sudah ada = tidak diubah.' },
  {
    key: 'password',
    label: 'Password Awal',
    secret: true,
    example: 'GantiSegera#2026',
    hint: 'Opsional, hanya untuk user baru (min. 8 karakter). Kosong = dibuat acak & disembunyikan; pemilik membuat password lewat email selamat datang / "Lupa password". TIDAK pernah diekspor.',
  },
  // informasi saja (ikut diekspor, diabaikan saat impor)
  { key: 'user_uuid', label: 'UUID', readonly: true },
  { key: 'last_login_at', label: 'Login terakhir', readonly: true },
]

interface UserRow {
  email: string
  username: string
  fullName?: string
  roles?: string[]
  /** tenant_id tujuan (null = global); tenantSpecified = sel tenant berisi. */
  tenantId: number | null
  tenantSpecified: boolean
  isActive?: boolean
  password: string
}

const norm = (s: string) => s.trim().toLowerCase()

// Setara dengan mail.ParseAddress(...).Address == email di backend untuk alamat polos.
const EMAIL_RE = /^[^\s@<>",;:()[\]\\]+@[^\s@<>",;:()[\]\\]+\.[^\s@<>",;:()[\]\\]+$/

function splitRoles(raw: string): string[] {
  const t = raw.trim()
  if (t.startsWith('[')) {
    try {
      const arr: unknown = JSON.parse(t)
      if (Array.isArray(arr)) return arr.map((x) => String(x).trim()).filter(Boolean)
    } catch {
      // jatuh ke pemisah biasa
    }
  }
  return t.split(/[|;]/).map((s) => s.trim()).filter(Boolean)
}

function sameSet(a: string[], b: string[]): boolean {
  const sa = new Set(a.map(norm))
  const sb = new Set(b.map(norm))
  if (sa.size !== sb.size) return false
  for (const x of sa) if (!sb.has(x)) return false
  return true
}

/** randomPassword membuat password acak kuat (24 karakter) dari CSPRNG. */
function randomPassword(): string {
  const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%^&*-_=+'
  const buf = new Uint32Array(24)
  crypto.getRandomValues(buf)
  let out = ''
  for (const n of buf) out += alphabet[n % alphabet.length]
  return out
}

function parseRow(rec: RawRecord, env: UserImportEnv): UserRow {
  const email = (rec.email ?? '').trim()
  if (email === '') throw new RowError('email wajib diisi')
  if (!EMAIL_RE.test(email) || email.length > LIMITS.user.email) throw new RowError(`email "${email}" tidak valid`)

  const username = (rec.username ?? '').trim()
  if (username.length > LIMITS.user.username) throw new RowError(`username maks. ${LIMITS.user.username} karakter`)
  const fullName = rec.full_name === undefined || rec.full_name === '' ? undefined : rec.full_name
  if (fullName !== undefined && fullName.length > LIMITS.user.fullName) throw new RowError(`full_name maks. ${LIMITS.user.fullName} karakter`)

  let roles: string[] | undefined
  const rolesRaw = rec.roles ?? ''
  if (rolesRaw !== '') {
    roles = []
    for (const r of splitRoles(rolesRaw)) {
      const ref = env.roles.find((x) => norm(x.code) === norm(r) || norm(x.name) === norm(r))
      if (!ref) throw new RowError(`role "${r}" tidak dikenal (tersedia: ${env.roles.map((x) => x.code).join(', ') || '-'})`)
      if (!roles.includes(ref.code)) roles.push(ref.code)
    }
    if (!env.isSuperadmin && roles.includes('SUPERADMIN')) {
      throw new RowError('hanya SUPERADMIN yang boleh memberikan role SUPERADMIN')
    }
  }

  const isActive = parseBool('is_active', rec.is_active ?? '')

  const password = rec.password ?? ''
  if (password !== '' && password.length < 8) throw new RowError('password minimal 8 karakter')

  const tenantRaw = (rec.tenant ?? '').trim()
  const tenantSpecified = tenantRaw !== ''
  let tenantId: number | null
  if (tenantRaw === '') {
    tenantId = env.isSuperadmin ? null : env.actorTenantId
  } else {
    const t = env.tenants.find((x) => norm(x.code) === norm(tenantRaw) || norm(x.name) === norm(tenantRaw) || String(x.id) === tenantRaw)
    if (!t) {
      throw new RowError(
        env.isSuperadmin ? `tenant "${tenantRaw}" tidak ditemukan` : `ADMIN hanya dapat mengelola user di tenant sendiri (tenant "${tenantRaw}" tidak diizinkan)`,
      )
    }
    tenantId = t.id
  }
  if (!env.isSuperadmin && tenantId === null) throw new RowError('akun Anda tidak terikat tenant — tidak bisa membuat/mengubah user')

  return { email, username, fullName, roles, tenantId, tenantSpecified, isActive, password }
}

interface UserPayload {
  row: UserRow
  ex?: User
  changes?: string[]
}

export function userAdapter(env: UserImportEnv): ImportAdapter {
  // diisi loadExisting; dipakai plan() untuk memeriksa bentrokan username
  let lastUsers: User[] = []
  const base = clientAdapter<UserRow, User>({
    entity: 'user',
    columns: userImportColumns,
    template: [
      {
        email: 'budi@ispjakarta.co.id',
        username: 'budi.jkt',
        full_name: 'Budi Santoso',
        roles: 'NOC',
        tenant: env.isSuperadmin ? (env.tenants[0]?.code ?? 'acme') : '',
        is_active: 'true',
        password: '',
      },
    ],
    parse: (rec) => parseRow(rec, env),
    key: (r) => norm(r.email),
    label: (r) => r.email,

    async loadExisting(_rows, signal) {
      const { rows } = await fetchAllPages<User>('/users', {}, { signal })
      const m = new Map<string, User>()
      for (const u of rows) m.set(norm(u.email), u)
      lastUsers = rows
      return m
    },

    diff(row, ex) {
      const ch: string[] = []
      if (row.fullName !== undefined && row.fullName !== ex.full_name) ch.push('full_name')
      if (row.isActive !== undefined && row.isActive !== ex.is_active) ch.push('is_active')
      if (row.roles !== undefined && !sameSet(row.roles, ex.roles ?? [])) ch.push('roles')
      return ch
    },

    async create(row) {
      const created = await api.post<User>('/users', {
        tenant_id: row.tenantId ?? undefined,
        username: row.username,
        email: row.email,
        // tanpa password di file → acak & tidak pernah ditampilkan (lihat catatan di atas)
        password: row.password !== '' ? row.password : randomPassword(),
        full_name: row.fullName ?? '',
        role_codes: row.roles ?? [],
      })
      if (row.isActive === false) {
        try {
          await api.patch(`/users/${created.id}`, { is_active: false })
        } catch (e) {
          throw new Error(`user dibuat, tetapi gagal menonaktifkan: ${e instanceof Error ? e.message : String(e)}`)
        }
      }
    },

    async update(row, ex, changes) {
      const body: Record<string, unknown> = {}
      if (changes.includes('full_name')) body.full_name = row.fullName
      if (changes.includes('is_active')) body.is_active = row.isActive
      if (Object.keys(body).length > 0) await api.patch(`/users/${ex.id}`, body)
      if (changes.includes('roles')) {
        try {
          await api.patch(`/users/${ex.id}/roles`, { role_codes: row.roles })
        } catch (e) {
          const partial = Object.keys(body).length > 0 ? ' (field lain sudah diterapkan)' : ''
          throw new Error(`gagal mengganti role${partial}: ${e instanceof Error ? e.message : String(e)}`)
        }
      }
    },
  })

  return {
    ...base,
    // Pemeriksaan yang membutuhkan data eksisting + RBAC dilakukan di pratinjau
    // (bukan baru gagal saat eksekusi).
    async plan(records: RawRecord[], mode: ImportMode, ctx) {
      lastUsers = []
      const items = await base.plan(records, mode, ctx)
      const byUsername = new Map<string, User>()
      for (const u of lastUsers) byUsername.set(norm(u.username), u)
      const seenUsername = new Map<string, number>()

      const fail = (it: (typeof items)[number], msg: string) => {
        it.action = 'error'
        it.error = msg
        it.changes = undefined
      }

      for (const it of items) {
        const p = it.payload as UserPayload | undefined
        if (!p || it.action === 'error') continue
        const { row, ex } = p
        const un = norm(row.username)

        if (un !== '') {
          const first = seenUsername.get(un)
          if (first !== undefined) {
            fail(it, `username "${row.username}" sama dengan baris ${first}`)
            continue
          }
          seenUsername.set(un, it.rowNo)
        }

        if (it.action === 'create') {
          if (un === '') {
            fail(it, 'username wajib diisi untuk user baru')
            continue
          }
          const clash = byUsername.get(un)
          if (clash) fail(it, `username "${row.username}" sudah dipakai user lain (${clash.email})`)
          continue
        }

        if (!ex || (it.action !== 'update' && it.action !== 'unchanged')) continue
        if (un !== '' && un !== norm(ex.username)) {
          fail(it, `username tidak dapat diubah lewat impor (saat ini "${ex.username}")`)
          continue
        }
        if (row.tenantSpecified && row.tenantId !== (ex.tenant_id ?? null)) {
          fail(it, 'tenant user tidak dapat dipindahkan lewat impor')
          continue
        }
        if (it.action === 'update' && ex.id === env.actorId) {
          if (it.changes?.includes('is_active') && row.isActive === false) {
            fail(it, 'tidak bisa menonaktifkan akun sendiri')
            continue
          }
          if (it.changes?.includes('roles') && !env.isSuperadmin && (ex.roles ?? []).includes('ADMIN') && !(row.roles ?? []).includes('ADMIN')) {
            fail(it, 'tidak bisa mencabut role ADMIN dari akun sendiri')
            continue
          }
        }
      }
      return items
    },
  }
}

// ---------------------------------------------------------------- ekspor

/** Kolom ekspor user. password/hash TIDAK ada. */
export function userExportColumns(tenants: { id: number; code: string }[]): ExportColumn<User>[] {
  const tenantCode = (u: User) => (u.tenant_id == null ? '' : (tenants.find((t) => t.id === u.tenant_id)?.code ?? String(u.tenant_id)))
  return [
    { key: 'email', value: (u) => u.email },
    { key: 'username', value: (u) => u.username },
    { key: 'full_name', value: (u) => u.full_name },
    { key: 'roles', value: (u) => (u.roles ?? []).join('|') },
    { key: 'tenant', value: tenantCode },
    { key: 'is_active', value: (u) => u.is_active },
    { key: 'user_uuid', value: (u) => u.user_uuid },
    { key: 'last_login_at', value: (u) => u.last_login_at },
  ]
}

export async function loadUserExportRows(
  filters: Record<string, string | number | undefined>,
  ctx: { signal: AbortSignal; onProgress: (done: number, total: number) => void },
): Promise<User[]> {
  const { rows, truncated } = await fetchAllPages<User>('/users', filters, { signal: ctx.signal, onProgress: ctx.onProgress })
  if (truncated) throw new Error('Data melebihi 50.000 baris — persempit dengan filter lalu ekspor lagi.')
  return rows
}
