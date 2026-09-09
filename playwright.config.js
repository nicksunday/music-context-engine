const { defineConfig } = require("@playwright/test");

module.exports = defineConfig({
  testDir: "./browser-tests",
  timeout: 30_000,
  use: {
    baseURL: "http://127.0.0.1:18787",
    headless: true,
  },
  webServer: {
    command: "go run ./browser-tests/server.go --addr 127.0.0.1:18787 --db browser-tests/.tmp/browser.db",
    url: "http://127.0.0.1:18787/api/batch/latest?mode=album",
    reuseExistingServer: false,
    timeout: 120_000,
  },
});