import { useEffect, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'
import { findRefIdByCode } from './hooks'
import type { RefLookup } from './types'

// useStatusParam — mendukung deep-link dari dashboard, mis. /devices?status=OFFLINE
// atau /tasks?status=FAILED. Kode status di URL dipetakan ke id ref_* begitu
// referensi termuat, lalu diterapkan SEKALI sebagai nilai awal filter (setelah
// itu user bebas mengganti filter tanpa ditimpa lagi).
export function useStatusParam(refs: RefLookup[] | undefined, apply: (id: string) => void) {
  const [params] = useSearchParams()
  const code = params.get('status')
  const applied = useRef(false)
  useEffect(() => {
    if (!code || applied.current || !refs) return
    applied.current = true
    const id = findRefIdByCode(refs, code.toUpperCase())
    if (id !== undefined) apply(String(id))
  }, [code, refs, apply])
}
