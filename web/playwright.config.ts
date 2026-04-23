import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 360_000,
  workers: 1,
  use: {
    baseURL: 'http://localhost:8080',
    trace: 'retain-on-failure',
    actionTimeout: 360_000,
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: 'go run . serve',
    cwd: '../',
    port: 8080,
    reuseExistingServer: !process.env.CI,
    env: { MOONSHOT_API_KEY: process.env.MOONSHOT_API_KEY ?? '' },
    timeout: 30_000,
  },
})
