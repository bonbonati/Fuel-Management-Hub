import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

// The API is reached through the gateway. In development Vite proxies /api to
// the local docker compose gateway (http://localhost:8080), so the app runs
// same-origin exactly like in production.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: { '/api': { target: process.env.VITE_GATEWAY ?? 'http://localhost:8080', changeOrigin: true } },
  },
  build: { target: 'es2023', sourcemap: true },
  test: { environment: 'jsdom', globals: true, setupFiles: ['./test/setup.ts'] },
});
