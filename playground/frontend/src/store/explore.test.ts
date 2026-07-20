import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('../api/client', () => ({
  startExplore: vi.fn(),
  exploreCmd: vi.fn(),
  closeExplore: vi.fn(),
  SessionGoneError: class SessionGoneError extends Error {},
}))

import { closeExplore, exploreCmd, startExplore } from '../api/client'
import { useExplore } from './explore'
import type { Envelope, TreeNodeDTO } from '../api/types'

const node = (id: number, label: string): TreeNodeDTO => ({
  id,
  label,
  sub: '',
  expandable: true,
  refusal: '',
  cycle: 0,
  shared: false,
  copied: '',
})

const envelope = (label: string): Envelope => ({
  eye_json_version: 1,
  models: [
    {
      label,
      passport: { type_name: 't', kind: 'структура', size: 8, align: 8, traits: [] },
      has_value: true,
      addr: '0x1',
      bytes: '00',
      regions: [],
      embeds: [],
      ifaces: [],
      sats: [],
      notes: [],
    },
  ],
})

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

// Переключение режима во время компиляции: поздний ответ start() не
// воскрешает отменённый сеанс — сирота закрывается сразу, не дожидаясь
// жнеца (и не выедает SessionMax повторами).
describe('гонка отменённого старта', () => {
  it('поздний ответ после stop() закрывает сеанс-сироту', async () => {
    let resolve!: (v: unknown) => void
    ;(startExplore as Mock).mockReturnValue(new Promise((r) => (resolve = r)))

    const ex = useExplore()
    const pending = ex.start('code')
    ex.stop() // пользователь ушёл в режим осмотра, компиляция ещё идёт

    resolve({ ok: true, session: 's1', roots: [node(1, 'корень')], diagnostics: [], compile_ms: 1 })
    await pending

    expect(ex.session).toBe('')
    expect(ex.roots).toHaveLength(0)
    expect(closeExplore).toHaveBeenCalledWith('s1')
    expect(ex.starting).toBe(false)
  })

  it('новый start() выигрывает у зависшего старого', async () => {
    let resolveOld!: (v: unknown) => void
    ;(startExplore as Mock)
      .mockReturnValueOnce(new Promise((r) => (resolveOld = r)))
      .mockResolvedValueOnce({
        ok: true,
        session: 's2',
        roots: [node(2, 'новый')],
        diagnostics: [],
        compile_ms: 1,
      })
    ;(exploreCmd as Mock).mockResolvedValue({ ok: true, eye: envelope('новый') })

    const ex = useExplore()
    const oldStart = ex.start('старый код')
    await ex.start('новый код')
    resolveOld({ ok: true, session: 's1', roots: [node(1, 'старый')], diagnostics: [], compile_ms: 1 })
    await oldStart

    expect(ex.session).toBe('s2')
    expect(closeExplore).toHaveBeenCalledWith('s1') // сирота закрыт
  })
})

// Ответы прошлого сеанса не трогают новое дерево и не гасят его.
describe('ответы из прошлого сеанса', () => {
  it('поздние kids не вливаются в новое дерево', async () => {
    const ex = useExplore()
    ex.session = 's1'
    ex.ingest([node(1, 'корень')], null)

    let resolveKids!: (v: unknown) => void
    ;(exploreCmd as Mock).mockReturnValueOnce(new Promise((r) => (resolveKids = r)))

    const pending = ex.toggle(1)
    // пока kids летели — сеанс перезапустили с другим деревом
    ex.session = 's2'
    ex.nodes = new Map()
    ex.ingest([node(9, 'новый корень')], null)

    resolveKids({ ok: true, nodes: [node(2, 'дитя прошлого')] })
    await pending

    expect(ex.nodes.has(2)).toBe(false)
    expect(ex.nodes.get(9)?.label).toBe('новый корень')
  })

  it('SessionGone прошлого сеанса не убивает новый', async () => {
    const { SessionGoneError } = await import('../api/client')
    const ex = useExplore()
    ex.session = 's1'
    ex.ingest([node(1, 'корень')], null)

    let rejectDetail!: (e: unknown) => void
    ;(exploreCmd as Mock).mockReturnValueOnce(new Promise((_, rej) => (rejectDetail = rej)))

    const pending = ex.select(1)
    ex.session = 's2' // рестарт, пока detail летел
    rejectDetail(new SessionGoneError('сеанс завершился'))
    await pending

    expect(ex.session).toBe('s2')
    expect(ex.error).toBe('')
  })
})

// Быстрые клики по дереву: устаревший ответ detail не перекрывает свежий.
describe('гонка устаревшего detail', () => {
  it('поздний ответ по прошлому узлу игнорируется', async () => {
    const ex = useExplore()
    ex.session = 's'
    ex.ingest([node(1, 'первый'), node(2, 'второй')], null)

    let resolveFirst!: (v: unknown) => void
    ;(exploreCmd as Mock)
      .mockReturnValueOnce(new Promise((r) => (resolveFirst = r)))
      .mockResolvedValueOnce({ ok: true, eye: envelope('второй') })

    const firstClick = ex.select(1)
    await ex.select(2) // второй клик успел раньше
    resolveFirst({ ok: true, eye: envelope('первый') })
    await firstClick

    expect(ex.selectedId).toBe(2)
    expect(ex.detail?.label).toBe('второй')
    expect(ex.detailLoading).toBe(false)
  })
})
