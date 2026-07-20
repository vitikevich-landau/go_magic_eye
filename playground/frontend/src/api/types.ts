// Типы — зеркало контракта playground/SPEC.md §2.1 (eye_json_version: 1)
// и ответов API §4.1. Фронт ничего не вычисляет про память — только рисует
// то, что добыла модель Ока.

export interface Passport {
  type_name: string
  kind: string
  size: number
  align: number
  traits: string[]
}

export type RegionKind = 'field' | 'padding' | 'word'

export interface Region {
  kind: RegionKind
  offset: number
  size: number
  name: string
  type_name: string
  value: string
  note: string
  from: string
}

export interface Embed {
  depth: number
  type_name: string
  field_name: string
  offset: number
  size: number
  promoted: string[]
  note: string
}

export interface IfaceMethod {
  name: string
  pc: string
  func: string
}

export interface Iface {
  where: string
  empty: boolean
  type_name: string
  dyn_type: string
  tab_addr: string
  data_addr: string
  hash: number
  methods: IfaceMethod[]
  typed_nil: boolean
  note: string
}

export interface Satellite {
  title: string
  addr: string
  size: number
  bytes: string // hex
  elems: string[]
  note: string
}

export interface EyeModel {
  label: string
  passport: Passport
  has_value: boolean
  addr: string
  bytes: string // hex: 2 символа на байт
  regions: Region[]
  embeds: Embed[]
  ifaces: Iface[]
  sats: Satellite[]
  notes: string[]
}

export interface Envelope {
  eye_json_version: number
  models: EyeModel[]
}

export interface Diagnostic {
  line: number
  col: number
  severity: 'error'
  message: string
}

export interface CheckResponse {
  ok: boolean
  diagnostics: Diagnostic[]
}

export interface RunResponse {
  ok: boolean
  diagnostics: Diagnostic[]
  eye: Envelope | null
  stdout: string
  stderr: string
  timed_out: boolean
  exit_code: number // 0 — чисто; смысла нет при timed_out
  compile_ms: number
  run_ms: number
}

export interface Example {
  id: string
  title: string
  topic: string
  code: string
}

// ── странствие (сеансовый протокол, eye_session_version: 1) ──────────

export interface TreeNodeDTO {
  id: number
  label: string
  sub: string
  expandable: boolean
  refusal: string
  cycle: number // id узла-оригинала; 0 — обычный узел
  shared: boolean // true — разделяемая ссылка ≡, false при cycle>0 — цикл ⟲
  copied: string
}

export interface ExploreResponse {
  ok: boolean
  diagnostics: Diagnostic[]
  session?: string
  roots?: TreeNodeDTO[]
  stdout?: string
  stderr?: string
  error?: string
  compile_ms: number
}

export interface ExploreCmdResponse {
  ok: boolean
  error?: string
  nodes?: TreeNodeDTO[]
  eye?: Envelope
  stdout?: string
}
