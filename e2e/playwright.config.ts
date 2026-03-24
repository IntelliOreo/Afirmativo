import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "@playwright/test";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const statePath = path.resolve(__dirname, ".tmp", "runtime-state.json");
const sharedEnv = {
  AFIRMATIVO_E2E_STATE_PATH: statePath,
};

export default defineConfig({
  testDir: "./tests",
  timeout: 60_000,
  workers: 1,
  expect: {
    timeout: 10_000,
  },
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["html", { open: "never" }], ["list"]] : "list",
  use: {
    baseURL: "http://127.0.0.1:3000",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: {
        browserName: "chromium",
      },
    },
  ],
  webServer: [
    {
      command: "node ./scripts/stripe-mock-server.mjs",
      port: 12111,
      reuseExistingServer: false,
      env: sharedEnv,
      timeout: 30_000,
    },
    {
      command: "node ./scripts/start-backend.mjs",
      port: 8080,
      reuseExistingServer: false,
      env: sharedEnv,
      timeout: 60_000,
    },
    {
      command: "cd ../frontend && npm run dev -- --hostname 127.0.0.1 --port 3000",
      port: 3000,
      reuseExistingServer: false,
      env: {
        ...sharedEnv,
        API_PROXY_TARGET: "http://127.0.0.1:8080",
        APP_ENV: "development",
      },
      timeout: 120_000,
    },
  ],
});
