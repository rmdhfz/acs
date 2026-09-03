import { useI18n } from '../lib/i18n'
import { scorePassword } from '../lib/password'

const BAR_COLOR = ['bg-red-500', 'bg-red-500', 'bg-amber-500', 'bg-emerald-500', 'bg-emerald-600']
const TEXT_COLOR = [
  'text-red-600 dark:text-red-400',
  'text-red-600 dark:text-red-400',
  'text-amber-600 dark:text-amber-400',
  'text-emerald-600 dark:text-emerald-400',
  'text-emerald-600 dark:text-emerald-400',
]
const LABEL_KEY = ['pw.veryWeak', 'pw.weak', 'pw.fair', 'pw.strong', 'pw.veryStrong']

export function PasswordStrengthBar({ password }: { password: string }) {
  const { t } = useI18n()
  const { score, hints } = scorePassword(password)
  if (!password) return null

  return (
    <div className="mt-1.5">
      <div className="flex gap-1">
        {[0, 1, 2, 3].map((i) => (
          <div
            key={i}
            className={`h-1 flex-1 rounded-full transition-colors ${i < score ? BAR_COLOR[score] : 'bg-slate-200 dark:bg-slate-700'}`}
          />
        ))}
      </div>
      <p className={`mt-1 text-xs ${TEXT_COLOR[score]}`}>
        {t(LABEL_KEY[score])}
        {hints.length > 0 && <span className="text-slate-400 dark:text-slate-500"> — {hints.join(', ')}</span>}
      </p>
    </div>
  )
}
