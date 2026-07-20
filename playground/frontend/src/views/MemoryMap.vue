<script setup lang="ts">
// Карта памяти — сердце playground. Сетка 8 колонок: байт со смещением N
// стоит в колонке N % 8, выравнивание видно глазами. Hover работает в обе
// стороны: байт ↔ поле в списке. Клик по целому полю играет анимацию
// little-endian — байты перелетают и выстраиваются в число старшим вперёд.
import { computed, ref } from 'vue'
import type { EyeModel, Region } from '../api/types'
import {
  buildCells,
  isLittleEndianCandidate,
  leSteps,
  MAX_CELLS,
  regionTone,
} from './memoryLayout'

const props = defineProps<{
  model: EyeModel
  litRegion: number | null
  litRange: { offset: number; size: number } | null
}>()
const emit = defineEmits<{ lit: [index: number | null] }>()

const cells = computed(() => buildCells(props.model))
const showAscii = ref(false)

// закреплённый кликом регион переживает уход курсора
const pinned = ref<number | null>(null)
const active = computed(() => props.litRegion ?? pinned.value)

function isLit(cell: { offset: number; regionIndex: number }): boolean {
  if (props.litRange) {
    return cell.offset >= props.litRange.offset && cell.offset < props.litRange.offset + props.litRange.size
  }
  return active.value !== null && cell.regionIndex === active.value
}

// выноска — регион под курсором или закреплённый
const spotlight = computed<Region | null>(() =>
  active.value === null ? null : (props.model.regions[active.value] ?? null),
)

// анимация little-endian: закреплённое целое поле (типу без объекта
// разворачивать нечего — байтов нет)
const le = computed(() => {
  if (pinned.value === null || !props.model.bytes) return null
  const r = props.model.regions[pinned.value]
  if (!r || !isLittleEndianCandidate(r)) return null
  return { region: r, ...leSteps(props.model, r) }
})
const leReplay = ref(0)

function clickRegion(i: number) {
  if (pinned.value === i) {
    leReplay.value++ // повторный клик — проиграть анимацию заново
  }
  pinned.value = i
  emit('lit', i)
}

// сетка рядами: rows[r][c] — ячейка или null (хвост последнего ряда)
const rows = computed(() => {
  const out: (ReturnType<typeof buildCells>[number] | null)[][] = []
  for (const cell of cells.value) {
    ;(out[cell.row] ??= Array(8).fill(null))[cell.col] = cell
  }
  return out
})

const fields = computed(() =>
  props.model.regions
    .map((r, i) => ({ r, i }))
    .filter(({ r }) => r.kind !== 'padding'),
)
</script>

