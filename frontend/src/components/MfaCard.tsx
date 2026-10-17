import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Download, ShieldCheck, ShieldOff } from 'lucide-react'
import { Modal } from './Modal'
import { CopyButton } from './CopyButton'
import { Spinner } from './Spinner'
import { ApiError, setAuthToken } from '../lib/api'
import { copyText } from '../lib/clipboard'
import { useToast } from '../lib/toast'
import { useDisableMfa, useEnableMfa, useMfaStatus, useSetupMfa } from '../lib/hooks'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const labelCls = 'mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400'
const primaryBtn =
  'rounded-lg bg-indigo-600 px-3.5 py-2 text-sm font-semibold text-white transition-colors hover:bg-indigo-500 disabled:opacity-50'
const ghostBtn =
  'rounded-lg border border-slate-300 px-3.5 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800'

// Kartu "Verifikasi dua langkah (TOTP)" di halaman Profil: status, aktifkan (QR + kode pemulihan),
// nonaktifkan (password + kode). Seluruh logika keamanan ada di backend (usecase/auth/mfa.go).
export function MfaCard() {
  const { data, isLoading, isError } = useMfaStatus()
  const [setupOpen, setSetupOpen] = useState(false)
  const [disableOpen, setDisableOpen] = useState(false)

  return (
    <div className="mt-5 rounded-lg border border-slate-200 p-3.5 dark:border-slate-700">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 gap-3">
          {data?.enabled ? (
            <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />
          ) : (
            <ShieldOff className="mt-0.5 h-4 w-4 shrink-0 text-slate-500 dark:text-slate-400" />
          )}
          <div>
            <p className="text-sm font-medium text-slate-800 dark:text-slate-200">
              Verifikasi dua langkah (TOTP)
              {data?.enabled && (
                <span className="ml-2 rounded bg-emerald-100 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-400">
                  Aktif
                </span>
              )}
            </p>
            <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">
              {isLoading && 'Memuat status…'}
              {isError && 'Status MFA tidak dapat dimuat.'}
              {data && !data.available && 'Fitur ini belum diaktifkan di instalasi ACS Anda.'}
              {data?.available && !data.enabled &&
                'Tambahkan lapisan keamanan: selain password, login meminta kode 6 digit dari aplikasi authenticator (Google Authenticator, Microsoft Authenticator, Authy, dll.).'}
              {data?.enabled && `Login meminta kode dari aplikasi authenticator. Sisa kode pemulihan: ${data.recovery_codes_left}.`}
            </p>
            {data?.enabled && data.recovery_codes_left <= 2 && (
              <p className="mt-1 text-xs font-medium text-amber-600 dark:text-amber-400">
                Kode pemulihan hampir habis — nonaktifkan lalu aktifkan ulang MFA untuk membuat set baru.
              </p>
            )}
          </div>
        </div>
        {data?.available && (
          data.enabled ? (
            <button onClick={() => setDisableOpen(true)} className={ghostBtn}>
              Nonaktifkan
            </button>
          ) : (
            <button onClick={() => setSetupOpen(true)} className={primaryBtn}>
              Aktifkan
            </button>
          )
        )}
      </div>
      {setupOpen && <SetupModal onClose={() => setSetupOpen(false)} />}
      {disableOpen && <DisableModal onClose={() => setDisableOpen(false)} />}
    </div>
  )
}

