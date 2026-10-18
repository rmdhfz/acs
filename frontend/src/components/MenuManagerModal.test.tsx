import { afterEach, describe, expect, it } from 'vitest'
import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MenuManagerModal } from './MenuManagerModal'
import { MENU_CATALOG } from '../lib/menu'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'

const allowed = MENU_CATALOG.filter((m) => !m.requireRole || ['VIEWER', 'ADMIN'].includes(m.requireRole))

function setup() {
  setAuthToken('JWT')
  const saved: unknown[] = []
  const calls = mockFetch(({ url, method, body }) => {
    if (url.endsWith('/auth/menu-layout') && method === 'PUT') {
      saved.push(body)
      return { json: { layout: body } }
    }
    if (url.endsWith('/auth/menu-layout') && method === 'DELETE') return { status: 204 }
  })
  let closed = 0
  renderApp(<MenuManagerModal saved={null} allowed={allowed} onClose={() => (closed += 1)} />)
  return { saved, calls, closed: () => closed }
}

afterEach(() => setAuthToken(null))

const nodesOf = (b: unknown) => (b as { nodes: { type: string; key?: string; label?: string; children?: string[]; id?: string }[] }).nodes

describe('MenuManagerModal', () => {
  it('pindah item ke grup lewat pemilih tujuan, sembunyikan item, tambah grup, lalu simpan', async () => {
    const { saved } = setup()
    const u = userEvent.setup()
    // Dashboard -> grup "Konfigurasi"
    await u.selectOptions(screen.getByLabelText('Pindahkan Dashboard ke'), 'konfigurasi')
    // Sembunyikan Task
    await u.click(screen.getByRole('button', { name: 'Sembunyikan Task' }))
    // tambah grup baru lalu beri nama
    await u.click(screen.getByRole('button', { name: /tambah grup/i }))
    const input = screen.getByLabelText('Nama grup 4')
    await u.clear(input)
    await u.type(input, 'Favorit')
    await u.click(screen.getByRole('button', { name: /^simpan$/i }))
    await waitFor(() => expect(saved).toHaveLength(1))
    const layout = saved[0] as { version: number; hidden: string[] }
    const nodes = nodesOf(layout)
    expect(layout.version).toBe(1)
    expect(layout.hidden).toContain('tasks')
    const cfg = nodes.find((n) => n.type === 'group' && n.label === 'Konfigurasi')!
    expect(cfg.children).toContain('dashboard')
    expect(nodes.some((n) => n.type === 'group' && n.label === 'Favorit')).toBe(true)
    // tidak ada item duplikat di seluruh layout
    const keys = [...nodes.flatMap((n) => (n.type === 'item' ? [n.key!] : (n.children ?? []))), ...layout.hidden]
    expect(new Set(keys).size).toBe(keys.length)
  })

  it('drag & drop: seret item ke grup (jatuhkan pada kotak grup) memindahkannya', async () => {
    const { saved } = setup()
    const u = userEvent.setup()
    const item = screen.getByTestId('item-devices')
    const group = screen.getByTestId('group-konfigurasi')
    const dt = { data: {} as Record<string, string>, setData(k: string, v: string) { this.data[k] = v }, getData(k: string) { return this.data[k] }, effectAllowed: '', dropEffect: '' }
    fireEvent.dragStart(item, { dataTransfer: dt })
    fireEvent.dragOver(group, { dataTransfer: dt })
    fireEvent.drop(group, { dataTransfer: dt })
    fireEvent.dragEnd(item, { dataTransfer: dt })
    // item kini muncul di dalam kotak grup
    await waitFor(() => expect(within(screen.getByTestId('group-konfigurasi')).getByTestId('item-devices')).toBeTruthy())
    await u.click(screen.getByRole('button', { name: /^simpan$/i }))
    await waitFor(() => expect(saved).toHaveLength(1))
    expect(nodesOf(saved[0]).find((n) => n.type === 'group' && n.id === 'konfigurasi')!.children).toContain('devices')
  })

  it('menolak nama grup kosong di klien (tidak mengirim request)', async () => {
    const { saved } = setup()
    const u = userEvent.setup()
    const first = screen.getByLabelText('Nama grup 1')
    await u.clear(first)
    await u.type(first, '   ')
    await u.click(screen.getByRole('button', { name: /^simpan$/i }))
    expect((await screen.findByRole('alert')).textContent).toMatch(/nama grup/i)
    expect(saved).toHaveLength(0)
  })

  it('"Kembalikan bawaan" memanggil DELETE dan menutup dialog', async () => {
    const { calls, closed } = setup()
    await userEvent.setup().click(screen.getByRole('button', { name: /kembalikan bawaan/i }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE')).toBe(true))
    await waitFor(() => expect(closed()).toBe(1))
  })
})
