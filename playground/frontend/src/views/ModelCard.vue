<script setup lang="ts">
// Карточка одной модели Ока. Держит общее состояние подсветки: карта
// памяти, список полей и этажи встраивания подсвечивают друг друга.
import { ref } from 'vue'
import type { EyeModel } from '../api/types'
import PassportCard from './PassportCard.vue'
import MemoryMap from './MemoryMap.vue'
import EmbedsView from './EmbedsView.vue'
import IfaceDiagram from './IfaceDiagram.vue'
import SatellitesView from './SatellitesView.vue'

defineProps<{ model: EyeModel }>()

// lit — что сейчас горит: регион по индексу или произвольный диапазон байт
// (от этажа встраивания)
const litRegion = ref<number | null>(null)
const litRange = ref<{ offset: number; size: number } | null>(null)

function onLitRegion(i: number | null) {
  litRegion.value = i
  if (i !== null) litRange.value = null
}
function onLitRange(r: { offset: number; size: number } | null) {
  litRange.value = r
  if (r !== null) litRegion.value = null
}
</script>

<template>
  <article class="rounded-lg border border-grimoire-border bg-grimoire-panel">
    <header class="flex items-baseline gap-3 border-b border-grimoire-border px-4 py-2">
      <h2 class="font-bold text-grimoire-gold">
        {{ model.label || model.passport.type_name }}
      </h2>
      <span class="text-sm text-grimoire-dim">{{ model.passport.type_name }}</span>
      <span v-if="model.addr" class="ml-auto text-xs text-grimoire-dim">@ {{ model.addr }}</span>
      <span v-else class="ml-auto text-xs text-grimoire-dim">тип без объекта</span>
    </header>

    <div class="space-y-4 p-4">
      <PassportCard :passport="model.passport" />

      <EmbedsView
        v-if="model.embeds.length"
        :embeds="model.embeds"
        :size="model.passport.size"
        @lit="onLitRange"
      />

      <MemoryMap
        v-if="model.bytes"
        :model="model"
        :lit-region="litRegion"
        :lit-range="litRange"
        @lit="onLitRegion"
      />

      <IfaceDiagram v-for="(ifc, i) in model.ifaces" :key="i" :iface="ifc" />

      <SatellitesView v-if="model.sats.length" :sats="model.sats" />

      <ul v-if="model.notes.length" class="space-y-1 text-sm text-grimoire-dim">
        <li v-for="(n, i) in model.notes" :key="i" class="flex gap-2">
          <span class="text-grimoire-gold">✦</span>
          <span class="whitespace-pre-wrap">{{ n }}</span>
        </li>
      </ul>
    </div>
  </article>
</template>
