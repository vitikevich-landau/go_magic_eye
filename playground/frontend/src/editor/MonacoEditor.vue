<script setup lang="ts">
// Обёртка Monaco: подсветка Go, тема «гримуар», маркеры диагностик от
// настоящего компилятора (/api/check), Ctrl/⌘+Enter — запуск.
// edcore.main — ядро редактора со всеми фичами (поиск, подсказки, свёртки),
// но БЕЗ пятидесяти языков editor.main: нам нужен только Go
import * as monaco from 'monaco-editor/esm/vs/editor/edcore.main'
import 'monaco-editor/esm/vs/basic-languages/go/go.contribution'
import editorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { Diagnostic } from '../api/types'

;(self as unknown as { MonacoEnvironment: object }).MonacoEnvironment = {
  getWorker: () => new editorWorker(),
}

const props = defineProps<{
  modelValue: string
  diagnostics: Diagnostic[]
}>()
const emit = defineEmits<{
  'update:modelValue': [code: string]
  run: []
}>()

const host = ref<HTMLElement | null>(null)
let editor: monaco.editor.IStandaloneCodeEditor | null = null

monaco.editor.defineTheme('grimoire', {
  base: 'vs-dark',
  inherit: true,
  rules: [
    { token: 'keyword', foreground: '9d7cd8' },
    { token: 'string', foreground: '7dc383' },
    { token: 'number', foreground: 'e6b450' },
    { token: 'comment', foreground: '7d6fa0', fontStyle: 'italic' },
    { token: 'type', foreground: 'e6b450' },
  ],
  colors: {
    'editor.background': '#1d1730',
    'editor.lineHighlightBackground': '#241c3a',
    'editorLineNumber.foreground': '#4a3f6b',
    'editorCursor.foreground': '#e6b450',
    'editor.selectionBackground': '#3a2f57',
  },
})

onMounted(() => {
  editor = monaco.editor.create(host.value!, {
    value: props.modelValue,
    language: 'go',
    theme: 'grimoire',
    fontSize: 14,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    automaticLayout: true,
    tabSize: 4,
    insertSpaces: false, // в Go — табы
    padding: { top: 12 },
  })
  editor.onDidChangeModelContent(() => {
    emit('update:modelValue', editor!.getValue())
  })
  editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => emit('run'))
})

onBeforeUnmount(() => editor?.dispose())

// код сменили снаружи (выбор примера, ссылка) — не сбивая курсор при
// обычном наборе: setValue только когда значения разошлись
watch(
  () => props.modelValue,
  (v) => {
    if (editor && editor.getValue() !== v) editor.setValue(v)
  },
)

watch(
  () => props.diagnostics,
  (diags) => {
    const model = editor?.getModel()
    if (!model) return
    monaco.editor.setModelMarkers(
      model,
      'go-build',
      diags.map((d) => ({
        severity: monaco.MarkerSeverity.Error,
        message: d.message,
        startLineNumber: d.line,
        startColumn: d.col,
        endLineNumber: d.line,
        endColumn: model.getLineMaxColumn(Math.min(d.line, model.getLineCount())),
      })),
    )
  },
  { deep: true },
)
</script>

<template>
  <div ref="host" class="h-full w-full"></div>
</template>
