import { useEffect } from 'react'
import { subscribeQueryErrors } from '../lib/queryErrors'
import { useToast } from '../lib/toast'

// Menampilkan galat pemuatan data (lib/queryErrors.ts) sebagai toast. Tidak merender apa pun.
export function QueryErrorToaster() {
  const toast = useToast()
  useEffect(() => subscribeQueryErrors((message) => toast.error(message, 'Gagal memuat data')), [toast])
  return null
}
