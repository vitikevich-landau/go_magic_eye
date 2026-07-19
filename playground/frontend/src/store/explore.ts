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
      this.starting = true
      this.error = ''
      this.diagnostics = []
      try {
        const res = await startExplore(code)
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
        this.error = (e as Error).message
      } finally {
        this.starting = false
      }
    },

    stop() {
      if (this.session) closeExplore(this.session)
      this.session = ''
      this.nodes = new Map()
      this.rootIds = []
      this.selectedId = 0
      this.detail = null
      this.stdoutLog = ''
      this.error = ''
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
        try {
          const res = await exploreCmd(this.session, 'kids', id)
          this.appendStdout(res.stdout)
          if (!res.ok) {
            // честный отказ (лист, nil…) — показываем причину у узла
            n.refusal = res.error || n.refusal
            n.expandable = false
            return
          }
          n.children = this.ingest(res.nodes ?? [], id)
        } catch (e) {
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
      try {
        const res = await exploreCmd(this.session, 'detail', id)
        this.appendStdout(res.stdout)
        this.detail = res.ok && res.eye ? (res.eye.models[0] ?? null) : null
      } catch (e) {
        if (e instanceof SessionGoneError) this.dead(e.message)
        else this.error = (e as Error).message
      } finally {
        this.detailLoading = false
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
