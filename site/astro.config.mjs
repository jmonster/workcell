import { defineConfig } from 'astro/config';

export default defineConfig({
  site: 'https://workcell-137.pages.dev',
  output: 'static',
  server: {
    host: '127.0.0.1',
    port: 4322,
  },
});
