import { defineConfig, loadEnv, type PluginOption } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

function redirectPlugin(): PluginOption {
  return {
    name: 'redirect-tastelingo',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        if (req.url === '/tastelingo') {
          res.writeHead(301, { Location: '/tastelingo/' })
          res.end()
          return
        }
        if (req.url === '/') {
          res.writeHead(302, { Location: '/tastelingo/' })
          res.end()
          return
        }
        if (req.url?.startsWith('/brand/')) {
          res.writeHead(301, { Location: `/tastelingo${req.url}` })
          res.end()
          return
        }
        next()
      })
    },
  }
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', 'TASTELINGO_API_PROXY')
  const apiProxyTarget = env.TASTELINGO_API_PROXY || 'http://127.0.0.1:8104'
  const base = process.env.VITE_BASE_PATH || '/tastelingo/'

  return {
    base,
    plugins: [react(), tailwindcss(), redirectPlugin()],
    server: {
      host: '0.0.0.0',
      port: 8105,
      strictPort: true,
      allowedHosts: true,
      proxy: {
        '/tastelingo/api': {
          target: apiProxyTarget,
          rewrite: (path) => path.replace(/^\/tastelingo/, ''),
        },
        '/api': apiProxyTarget,
      },
    },
    preview: { host: '0.0.0.0', port: 8105 },
  }
})
