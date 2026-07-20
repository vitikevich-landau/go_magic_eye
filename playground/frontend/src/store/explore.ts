import { defineStore } from 'pinia'
import { closeExplore, exploreCmd, SessionGoneError, startExplore } from '../api/client'
import type { Diagnostic, EyeModel, TreeNodeDTO } from '../api/types'

// TreeNode — узел дерева на клиенте: DTO протокола + локальное состояние.
export interface TreeNode extends TreeNodeDTO {
  children: TreeNode[] | null // null — дети ещё не запрашивались
  expanded: boolean
  parentId: number | null
}

export const useExplore = defineStore('explore', {
  state: () => ({
    session: '' as string,
    // все узлы по id — циклы прыгают к оригиналу через эту карту
    nodes: new Map<number, TreeNode>(),
    rootIds: [] as number[],
    selectedId: 0 as number,
    detail: null as EyeModel | null,
    detailLoading: false,
    stdoutLog: '' as string,
    diagnostics: [] as Diagnostic[],
    error: '' as string, // причина, почему сеанса нет (нет Explore, умер…)
    starting: false,
    // поколение старта: stop()/новый start() его двигают, и ответ сервера,
    // прилетевший из прошлого поколения, не воскрешает отменённый сеанс
    startGen: 0,
  }),
  getters: {
    roots(state): TreeNode[] {
      return state.rootIds.map((id) => state.nodes.get(id)!).filter(Boolean)
    },
    selected(state): TreeNode | null {
      return state.nodes.get(state.selectedId) ?? null
    },
    // крошки пути до выбранного узла — «где я», как в TUI
    breadcrumbs(state): string[] {
      const segs: string[] = []
      for (let n = state.nodes.get(state.selectedId); n; ) {
        segs.unshift(n.label)
        n = n.parentId === null ? undefined : state.nodes.get(n.parentId)
      }
      return segs
    },
  },
  actions: {
    ingest(dtos: TreeNodeDTO[], parentId: number | null): TreeNode[] {
      return dtos.map((dto) => {
        const node: TreeNode = { ...dto, children: null, expanded: false, parentId }
        this.nodes.set(node.id, node)
        return node
      })
    },

    appendStdout(s?: string) {
      if (s) this.stdoutLog += s
    },

    async start(code: string) {
      this.stop() // старый сеанс — вежливо закрыть
      const gen = ++this.startGen
      this.starting = true
      this.error = ''
      this.diagnostics = []
      try {
        const res = await startExplore(code)
        if (gen !== this.startGen) {
          // пока компилировались — передумали (stop или новый start):
          // сеанс-сирота закрывается сразу, не дожидаясь жнеца
          if (res.ok && res.session) closeExplore(res.session)
          return
        }
        this.diagnostics = res.diagnostics
        this.appendStdout(res.stdout)
        if (!res.ok || !res.session || !res.roots) {
          this.error = res.error || (res.diagnostics.length ? 'снипетт не собрался' : 'сеанс не начался')
          return
        }
        this.session = res.session
        const roots = this.ingest(res.roots, null)
        this.rootIds = roots.map((n) => n.id)
        if (roots.length > 0) this.select(roots[0].id)
      } catch (e) {
        if (gen === this.startGen) this.error = (e as Error).message
      } finally {
        if (gen === this.startGen) this.starting = false
      }
    },

    stop() {
      this.startGen++ // поздний ответ start() из прошлого поколения — сирота
      if (this.session) closeExplore(this.session)
      this.session = ''
      this.nodes = new Map()
      this.rootIds = []
      this.selectedId = 0
      this.detail = null
      this.stdoutLog = ''
      this.error = ''
      this.starting = false
    },

    dead(msg: string) {
      this.session = ''
      this.error = msg + ' — запусти странствие заново'
    },

    async toggle(id: number) {
      const n = this.nodes.get(id)
      if (!n || !this.session) return
      if (n.expanded) {
        n.expanded = false
        return
      }
      if (n.children === null) {
        const session = this.session
        try {
          const res = await exploreCmd(session, 'kids', id)
          if (this.session !== session) return // сеанс сменился: дети из прошлого дерева
          this.appendStdout(res.stdout)
          if (!res.ok) {
            // честный отказ (лист, nil…) — показываем причину у узла
            n.refusal = res.error || n.refusal
            n.expandable = false
            return
          }
          n.children = this.ingest(res.nodes ?? [], id)
        } catch (e) {
          if (this.session !== session) return // беда прошлого сеанса — не наша
          if (e instanceof SessionGoneError) this.dead(e.message)
          else this.error = (e as Error).message
          return
        }
      }
      n.expanded = true
    },

    async select(id: number) {
      const n = this.nodes.get(id)
      if (!n || !this.session) return
      this.selectedId = id
      this.detailLoading = true
      const session = this.session
      try {
        const res = await exploreCmd(session, 'detail', id)
        if (this.session !== session) return // сеанс сменился — ответ из прошлого
        this.appendStdout(res.stdout)
        if (this.selectedId !== id) return // быстрые клики: выбор уже уехал
        this.detail = res.ok && res.eye ? (res.eye.models[0] ?? null) : null
      } catch (e) {
        if (this.session !== session) return // смерть ПРОШЛОГО сеанса не гасит новый
        if (e instanceof SessionGoneError) this.dead(e.message)
        else this.error = (e as Error).message
      } finally {
        if (this.session === session && this.selectedId === id) this.detailLoading = false
      }
    },

    // прыжок по циклу ⟲/≡ — к уже показанному оригиналу: раскрыть предков
    // (дети у них уже загружены — оригинал был виден) и выбрать его
    jumpTo(id: number) {
      const target = this.nodes.get(id)
      if (!target) return
      for (let p = target.parentId; p !== null; ) {
        const parent = this.nodes.get(p)
        if (!parent) break
        parent.expanded = true
        p = parent.parentId
      }
      void this.select(id)
    },
  },
})
