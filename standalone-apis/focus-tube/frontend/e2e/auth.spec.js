import { test, expect } from "@playwright/test";

// These tests require:
// - Firestore emulator on :9090
// - Focus-tube backend on :8082
// - Vite dev server on :5173 (auto-started by playwright)

test.describe("Auth flow", () => {
  test.beforeEach(async ({ page, baseURL }) => {
    await page.goto(baseURL);
    await page.evaluate(() => localStorage.clear());
    await page.goto(baseURL);
    await page.waitForLoadState("networkidle");
  });

  test("shows login page when not authenticated", async ({ page }) => {
    await expect(page.getByRole("heading", { name: "FocusTube" })).toBeVisible();
    await expect(page.getByRole("textbox", { name: "Email" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  });

  test("full login flow: email → code → auto-verify → main app", async ({ page }) => {
    await page.getByRole("textbox", { name: "Email" }).fill("e2e-test@example.com");
    await page.getByRole("button", { name: "Sign in" }).click();

    // Should auto-verify and land on main app
    await expect(page.getByRole("button", { name: "Logout" })).toBeVisible({ timeout: 5000 });

    // Token and email should be stored
    const token = await page.evaluate(() => localStorage.getItem("token"));
    expect(token).toBeTruthy();
    expect(token.length).toBeGreaterThan(10);

    const email = await page.evaluate(() => localStorage.getItem("user_email"));
    expect(email).toBe("e2e-test@example.com");

    // Email should be visible in the sidebar
    await expect(page.getByText("e2e-test@example.com")).toBeVisible();

    // App should have loaded (not stuck on empty state from pre-login)
    // "All channels" button confirms the main app rendered and init() ran
    await expect(page.getByRole("button", { name: "All channels" })).toBeVisible();
  });

  test("session persists across page reload", async ({ page }) => {
    // Login first
    await page.getByRole("textbox", { name: "Email" }).fill("e2e-persist@example.com");
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByRole("button", { name: "Logout" })).toBeVisible({ timeout: 5000 });

    // Reload
    await page.reload();
    await page.waitForLoadState("networkidle");

    // Should still be logged in with email visible
    await expect(page.getByRole("button", { name: "Logout" })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("textbox", { name: "Email" })).not.toBeVisible();
    await expect(page.getByText("e2e-persist@example.com")).toBeVisible();
  });

  test("logout returns to login page", async ({ page }) => {
    // Login
    await page.getByRole("textbox", { name: "Email" }).fill("e2e-logout@example.com");
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByRole("button", { name: "Logout" })).toBeVisible({ timeout: 5000 });

    // Logout
    await page.getByRole("button", { name: "Logout" }).click();

    // Should see login page
    await expect(page.getByRole("textbox", { name: "Email" })).toBeVisible({ timeout: 5000 });

    // Token and email should be cleared
    const token = await page.evaluate(() => localStorage.getItem("token"));
    expect(token).toBeNull();
    const email = await page.evaluate(() => localStorage.getItem("user_email"));
    expect(email).toBeNull();
  });

  test("invalid email stays on login page", async ({ page }) => {
    // type="email" validation — "not-an-email" won't submit
    await page.getByRole("textbox", { name: "Email" }).fill("not-an-email");
    await page.getByRole("button", { name: "Sign in" }).click();

    // Should stay on login page
    await expect(page.getByRole("textbox", { name: "Email" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  });
});
