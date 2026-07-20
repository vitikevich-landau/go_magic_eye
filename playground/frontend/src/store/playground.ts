import { defineStore } from 'pinia'
import { checkCode, fetchExamples, runCode } from '../api/client'
import type { Diagnostic, Example, RunResponse } from '../api/types'

const STORAGE_KEY = 'eye-playground-code'

// Код в ссылке: #code=base64url — снипетт воспроизводится без сервера.
function codeFromHash(): string | null {
  const m = location.hash.match(/^#code=(.+)$/)
  if (!m) return null
  try {
    return decodeURIComponent(escape(atob(m[1].replace(/-/g, '+').replace(/_/g, '/'))))
  } catch {
    return null
  }
}

export function codeToHash(code: string): string {
  return btoa(unescape(encodeURIComponent(code))).replace(/\+/g, '-').replace(/\//g, '_')
}

export const usePlayground = defineStore('playground', {
  state: () => ({
    code: '',
    examples: [] as Example[],
    activeExample: '' as string,
    result: null as RunResponse | null,
    diagnostics: [] as Diagnostic[],
    running: false,
    // ошибка самого API (сеть, 429, 500) — не ошибка компиляции снипетта
    apiError: '' as string,
  }),
  actions: {
    async init() {
      try {
        this.examples = await fetchExamples()
      } catch (e) {
        this.apiError = `галерея примеров не загрузилась: ${(e as Error).message}`
      }
      const fromHash = codeFromHash()
      const saved = localStorage.getItem(STORAGE_KEY)
      if (fromHash) {
        this.code = fromHash
      } else if (saved) {
        this.code = saved
      } else if (this.examples.length > 0) {
        this.selectExample(this.examples[0].id)
      }
    },

    setCode(code: string) {
      this.code = code
      localStorage.setItem(STORAGE_KEY, code)
    },

    selectExample(id: string) {
      const ex = this.examples.find((e) => e.id === id)
      if (!ex) return
      this.activeExample = id
      this.setCode(ex.code)
      this.result = null
      this.diagnostics = []
    },

    // check — фоновая проверка компилятором (маркеры в редакторе);
    // молчит про сетевые беды, чтобы не мигать на каждый чих.
    async check() {
      if (!this.code.trim() || this.running) return
      const code = this.code
      try {
        const res = await checkCode(code)
        // медленная проверка старого текста не вешает маркеры с чужими
        // номерами строк на уже отредактированный код
        if (this.code === code) this.diagnostics = res.diagnostics
      } catch {
        /* фоновая проверка не имеет права шуметь */
      }
    },

    async run() {
      if (!this.code.trim() || this.running) return
      this.running = true
      this.apiError = ''
      // прошлый результат гасится сразу: сбой запроса (сеть, 429, 500) не
      // должен оставить старую карту памяти под баннером ошибки — её легко
      // принять за вывод нового кода
      this.result = null
      try {
        const res = await runCode(this.code)
        this.result = res
        this.diagnostics = res.diagnostics
      } catch (e) {
        this.apiError = (e as Error).message
      } finally {
        this.running = false
      }
    },

    shareLink(): string {
      const url = new URL(location.href)
      url.hash = `code=${codeToHash(this.code)}`
      return url.toString()
    },
  },
})
