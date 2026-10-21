import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RetryTaskButton, TaskSummaryView } from './TaskParts'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'
import type { Task } from '../lib/types'

function login(roles: string[]) {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'u', roles, tenant_id: 7 }))
}

const STATUS = [
  { id: 1, code: 'PENDING', name: 'Menunggu' },
  { id: 3, code: 'COMPLETED', name: 'Berhasil' },
  { id: 4, code: 'FAILED', name: 'Gagal' },
]
const TYPES = [
  { id: 10, code: 'GET_PARAMETER_VALUES', name: 'Ambil' },
  { id: 11, code: 'SET_PARAMETER_VALUES', name: 'Ubah' },
]

const task = (over: Partial<Task> = {}) =>
  ({ id: 886, task_uuid: 'u', device_id: 5, task_type_id: 11, task_status_id: 4, priority: 5, parameters: { values: { 'InternetGatewayDevice.LANDevice.1.WLANConfiguration.2.SSID': 'Rumah' } },
    response: null, error_message: null, retry_count: 3, max_retries: 3, scheduled_at: null, sent_at: null, completed_at: null, created_at: '', updated_at: '', ...over }) as Task

function handler(extra?: (r: { url: string; method: string }) => { status?: number; json?: unknown } | undefined) {
  return (r: { url: string; method: string }) => {
    if (r.url.endsWith('/refs/ref_task_status')) return { json: STATUS }
    if (r.url.endsWith('/refs/ref_task_types')) return { json: TYPES }
    return extra?.(r)
  }
}

beforeEach(() => localStorage.clear())
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('TaskSummaryView', () => {
  it('menampilkan apa yang dikerjakan task, bukan JSON mentah', async () => {
    login(['ADMIN'])
    mockFetch(handler())
    renderApp(<TaskSummaryView task={task()} />)
    expect(await screen.findByText('Ubah WiFi #2')).toBeTruthy()
    expect(screen.getByText('SSID WiFi #2 → Rumah')).toBeTruthy()
  })
})

describe('RetryTaskButton', () => {
  it('task gagal: "Coba lagi" memanggil POST /tasks/:id/retry (task yang sama)', async () => {
    login(['ADMIN'])
    const calls = mockFetch(handler((r) => (r.url.endsWith('/tasks/886/retry') ? { json: { id: 886 } } : undefined)))
    renderApp(<RetryTaskButton task={task()} />)
    await userEvent.setup().click(await screen.findByRole('button', { name: /coba lagi task 886/i }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/tasks/886/retry'))).toBe(true))
  })

  it('pending yang menunggu retry: "Kirim sekarang"; pending baru & selesai: tidak ada tombol', async () => {
    login(['ADMIN'])
    mockFetch(handler())
    const { unmount } = renderApp(<RetryTaskButton task={task({ task_status_id: 1, retry_count: 2 })} />)
    expect(await screen.findByRole('button', { name: /kirim sekarang task 886/i })).toBeTruthy()
    unmount()

    renderApp(
      <>
        <RetryTaskButton task={task({ id: 1, task_status_id: 1, retry_count: 0 })} />
        <RetryTaskButton task={task({ id: 2, task_status_id: 3, retry_count: 1 })} />
      </>,
    )
    await screen.findByRole('button', { name: /kirim sekarang task 886/i }).catch(() => null)
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('NOC: boleh mengulang task baca, tidak boleh task ubah (tombol tersembunyi); VIEWER tidak melihat tombol', async () => {
    login(['NOC'])
    mockFetch(handler())
    const { unmount } = renderApp(<RetryTaskButton task={task({ task_type_id: 10 })} />)
    expect(await screen.findByRole('button', { name: /coba lagi/i })).toBeTruthy()
    unmount()

    const r2 = renderApp(<RetryTaskButton task={task({ task_type_id: 11 })} />)
    await waitFor(() => expect(screen.queryByRole('button')).toBeNull())
    r2.unmount()

    login(['VIEWER'])
    renderApp(<RetryTaskButton task={task({ task_type_id: 10 })} />)
    await waitFor(() => expect(screen.queryByRole('button')).toBeNull())
  })
})
