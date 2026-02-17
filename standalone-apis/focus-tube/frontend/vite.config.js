import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: process.env.VITE_BASE || '/',
  server: {
    proxy: {
      '/api': 'http://localhost:8082',
      '/auth': 'http://localhost:8082',
      '/buckets': 'http://localhost:8082',
    },
  },
})
