<script setup lang="ts">
import { onMounted, ref } from 'vue'
import MonacoEditor from './editor/MonacoEditor.vue'
import ModelCard from './views/ModelCard.vue'
import OutputPanel from './views/OutputPanel.vue'
import { usePlayground } from './store/playground'

const pg = usePlayground()
onMounted(() => pg.init())

// фоновая проверка компилятором: debounce после паузы в наборе
let checkTimer: ReturnType<typeof setTimeout> | undefined
function onCode(code: string) {
  pg.setCode(code)
  clearTimeout(checkTimer)
  checkTimer = setTimeout(() => pg.check(), 700)
}

// перетаскиваемый разделитель: доля левой зоны в процентах
const split = ref(46)
let dragging = false
function startDrag() {
  dragging = true
  const move = (e: PointerEvent) => {
    if (!dragging) return
    split.value = Math.min(75, Math.max(25, (e.clientX / window.innerWidth) * 100))
  }
  const up = () => {
    dragging = false
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}

const shareCopied = ref(false)
async function share() {
  await navigator.clipboard.writeText(pg.shareLink())
  shareCopied.value = true
  setTimeout(() => (shareCopied.value = false), 1500)
}
</script>

<template>
  <div class="flex h-full flex-col font-mono">
    <!-- шапка -->
    <header
      class="flex items-center gap-3 border-b border-grimoire-border bg-grimoire-panel px-4 py-2"
    >
      <span class="text-lg">👁</span>
      <h1 class="text-grimoire-gold font-bold">Око мага — playground</h1>
      <select
        class="ml-4 max-w-72 rounded border border-grimoire-border bg-grimoire-panel2 px-2 py-1 text-sm"
        :value="pg.activeExample"
        @change="pg.selectExample(($event.target as HTMLSelectElement).value)"
      >
        <option value="" disabled>— пример-урок —</option>
        <option v-for="ex in pg.examples" :key="ex.id" :value="ex.id">
          {{ ex.title }}
        </option>
      </select>
      <div class="grow"></div>
      <button
        class="rounded border border-grimoire-border px-3 py-1 text-sm text-grimoire-dim hover:text-grimoire-ink"
        @click="share"
      >
        {{ shareCopied ? 'скопировано ✓' : 'поделиться' }}
      </button>
      <button
        class="rounded bg-grimoire-gold px-4 py-1 font-bold text-grimoire-bg transition hover:brightness-110 disabled:opacity-50"
        :disabled="pg.running"
        @click="pg.run()"
      >
        {{ pg.running ? '…смотрю' : 'Взглянуть ⌘⏎' }}
      </button>
    </header>

    <!-- сбой API (сеть, очередь, песочница) -->
    <div
      v-if="pg.apiError"
      class="border-b border-grimoire-danger/40 bg-grimoire-danger/10 px-4 py-1 text-sm text-grimoire-danger"
    >
      {{ pg.apiError }}
    </div>

    <!-- две зоны -->
    <main class="flex min-h-0 grow">
      <section :style="{ width: split + '%' }" class="min-w-0">
        <MonacoEditor :model-value="pg.code" :diagnostics="pg.diagnostics" @update:model-value="onCode" @run="pg.run()" />
      </section>

      <div
        class="w-1 shrink-0 cursor-col-resize bg-grimoire-border transition hover:bg-grimoire-gold"
        @pointerdown.prevent="startDrag"
      ></div>

      <section class="min-w-0 grow overflow-y-auto bg-grimoire-bg p-4">
        <template v-if="pg.result?.eye">
          <ModelCard
            v-for="(m, i) in pg.result.eye.models"
            :key="i"
            :model="m"
            class="mb-6"
          />
        </template>
        <div
          v-else-if="!pg.result"
          class="mt-16 text-center text-grimoire-dim"
        >
          <div class="mb-3 text-4xl">👁</div>
          <p>Опиши структуры слева — как в примерах —</p>
          <p>и нажми «Взглянуть», чтобы Око разобрало память.</p>
        </div>

        <OutputPanel v-if="pg.result" :result="pg.result" class="mt-2" />
      </section>
    </main>
  </div>
</template>
