import { useI18n } from '../lib/i18n'

export const CREDIT_NAME = 'Hafiz Ramadhan'
export const CREDIT_URL = 'https://linkedin.com/in/hfzrmd'

// CreditFooter — atribusi pembuat; nama membuka profil LinkedIn di tab baru
// (noopener noreferrer: halaman tujuan tidak mendapat akses ke window.opener).
export function CreditFooter({ className = '' }: { className?: string }) {
  const { t } = useI18n()
  return (
    <p className={`text-xs text-slate-500 dark:text-slate-400 ${className}`}>
      © {new Date().getFullYear()} ACS Console · {t('footer.madeBy')}{' '}
      <a
        href={CREDIT_URL}
        target="_blank"
        rel="noopener noreferrer"
        className="font-medium text-slate-500 underline-offset-2 transition-colors hover:text-indigo-600 hover:underline dark:text-slate-400 dark:hover:text-indigo-400"
      >
        {CREDIT_NAME}
      </a>
    </p>
  )
}
