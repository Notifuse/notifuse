import { defineConfig, devices } from '@playwright/test';

/**
 * Browser-behaviour suite for the SDK: real browsers and emulated devices,
 * offline periods, clock skew, reloads. The server pipeline is covered
 * separately by the Go integration suite (real Postgres, real bundle), so
 * beats land in an in-memory collector here — no database, no API to boot.
 */
export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false, // Sequential for state-dependent tests
  retries: 1,
  workers: 1,
  reporter: 'html',
  timeout: 30000,

  use: {
    baseURL: 'http://localhost:3333',
    trace: 'on-first-retry',
    // Avoid bot detection
    launchOptions: {
      args: ['--disable-blink-features=AutomationControlled'],
    },
  },

  webServer: [
    {
      // Static file server for HTML pages + SDK bundle
      command: 'npx tsx tests/e2e/helpers/static-server.ts',
      url: 'http://localhost:3333/health',
      reuseExistingServer: !process.env.CI,
      timeout: 30000,
      stdout: 'pipe',
      stderr: 'pipe',
    },
    {
      // Stand-in for POST /track; the specs read the beats back from it
      command: 'npx tsx tests/e2e/helpers/collector-server.ts',
      url: 'http://localhost:4555/health',
      reuseExistingServer: !process.env.CI,
      timeout: 30000,
      stdout: 'pipe',
      stderr: 'pipe',
    },
  ],

  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    // Add more browsers later:
    // { name: 'firefox', use: { ...devices['Desktop Firefox'] } },
    // { name: 'webkit', use: { ...devices['Desktop Safari'] } },
    // { name: 'mobile-chrome', use: { ...devices['Pixel 5'] } },
    // { name: 'mobile-safari', use: { ...devices['iPhone 12'] } },
  ],
});
