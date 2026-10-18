import { describe, expect, it } from 'vitest'
import { fitSize, prepareScreenshot, ScreenshotError } from './screenshot'

describe('fitSize', () => {
  it('memperkecil sisi terpanjang ke batas dengan rasio tetap', () => {
    expect(fitSize(3840, 2160)).toEqual({ width: 1920, height: 1080 })
    expect(fitSize(1000, 4000, 2000)).toEqual({ width: 500, height: 2000 })
  })
  it('tidak pernah membesarkan dan menjaga minimal 1 piksel', () => {
    expect(fitSize(800, 600)).toEqual({ width: 800, height: 600 })
    expect(fitSize(10000, 1)).toEqual({ width: 1920, height: 1 })
  })
  it('ukuran tak valid -> 0', () => {
    expect(fitSize(0, 100)).toEqual({ width: 0, height: 0 })
    expect(fitSize(-5, 100)).toEqual({ width: 0, height: 0 })
  })
})

describe('prepareScreenshot', () => {
  it('menolak berkas yang bukan gambar (tipe di luar daftar putih, mis. SVG/HTML/teks)', async () => {
    for (const [type, body] of [
      ['image/svg+xml', '<svg><script>alert(1)</script></svg>'],
      ['text/html', '<script>1</script>'],
      ['application/pdf', '%PDF'],
      ['', 'apa saja'],
    ] as const) {
      await expect(prepareScreenshot(new Blob([body], { type }))).rejects.toBeInstanceOf(ScreenshotError)
    }
  })
})
