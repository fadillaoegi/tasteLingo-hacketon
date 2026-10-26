import { defineConfig, loadEnv, type PluginOption } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// redirectToBase sends dev-server visits to / or the base without its trailing slash to the base path.
function redirectToBase(base: string): PluginOption {
  return {
    name: 'redirect-to-base',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        // The project has no Node typings, so read url without them.
        const { url } = req as { url?: string }
        if (base !== '/' && (url === '/' || url === base.slice(0, -1))) {
          res.writeHead(302, { Location: base })
          res.end()
          return
        }
        next()
      })
    },
  }
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', 'TASTELINGO_')
  // TASTELINGO_BASE serves the app from a subpath, for example /tastelingo/.
  const base = `/${(env.TASTELINGO_BASE || '/').replace(/^\/+|\/+$/g, '')}/`.replace('//', '/')
  const api = `${base}api`
  return {
    base,
    plugins: [react(), tailwindcss(), redirectToBase(base)],
    server: {
      host: '0.0.0.0',
      port: 8105,
      strictPort: true,
      // The dev server is reached through a reverse proxy/tunnel on this host.
      allowedHosts: true,
      proxy: {
        [api]: {
          target: env.TASTELINGO_API_PROXY || 'http://127.0.0.1:8104',
          rewrite: (path) => '/api' + path.slice(api.length),
        },
      },
    },
    preview: { host: '0.0.0.0', port: 8105 },
  }
})
