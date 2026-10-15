import { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { copyText } from '../lib/clipboard'
import { useI18n } from '../lib/i18n'

// Tombol ikon kecil "salin" dengan umpan balik centang 1,5 detik.
export function CopyButton({ value, label }: { value: string; label?: string }) {
  const { t } = useI18n()
  const [done, setDone] = useState(false)

  async function onClick() {
    if (await copyText(value)) {
      setDone(true)
      window.setTimeout(() => setDone(false), 1500)
    }
  }

  return (
    <button
      type="button"
      onClick={onClick}
      title={done ? t('profile.copied') : (label ?? t('profile.copy'))}
      aria-label={label ?? t('profile.copy')}
      className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-slate-400 transition-colors hover:bg-slate-100 hover:text-slate-700 dark:hover:bg-slate-800 dark:hover:text-slate-200"
    >
      {done ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
    </button>
  )
}
