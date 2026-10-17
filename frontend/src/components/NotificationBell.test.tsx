import { describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { NotificationBell } from './NotificationBell'
import { NotificationProvider } from '../lib/notifications'
import { mockFetch, renderApp } from '../test/utils'

const mk = (id: string, title: string, read: string | null, extra: Record<string, unknown> = {}) => ({
  id, type: 'TASK_FAILURES', type_name: 'Task gagal', severity: 'WARNING', title, body: 'Detail ' + title,
  link: '/tasks', entity_type: null, entity_id: null, read_at: read, created_at: new Date().toISOString(), ...extra,
})

function setup(initial = [mk('n1', '5 task gagal', null), mk('n2', 'Rollout dihentikan', '2026-01-01T00:00:00Z', { severity: 'CRITICAL', link: '/firmware' })]) {
  let items = initial
  const calls = mockFetch(({ url, method, body }) => {
    if (url.includes('/notifications/unread-count')) return { json: { count: items.filter((i) => !i.read_at).length } }
    if (url.includes('/notifications/read') && method === 'POST') {
      const b = body as { ids?: string[]; all?: boolean }
      items = items.map((i) => (b.all || b.ids?.includes(i.id) ? { ...i, read_at: new Date().toISOString() } : i))
      return { status: 204 }
    }
    if (url.includes('/notifications?')) return { json: { data: items, meta: { total: items.length, unread: items.filter((i) => !i.read_at).length, page: 1, limit: 15 } } }
  })
  renderApp(
    <NotificationProvider>
      <NotificationBell />
    </NotificationProvider>,
  )
  return calls
}

describe('NotificationBell', () => {
  it('menampilkan jumlah belum dibaca pada lonceng dan daftar saat dibuka', async () => {
    setup()
    const bell = await screen.findByRole('button', { name: /1 belum dibaca/i })
    await userEvent.click(bell)
    expect(await screen.findByText('5 task gagal')).toBeTruthy()
    expect(screen.getByText('Rollout dihentikan')).toBeTruthy()
    // tab "Belum dibaca" hanya memuat yang belum dibaca
    await userEvent.click(screen.getByRole('tab', { name: /belum dibaca/i }))
    expect(screen.queryByText('Rollout dihentikan')).toBeNull()
    expect(screen.getByText('5 task gagal')).toBeTruthy()
  })

  it('klik item menandainya dibaca (POST /notifications/read) dan menutup panel', async () => {
    const calls = setup()
    await userEvent.click(await screen.findByRole('button', { name: /belum dibaca/i }))
    await userEvent.click(await screen.findByText('5 task gagal'))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.includes('/notifications/read') && JSON.stringify(c.body) === JSON.stringify({ ids: ['n1'] }))).toBe(true))
    expect(screen.queryByRole('dialog', { name: 'Notifikasi' })).toBeNull()
  })

  it('"Tandai dibaca" mengirim all:true dan tombol nonaktif bila tak ada yang belum dibaca', async () => {
    const calls = setup([mk('n1', 'Satu', null)])
    await userEvent.click(await screen.findByRole('button', { name: /belum dibaca/i }))
    await userEvent.click(screen.getByRole('button', { name: /tandai dibaca/i }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && JSON.stringify(c.body) === JSON.stringify({ all: true }))).toBe(true))
    await waitFor(() => expect((screen.getByRole('button', { name: /tandai dibaca/i }) as HTMLButtonElement).disabled).toBe(true))
  })

  it('keadaan kosong menjelaskan apa yang akan muncul', async () => {
    setup([])
    await userEvent.click(await screen.findByRole('button', { name: /^notifikasi$/i }))
    expect(await screen.findByText('Belum ada notifikasi')).toBeTruthy()
  })
})
