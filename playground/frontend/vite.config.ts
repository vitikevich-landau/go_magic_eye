import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// Сборка кладётся в backend/web/dist — оттуда её вшивает go:embed.
// Каталог в .gitignore: собранный фронт — артефакт, не исходник.
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  build: {
    outDir: '../backend/web/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      // дев-режим: vite отдаёт фронт, бэкенд живёт отдельно на :8080
      '/api': 'http://localhost:8080',
    },
  },
})
