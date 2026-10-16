import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import { describeQueryError, reportQueryError, subscribeQueryErrors } from './queryErrors'

describe('describeQueryError', () => {
  it('401 tidak dilaporkan (sesi berakhir ditangani logout otomatis)', () => {
    expect(describeQueryError(new ApiError(401, 'x'))).toBeNull()
  })
  it('5xx, 403, galat bisnis, dan galat jaringan punya pesan yang tepat', () => {
    expect(describeQueryError(new ApiError(500, 'internal'))).toContain('Server bermasalah')
    expect(describeQueryError(new ApiError(403, 'forbidden'))).toContain('tidak punya akses')
    expect(describeQueryError(new ApiError(400, 'filter tidak valid'))).toBe('filter tidak valid')
    expect(describeQueryError(new TypeError('Failed to fetch'))).toContain('Tidak dapat menjangkau server')
  })
  it('AbortError diabaikan', () => {
    expect(describeQueryError(new DOMException('x', 'AbortError'))).toBeNull()
  })
})

describe('reportQueryError', () => {
  it('dedupe per query key dalam jendela waktu', () => {
    const got: string[] = []
    const off = subscribeQueryErrors((m) => got.push(m))
    const key = ['uji-dedupe', Math.random()]
    reportQueryError(new ApiError(500, 'x'), key)
    reportQueryError(new ApiError(500, 'x'), key) // duplikat -> diam
    reportQueryError(new ApiError(500, 'x'), ['key-lain', Math.random()])
    reportQueryError(new ApiError(401, 'x'), ['k401', Math.random()]) // diabaikan
    off()
    expect(got).toHaveLength(2)
  })
})
