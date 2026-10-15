import { fetchAllPages, type ExportColumn } from '../lib/exporter'
import type { RefLookup } from '../lib/types'
import type { WebhookSubscription } from '../lib/hooks'

// Ekspor WEBHOOK (GET /webhooks). Hanya ekspor — tidak ada impor karena
// langganan webhook memuat rahasia penandatangan (HMAC secret). Secret tidak
// pernah dikirim backend pada list dan tidak ada di file ekspor; bagian URL yang
// bisa berisi rahasia (user:password@host, parameter token/key/secret/…) disamarkan.

const SENSITIVE_PARAM = /token|key|secret|pass|pwd|sig|auth|credential/i

/** redactUrl menyamarkan kredensial yang tertanam di URL tujuan. */
export function redactUrl(raw: string): string {
  try {
    const u = new URL(raw)
    if (u.password) u.password = '***'
    for (const k of [...u.searchParams.keys()]) {
      if (SENSITIVE_PARAM.test(k)) u.searchParams.set(k, '***')
    }
    // URL.toString() mengubah "***" menjadi "%2A%2A%2A"; kembalikan agar terbaca
    return u.toString().replace(/%2A%2A%2A/g, '***')
  } catch {
    return raw
  }
}

export function webhookExportColumns(eventTypes: RefLookup[] | undefined): ExportColumn<WebhookSubscription>[] {
  return [
    { key: 'name', value: (w) => w.name },
    { key: 'event_type', value: (w) => eventTypes?.find((r) => r.id === w.event_type_id)?.code ?? String(w.event_type_id) },
    { key: 'target_url', value: (w) => redactUrl(w.target_url) },
    { key: 'is_active', value: (w) => w.is_active },
    { key: 'description', value: (w) => w.description },
    { key: 'tenant_id', value: (w) => w.tenant_id },
    { key: 'subscription_uuid', value: (w) => w.subscription_uuid },
    { key: 'created_at', value: (w) => w.created_at },
  ]
}

export async function loadWebhookExportRows(ctx: { signal: AbortSignal; onProgress: (done: number, total: number) => void }): Promise<WebhookSubscription[]> {
  const { rows, truncated } = await fetchAllPages<WebhookSubscription>('/webhooks', {}, { signal: ctx.signal, onProgress: ctx.onProgress })
  if (truncated) throw new Error('Data melebihi 50.000 baris.')
  return rows
}
