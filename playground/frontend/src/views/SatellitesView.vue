<script setup lang="ts">
// Спутники — память, живущая вне объекта: буфер строки, хребет среза,
// содержимое map, цель указателя.
import type { Satellite } from '../api/types'
import { hexToBytes, toAscii } from './memoryLayout'

defineProps<{ sats: Satellite[] }>()

function dump(hex: string): { hex: string; ascii: string }[] {
  return hexToBytes(hex).map((b) => ({
    hex: b.toString(16).padStart(2, '0'),
    ascii: toAscii(b),
  }))
}
</script>

<template>
  <section>
    <h3 class="mb-2 text-xs font-bold uppercase tracking-wider text-grimoire-dim">
      спутники — память вне объекта
    </h3>
    <div class="space-y-2">
      <div
        v-for="(s, i) in sats"
        :key="i"
        class="rounded border border-grimoire-border bg-grimoire-panel2 p-3 text-sm"
      >
        <div class="mb-1">
          <span class="font-bold text-grimoire-gold">🛰 {{ s.title }}</span>
          <span v-if="s.addr" class="text-xs text-grimoire-dim"> @ {{ s.addr }}</span>
          <span class="text-xs text-grimoire-dim"> · {{ s.size }} Б</span>
        </div>

        <div v-if="s.bytes" class="flex max-w-full flex-wrap gap-px font-mono text-xs">
          <span
            v-for="(b, j) in dump(s.bytes)"
            :key="j"
            class="byte-cell tone-fieldA rounded-sm px-1 py-0.5"
            :title="b.ascii"
            >{{ b.hex }}</span
          >
          <!-- конверт несёт усечённый дамп: сериализация режет гигантов,
               чтобы модель выживала под потолками транспорта -->
          <span v-if="s.bytes.length / 2 < s.size" class="px-1 py-0.5 text-grimoire-dim">
            ⋯ ещё {{ s.size - s.bytes.length / 2 }} Б ⋯
          </span>
        </div>

        <ul v-if="s.elems.length" class="mt-1 space-y-0.5 text-xs text-grimoire-purple">
          <li v-for="(el, j) in s.elems" :key="j">{{ el }}</li>
        </ul>

        <p v-if="s.note" class="mt-1 whitespace-pre-wrap text-xs text-grimoire-dim">{{ s.note }}</p>
      </div>
    </div>
  </section>
</template>
