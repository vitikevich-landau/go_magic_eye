import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('../api/client', () => ({
  checkCode: vi.fn(),
  runCode: vi.fn(),
  fetchExamples: vi.fn(),
}))

import { checkCode, runCode } from '../api/client'
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

// Правка кода во время прогона: ответ старого кода не ложится на новый.
describe('гонка запуска', () => {
  it('устаревший результат run игнорируется', async () => {
    const pg = usePlayground()
    pg.code = 'старый код'

    let resolveRun!: (v: unknown) => void
    ;(runCode as Mock).mockReturnValueOnce(new Promise((r) => (resolveRun = r)))

    const running = pg.run()
    pg.code = 'новый код' // отредактировали, пока прогон летел

    resolveRun({
      ok: true,
      diagnostics: [{ line: 7, col: 1, severity: 'error', message: 'из прошлого прогона' }],
      eye: { eye_json_version: 1, models: [{ label: 'старый' }] },
      stdout: '',
      stderr: '',
      timed_out: false,
      exit_code: 0,
      compile_ms: 1,
      run_ms: 1,
    })
    await running

    expect(pg.result).toBeNull() // ответ старого кода не применён
    expect(pg.diagnostics).toEqual([])
  })
})
