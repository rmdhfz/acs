import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { AlertCircle, ArrowLeft, Building2, Eye, EyeOff, Lock, Mail, MailCheck, User } from 'lucide-react'
import { ApiError } from '../lib/api'
import { useAuthConfig, useSignup } from '../lib/hooks'
import { AuthShell } from '../components/AuthShell'
import { PasswordStrengthBar } from '../components/PasswordStrengthBar'
import { Spinner } from '../components/Spinner'
import { scorePassword } from '../lib/password'
import { authInputCls, authPrimaryBtnCls } from '../components/authStyles'

const labelCls = 'mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-300'
const hintCls = 'mt-1 text-xs text-slate-600 dark:text-slate-400'
const CONTACT_EMAIL = (import.meta.env.VITE_CONTACT_EMAIL as string | undefined) ?? ''

// Pendaftaran tenant mandiri. Fitur bawaan MATI di backend (ACS_SELF_SIGNUP): halaman ini menanyakan
// /auth/config dan menampilkan keterangan jelas bila ditutup. Backend memverifikasi email dulu sebelum tenant dibuat.
export function SignupPage() {
  const mut = useSignup()
  const { data: cfg, isLoading: cfgLoading, isError: cfgError } = useAuthConfig()
  // null = belum tahu (memuat); false = ditutup atau konfigurasi tak terjangkau (gagal-tertutup, dengan pesan jelas)
  const enabled: boolean | null = cfgLoading ? null : cfgError ? false : Boolean(cfg?.self_signup_enabled)
  const [company, setCompany] = useState('')
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [show, setShow] = useState(false)
  const [terms, setTerms] = useState(false)
  const [website, setWebsite] = useState('') // honeypot
  const [error, setError] = useState<string | null>(null)
  const [sentTo, setSentTo] = useState<string | null>(null)

  const pwScore = scorePassword(password).score
  const mismatch = confirm !== '' && confirm !== password
  const ready = company.trim().length >= 3 && name.trim().length >= 2 && email.includes('@') && password.length >= 10 && !mismatch && confirm !== '' && terms

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (company.trim().length < 3) return setError('Nama perusahaan minimal 3 karakter.')
    if (name.trim().length < 2) return setError('Nama lengkap minimal 2 karakter.')
    if (password.length < 10) return setError('Password minimal 10 karakter.')
    if (pwScore < 2) return setError('Password terlalu lemah — gunakan kombinasi huruf, angka, dan simbol yang lebih panjang.')
    if (password !== confirm) return setError('Konfirmasi password tidak cocok.')
    if (!terms) return setError('Setujui ketentuan penggunaan untuk melanjutkan.')
    try {
      await mut.mutateAsync({ company_name: company.trim(), full_name: name.trim(), email: email.trim(), password, accept_terms: terms, website })
      setSentTo(email.trim())
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) setError('Terlalu banyak percobaan. Coba lagi beberapa saat lagi.')
      else if (err instanceof ApiError && err.status === 503) setError('Pendaftaran sedang penuh. Coba lagi besok atau hubungi kami.')
      else if (err instanceof ApiError && err.status === 403) setError('Pendaftaran mandiri tidak diaktifkan.')
      else setError(err instanceof ApiError ? err.message : 'Tidak dapat menjangkau server. Periksa koneksi Anda lalu coba lagi.')
    }
  }

  return (
    <AuthShell>
      <Link to="/login" className="mb-6 inline-flex items-center gap-1.5 text-sm font-medium text-slate-600 transition-colors hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-100">
        <ArrowLeft className="h-4 w-4" /> Kembali ke login
      </Link>

      {enabled === false ? (
        <div role="status" className="text-center">
          <span className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300">
            <Lock className="h-7 w-7" strokeWidth={1.75} aria-hidden="true" />
          </span>
          <h1 className="mt-5 text-[1.75rem] font-normal leading-tight text-center text-slate-900 dark:text-slate-100">Pendaftaran belum dibuka</h1>
          <p className="mt-3 text-sm leading-relaxed text-slate-600 dark:text-slate-400">
            Instalasi ini belum menerima pendaftaran tenant mandiri. Administrator platform dapat membuatkan akun untuk Anda, atau hubungi kami untuk jadwal demo dan rencana pilot.
          </p>
          <div className="mt-7 flex flex-wrap justify-center gap-3">
            {CONTACT_EMAIL && (
              <a href={`mailto:${CONTACT_EMAIL}?subject=${encodeURIComponent('Permintaan akses ACS Console')}`} className="inline-flex h-11 items-center rounded-full bg-indigo-600 px-6 text-sm font-medium text-white hover:bg-indigo-700">
                Hubungi kami
              </a>
            )}
            <Link to="/login" className="inline-flex h-11 items-center rounded-full border border-slate-300 px-6 text-sm font-medium text-indigo-700 hover:bg-indigo-50 dark:border-slate-600 dark:text-indigo-300 dark:hover:bg-slate-800">
              Sudah punya akun? Masuk
            </Link>
          </div>
        </div>
      ) : sentTo ? (
        <div>
          <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400">
            <MailCheck className="h-7 w-7" />
          </div>
          <h1 className="mt-5 text-[1.75rem] font-normal leading-tight text-center text-slate-900 dark:text-slate-100">Cek email Anda</h1>
          <p className="mt-2 text-sm leading-relaxed text-slate-600 dark:text-slate-400" role="status">
            Jika data valid, kami telah mengirim tautan verifikasi ke <strong>{sentTo}</strong>. Klik tautan itu (berlaku 24 jam) untuk mengaktifkan tenant dan akun administrator Anda.
          </p>
          <p className="mt-3 text-sm text-slate-600 dark:text-slate-400">Tidak ada email? Periksa folder spam. Email yang sudah terdaftar tidak menerima email baru.</p>
        </div>
      ) : (
        <>
          <div className="mb-6">
            <h1 className="text-[1.75rem] font-normal leading-tight text-center text-slate-900 dark:text-slate-100">Buat akun ACS Console</h1>
            <p className="mt-2 text-center text-sm text-slate-600 dark:text-slate-400">Kelola perangkat ONT/CPE Anda dari satu konsol. Paket awal gratis dengan batas perangkat; email diverifikasi sebelum akun dibuat.</p>
          </div>

          <form onSubmit={handleSubmit} noValidate className="space-y-4">
            <div>
              <label htmlFor="su-company" className={labelCls}>Nama perusahaan / ISP</label>
              <div className="relative">
                <Building2 className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
                <input id="su-company" value={company} onChange={(e) => setCompany(e.target.value)} required minLength={3} maxLength={128} autoComplete="organization" className={`${authInputCls} pl-10`} placeholder="PT Contoh Jaringan" />
              </div>
            </div>
            <div>
              <label htmlFor="su-name" className={labelCls}>Nama lengkap administrator</label>
              <div className="relative">
                <User className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
                <input id="su-name" value={name} onChange={(e) => setName(e.target.value)} required minLength={2} maxLength={128} autoComplete="name" className={`${authInputCls} pl-10`} placeholder="Nama Anda" />
              </div>
            </div>
            <div>
              <label htmlFor="su-email" className={labelCls}>Email (untuk login & verifikasi)</label>
              <div className="relative">
                <Mail className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
                <input id="su-email" type="email" inputMode="email" value={email} onChange={(e) => setEmail(e.target.value)} required maxLength={191} autoComplete="email" autoCapitalize="none" spellCheck={false} className={`${authInputCls} pl-10`} placeholder="nama@perusahaan.com" />
              </div>
            </div>
            <div>
              <label htmlFor="su-pass" className={labelCls}>Password</label>
              <div className="relative">
                <Lock className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
                <input id="su-pass" type={show ? 'text' : 'password'} value={password} onChange={(e) => setPassword(e.target.value)} required minLength={10} maxLength={128} autoComplete="new-password" className={`${authInputCls} pl-10 pr-11`} aria-describedby="su-pass-hint" />
                <button type="button" onClick={() => setShow((v) => !v)} aria-label={show ? 'Sembunyikan password' : 'Tampilkan password'} aria-pressed={show} className="absolute right-2 top-1/2 -translate-y-1/2 rounded-md p-1.5 text-slate-500 hover:text-slate-700 dark:hover:text-slate-300">
                  {show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
              <p id="su-pass-hint" className={hintCls}>Minimal 10 karakter; jangan memuat email atau nama perusahaan.</p>
              <PasswordStrengthBar password={password} />
            </div>
            <div>
              <label htmlFor="su-confirm" className={labelCls}>Ulangi password</label>
              <input minLength={10} id="su-confirm" type={show ? 'text' : 'password'} value={confirm} onChange={(e) => setConfirm(e.target.value)} required maxLength={128} autoComplete="new-password" aria-invalid={mismatch} className={authInputCls} />
              {mismatch && <p role="alert" className="mt-1 text-xs text-red-700 dark:text-red-400">Konfirmasi password tidak cocok.</p>}
            </div>

            {/* Honeypot: tersembunyi dari manusia & pembaca layar; bot sederhana mengisinya. */}
            <div aria-hidden="true" style={{ position: 'absolute', left: '-10000px', width: 1, height: 1, overflow: 'hidden' }}>
              <label htmlFor="su-website">Website (jangan diisi)</label>
              <input maxLength={128} id="su-website" tabIndex={-1} autoComplete="off" value={website} onChange={(e) => setWebsite(e.target.value)} />
            </div>

            <label className="flex cursor-pointer items-start gap-2 text-sm text-slate-700 dark:text-slate-300">
              <input type="checkbox" checked={terms} onChange={(e) => setTerms(e.target.checked)} required className="mt-0.5 h-4 w-4 rounded border-slate-300 text-indigo-600 dark:border-slate-600 dark:bg-slate-900" />
              <span>
                Saya menyetujui <Link to="/terms" target="_blank" className="font-medium text-indigo-700 underline dark:text-indigo-300">Ketentuan Penggunaan</Link> dan{' '}
                <Link to="/privacy" target="_blank" className="font-medium text-indigo-700 underline dark:text-indigo-300">Kebijakan Privasi</Link>.
              </span>
            </label>

            {error && (
              <div role="alert" className="flex items-start gap-2 rounded-lg bg-red-50 px-3.5 py-2.5 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-400">
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
                <span>{error}</span>
              </div>
            )}

            <button type="submit" disabled={mut.isPending || !ready || enabled === null} className={authPrimaryBtnCls}>
              {mut.isPending ? (<><Spinner className="h-4 w-4 text-white" /> Mengirim…</>) : 'Daftar & kirim verifikasi'}
            </button>
          </form>
        </>
      )}
    </AuthShell>
  )
}
