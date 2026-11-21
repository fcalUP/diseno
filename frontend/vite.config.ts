import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// En desarrollo el API corre aparte con `go run` en el puerto 8080.
export default defineConfig({
  plugins: [react()],
  build: {
    // Lightning CSS redondea los porcentajes (h-5/6 → 83.3333%) y el panel rápido
    // quedaba 1/64 px distinto al CDN de Tailwind que usaba la versión anterior.
    cssMinify: false,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
      '/media': 'http://localhost:8080',
    },
  },
})
