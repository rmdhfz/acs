import { useState } from 'react'
import { ArrowLeftRight, Copy, Download, Play, Square } from 'lucide-react'
import { PageSpinner } from './Spinner'
import { EmptyState } from './EmptyState'
import { api, ApiError } from '../lib/api'
import { formatDateTime } from '../lib/format'
import { useToast } from '../lib/toast'
import { useCaptureMessage, useCaptureStatus, useCaptures, useDisableCapture, useEnableCapture } from '../lib/hooks'

// Capture sesi CWMP per perangkat: untuk mendiagnosis kuirk firmware (mis. Fault 9002) tanpa menebak. Hanya ADMIN/SUPERADMIN.
// Nilai rahasia sudah disamarkan di server sebelum disimpan; di sini hanya menampilkan.

const PAGE_SIZE = 25
const DURATIONS = [15, 30, 60] as const

const btnCls =
  'inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border border-slate-300 bg-white px-3 py-1.5 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 disabled:opacity-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800'
const primaryBtnCls =
  'inline-flex items-center gap-1.5 whitespace-nowrap rounded-full bg-indigo-600 px-4 py-1.5 text-sm font-medium text-white transition-colors hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-indigo-300 dark:text-slate-900 dark:hover:bg-indigo-200'

// Indentasi sederhana agar XML satu baris terbaca; tidak mengubah isi teks. Elemen daun (<a>teks</a>) tetap satu baris.
export function prettyXml(xml: string): string {
  const tokens = xml.replace(/>\s+</g, '><').match(/<[^>]+>|[^<]+/g) ?? []
  const nameOf = (t: string) => /^<\/?([^\s/>]+)/.exec(t)?.[1]
  const lines: string[] = []
  let depth = 0
  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i]
    const isTag = t.startsWith('<')
    if (!isTag) {
      lines.push('  '.repeat(depth) + t.trim())
    } else if (t.startsWith('</')) {
      depth = Math.max(0, depth - 1)
      lines.push('  '.repeat(depth) + t)
    } else if (t.startsWith('<?') || t.startsWith('<!') || t.endsWith('/>')) {
      lines.push('  '.repeat(depth) + t)
    } else if (tokens[i + 1] !== undefined && !tokens[i + 1].startsWith('<') && tokens[i + 2] === '</' + nameOf(t) + '>') {
      lines.push('  '.repeat(depth) + t + tokens[i + 1] + tokens[i + 2]) // elemen daun
      i += 2
    } else if (tokens[i + 1] === '</' + nameOf(t) + '>') {
      lines.push('  '.repeat(depth) + t + tokens[i + 1]) // elemen kosong
      i += 1
    } else {
      lines.push('  '.repeat(depth) + t)
      depth++
    }
  }
  return lines.join('\n')
}

function remaining(until: string | null, nowMs: number): string {
  if (!until) return ''
  const s = Math.max(0, Math.round((new Date(until).getTime() - nowMs) / 1000))
  return `${Math.floor(s / 60)} mnt ${String(s % 60).padStart(2, '0')} dtk`
}

