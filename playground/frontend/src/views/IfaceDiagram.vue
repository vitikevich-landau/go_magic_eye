<script setup lang="ts">
// Анатомия interface-значения: «значение → два слова → itab → слот → код».
// Стрелки прорисовываются анимированно; typed nil горит тревожным цветом.
import { computed } from 'vue'
import type { Iface } from '../api/types'

const props = defineProps<{ iface: Iface }>()

// nil-интерфейс определяется по сырым словам (оба нулевые — рисовать
// нечего), а не по имени динамического типа: пользовательский тип
// `nilBox` не должен прикидываться nil'ом из-за подстроки
const isNil = computed(
  () => !props.iface.tab_addr && !props.iface.data_addr && !props.iface.typed_nil,
)
</script>

<template>
  <section class="rounded border border-grimoire-border bg-grimoire-panel2 p-3">
    <h3 class="mb-2 text-xs font-bold uppercase tracking-wider text-grimoire-dim">
      интерфейс · {{ iface.where }}
      <span
        v-if="iface.typed_nil"
        class="ml-2 rounded bg-grimoire-danger/20 px-2 py-0.5 normal-case text-grimoire-danger"
        >⚠ ловушка typed nil</span
      >
    </h3>

    <div class="flex flex-wrap items-stretch gap-0 text-sm">
      <!-- два слова значения -->
      <div class="rounded border border-grimoire-purple/50">
        <div class="border-b border-grimoire-purple/30 px-3 py-1 text-xs text-grimoire-dim">
          {{ iface.type_name }} — два слова
        </div>
        <div class="grid grid-cols-2 divide-x divide-grimoire-purple/30">
          <div class="px-3 py-2">
            <div class="text-xs text-grimoire-dim">{{ iface.empty ? '_type' : 'itab' }}</div>
            <div :class="iface.tab_addr ? 'text-grimoire-purple' : 'text-grimoire-dim'">
              {{ iface.tab_addr || 'nil' }}
            </div>
          </div>
          <div class="px-3 py-2">
            <div class="text-xs text-grimoire-dim">data</div>
            <div :class="iface.data_addr ? 'text-grimoire-purple' : (iface.typed_nil ? 'text-grimoire-danger' : 'text-grimoire-dim')">
              {{ iface.data_addr || 'nil' }}
            </div>
          </div>
        </div>
      </div>

      <!-- стрелка к itab -->
      <svg v-if="!isNil" width="48" height="72" class="shrink-0">
        <path
          d="M 4 36 C 20 36, 28 36, 40 36"
          fill="none"
          stroke="var(--color-grimoire-gold)"
          stroke-width="1.5"
          class="arrow-draw"
        />
        <path d="M 40 36 l -6 -4 v 8 z" fill="var(--color-grimoire-gold)" />
      </svg>

      <!-- itab и слоты -->
      <div v-if="!isNil" class="rounded border border-grimoire-gold/40">
        <div class="border-b border-grimoire-gold/30 px-3 py-1 text-xs text-grimoire-dim">
          {{ iface.empty ? 'дескриптор типа' : 'itab — «vtable» Go' }}
          <span v-if="iface.hash" class="ml-2">hash {{ iface.hash.toString(16) }}</span>
        </div>
        <div class="px-3 py-2">
          <div class="mb-1 text-xs text-grimoire-dim">
            динамический тип: <span class="text-grimoire-gold">{{ iface.dyn_type }}</span>
          </div>
          <table v-if="iface.methods.length" class="text-xs">
            <tbody>
              <tr v-for="(m, i) in iface.methods" :key="i" class="group">
                <td class="pr-3 text-grimoire-dim">fun[{{ i }}]</td>
                <td class="pr-3 font-bold">{{ m.name }}</td>
                <td class="pr-3 text-grimoire-dim">{{ m.pc }}</td>
                <td class="text-grimoire-purple">→ {{ m.func }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <p v-if="iface.note" class="mt-2 whitespace-pre-wrap text-xs text-grimoire-dim">{{ iface.note }}</p>
  </section>
</template>
