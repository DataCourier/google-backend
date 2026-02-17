import { test, expect } from "@playwright/test";
import { createRequire } from "module";
const require = createRequire(import.meta.url);
const seedVideos = require("./seed-videos.json");

// These tests require:
// - Firestore emulator on :9090
// - Focus-tube backend on :8082
// - Vite dev server on :5173 (auto-started by playwright)

const API_BASE = "http://localhost:8082";

async function login(page, baseURL) {
  await page.goto(baseURL);
  await page.evaluate(() => localStorage.clear());
  await page.goto(baseURL);
  await page.waitForLoadState("networkidle");
  await page.getByRole("textbox", { name: "Email" }).fill("e2e-pagination@example.com");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("button", { name: "Logout" })).toBeVisible({ timeout: 5000 });
}

async function getToken(page) {
  return page.evaluate(() => localStorage.getItem("token"));
}

async function seedData(page) {
  const token = await getToken(page);
  // Batch in chunks of 100 (backend limit)
  for (let i = 0; i < seedVideos.length; i += 100) {
    const chunk = seedVideos.slice(i, i + 100);
    const res = await page.evaluate(
      async ({ apiBase, token, bucket, records }) => {
        const resp = await fetch(`${apiBase}/buckets/mine/${bucket}/batch`, {
          method: "POST",
          headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
          body: JSON.stringify(records),
        });
        return { status: resp.status, body: await resp.json() };
      },
      { apiBase: API_BASE, token, bucket: "videos", records: chunk }
    );
    if (res.status !== 200) throw new Error(`Seed failed: ${JSON.stringify(res.body)}`);
  }
}

async function deleteAllVideos(page) {
  const token = await getToken(page);
  const videos = await page.evaluate(
    async ({ apiBase, token }) => {
      const resp = await fetch(`${apiBase}/buckets/mine/videos`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      const body = await resp.json();
      return body.data || [];
    },
    { apiBase: API_BASE, token }
  );
  for (const v of videos) {
    await page.evaluate(
      async ({ apiBase, token, id }) => {
        await fetch(`${apiBase}/buckets/mine/videos/${id}`, {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        });
      },
      { apiBase: API_BASE, token, id: v.id }
    );
  }
}

test.describe("Pagination", () => {
  test.beforeEach(async ({ page, baseURL }) => {
    await login(page, baseURL);
    // Prevent auto-refresh from interfering
    await page.evaluate(() => localStorage.setItem("last_refresh_date", new Date().toISOString().slice(0, 10)));
    await deleteAllVideos(page);
    await seedData(page);
  });

  test("loads first 100 videos and shows load more button", async ({ page, baseURL }) => {
    await page.goto(baseURL);
    await page.waitForLoadState("networkidle");

    // Should show "X videos" count in the feed header
    const videoCount = page.getByText(/\d+ videos/);
    await expect(videoCount).toBeVisible({ timeout: 5000 });

    // Should have load more button since we seeded 150
    const loadMore = page.getByRole("button", { name: /Load more/ });
    await expect(loadMore).toBeVisible();
    await expect(loadMore).toContainText("100 of 150");
  });

  test("load more appends next batch of videos", async ({ page, baseURL }) => {
    await page.goto(baseURL);
    await page.waitForLoadState("networkidle");

    const loadMore = page.getByRole("button", { name: /Load more/ });
    await expect(loadMore).toBeVisible({ timeout: 5000 });
    await expect(loadMore).toContainText("100 of 150");

    await loadMore.click();

    // After loading more, all 150 should be loaded and button should disappear
    await expect(loadMore).not.toBeVisible({ timeout: 5000 });
  });

  test("pagination API returns correct limit and total", async ({ page }) => {
    const token = await getToken(page);

    const result = await page.evaluate(
      async ({ apiBase, token }) => {
        const resp = await fetch(
          `${apiBase}/buckets/mine/videos?limit=2&order_by=published&order_dir=desc`,
          { headers: { Authorization: `Bearer ${token}` } }
        );
        return resp.json();
      },
      { apiBase: API_BASE, token }
    );

    expect(result.data).toHaveLength(2);
    expect(result.total).toBe(150);
    // Verify desc order — first video should have later published date
    expect(new Date(result.data[0].published).getTime()).toBeGreaterThanOrEqual(
      new Date(result.data[1].published).getTime()
    );
  });

  test("pagination API respects offset", async ({ page }) => {
    const token = await getToken(page);

    const page1 = await page.evaluate(
      async ({ apiBase, token }) => {
        const resp = await fetch(
          `${apiBase}/buckets/mine/videos?limit=5&offset=0&order_by=published&order_dir=desc`,
          { headers: { Authorization: `Bearer ${token}` } }
        );
        return resp.json();
      },
      { apiBase: API_BASE, token }
    );

    const page2 = await page.evaluate(
      async ({ apiBase, token }) => {
        const resp = await fetch(
          `${apiBase}/buckets/mine/videos?limit=5&offset=5&order_by=published&order_dir=desc`,
          { headers: { Authorization: `Bearer ${token}` } }
        );
        return resp.json();
      },
      { apiBase: API_BASE, token }
    );

    expect(page1.data).toHaveLength(5);
    expect(page2.data).toHaveLength(5);
    expect(page1.total).toBe(150);
    expect(page2.total).toBe(150);

    // No overlap between pages
    const page1Ids = new Set(page1.data.map((v) => v.video_id));
    for (const v of page2.data) {
      expect(page1Ids.has(v.video_id)).toBe(false);
    }
  });
});
