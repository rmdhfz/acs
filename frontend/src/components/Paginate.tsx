import type { ReactNode } from 'react'
import { Pager } from './Pager'
import { usePaged } from '../lib/usePaged'

interface Props<T> {
  items: readonly T[]
  /** Nama pada aria-label pager, mis. "tenant". */
  label: string
  initialSize?: number
  /** Render baris halaman ini + elemen pager (tempatkan di bawah tabel). */
  children: (rows: T[], pager: ReactNode) => ReactNode
}

// Paginate membungkus tabel yang datanya sudah dimuat penuh: hanya `initialSize` baris dirender per halaman dan pager
// ditampilkan hanya bila lebih dari satu halaman ATAU lebih dari 10 baris (daftar kecil tidak diberi kontrol).
export function Paginate<T>({ items, label, initialSize = 25, children }: Props<T>) {
  const p = usePaged(items, initialSize)
  const showPager = items.length > 10
  return <>{children(p.rows, showPager ? <Pager {...p.pagerProps} label={label} /> : null)}</>
}
