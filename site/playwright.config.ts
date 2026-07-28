import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 15_000,
  workers: 1,
  reporter: 'line',
  use: {
    baseURL: 'http://127.0.0.1:41731',
  },
  webServer: {
    command: 'pnpm preview --host 127.0.0.1 --port 41731',
    url: 'http://127.0.0.1:41731/',
    reuseExistingServer: false,
    timeout: 30_000,
  },
});
