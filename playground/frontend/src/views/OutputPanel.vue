<script setup lang="ts">
// stdout/stderr программы и время прогона. Конверты Ока сюда не попадают —
// их уже выудил сервер; здесь только печать самого пользователя.
import type { RunResponse } from '../api/types'

defineProps<{ result: RunResponse }>()
</script>

<template>
  <section class="text-sm">
    <div
      v-if="result.timed_out"
      class="mb-2 rounded border border-grimoire-danger/40 bg-grimoire-danger/10 px-3 py-2 text-grimoire-danger"
    >
      {{ result.stderr }}
    </div>

    <div v-if="result.stdout" class="mb-2">
      <h3 class="mb-1 text-xs font-bold uppercase tracking-wider text-grimoire-dim">stdout</h3>
      <pre class="overflow-x-auto rounded bg-black/30 px-3 py-2 text-grimoire-ink">{{ result.stdout }}</pre>
    </div>

    <div v-if="result.stderr && !result.timed_out" class="mb-2">
      <h3 class="mb-1 text-xs font-bold uppercase tracking-wider text-grimoire-dim">stderr</h3>
      <pre class="overflow-x-auto rounded bg-black/30 px-3 py-2 text-grimoire-danger">{{ result.stderr }}</pre>
    </div>

    <p v-if="result.ok" class="text-right text-xs text-grimoire-dim">
      компиляция {{ result.compile_ms }} мс · запуск {{ result.run_ms }} мс
    </p>
  </section>
</template>