function SetupModal({ onClose }: { onClose: () => void }) {
  const setup = useSetupMfa()
  const enable = useEnableMfa()
  const toast = useToast()
  const [secret, setSecret] = useState<string | null>(null)
  const [qr, setQr] = useState<string | null>(null)
  const [code, setCode] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [codes, setCodes] = useState<string[] | null>(null)
  const [saved, setSaved] = useState(false)
  const started = useRef(false)

  // Rahasia dibuat SEKALI saat modal dibuka (ref mencegah panggilan ganda StrictMode yang
  // membuat dua rahasia berbeda).
  useEffect(() => {
    if (started.current) return
    started.current = true
    setup.mutateAsync().then(
      async (res) => {
        setSecret(res.secret)
        const QRCode = (await import('qrcode')).default
        setQr(await QRCode.toDataURL(res.otpauth_uri, { margin: 1, width: 208, errorCorrectionLevel: 'M' }))
      },
      (err) => setError(err instanceof ApiError ? err.message : 'Gagal memulai setup MFA'),
    )
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function handleEnable(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (!/^\d{6}$/.test(code)) return setError('Masukkan 6 digit kode dari aplikasi authenticator.')
    try {
      const res = await enable.mutateAsync(code)
      setCodes(res.recovery_codes)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal mengaktifkan MFA')
      setCode('')
    }
  }

  function downloadCodes() {
    if (!codes) return
    const blob = new Blob([`Kode pemulihan ACS (sekali pakai)\n\n${codes.join('\n')}\n`], { type: 'text/plain' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = 'acs-kode-pemulihan.txt'
    a.click()
    URL.revokeObjectURL(a.href)
  }

  if (codes) {
    return (
      <Modal title="Simpan kode pemulihan" onClose={onClose} dismissable={false}>
        <div className="space-y-4">
          <p className="text-sm text-slate-600 dark:text-slate-400">
            MFA aktif. Kode di bawah dipakai <strong>sekali</strong> bila Anda kehilangan perangkat authenticator. Kode hanya
            ditampilkan sekarang — simpan di tempat aman (mis. password manager).
          </p>
          <ul className="grid grid-cols-2 gap-2 rounded-lg bg-slate-50 p-3 font-mono text-sm dark:bg-slate-800/60" aria-label="Kode pemulihan">
            {codes.map((c) => (
              <li key={c} className="tabular-nums text-slate-800 dark:text-slate-200">{c}</li>
            ))}
          </ul>
          <div className="flex flex-wrap gap-2">
            <button type="button" className={ghostBtn} onClick={async () => (await copyText(codes.join('\n'))) && toast.success('Kode pemulihan disalin')}>
              Salin semua
            </button>
            <button type="button" className={`${ghostBtn} inline-flex items-center gap-1.5`} onClick={downloadCodes}>
              <Download className="h-4 w-4" /> Unduh .txt
            </button>
          </div>
          <label className="flex cursor-pointer items-start gap-2 text-sm text-slate-600 dark:text-slate-400">
            <input type="checkbox" checked={saved} onChange={(e) => setSaved(e.target.checked)} className="mt-0.5 h-4 w-4 rounded border-slate-300 text-indigo-600" />
            Saya sudah menyimpan kode pemulihan ini.
          </label>
          <div className="flex justify-end">
            <button type="button" disabled={!saved} onClick={onClose} className={primaryBtn}>
              Selesai
            </button>
          </div>
        </div>
      </Modal>
    )
  }

  return (
    <Modal title="Aktifkan verifikasi dua langkah" onClose={onClose}>
      <form onSubmit={handleEnable} className="space-y-4">
        <ol className="list-decimal space-y-1 pl-5 text-sm text-slate-600 dark:text-slate-400">
          <li>Buka aplikasi authenticator di ponsel Anda.</li>
          <li>Pindai kode QR di bawah (atau masukkan kunci manual).</li>
          <li>Ketik kode 6 digit yang tampil di aplikasi.</li>
        </ol>
        <div className="flex min-h-[13rem] flex-col items-center justify-center gap-2">
          {qr ? (
            <img src={qr} alt="Kode QR untuk aplikasi authenticator" width={208} height={208} className="rounded-lg border border-slate-200 bg-white p-1 dark:border-slate-700" />
          ) : error && !secret ? null : (
            <Spinner className="h-5 w-5 text-slate-500 dark:text-slate-400" />
          )}
          {secret && (
            <div className="flex items-center gap-1.5">
              <code className="select-all break-all rounded bg-slate-100 px-2 py-1 font-mono text-xs text-slate-700 dark:bg-slate-800 dark:text-slate-300">{secret}</code>
              <CopyButton value={secret} label="Salin kunci" />
            </div>
          )}
        </div>
        <div>
          <label htmlFor="mfa-enable-code" className={labelCls}>Kode 6 digit</label>
          <input
            id="mfa-enable-code"
            value={code}
            onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern="[0-9]{6}"
            maxLength={6}
            placeholder="123456"
            required
            disabled={!secret}
            className={`${inputCls} text-center font-mono tracking-[0.4em]`}
          />
        </div>
        {error && <p role="alert" className="text-sm text-red-600 dark:text-red-400">{error}</p>}
        <div className="flex justify-end gap-2.5 pt-1">
          <button type="button" onClick={onClose} className={ghostBtn}>Batal</button>
          <button type="submit" disabled={!secret || code.length !== 6 || enable.isPending} className={primaryBtn}>
            {enable.isPending ? 'Memverifikasi…' : 'Aktifkan'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

function DisableModal({ onClose }: { onClose: () => void }) {
  const disable = useDisableMfa()
  const toast = useToast()
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      const res = await disable.mutateAsync({ password, code: code.trim() })
      setAuthToken(res.access_token) // sesi lama dicabut server; pakai JWT baru
      toast.success('Verifikasi dua langkah dinonaktifkan.')
      onClose()
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) setError('Password salah.')
      else setError(err instanceof ApiError ? err.message : 'Gagal menonaktifkan MFA')
    }
  }

  return (
    <Modal title="Nonaktifkan verifikasi dua langkah" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <p className="text-sm text-slate-600 dark:text-slate-400">
          Demi keamanan, konfirmasi dengan password DAN kode dari aplikasi authenticator (atau satu kode pemulihan).
          Sesi di perangkat lain akan berakhir.
        </p>
        <div>
          <label htmlFor="mfa-dis-pw" className={labelCls}>Password</label>
          <input maxLength={128} id="mfa-dis-pw" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required className={inputCls} />
        </div>
        <div>
          <label htmlFor="mfa-dis-code" className={labelCls}>Kode authenticator / kode pemulihan</label>
          <input id="mfa-dis-code" value={code} onChange={(e) => setCode(e.target.value)} autoComplete="one-time-code" required minLength={6} maxLength={16} placeholder="123456 atau XXXXX-XXXXX" className={`${inputCls} font-mono`} />
        </div>
        {error && <p role="alert" className="text-sm text-red-600 dark:text-red-400">{error}</p>}
        <div className="flex justify-end gap-2.5 pt-1">
          <button type="button" onClick={onClose} className={ghostBtn}>Batal</button>
          <button type="submit" disabled={!password || code.trim().length < 6 || disable.isPending} className="rounded-full bg-red-600 px-3.5 py-2 text-sm font-semibold text-white hover:bg-red-500 disabled:opacity-50">
            {disable.isPending ? 'Memproses…' : 'Nonaktifkan'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
