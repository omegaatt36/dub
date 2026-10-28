const { defineConfig } = require('@playwright/test');

module.exports = defineConfig({
  testDir: '.',
  testMatch: 'smoke.spec.js',
  workers: 1,
  fullyParallel: false,
  retries: 0,
  use: {
    browserName: 'chromium',
    viewport: { width: 1400, height: 900 },
    trace: 'retain-on-failure',
  },
});
