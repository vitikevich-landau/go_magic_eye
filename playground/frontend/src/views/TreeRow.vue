<script setup lang="ts">
// Рекурсивная строка дерева странствия: шеврон раскрытия, метка, аннотация;
// циклы ⟲/≡ — бейджем-кнопкой прыжка к оригиналу, отказы — подсказкой.
import { useExplore, type TreeNode } from '../store/explore'

const props = defineProps<{ node: TreeNode; depth: number }>()
const ex = useExplore()

function chevron(): string {
  if (props.node.cycle) return props.node.shared ? '≡' : '⟲'
  if (!props.node.expandable) return '·'
  return props.node.expanded ? '▾' : '▸'
}

function onRowClick() {
  if (props.node.cycle) {
    ex.jumpTo(props.node.cycle)
    return
  }
  void ex.select(props.node.id)
}
</script>

<template>
  <div>
    <div
      class="flex cursor-pointer items-baseline gap-1.5 rounded px-1 py-0.5 text-sm transition"
      :class="ex.selectedId === node.id ? 'bg-grimoire-panel2 text-grimoire-ink' : 'text-grimoire-dim hover:bg-grimoire-panel2/60'"
      :style="{ paddingLeft: depth * 1.1 + 0.25 + 'rem' }"
      :title="node.refusal || node.copied || undefined"
      @click="onRowClick"
    >
      <button
        class="w-4 shrink-0 text-center"
        :class="node.cycle ? 'text-grimoire-purple' : 'text-grimoire-gold'"
        @click.stop="node.cycle ? ex.jumpTo(node.cycle) : ex.toggle(node.id)"
      >
        {{ chevron() }}
      </button>
      <span class="truncate text-grimoire-ink">{{ node.label }}</span>
      <span v-if="node.sub" class="truncate text-xs">{{ node.sub }}</span>
      <span v-if="node.copied" class="shrink-0 text-xs text-grimoire-gold" title="копия, не живая память">⚠</span>
      <span v-if="node.refusal" class="shrink-0 truncate text-xs text-grimoire-danger/80">{{ node.refusal }}</span>
    </div>

    <template v-if="node.expanded && node.children">
      <TreeRow v-for="k in node.children" :key="k.id" :node="k" :depth="depth + 1" />
    </template>
  </div>
</template>
