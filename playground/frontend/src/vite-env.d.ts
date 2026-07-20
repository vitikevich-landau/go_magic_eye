/// <reference types="vite/client" />

// edcore.main — тот же API, что «monaco-editor», но без пакета языков;
// типы у него общие с корневым модулем
declare module 'monaco-editor/esm/vs/editor/edcore.main' {
  export * from 'monaco-editor/esm/vs/editor/editor.api'
}

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<object, object, unknown>
  export default component
}
