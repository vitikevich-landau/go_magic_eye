<script setup lang="ts">
// Встраивание — этажи под-объектов. Hover по этажу подсвечивает его байты
// на карте памяти (событие lit с диапазоном offset‥offset+size).
import type { Embed } from '../api/types'

defineProps<{ embeds: Embed[]; size: number }>()
const emit = defineEmits<{ lit: [range: { offset: number; size: number } | null] }>()
</script>

<template>
  <section>
    <h3 class="mb-2 text-xs font-bold uppercase tracking-wider text-grimoire-dim">
      встраивание — композиция вместо наследования
    </h3>
    <div class="space-y-1">
      <div
        v-for="(e, i) in embeds"
        :key="i"
        class="cursor-pointer rounded border border-grimoire-purple/40 bg-grimoire-panel2 px-3 py-1.5 text-sm transition hover:border-grimoire-gold"
        :style="{ marginLeft: e.depth * 1.5 + 'rem' }"
        @mouseenter="emit('lit', { offset: e.offset, size: e.size })"
        @mouseleave="emit('lit', null)"
      >
        <span class="font-bold text-grimoire-purple">{{ e.field_name || e.type_name }}</span>
        <span class="text-grimoire-dim"> · {{ e.type_name }} · байты {{ e.offset }}‥{{ e.offset + e.size }}</span>
        <span v-if="e.promoted.length" class="text-xs text-grimoire-dim">
          · наружу: <span class="text-grimoire-ok">{{ e.promoted.join(', ') }}</span>
        </span>
        <div v-if="e.note" class="text-xs text-grimoire-dim">{{ e.note }}</div>
      </div>
    </div>
  </section>
</template>
