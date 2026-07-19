// Чистая логика карты памяти — без Vue, чтобы её можно было тестировать.
//
// Главный инвариант унаследован от TUI: байт со смещением N стоит в колонке
// N % 8 — выравнивание видно глазами. Регион может занимать несколько рядов.

import type { EyeModel, Region } from '../api/types'

export interface ByteCell {
  offset: number      // смещение байта в объекте
  row: number         // ряд сетки (по 8 байт)
  col: number         // колонка 0..7 = offset % 8
  hex: string         // «64»
  ascii: string       // печатный символ или «·»
  regionIndex: number // индекс региона в model.regions (-1 — вне регионов)
}

// hexToBytes — строка hex контракта → числа. Нечётный/битый hex невозможен
// по построению конверта, но на мусор отвечаем пустотой, а не NaN'ами.
export function hexToBytes(hex: string): number[] {
  if (hex.length % 2 !== 0 || /[^0-9a-fA-F]/.test(hex)) return []
  const out: number[] = []
  for (let i = 0; i < hex.length; i += 2) out.push(parseInt(hex.slice(i, i + 2), 16))
  return out
}

export function toAscii(b: number): string {
  return b >= 0x20 && b <= 0x7e ? String.fromCharCode(b) : '·'
}

// regionAt — какой регион накрывает байт offset (регионы отсортированы и
// не пересекаются — это инвариант модели Ока).
export function regionAt(regions: Region[], offset: number): number {
  for (let i = 0; i < regions.length; i++) {
    const r = regions[i]
    if (offset >= r.offset && offset < r.offset + r.size) return i
  }
  return -1
}

// buildCells — все байты объекта как ячейки сетки 8 колонок.
export function buildCells(model: EyeModel): ByteCell[] {
  const bytes = hexToBytes(model.bytes)
  return bytes.map((b, offset) => ({
    offset,
    row: Math.floor(offset / 8),
    col: offset % 8,
    hex: b.toString(16).padStart(2, '0'),
    ascii: toAscii(b),
    regionIndex: regionAt(model.regions, offset),
  }))
}

export function rowCount(model: EyeModel): number {
  return Math.ceil(model.passport.size / 8)
}

// isLittleEndianCandidate — по такому региону есть смысл играть анимацию
// «байты выстраиваются в число»: целое поле шире одного байта.
export function isLittleEndianCandidate(r: Region): boolean {
  if (r.kind !== 'field' || r.size < 2 || r.size > 8) return false
  return /^u?int(8|16|32|64)?$|^uintptr$/.test(r.type_name)
}

// leSteps — байты региона в порядке памяти и в порядке числа (старший
// первым): материал для анимации little-endian.
export function leSteps(model: EyeModel, r: Region): { memory: string[]; number: string[] } {
  const bytes = hexToBytes(model.bytes).slice(r.offset, r.offset + r.size)
  const memory = bytes.map((b) => b.toString(16).padStart(2, '0'))
  return { memory, number: [...memory].reverse() }
}

// Палитра региона — согласована между картой, списком полей и легендой.
// Поля чередуются двумя тонами, чтобы соседние были различимы.
export function regionTone(regions: Region[], index: number): string {
  const r = regions[index]
  if (!r) return 'none'
  if (r.kind === 'padding') return 'padding'
  if (r.kind === 'word') return 'word'
  let fieldOrdinal = 0
  for (let i = 0; i < index; i++) if (regions[i].kind === 'field') fieldOrdinal++
  return fieldOrdinal % 2 === 0 ? 'fieldA' : 'fieldB'
}
