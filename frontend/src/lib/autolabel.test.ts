import { afterEach, describe, expect, it } from 'vitest'
import { installAutoLabels } from './autolabel'

const nextFrame = () => new Promise((r) => requestAnimationFrame(() => r(null)))

describe('installAutoLabels', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('menautkan label ke kontrol saudara & kontrol di dalam pembungkus', async () => {
    document.body.innerHTML = `
      <div><label id="a">Nama</label><input id="" /></div>
      <div><label id="b">Role</label><div class="relative"><select></select></div></div>`
    installAutoLabels()
    const a = document.getElementById('a') as HTMLLabelElement
    const b = document.getElementById('b') as HTMLLabelElement
    expect(a.htmlFor).toBeTruthy()
    expect(document.getElementById(a.htmlFor)?.tagName).toBe('INPUT')
    expect(document.getElementById(b.htmlFor)?.tagName).toBe('SELECT')
  })

  it('mempertahankan id yang sudah ada & tidak menyentuh label yang membungkus kontrol', () => {
    document.body.innerHTML = `
      <div><label id="a">X</label><input id="sudah-ada" /></div>
      <label id="w">Bungkus <input id="dalam" /></label>`
    installAutoLabels()
    expect((document.getElementById('a') as HTMLLabelElement).htmlFor).toBe('sudah-ada')
    expect((document.getElementById('w') as HTMLLabelElement).htmlFor).toBe('')
  })

  it('menangani DOM yang muncul belakangan (mis. modal)', async () => {
    installAutoLabels()
    const host = document.createElement('div')
    host.innerHTML = '<label id="m">Judul</label><textarea></textarea>'
    document.body.appendChild(host)
    await nextFrame()
    await nextFrame()
    const lab = document.getElementById('m') as HTMLLabelElement
    expect(document.getElementById(lab.htmlFor)?.tagName).toBe('TEXTAREA')
  })

  it('tidak menautkan bila tidak ada kontrol sesudahnya', () => {
    document.body.innerHTML = '<div><label id="x">Sendiri</label><p>teks</p></div>'
    installAutoLabels()
    expect((document.getElementById('x') as HTMLLabelElement).htmlFor).toBe('')
  })
})
