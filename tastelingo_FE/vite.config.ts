import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', 'TASTELINGO_')
  // TASTELINGO_BASE serves the app from a subpath, for example /tastelingo/.
  const base = `/${(env.TASTELINGO_BASE || '/').replace(/^\/+|\/+$/g, '')}/`.replace('//', '/')
  const api = `${base}api`
  return {
    base,
    plugins: [react(), tailwindcss()],
    server: {
      host: '127.0.0.1',
      port: 5190,
      strictPort: true,
      proxy: {
        [api]: {
          target: env.TASTELINGO_API_PROXY || 'http://127.0.0.1:8110',
          rewrite: (path) => '/api' + path.slice(api.length),
        },
      },
    },
    preview: { host: '127.0.0.1', port: 5190 },
  }
})
