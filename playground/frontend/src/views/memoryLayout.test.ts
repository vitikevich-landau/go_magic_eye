import { describe, expect, it } from 'vitest'
import {
  buildCells,
  hexToBytes,
  isLittleEndianCandidate,
  leSteps,
  regionAt,
  regionTone,
} from './memoryLayout'
import type { EyeModel, Region } from '../api/types'

// Модель-образец: gauge{HP int32; Armor int8; _ [3]byte; Speed float64} —
// та же, что в golden-тестах контракта на стороне Go.
const regions: Region[] = [
  { kind: 'field', offset: 0, size: 4, name: 'HP', type_name: 'int32', value: '100 (0x64)', note: '', from: '' },
  { kind: 'field', offset: 4, size: 1, name: 'Armor', type_name: 'int8', value: '7', note: '', from: '' },
  { kind: 'padding', offset: 5, size: 3, name: '', type_name: '', value: '', note: 'дыра', from: '' },
  { kind: 'field', offset: 8, size: 8, name: 'Speed', type_name: 'float64', value: '1.5', note: '', from: '' },
]

const model: EyeModel = {
  label: 'датчик',
  passport: { type_name: 'gauge', kind: 'структура', size: 16, align: 8, traits: [] },
  has_value: true,
  addr: '0xc000010090',
  bytes: '64000000' + '07' + '000000' + '000000000000f83f',
  regions,
  embeds: [],
  ifaces: [],
  sats: [],
  notes: [],
}

describe('hexToBytes', () => {
  it('разбирает hex контракта', () => {
    expect(hexToBytes('64ff00')).toEqual([0x64, 0xff, 0x00])
  })
  it('на мусор отвечает пустотой', () => {
    expect(hexToBytes('xyz')).toEqual([])
    expect(hexToBytes('123')).toEqual([])
  })
})

describe('buildCells', () => {
  const cells = buildCells(model)

  it('байт со смещением N стоит в колонке N % 8', () => {
    for (const c of cells) {
      expect(c.col).toBe(c.offset % 8)
      expect(c.row).toBe(Math.floor(c.offset / 8))
    }
    expect(cells).toHaveLength(16)
  })

  it('каждый байт знает свой регион', () => {
    expect(cells[0].regionIndex).toBe(0) // HP
    expect(cells[4].regionIndex).toBe(1) // Armor
    expect(cells[5].regionIndex).toBe(2) // дыра
    expect(cells[8].regionIndex).toBe(3) // Speed
  })

  it('младший байт HP=100 — первый (little-endian)', () => {
    expect(cells[0].hex).toBe('64')
    expect(cells[1].hex).toBe('00')
  })
})

describe('regionAt', () => {
  it('накрывает границы включительно-исключительно', () => {
    expect(regionAt(regions, 3)).toBe(0)
    expect(regionAt(regions, 4)).toBe(1)
    expect(regionAt(regions, 7)).toBe(2)
    expect(regionAt(regions, 99)).toBe(-1)
  })
})

describe('little-endian', () => {
  it('кандидаты — целые шире байта', () => {
    expect(isLittleEndianCandidate(regions[0])).toBe(true) // int32
    expect(isLittleEndianCandidate(regions[1])).toBe(false) // int8: разворачивать нечего
    expect(isLittleEndianCandidate(regions[3])).toBe(false) // float64 — не целое
    expect(isLittleEndianCandidate(regions[2])).toBe(false) // дыра
  })

  it('leSteps разворачивает порядок байт', () => {
    const { memory, number } = leSteps(model, regions[0])
    expect(memory).toEqual(['64', '00', '00', '00'])
    expect(number).toEqual(['00', '00', '00', '64'])
  })
})

describe('regionTone', () => {
  it('поля чередуют два тона, служебные сорта — свои', () => {
    expect(regionTone(regions, 0)).toBe('fieldA')
    expect(regionTone(regions, 1)).toBe('fieldB')
    expect(regionTone(regions, 2)).toBe('padding')
    expect(regionTone(regions, 3)).toBe('fieldA')
  })
})