<template>
  <section>
    <div class="mb-2 flex items-center gap-3 text-xs text-grimoire-dim">
      <h3 class="font-bold uppercase tracking-wider">память</h3>
      <span class="rounded-sm px-1" style="background: var(--color-tone-fieldA)">поле</span>
      <span class="byte-cell tone-padding rounded-sm px-1">padding</span>
      <span class="rounded-sm px-1" style="background: var(--color-tone-word)">служебное слово</span>
      <button
        class="ml-auto rounded border border-grimoire-border px-2 py-0.5 hover:text-grimoire-ink"
        @click="showAscii = !showAscii"
      >
        {{ showAscii ? 'hex' : 'ascii' }}
      </button>
    </div>

    <div class="flex flex-wrap gap-6">
      <!-- сетка байтов -->
      <div class="shrink-0">
        <div class="mb-1 grid grid-cols-[3.5rem_repeat(8,2rem)] gap-px text-center text-[10px] text-grimoire-dim">
          <div></div>
          <div v-for="c in 8" :key="c">+{{ c - 1 }}</div>
        </div>
        <div
          v-for="(rowCells, row) in rows"
          :key="row"
          class="grid grid-cols-[3.5rem_repeat(8,2rem)] gap-px"
        >
          <div class="pr-2 text-right text-[11px] leading-7 text-grimoire-dim">
            {{ row * 8 }}
          </div>
          <template v-for="(cell, col) in rowCells" :key="col">
            <div
              v-if="cell"
              class="byte-cell h-7 text-center text-xs leading-7"
              :class="['tone-' + regionTone(model.regions, cell.regionIndex), { lit: isLit(cell) }]"
              @mouseenter="emit('lit', cell.regionIndex)"
              @mouseleave="emit('lit', null)"
              @click="clickRegion(cell.regionIndex)"
            >
              {{ showAscii ? cell.ascii : cell.hex }}
            </div>
            <div v-else class="h-7"></div>
          </template>
        </div>
      </div>

      <!-- список полей: hover в обратную сторону -->
      <ul class="min-w-56 grow space-y-px text-sm">
        <li
          v-for="{ r, i } in fields"
          :key="i"
          class="cursor-pointer rounded px-2 py-1 transition"
          :class="active === i ? 'bg-grimoire-panel2 text-grimoire-ink' : 'text-grimoire-dim hover:bg-grimoire-panel2'"
          @mouseenter="emit('lit', i)"
          @mouseleave="emit('lit', null)"
          @click="clickRegion(i)"
        >
          <span class="text-grimoire-ink">{{ r.name || '(без имени)' }}</span>
          <span class="text-xs">&nbsp;· {{ r.type_name }}</span>
          <span class="float-right text-xs">{{ r.offset }}‥{{ r.offset + r.size }}</span>
          <div v-if="r.value" class="truncate text-xs text-grimoire-purple">{{ r.value }}</div>
        </li>
      </ul>
    </div>

    <p v-if="model.passport.size > MAX_CELLS" class="mt-2 text-xs text-grimoire-dim">
      ⋯ показаны первые {{ MAX_CELLS }} Б из {{ model.passport.size }} — дальше карта была бы обоями ⋯
    </p>

    <!-- выноска: урок выбранного региона -->
    <div
      v-if="spotlight"
      class="mt-3 rounded border border-grimoire-border bg-grimoire-panel2 px-3 py-2 text-sm"
    >
      <div>
        <span class="font-bold text-grimoire-gold">{{ spotlight.name || (spotlight.kind === 'padding' ? 'дыра выравнивания' : 'служебное слово') }}</span>
        <span v-if="spotlight.type_name" class="text-grimoire-dim"> · {{ spotlight.type_name }}</span>
        <span class="text-grimoire-dim"> · байты {{ spotlight.offset }}‥{{ spotlight.offset + spotlight.size }}</span>
        <span v-if="spotlight.from" class="text-grimoire-dim"> · из {{ spotlight.from }}</span>
      </div>
      <div v-if="spotlight.value" class="text-grimoire-purple">{{ spotlight.value }}</div>
      <div v-if="spotlight.note" class="mt-1 whitespace-pre-wrap text-grimoire-dim">{{ spotlight.note }}</div>
    </div>

    <!-- урок little-endian: байты перелетают в число -->
    <div
      v-if="le"
      :key="leReplay"
      class="mt-3 rounded border border-grimoire-border bg-grimoire-panel2 px-3 py-2 text-sm"
    >
      <div class="mb-2 text-grimoire-dim">
        little-endian: в памяти — младший байт первым; число читается наоборот
      </div>
      <div class="flex items-center gap-4">
        <div class="flex gap-1">
          <span
            v-for="(b, i) in le.memory"
            :key="'m' + i"
            class="rounded bg-grimoire-panel px-2 py-1 text-xs"
            >{{ b }}</span
          >
        </div>
        <span class="text-grimoire-gold">→</span>
        <div class="flex gap-1">
          <span
            v-for="(b, i) in le.number"
            :key="'n' + i"
            class="le-byte rounded bg-grimoire-gold/20 px-2 py-1 text-xs text-grimoire-gold"
            :style="{ animationDelay: i * 0.12 + 's' }"
            >{{ b }}</span
          >
        </div>
        <span class="text-grimoire-dim">= 0x{{ le.number.join('') }}</span>
      </div>
    </div>
  </section>
</template>
