<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import MonacoEditor from './editor/MonacoEditor.vue'
import ModelCard from './views/ModelCard.vue'
import OutputPanel from './views/OutputPanel.vue'
import ExploreView from './views/ExploreView.vue'
import { usePlayground } from './store/playground'
import { useExplore } from './store/explore'

const pg = usePlayground()
const ex = useExplore()
onMounted(() => pg.init())

// режим: осмотр (разовый прогон) или странствие (живой сеанс)
const mode = ref<'inspect' | 'explore'>('inspect')

function setMode(m: 'inspect' | 'explore') {
  if (mode.value === m) return
  mode.value = m
  if (m === 'inspect') ex.stop() // ушли из странствия — сеанс серверу не нужен
}

async function look() {
  if (mode.value === 'explore') {
    await ex.start(pg.code)
    pg.diagnostics = ex.diagnostics // маркеры компилятора — в редактор
  } else {
    void pg.run()
  }
}

const onUnload = () => ex.stop()
window.addEventListener('beforeunload', onUnload)
onBeforeUnmount(() => window.removeEventListener('beforeunload', onUnload))

// фоновая проверка компилятором: debounce после паузы в наборе
let checkTimer: ReturnType<typeof setTimeout> | undefined
function onCode(code: string) {
  const changed = code !== pg.code
  pg.setCode(code)
  // правка кода в странствии рвёт связь дерева с живой памятью: дерево
  // описывало ПРОШЛЫЙ код. Гасим сеанс (и инвалидируем летящий start через
  // поколение внутри stop) — как stale-guard'ы у run/check. Условие на
  // changed бережёт от лишних stop при программной установке того же кода
  if (changed && mode.value === 'explore' && (ex.session || ex.starting)) {
    ex.stop()
  }
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
      <!-- режим: осмотр | странствие -->
      <div class="ml-2 flex overflow-hidden rounded border border-grimoire-border text-sm">
        <button
          class="px-3 py-1 transition"
          :class="mode === 'inspect' ? 'bg-grimoire-panel2 text-grimoire-gold' : 'text-grimoire-dim hover:text-grimoire-ink'"
          @click="setMode('inspect')"
        >
          осмотр
        </button>
        <button
          class="px-3 py-1 transition"
          :class="mode === 'explore' ? 'bg-grimoire-panel2 text-grimoire-gold' : 'text-grimoire-dim hover:text-grimoire-ink'"
          @click="setMode('explore')"
        >
          странствие
        </button>
      </div>
      <div class="grow"></div>
      <button
        class="rounded border border-grimoire-border px-3 py-1 text-sm text-grimoire-dim hover:text-grimoire-ink"
        @click="share"
      >
        {{ shareCopied ? 'скопировано ✓' : 'поделиться' }}
      </button>
      <button
        class="rounded bg-grimoire-gold px-4 py-1 font-bold text-grimoire-bg transition hover:brightness-110 disabled:opacity-50"
        :disabled="pg.running || ex.starting"
        @click="look()"
      >
        {{ pg.running || ex.starting ? '…смотрю' : 'Взглянуть ⌘⏎' }}
      </button>
    </header>

    <!-- сбой API (сеть, очередь, песочница) или беда сеанса -->
    <div
      v-if="pg.apiError || (mode === 'explore' && ex.error)"
      class="border-b border-grimoire-danger/40 bg-grimoire-danger/10 px-4 py-1 text-sm text-grimoire-danger"
    >
      {{ mode === 'explore' && ex.error ? ex.error : pg.apiError }}
    </div>

    <!-- две зоны -->
    <main class="flex min-h-0 grow">
      <section :style="{ width: split + '%' }" class="min-w-0">
        <MonacoEditor :model-value="pg.code" :diagnostics="pg.diagnostics" @update:model-value="onCode" @run="look()" />
      </section>

      <div
        class="w-1 shrink-0 cursor-col-resize bg-grimoire-border transition hover:bg-grimoire-gold"
        @pointerdown.prevent="startDrag"
      ></div>

      <section class="min-w-0 grow overflow-y-auto bg-grimoire-bg p-4">
        <!-- странствие: живое дерево -->
        <ExploreView v-if="mode === 'explore' && ex.session" class="h-full" />
        <div
          v-else-if="mode === 'explore'"
          class="mt-16 text-center text-grimoire-dim"
        >
          <div class="mb-3 text-4xl">🧭</div>
          <p>Странствие — прогулка по живому объекту: дерево, указатели, циклы.</p>
          <p>В снипетте должен быть <code class="text-grimoire-gold">eye.Explore(&объект)</code> — возьми пример</p>
          <p>«Странствие: живое дерево с циклами» и нажми «Взглянуть».</p>
        </div>

        <!-- осмотр: разовый прогон -->
        <template v-else>
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
        </template>
      </section>
    </main>
  </div>
</template>