export function SessionCapture({ deviceId }: { deviceId: number }) {
  const toast = useToast()
  const status = useCaptureStatus(deviceId)
  const active = status.data?.active ?? false
  const [page, setPage] = useState(1)
  const list = useCaptures(deviceId, page, PAGE_SIZE, active)
  const enable = useEnableCapture()
  const disable = useDisableCapture()
  const [minutes, setMinutes] = useState<number>(15)
  const [openId, setOpenId] = useState<string | null>(null)
  const message = useCaptureMessage(deviceId, openId)
  // eslint-disable-next-line react-hooks/purity -- sisa waktu hanya tampilan; diperbarui tiap refetch status
  const nowMs = Date.now()

  const rows = list.data?.data ?? []
  const total = list.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const onEnable = () =>
    enable.mutate(
      { deviceId, minutes },
      {
        onSuccess: () => toast.success(`Capture dinyalakan ${minutes} menit. Tunggu Inform berikutnya atau klik "Refresh ONT".`, 'Berhasil'),
        onError: (e) => toast.error(e instanceof ApiError ? e.message : 'Gagal menyalakan capture'),
      },
    )
  const onDisable = () =>
    disable.mutate(deviceId, {
      onSuccess: () => toast.success('Capture dimatikan.', 'Berhasil'),
      onError: (e) => toast.error(e instanceof ApiError ? e.message : 'Gagal mematikan capture'),
    })
  const onExport = async () => {
    try {
      const blob = await api.blob(`/devices/${deviceId}/captures/export`)
      const a = document.createElement('a')
      a.href = URL.createObjectURL(blob)
      a.download = `capture-device-${deviceId}.ndjson`
      a.click()
      URL.revokeObjectURL(a.href)
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : 'Gagal mengunduh capture')
    }
  }
  const onCopy = async () => {
    if (!message.data) return
    try {
      await navigator.clipboard.writeText(message.data.body ?? '')
      toast.success('Isi pesan disalin.', 'Berhasil')
    } catch {
      toast.error('Browser menolak akses clipboard.')
    }
  }

  if (status.isLoading) return <PageSpinner />

  return (
    <div className="space-y-4">
      <section className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900" aria-label="Kontrol capture">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 className="flex items-center gap-2 text-sm font-medium text-slate-900 dark:text-slate-100">
              <ArrowLeftRight className="h-4 w-4" aria-hidden="true" /> Capture sesi CWMP
              <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${active ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-300' : 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-400'}`}>
                {active ? `Aktif · sisa ${remaining(status.data?.until ?? null, nowMs)}` : 'Tidak aktif'}
              </span>
            </h3>
            <p className="mt-1 max-w-2xl text-xs text-slate-500 dark:text-slate-400">
              Merekam pesan SOAP perangkat ini (CPE → ACS dan ACS → CPE) untuk diagnosis firmware. <b>Password/passphrase/PSK disamarkan</b> sebelum disimpan;
              header HTTP tidak disimpan. Hanya {200} pesan terbaru yang disimpan, dibuang setelah 7 hari, dan capture berhenti sendiri saat waktunya habis.
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <label htmlFor="cap-min" className="sr-only">Lama capture (menit)</label>
            <select
              id="cap-min"
              value={minutes}
              onChange={(e) => setMinutes(Number(e.target.value))}
              className="rounded-lg border border-slate-300 bg-white px-2 py-1.5 text-sm dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100"
            >
              {DURATIONS.map((m) => (
                <option key={m} value={m}>{m} menit</option>
              ))}
            </select>
            <button type="button" onClick={onEnable} disabled={enable.isPending} className={primaryBtnCls}>
              <Play className="h-4 w-4" /> {active ? 'Perpanjang' : 'Nyalakan'}
            </button>
            {active && (
              <button type="button" onClick={onDisable} disabled={disable.isPending} className={btnCls}>
                <Square className="h-4 w-4" /> Matikan
              </button>
            )}
            <button type="button" onClick={() => void onExport()} disabled={total === 0} className={btnCls}>
              <Download className="h-4 w-4" /> Unduh NDJSON
            </button>
          </div>
        </div>
      </section>

      {list.isLoading ? (
        <PageSpinner />
      ) : rows.length === 0 ? (
        <EmptyState
          icon={ArrowLeftRight}
          title="Belum ada pesan terekam"
          description={active ? 'Menunggu perangkat Inform. Klik "Refresh ONT" di header agar ONT segera Inform.' : 'Nyalakan capture, lalu picu Inform perangkat.'}
        />
      ) : (
        <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
          <div className="table-scroll">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                  <th className="px-4 py-3">Waktu</th>
                  <th className="px-4 py-3">Arah</th>
                  <th className="px-4 py-3">RPC</th>
                  <th className="px-4 py-3">HTTP</th>
                  <th className="px-4 py-3">Ukuran</th>
                  <th className="px-4 py-3">Sesi</th>
                  <th className="px-4 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {rows.map((m) => (
                  <tr key={m.id} className={`text-slate-700 dark:text-slate-300 ${openId === m.id ? 'bg-indigo-50/60 dark:bg-indigo-500/10' : ''}`}>
                    <td className="whitespace-nowrap px-4 py-2.5">{formatDateTime(m.created_at)}</td>
                    <td className="whitespace-nowrap px-4 py-2.5">{m.from_cpe ? 'CPE → ACS' : 'ACS → CPE'}</td>
                    <td className="px-4 py-2.5 font-mono text-xs">{m.rpc_name}</td>
                    <td className="px-4 py-2.5">{m.http_status ?? '-'}</td>
                    <td className="whitespace-nowrap px-4 py-2.5 tabular-nums">
                      {m.body_bytes.toLocaleString('id-ID')} B{m.body_truncated && <span className="ml-1 text-xs text-amber-700 dark:text-amber-400">(dipotong)</span>}
                    </td>
                    <td className="px-4 py-2.5 font-mono text-xs text-slate-500 dark:text-slate-400">{m.session_ref ?? '-'}</td>
                    <td className="px-4 py-2.5 text-right">
                      <button type="button" onClick={() => setOpenId(openId === m.id ? null : m.id)} className="rounded-full px-3 py-1 text-sm font-medium text-indigo-700 hover:bg-indigo-50 dark:text-indigo-300 dark:hover:bg-indigo-500/10">
                        {openId === m.id ? 'Tutup' : 'Lihat'}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {totalPages > 1 && (
            <div className="flex items-center justify-between border-t border-slate-200 px-4 py-3 text-sm dark:border-slate-800">
              <button type="button" disabled={page <= 1} onClick={() => setPage((p) => p - 1)} className="rounded-full px-3 py-1.5 font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-40 dark:text-slate-300 dark:hover:bg-slate-800">Sebelumnya</button>
              <span className="text-slate-600 dark:text-slate-400">Halaman {page} dari {totalPages}</span>
              <button type="button" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)} className="rounded-full px-3 py-1.5 font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-40 dark:text-slate-300 dark:hover:bg-slate-800">Berikutnya</button>
            </div>
          )}
        </div>
      )}

      {openId && (
        <section className="rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900" aria-label="Isi pesan">
          <div className="flex items-center justify-between border-b border-slate-100 px-5 py-3 dark:border-slate-800">
            <h3 className="text-sm font-medium text-slate-900 dark:text-slate-100">
              {message.data ? `${message.data.from_cpe ? 'CPE → ACS' : 'ACS → CPE'} · ${message.data.rpc_name}` : 'Memuat…'}
            </h3>
            <button type="button" onClick={() => void onCopy()} disabled={!message.data} className={btnCls}>
              <Copy className="h-4 w-4" /> Salin
            </button>
          </div>
          <pre className="max-h-[32rem] overflow-auto p-5 font-mono text-xs leading-relaxed text-slate-800 dark:text-slate-200">
            {message.isLoading ? '…' : message.data?.body ? prettyXml(message.data.body) : '(pesan kosong — mis. POST kosong atau respons 204)'}
          </pre>
        </section>
      )}
    </div>
  )
}
