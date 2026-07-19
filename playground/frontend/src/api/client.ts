import type { CheckResponse, Example, RunResponse } from './types'

// Ошибки API приходят JSON'ом {error}; всё прочее — сетевая беда.
async function post<T>(url: string, code: string): Promise<T> {
  const resp = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code }),
  })
  const body = await resp.json().catch(() => null)
  if (!resp.ok) {
    const msg = body && typeof body.error === 'string' ? body.error : `HTTP ${resp.status}`
    throw new Error(msg)
  }
  return body as T
}

export const checkCode = (code: string) => post<CheckResponse>('/api/check', code)
export const runCode = (code: string) => post<RunResponse>('/api/run', code)

export async function fetchExamples(): Promise<Example[]> {
  const resp = await fetch('/api/examples')
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  return resp.json()
}
