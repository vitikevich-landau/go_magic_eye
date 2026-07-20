<script setup lang="ts">
import type { Passport } from '../api/types'

defineProps<{ passport: Passport }>()
</script>

<template>
  <section class="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
    <div>
      <span class="text-grimoire-dim">размер </span>
      <span class="text-lg font-bold text-grimoire-gold">{{ passport.size }} Б</span>
    </div>
    <div>
      <span class="text-grimoire-dim">выравнивание </span>
      <span class="text-lg font-bold text-grimoire-gold">{{ passport.align }}</span>
    </div>
    <div>
      <span class="text-grimoire-dim">род </span>
      <span>{{ passport.kind }}</span>
    </div>
    <span
      v-for="t in passport.traits"
      :key="t"
      class="rounded-full border border-grimoire-border bg-grimoire-panel2 px-2 py-0.5 text-xs text-grimoire-purple"
    >
      {{ t }}
    </span>
  </section>
</template>
