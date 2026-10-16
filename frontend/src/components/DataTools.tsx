import { useState } from 'react'
import { Upload } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { ExportMenu, type ExportContext } from './ExportMenu'
import { ImportWizard } from './ImportWizard'
import type { ExportColumn } from '../lib/exporter'
import type { ImportAdapter } from '../lib/importer'

interface Props<T> {
  /** Ekspor (selalu tersedia). */
  exportCfg: {
    filename: string
    columns: ExportColumn<T>[]
    getRows: (ctx: ExportContext) => Promise<T[]>
    hint?: string
    label?: string
  }
  /** Impor (opsional; tampil hanya bila diberikan — pemanggil menyaring menurut peran). */
  importCfg?: {
    title: string
    /** Dibangun saat wizard dibuka (mis. memakai vendor yang sudah termuat). */
    makeAdapter: () => ImportAdapter
    /** Awalan queryKey yang di-refresh setelah ada perubahan data. */
    invalidate: string[]
    label?: string
  }
}

const importBtn =
  'flex items-center gap-2 rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800'

// DataTools — pasangan tombol Ekspor + Impor yang seragam di header halaman modul.
export function DataTools<T>({ exportCfg, importCfg }: Props<T>) {
  const qc = useQueryClient()
  const [adapter, setAdapter] = useState<ImportAdapter | null>(null)

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <ExportMenu {...exportCfg} />
        {importCfg && (
          <button type="button" className={importBtn} onClick={() => setAdapter(importCfg.makeAdapter())}>
            <Upload className="h-4 w-4" /> {importCfg.label ?? 'Impor'}
          </button>
        )}
      </div>
      {adapter && importCfg && (
        <ImportWizard
          title={importCfg.title}
          adapter={adapter}
          onClose={() => setAdapter(null)}
          onChanged={() => {
            for (const k of importCfg.invalidate) qc.invalidateQueries({ queryKey: [k] })
          }}
        />
      )}
    </>
  )
}
