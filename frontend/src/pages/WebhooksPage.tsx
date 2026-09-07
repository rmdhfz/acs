import { useState } from 'react'
import { Bell, BellOff, Check, ChevronDown, ChevronRight, Copy, Globe, Plus, RefreshCw, Send, TestTube, Trash2, Zap } from 'lucide-react'
import {
  useCreateWebhook,
  useDeleteWebhook,
  useRefs,
  useTestWebhook,
  useUpdateWebhook,
  useWebhookDeliveries,
  useWebhooks,
  type CreateWebhookInput,
  type WebhookDelivery,
  type WebhookSubscription,
} from '../lib/hooks'
import { PageSpinner } from '../components/Spinner'
import { EmptyState } from '../components/EmptyState'
import { Modal } from '../components/Modal'
import { useToast } from '../lib/toast'
import { fmtDatetime } from '../lib/format'
import { LIMITS } from '../lib/limits'

// ---- Badge ----

function DeliveryStatusBadge({ status }: { status: string }) {
  const cls =
    status === 'DELIVERED'
      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
      : status === 'FAILED'
        ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
        : 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
  return (
    <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${cls}`}>
      {status}
    </span>
  )
}

// ---- Delivery log panel ----

function DeliveryPanel({ sub }: { sub: WebhookSubscription }) {
  const { data, isLoading } = useWebhookDeliveries(sub.id)
  const deliveries = data?.data ?? []

  return (
    <div className="mt-2 space-y-1.5">
      {isLoading ? (
        <PageSpinner />
      ) : deliveries.length === 0 ? (
        <p className="py-4 text-center text-xs text-slate-400">Belum ada delivery log</p>
      ) : (
        deliveries.map((d: WebhookDelivery) => (
          <div
            key={d.id}
            className="flex items-start justify-between gap-3 rounded-lg border border-slate-100 bg-slate-50 px-3 py-2 text-xs dark:border-slate-800 dark:bg-slate-900/50"
          >
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <DeliveryStatusBadge status={d.status} />
                <span className="text-slate-400">
                  {fmtDatetime(d.created_at)} · percobaan {d.attempt_count}/{d.max_attempts}
                  {d.response_status ? ` · HTTP ${d.response_status}` : ''}
                </span>
              </div>
              {d.error_message && (
                <p className="mt-1 truncate text-red-500">{d.error_message}</p>
              )}
            </div>
          </div>
        ))
      )}
    </div>
  )
}

// ---- Subscription row ----

function SubscriptionRow({ sub, eventLabel }: { sub: WebhookSubscription; eventLabel: string }) {
  const [expanded, setExpanded] = useState(false)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const updateMut = useUpdateWebhook()
  const deleteMut = useDeleteWebhook()
  const testMut = useTestWebhook()
  const toast = useToast()

  function toggleActive() {
    updateMut.mutate(
      { id: sub.id, input: { name: sub.name, target_url: sub.target_url, is_active: !sub.is_active, description: sub.description ?? undefined } },
      {
        onSuccess: () => toast.success(sub.is_active ? `Webhook "${sub.name}" dinonaktifkan.` : `Webhook "${sub.name}" diaktifkan.`),
        onError: (e: Error) => toast.error(e.message),
      },
    )
  }

  function handleTest() {
    testMut.mutate(sub.id, {
      onSuccess: () => toast.success('Test delivery diantre. Lihat log delivery untuk hasilnya.', 'Terkirim'),
      onError: (e: Error) => toast.error(e.message),
    })
  }

  function handleDelete() {
    deleteMut.mutate(sub.id, {
      onSuccess: () => {
        toast.success(`Webhook "${sub.name}" dihapus.`)
        setShowDeleteConfirm(false)
      },
      onError: (e: Error) => toast.error(e.message),
    })
  }

  function copyUUID() {
    navigator.clipboard.writeText(sub.subscription_uuid).then(
      () => toast.success('UUID disalin ke clipboard.'),
      () => toast.error('Gagal menyalin.'),
    )
  }

  return (
    <>
      <div className="rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {/* Header */}
        <div className="flex items-center gap-3 p-4">
          {/* Status indicator */}
          <div
            className={`flex-shrink-0 h-2.5 w-2.5 rounded-full ${sub.is_active ? 'bg-emerald-500' : 'bg-slate-300 dark:bg-slate-600'}`}
          />

          {/* Info */}
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium text-slate-900 dark:text-slate-100">{sub.name}</span>
              <span className="inline-flex items-center gap-1 rounded-full bg-indigo-50 px-2 py-0.5 text-xs font-medium text-indigo-600 dark:bg-indigo-900/20 dark:text-indigo-400">
                <Zap className="h-2.5 w-2.5" />
                {eventLabel}
              </span>
              {!sub.is_active && (
                <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-500 dark:bg-slate-800 dark:text-slate-400">
                  Nonaktif
                </span>
              )}
            </div>
            <div className="mt-0.5 flex items-center gap-2 text-xs text-slate-400">
              <Globe className="h-3 w-3" />
              <span className="truncate">{sub.target_url}</span>
              <button onClick={copyUUID} className="flex items-center gap-1 hover:text-slate-600 dark:hover:text-slate-300">
                <Copy className="h-2.5 w-2.5" />
                <span className="hidden sm:inline">{sub.subscription_uuid.slice(0, 8)}…</span>
              </button>
            </div>
          </div>

          {/* Actions */}
          <div className="flex flex-shrink-0 items-center gap-1">
            <button
              onClick={handleTest}
              disabled={testMut.isPending || !sub.is_active}
              title="Kirim test delivery"
              className="rounded-lg p-2 text-slate-400 hover:bg-slate-100 hover:text-slate-600 disabled:opacity-40 dark:hover:bg-slate-800 dark:hover:text-slate-300"
            >
              <TestTube className="h-4 w-4" />
            </button>
            <button
              onClick={toggleActive}
              disabled={updateMut.isPending}
              title={sub.is_active ? 'Nonaktifkan' : 'Aktifkan'}
              className="rounded-lg p-2 text-slate-400 hover:bg-slate-100 hover:text-slate-600 disabled:opacity-40 dark:hover:bg-slate-800 dark:hover:text-slate-300"
            >
              {sub.is_active ? <Bell className="h-4 w-4" /> : <BellOff className="h-4 w-4" />}
            </button>
            <button
              onClick={() => setShowDeleteConfirm(true)}
              title="Hapus"
              className="rounded-lg p-2 text-slate-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
            >
              <Trash2 className="h-4 w-4" />
            </button>
            <button
              onClick={() => setExpanded((v) => !v)}
              title={expanded ? 'Tutup delivery log' : 'Buka delivery log'}
              className="rounded-lg p-2 text-slate-400 hover:bg-slate-100 hover:text-slate-600 dark:hover:bg-slate-800 dark:hover:text-slate-300"
            >
              {expanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            </button>
          </div>
        </div>

        {/* Delivery log (collapsible) */}
        {expanded && (
          <div className="border-t border-slate-100 px-4 pb-4 pt-2 dark:border-slate-800">
            <div className="mb-2 flex items-center justify-between">
              <p className="text-xs font-medium text-slate-500 dark:text-slate-400">Log Delivery (50 terbaru)</p>
            </div>
            <DeliveryPanel sub={sub} />
          </div>
        )}
      </div>

      {/* Delete confirm modal */}
      {showDeleteConfirm && (
        <Modal title="Hapus Webhook?" onClose={() => setShowDeleteConfirm(false)}>
          <div className="p-6 pt-2">
          <p className="mt-2 text-sm text-slate-500">
            Webhook <span className="font-medium text-slate-700 dark:text-slate-300">{sub.name}</span> akan dihapus permanen.
            Delivery log yang belum terkirim tidak akan dikirim lagi.
          </p>
          <div className="mt-5 flex justify-end gap-3">
            <button
              onClick={() => setShowDeleteConfirm(false)}
              className="rounded-lg border border-slate-200 px-4 py-2 text-sm font-medium text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
            >
              Batal
            </button>
            <button
              onClick={handleDelete}
              disabled={deleteMut.isPending}
              className="flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-60"
            >
              {deleteMut.isPending ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
              Hapus
            </button>
          </div>
        </div>
        </Modal>
      )}
    </>
  )
}

// ---- Create Form ----

const EMPTY_FORM: CreateWebhookInput = {
  event_type: '',
  name: '',
  target_url: '',
  description: undefined,
}

function CreateWebhookModal({ onClose }: { onClose: () => void }) {
  const { data: eventRefs } = useRefs('ref_webhook_event_types')
  const createMut = useCreateWebhook()
  const toast = useToast()
  const [form, setForm] = useState<CreateWebhookInput>(EMPTY_FORM)
  const [createdSecret, setCreatedSecret] = useState<string | null>(null)

  function field<K extends keyof CreateWebhookInput>(key: K) {
    return {
      value: form[key] ?? '',
      onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>) =>
        setForm((prev) => ({ ...prev, [key]: e.target.value || undefined })),
    }
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!form.event_type || !form.name || !form.target_url) return
    createMut.mutate(form, {
      onSuccess: (data) => {
        if (data.secret) setCreatedSecret(data.secret)
        else {
          toast.success('Webhook dibuat.')
          onClose()
          setForm(EMPTY_FORM)
        }
      },
      onError: (e: Error) => toast.error(e.message),
    })
  }

  function handleCloseSecret() {
    setCreatedSecret(null)
    toast.success('Webhook dibuat. Simpan secret yang tadi ditampilkan — tidak bisa dilihat lagi.')
    onClose()
    setForm(EMPTY_FORM)
  }

  if (createdSecret) {
    return (
      <Modal title="Webhook Dibuat!" onClose={handleCloseSecret}>
        <div className="p-6 pt-2">
          <div className="mb-4 flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-full bg-emerald-100 dark:bg-emerald-900/30">
              <Check className="h-5 w-5 text-emerald-600 dark:text-emerald-400" />
            </div>
            <div>
              <p className="text-xs text-slate-500">Salin secret sekarang — tidak akan ditampilkan lagi</p>
            </div>
          </div>
          <div className="rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-800/50 dark:bg-amber-900/20">
            <p className="mb-1 text-xs font-medium text-amber-700 dark:text-amber-400">HMAC Secret (SHA-256)</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 break-all rounded bg-amber-100/60 px-2 py-1 text-xs text-amber-900 dark:bg-amber-900/30 dark:text-amber-100">
                {createdSecret}
              </code>
              <button
                onClick={() => navigator.clipboard.writeText(createdSecret).then(() => toast.success('Secret disalin.'), () => toast.error('Gagal menyalin.'))}
                className="flex-shrink-0 rounded-lg p-1.5 text-amber-600 hover:bg-amber-100 dark:text-amber-400 dark:hover:bg-amber-900/30"
              >
                <Copy className="h-4 w-4" />
              </button>
            </div>
          </div>
          <p className="mt-3 text-xs text-slate-400">
            Gunakan secret ini untuk memverifikasi HMAC signature di header <code>X-ACS-Signature</code> pada setiap delivery.
          </p>
          <div className="mt-5 flex justify-end">
            <button
              onClick={handleCloseSecret}
              className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-700 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-slate-200"
            >
              Tutup
            </button>
          </div>
        </div>
      </Modal>
    )
  }

  const inputCls =
    'w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 focus:border-indigo-400 focus:outline-none dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100 dark:focus:border-indigo-500'
  const labelCls = 'mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400'

  return (
    <Modal title="Tambah Webhook Baru" onClose={onClose}>
      <form onSubmit={handleSubmit} className="p-6 pt-2">

        <div className="space-y-4">
          <div>
            <label className={labelCls}>Nama *</label>
            <input {...field('name')} required maxLength={LIMITS.webhook.name} placeholder="mis. BSS Fault Notifier" className={inputCls} />
          </div>
          <div>
            <label className={labelCls}>Event Type *</label>
            <select {...field('event_type')} required className={inputCls}>
              <option value="">— pilih event —</option>
              {(eventRefs ?? []).map((r) => (
                <option key={r.id} value={r.code}>{r.code}</option>
              ))}
            </select>
          </div>
          <div>
            <label className={labelCls}>Target URL *</label>
            <input {...field('target_url')} required type="url" maxLength={LIMITS.webhook.targetUrl} placeholder="https://bss.example.com/acs-webhook" className={inputCls} />
          </div>
          <div>
            <label className={labelCls}>Deskripsi</label>
            <textarea
              value={form.description ?? ''}
              onChange={(e) => setForm((prev) => ({ ...prev, description: e.target.value || undefined }))}
              maxLength={LIMITS.description}
              rows={2}
              className={inputCls}
              placeholder="opsional"
            />
          </div>
        </div>

        <div className="mt-6 flex justify-end gap-3">
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-slate-200 px-4 py-2 text-sm font-medium text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
          >
            Batal
          </button>
          <button
            type="submit"
            disabled={createMut.isPending}
            className="flex items-center gap-2 rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-60"
          >
            {createMut.isPending ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
            Buat Webhook
          </button>
        </div>
      </form>
    </Modal>
  )
}

// ---- Main Page ----

export function WebhooksPage() {
  const [showCreate, setShowCreate] = useState(false)
  const { data: eventRefs } = useRefs('ref_webhook_event_types')
  const { data, isLoading } = useWebhooks()
  const subscriptions = data?.data ?? []

  function eventLabel(id: number) {
    return eventRefs?.find((r) => r.id === id)?.code ?? `#${id}`
  }

  return (
    <div className="mx-auto max-w-4xl px-6 py-8">
      {/* Header */}
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Webhooks</h1>
          <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
            Kelola langganan webhook keluar — event ACS dikirim otomatis ke sistem BSS/OSS/NMS Anda.
          </p>
        </div>
        <button
          id="webhook-create-btn"
          onClick={() => setShowCreate(true)}
          className="flex items-center gap-2 rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700"
        >
          <Plus className="h-4 w-4" />
          Tambah Webhook
        </button>
      </div>

      {/* Info banner */}
      <div className="mb-6 rounded-xl border border-indigo-100 bg-indigo-50 p-4 dark:border-indigo-900/30 dark:bg-indigo-900/10">
        <div className="flex gap-3">
          <Send className="mt-0.5 h-4 w-4 flex-shrink-0 text-indigo-500" />
          <div className="text-xs text-indigo-700 dark:text-indigo-300">
            <strong>HMAC Signature</strong> dikirim di header <code className="rounded bg-indigo-100 px-1 dark:bg-indigo-900/30">X-ACS-Signature</code>.
            {' '}Format: <code className="rounded bg-indigo-100 px-1 dark:bg-indigo-900/30">sha256=&lt;hex&gt;</code>.
            {' '}Verifikasi menggunakan secret yang ditampilkan sekali saat webhook dibuat.
            {' '}Worker retry otomatis dengan backoff eksponensial (1m, 2m, 4m… hingga 30m, max 6 percobaan).
          </div>
        </div>
      </div>

      {/* List */}
      {isLoading ? (
        <PageSpinner />
      ) : subscriptions.length === 0 ? (
        <EmptyState icon={Bell} title="Belum ada webhook" description="Buat webhook untuk menerima notifikasi event dari ACS." />
      ) : (
        <div className="space-y-3">
          {subscriptions.map((sub: WebhookSubscription) => (
            <SubscriptionRow key={sub.id} sub={sub} eventLabel={eventLabel(sub.event_type_id)} />
          ))}
        </div>
      )}

      {/* Event types reference */}
      {(eventRefs?.length ?? 0) > 0 && (
        <div className="mt-8 rounded-xl border border-slate-100 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-900/50">
          <h3 className="mb-3 text-xs font-semibold uppercase tracking-wide text-slate-500">Event Types Tersedia</h3>
          <div className="space-y-1.5">
            {(eventRefs ?? []).map((r) => (
              <div key={r.id} className="flex items-start gap-2 text-xs">
                <Zap className="mt-0.5 h-3 w-3 flex-shrink-0 text-indigo-500" />
                <div>
                  <code className="font-medium text-slate-700 dark:text-slate-300">{r.code}</code>
                  <span className="ml-2 text-slate-400">{r.name}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {showCreate && <CreateWebhookModal onClose={() => setShowCreate(false)} />}
    </div>
  )
}

