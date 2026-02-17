import { test, expect } from "@playwright/test";

// These tests require:
// - Firestore emulator on :9090
// - Focus-tube backend on :8082
// - Vite dev server on :5173 (auto-started by playwright)

const API_BASE = "http://localhost:8082";

async function login(page, baseURL, email = "e2e-notes@example.com") {
  await page.goto(baseURL);
  await page.evaluate(() => localStorage.clear());
  await page.goto(baseURL);
  await page.waitForLoadState("networkidle");
  await page.getByRole("textbox", { name: "Email" }).fill(email);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("button", { name: "Logout" })).toBeVisible({ timeout: 5000 });
  // Prevent auto-refresh from interfering
  await page.evaluate(() => localStorage.setItem("last_refresh_date", new Date().toISOString().slice(0, 10)));
}

async function getToken(page) {
  return page.evaluate(() => localStorage.getItem("token"));
}

async function seedVideo(page, overrides = {}) {
  const token = await getToken(page);
  const video = {
    id: "notes-test-video-1",
    video_id: "dQw4w9WgXcQ",
    title: "Test Video for Notes",
    published: "2024-01-01T00:00:00Z",
    channel: "Test Channel",
    channel_id: "UC_test",
    watched: false,
    favorited: false,
    ...overrides,
  };
  await page.evaluate(
    async ({ apiBase, token, record }) => {
      const resp = await fetch(`${apiBase}/buckets/mine/videos/batch`, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify([record]),
      });
      if (!resp.ok) throw new Error(`Seed failed: ${resp.status}`);
    },
    { apiBase: API_BASE, token, record: video }
  );
  return video;
}

async function getVideoFromAPI(page, videoDocId) {
  const token = await getToken(page);
  return page.evaluate(
    async ({ apiBase, token, id }) => {
      const resp = await fetch(`${apiBase}/buckets/mine/videos/${id}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      const json = await resp.json();
      return json.data;
    },
    { apiBase: API_BASE, token, id: videoDocId }
  );
}

async function updateVideoDirectly(page, videoDocId, data) {
  const token = await getToken(page);
  return page.evaluate(
    async ({ apiBase, token, id, data }) => {
      const resp = await fetch(`${apiBase}/buckets/mine/videos/${id}`, {
        method: "PUT",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      return { status: resp.status, body: await resp.json() };
    },
    { apiBase: API_BASE, token, id: videoDocId, data }
  );
}

test.describe("Notes debounce and versioning", () => {
  test.beforeEach(async ({ page, baseURL }) => {
    await login(page, baseURL);
  });

  test("typing saves after 2s debounce with version tracking", async ({ page, baseURL }) => {
    await seedVideo(page);
    await page.goto(baseURL);
    await page.waitForLoadState("networkidle");

    // Click the video to open it
    await page.getByText("Test Video for Notes").click();
    await expect(page.getByPlaceholder("Add notes about this video...")).toBeVisible();

    // Type a note
    const textarea = page.getByPlaceholder("Add notes about this video...");
    await textarea.fill("hello from e2e");

    // Should show "unsaved" immediately
    await expect(page.getByText("unsaved", { exact: true })).toBeVisible();

    // Should NOT have saved yet (debounce is 2s)
    await page.waitForTimeout(500);
    const beforeSave = await getVideoFromAPI(page, "notes-test-video-1");
    expect(beforeSave.notes).toBeFalsy();

    // Wait for debounce to fire and save to complete
    await expect(page.getByText("saved", { exact: true })).toBeVisible({ timeout: 5000 });

    // Verify the note was persisted with a version
    const afterSave = await getVideoFromAPI(page, "notes-test-video-1");
    expect(afterSave.notes).toBe("hello from e2e");
    expect(afterSave.version).toBe(1);
  });

  test("debounce resets when typing continues", async ({ page, baseURL }) => {
    await seedVideo(page);
    await page.goto(baseURL);
    await page.waitForLoadState("networkidle");

    await page.getByText("Test Video for Notes").click();
    const textarea = page.getByPlaceholder("Add notes about this video...");

    // Type first part
    await textarea.fill("part one");
    await page.waitForTimeout(1500);

    // Type more before the 2s debounce fires — should reset the timer
    await textarea.fill("part one part two");

    // Wait for the save (2s from last keystroke)
    await expect(page.getByText("saved", { exact: true })).toBeVisible({ timeout: 5000 });

    // Only the final text should have been saved (single save, not two)
    const result = await getVideoFromAPI(page, "notes-test-video-1");
    expect(result.notes).toBe("part one part two");
    expect(result.version).toBe(1);
  });

  test("version conflict shows warning instead of overwriting", async ({ page, baseURL }) => {
    // Seed a video with notes and version already set
    await seedVideo(page, { notes: "original note", version: 1 });
    await page.goto(baseURL);
    await page.waitForLoadState("networkidle");

    await page.getByText("Test Video for Notes").click();
    await expect(page.getByPlaceholder("Add notes about this video...")).toBeVisible();

    // Simulate a concurrent edit by bumping the version on the server
    const updateResult = await updateVideoDirectly(page, "notes-test-video-1", {
      notes: "updated by someone else",
      version: 1,
    });
    expect(updateResult.status).toBe(200);

    // Now the server has version=2, but our page still thinks it's version=1
    // Type a note — this should trigger a conflict on save
    const textarea = page.getByPlaceholder("Add notes about this video...");
    await textarea.fill("my conflicting edit");

    // Wait for save attempt and conflict message
    await expect(page.getByText("conflict")).toBeVisible({ timeout: 5000 });

    // The server should still have the other person's note, not ours
    const result = await getVideoFromAPI(page, "notes-test-video-1");
    expect(result.notes).toBe("updated by someone else");
    expect(result.version).toBe(2);
  });
});
