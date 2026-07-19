import type {
  CheckResponse,
  Example,
  ExploreCmdResponse,
  ExploreResponse,
  RunResponse,
} from './types'

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

// ── странствие ───────────────────────────────────────────────────────

export const startExplore = (code: string) => post<ExploreResponse>('/api/explore', code)

export async function exploreCmd(
  session: string,
  cmd: 'kids' | 'detail',
  node: number,
): Promise<ExploreCmdResponse> {
  const resp = await fetch('/api/explore/cmd', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session, cmd, node }),
  })
  const body = await resp.json().catch(() => null)
  if (resp.status === 410) throw new SessionGoneError(body?.error ?? 'сеанс завершился')
  if (!resp.ok) throw new Error(body?.error ?? `HTTP ${resp.status}`)
  return body as ExploreCmdResponse
}

export function closeExplore(session: string) {
  // fire-and-forget: сеанс всё равно доживёт до жнеца, но вежливость лучше
  const body = JSON.stringify({ session })
  if (navigator.sendBeacon?.('/api/explore/close', new Blob([body], { type: 'application/json' }))) {
    return
  }
  void fetch('/api/explore/close', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body,
    keepalive: true,
  }).catch(() => {})
}

export class SessionGoneError extends Error {}
