import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('../api/client', () => ({
  checkCode: vi.fn(),
  runCode: vi.fn(),
  fetchExamples: vi.fn(),
}))

import { checkCode } from '../api/client'
import { usePlayground } from './playground'

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

// Медленная проверка старого текста не вешает маркеры с чужими номерами
// строк на уже отредактированный код.
describe('гонка фоновой проверки', () => {
  it('устаревшие диагностики не применяются', async () => {
    const pg = usePlayground()
    pg.code = 'старый код'

    let resolveOld!: (v: unknown) => void
    ;(checkCode as Mock)
      .mockReturnValueOnce(new Promise((r) => (resolveOld = r)))
      .mockResolvedValueOnce({ ok: true, diagnostics: [] })

    const oldCheck = pg.check()
    pg.code = 'новый код'
    await pg.check() // свежая проверка: чисто

    resolveOld({
      ok: false,
      diagnostics: [{ line: 42, col: 1, severity: 'error', message: 'из прошлого' }],
    })
    await oldCheck

    expect(pg.diagnostics).toEqual([])
  })
})
