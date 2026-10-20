import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './test/e2e/specs',
  fullyParallel: false,
  workers: 1,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  updateSnapshots: 'none',
  outputDir: './test-results/artifacts',
  timeout: 30_000,
  expect: {
    timeout: 5_000,
    toHaveScreenshot: {
      animations: 'disabled',
      caret: 'hide',
      maxDiffPixels: 0,
      threshold: 0,
    },
  },
  snapshotPathTemplate: '{testDir}/../snapshots/{projectName}/{testFilePath}/{arg}{ext}',
  reporter: [['list'], ['html', { open: 'never', outputFolder: 'playwright-report/html' }]],
  use: {
    baseURL: 'http://127.0.0.1:18763',
    httpCredentials: { username: 'admin', password: 'test-password' },
    extraHTTPHeaders: {
      Authorization: `Basic ${Buffer.from('admin:test-password').toString('base64')}`,
    },
    locale: 'en-GB',
    timezoneId: 'UTC',
    viewport: { width: 1440, height: 1000 },
    deviceScaleFactor: 1,
    reducedMotion: 'reduce',
    colorScheme: 'light',
    serviceWorkers: 'block',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        browserName: 'chromium',
        launchOptions: { args: ['--disable-gpu'] },
      },
    },
    {
      name: 'firefox',
      use: { browserName: 'firefox' },
      testMatch: '**/behavior.spec.js',
    },
  ],
  webServer: {
    command: process.env.E2E_SERVER_COMMAND || 'go run ./test/e2e/server --addr 127.0.0.1:18763',
    url: 'http://127.0.0.1:18763/__test/ready',
    reuseExistingServer: false,
    timeout: 120_000,
    env: { TZ: 'UTC' },
  },
});
