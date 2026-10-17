import { useState } from 'react'
import { Upload } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { ExportMenu } from './ExportMenu'
import { ImportWizard } from './ImportWizard'
import type { ImportAdapter } from '../lib/importer'
import {
  loadMappingExportRows,
  loadModelExportRows,
  loadOUIExportRows,
  loadVendorExportRows,
  mappingAdapter,
  mappingExportColumns,
  modelAdapter,
  modelExportColumns,
  ouiAdapter,
  ouiExportColumns,
  vendorAdapter,
  vendorExportColumns,
} from '../imports/catalog'

type Tab = 'vendors' | 'models' | 'mappings'

const importBtn =
  'flex items-center gap-2 rounded-full border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800'

// Ekspor/Impor katalog per tab. Menambah vendor/model/mapping baru adalah
// operasi data (CLAUDE.md) — impor massal inilah jalur onboarding vendor.
export function CatalogDataTools({ tab }: { tab: Tab }) {
  const qc = useQueryClient()
  const [wizard, setWizard] = useState<{ title: string; adapter: ImportAdapter } | null>(null)

  const open = (title: string, adapter: ImportAdapter) => setWizard({ title, adapter })

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        {tab === 'vendors' && (
          <>
            <ExportMenu label="Ekspor Vendor" filename="vendor" columns={vendorExportColumns} getRows={(c) => loadVendorExportRows(c)} />
            <ExportMenu label="Ekspor OUI" filename="vendor-oui" columns={ouiExportColumns} getRows={(c) => loadOUIExportRows(c)} hint="Seluruh OUI semua vendor (ribuan baris)." />
            <button type="button" className={importBtn} onClick={() => open('Impor Vendor', vendorAdapter())}>
              <Upload className="h-4 w-4" /> Impor Vendor
            </button>
            <button type="button" className={importBtn} onClick={() => open('Impor OUI', ouiAdapter())}>
              <Upload className="h-4 w-4" /> Impor OUI
            </button>
          </>
        )}
        {tab === 'models' && (
          <>
            <ExportMenu label="Ekspor Model" filename="device-model" columns={modelExportColumns} getRows={(c) => loadModelExportRows(c)} />
            <button type="button" className={importBtn} onClick={() => open('Impor Device Model', modelAdapter())}>
              <Upload className="h-4 w-4" /> Impor Model
            </button>
          </>
        )}
        {tab === 'mappings' && (
          <>
            <ExportMenu label="Ekspor Mapping" filename="parameter-mapping" columns={mappingExportColumns} getRows={(c) => loadMappingExportRows(c)} />
            <button type="button" className={importBtn} onClick={() => open('Impor Parameter Mapping', mappingAdapter())}>
              <Upload className="h-4 w-4" /> Impor Mapping
            </button>
          </>
        )}
      </div>
      {wizard && (
        <ImportWizard
          title={wizard.title}
          adapter={wizard.adapter}
          onClose={() => setWizard(null)}
          onChanged={() => {
            for (const k of ['vendors', 'vendor-ouis', 'device-models', 'parameter-mappings', 'vendor']) qc.invalidateQueries({ queryKey: [k] })
          }}
        />
      )}
    </>
  )
}
