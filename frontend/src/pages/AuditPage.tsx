import { useMemo, useState } from 'react'
import { ScrollText, User, Cog } from 'lucide-react'
import { EmptyState } from '../components/EmptyState'
import { PageSpinner } from '../components/Spinner'
import { useI18n } from '../lib/i18n'
import { useActivityLog } from '../lib/hooks'
import { formatDateTime, formatRelativeTime } from '../lib/format'

const selectCls =
  'rounded-lg border border-slate-300 px-3 py-1.5 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'

// Warna badge per prefix aksi — hijau untuk create/apply, merah untuk delete/fail,
// biru untuk update, abu untuk lainnya.
function actionTone(action: string): string {
  const a = action.toUpperCase()
  if (a.includes('DELETE') || a.includes('REVOKE') || a.includes('FAILED') || a.includes('REMOVE'))
    return 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300'
  if (a.includes('CREATE') || a.includes('ADD') || a.includes('APPLY') || a.includes('ASSIGN'))
    return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
  if (a.includes('UPDATE') || a.includes('UPSERT') || a.includes('REPLACE') || a.includes('SET'))
    return 'bg-blue-100 text-blue-700 dark:bg-blue-500/15 dark:text-blue-300'
  return 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300'
}

export default function AuditPage() {
  const { t } = useI18n()
  const [action, setAction] = useState('')
  const [entityType, setEntityType] = useState('')
  const { data, isLoading } = useActivityLog({
    action: action || undefined,
    entity_type: entityType || undefined,
  })
  const rows = useMemo(() => data?.data ?? [], [data])

  // Opsi filter diturunkan dari data yang termuat (tanpa endpoint terpisah).
  const { actions, entities } = useMemo(() => {
    const a = new Set<string>()
    const e = new Set<string>()
    for (const r of rows) {
      a.add(r.action)
      e.add(r.entity_type)
    }
    return { actions: [...a].sort(), entities: [...e].sort() }
  }, [rows])

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      <div className="mb-4">
        <h1 className="flex items-center gap-2 text-xl font-semibold text-slate-900 dark:text-slate-100">
          <ScrollText className="h-5 w-5 text-slate-400" /> {t('audit.title')}
        </h1>
        <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">{t('audit.subtitle')}</p>
      </div>

      <div className="mb-4 flex flex-wrap gap-2">
        <select value={action} onChange={(e) => setAction(e.target.value)} className={selectCls}>
          <option value="">{t('audit.filterAction')}</option>
          {actions.map((a) => (
            <option key={a} value={a}>{a}</option>
          ))}
        </select>
        <select value={entityType} onChange={(e) => setEntityType(e.target.value)} className={selectCls}>
          <option value="">{t('audit.filterEntity')}</option>
          {entities.map((e) => (
            <option key={e} value={e}>{e}</option>
          ))}
        </select>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {isLoading ? (
          <PageSpinner />
        ) : rows.length === 0 ? (
          <EmptyState icon={ScrollText} title={t('audit.empty')} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                  <th className="px-5 py-3">{t('audit.who')}</th>
                  <th className="px-5 py-3">{t('audit.action')}</th>
                  <th className="px-5 py-3">{t('audit.entity')}</th>
                  <th className="px-5 py-3">{t('audit.ip')}</th>
                  <th className="px-5 py-3">{t('audit.when')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {rows.map((r) => (
                  <tr key={r.id}>
                    <td className="px-5 py-3">
                      <span className="flex items-center gap-1.5 text-slate-700 dark:text-slate-300">
                        {r.username ? <User className="h-3.5 w-3.5 text-slate-400" /> : <Cog className="h-3.5 w-3.5 text-slate-400" />}
                        {r.username ?? <span className="italic text-slate-400">sistem</span>}
                      </span>
                    </td>
                    <td className="px-5 py-3">
                      <span className={`rounded px-1.5 py-0.5 font-mono text-[11px] font-medium ${actionTone(r.action)}`}>{r.action}</span>
                      {r.description && <p className="mt-0.5 max-w-md truncate text-xs text-slate-400" title={r.description}>{r.description}</p>}
                    </td>
                    <td className="px-5 py-3 text-slate-500 dark:text-slate-400">
                      {r.entity_type}
                      {r.entity_id != null && <span className="text-slate-400"> #{r.entity_id}</span>}
                    </td>
                    <td className="px-5 py-3 font-mono text-xs text-slate-400">{r.ip_address ?? '—'}</td>
                    <td className="whitespace-nowrap px-5 py-3 text-slate-500 dark:text-slate-400" title={formatDateTime(r.created_at)}>
                      {formatRelativeTime(r.created_at)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
