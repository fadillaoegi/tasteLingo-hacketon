import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig(({ mode }) => ({
  plugins: [react(), tailwindcss()],
  server: { host: '127.0.0.1', port: 5190, strictPort: true, proxy: { '/api': loadEnv(mode, '.', 'TASTELINGO_API_PROXY').TASTELINGO_API_PROXY || 'http://127.0.0.1:8110' } },
  preview: { host: '127.0.0.1', port: 5190 },
}))
