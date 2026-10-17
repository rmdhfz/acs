import { Link } from 'react-router-dom'
import { Compass } from 'lucide-react'
import { AuthShell } from '../components/AuthShell'

// 404 sungguhan (bukan redirect diam-diam ke beranda): tautan salah tampak jelas bagi pengguna dan tidak dianggap
// duplikat beranda oleh mesin pencari. Server statis tetap menjawab 200 untuk rute SPA, jadi halaman ini juga di-noindex
// (useRouteSeo di App.tsx).
export function NotFoundPage() {
  return (
    <AuthShell>
      <div className="text-center">
        <span className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-indigo-50 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300">
          <Compass className="h-7 w-7" strokeWidth={1.75} aria-hidden="true" />
        </span>
        <h1 className="mt-5 text-2xl font-normal text-slate-900 dark:text-slate-100">Halaman tidak ditemukan</h1>
        <p className="mt-2 text-sm leading-relaxed text-slate-600 dark:text-slate-400">
          Alamat yang Anda buka tidak ada atau sudah dipindahkan. Periksa kembali tautannya, atau kembali ke beranda.
        </p>
        <div className="mt-7 flex flex-wrap justify-center gap-3">
          <Link to="/" className="inline-flex h-11 items-center rounded-full bg-indigo-600 px-6 text-sm font-medium text-white hover:bg-indigo-700">
            Ke beranda
          </Link>
          <Link to="/login" className="inline-flex h-11 items-center rounded-full border border-slate-300 px-6 text-sm font-medium text-indigo-700 hover:bg-indigo-50 dark:border-slate-600 dark:text-indigo-300 dark:hover:bg-slate-800">
            Masuk
          </Link>
        </div>
      </div>
    </AuthShell>
  )
}
