import { defineConfig } from "@playwright/test";

const baseURL = process.env.BASE_URL || "http://localhost:5173";
const isLocal = baseURL.includes("localhost");

export default defineConfig({
  testDir: "./e2e",
  timeout: 15000,
  use: { baseURL },
  ...(isLocal && {
    webServer: {
      command: "npm run dev",
      url: "http://localhost:5173",
      reuseExistingServer: true,
    },
  }),
});
