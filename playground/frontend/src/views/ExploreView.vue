<script setup lang="ts">
// Странствие в браузере: слева живое дерево (ленивые раскрытия по протоколу
// сеанса), справа — Гримуар выбранного узла (тот же ModelCard, что в
// осмотре), сверху — крошки пути, внизу — печать программы.
import { useExplore } from '../store/explore'
import TreeRow from './TreeRow.vue'
import ModelCard from './ModelCard.vue'

const ex = useExplore()
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <!-- крошки: где я -->
    <div
      v-if="ex.breadcrumbs.length"
      class="mb-2 truncate border-b border-grimoire-border pb-2 text-xs text-grimoire-dim"
    >
      <template v-for="(seg, i) in ex.breadcrumbs" :key="i">
        <span v-if="i > 0" class="text-grimoire-gold"> › </span>
        <span :class="i === ex.breadcrumbs.length - 1 ? 'text-grimoire-ink' : ''">{{ seg }}</span>
      </template>
    </div>

    <div class="flex min-h-0 grow gap-4">
      <!-- дерево -->
      <div class="w-2/5 min-w-56 shrink-0 overflow-y-auto rounded border border-grimoire-border bg-grimoire-panel p-2">
        <TreeRow v-for="r in ex.roots" :key="r.id" :node="r" :depth="0" />
        <p v-if="!ex.roots.length" class="p-2 text-sm text-grimoire-dim">дерево пусто</p>
      </div>

      <!-- Гримуар узла -->
      <div class="min-w-0 grow overflow-y-auto">
        <ModelCard v-if="ex.detail" :model="ex.detail" />
        <div v-else-if="ex.detailLoading" class="mt-8 text-center text-grimoire-dim">…смотрю</div>
        <div v-else class="mt-8 text-center text-sm text-grimoire-dim">
          выбери узел в дереве — его Гримуар появится здесь
        </div>
      </div>
    </div>

    <!-- печать программы (горутины могут говорить и во время странствия) -->
    <pre
      v-if="ex.stdoutLog"
      class="mt-3 max-h-32 shrink-0 overflow-y-auto rounded bg-black/30 px-3 py-2 text-xs text-grimoire-ink"
    >{{ ex.stdoutLog }}</pre>
  </div>
</template>
