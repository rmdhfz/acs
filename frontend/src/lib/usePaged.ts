import { useEffect, useMemo, useState } from 'react'

// Paginasi sisi klien untuk daftar yang sudah dimuat penuh (mis. tenant, tag, preset). Halaman otomatis kembali valid
// bila jumlah data berubah (filter/pencarian/hapus).
export function usePaged<T>(items: readonly T[], initialSize = 25) {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(initialSize)
  const pages = Math.max(1, Math.ceil(items.length / pageSize))
  useEffect(() => {
    if (page > pages) setPage(pages)
  }, [page, pages])
  const cur = Math.min(page, pages)
  const rows = useMemo(() => items.slice((cur - 1) * pageSize, cur * pageSize), [items, cur, pageSize])
  return {
    rows,
    pagerProps: {
      page: cur,
      pageSize,
      total: items.length,
      onPage: setPage,
      onPageSize: (n: number) => {
        setPageSize(n)
        setPage(1)
      },
    },
  }
}
