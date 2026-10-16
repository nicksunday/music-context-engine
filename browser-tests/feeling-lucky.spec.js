const { test, expect } = require("@playwright/test");

test("Feeling Lucky has a dedicated route and isolated refresh state", async ({ page }) => {
  await page.goto("/recommendations/lucky");
  await expect(page).toHaveTitle("Music Vault");
  await expect(page.locator("#pageTitle")).toHaveText("Feeling Lucky");
  await expect(page.locator("#pageDescription")).toContainText("random surprise");
  await expect(page.locator("#limit")).toHaveAttribute("max", "10");
  await expect(page.locator("#promptForm")).toBeHidden();
  await expect(page.locator("#feelingLuckyControls")).toBeVisible();
  await expect(page.locator("#feelingLuckyButton")).toHaveText("Feeling Lucky");
  await expect(page.locator("#message")).toBeHidden();
  await expect(page.locator("#mood")).toBeHidden();
  await expect(page.locator("#avoid")).toBeHidden();
  await expect(page.locator("#limit")).toBeHidden();
  await expect(page.locator("#candidateList")).toContainText("No current batch");
  await expect(page.locator(".recommendation-nav a[data-mode='feeling_lucky']")).toHaveClass(/active/);

  await page.reload();
  await expect(page.locator("#pageTitle")).toHaveText("Feeling Lucky");
  await expect(page.locator("#candidateList")).toContainText("No current batch");
  await expect(page.locator("#sessionHeading")).toHaveText("Feeling Lucky Sessions");
});

test("Feeling Lucky navigation does not leak the album session", async ({ page }) => {
  await page.goto("/recommendations/lucky");
  await expect(page.locator("#candidateList")).not.toContainText("Browser Album");
  await page.goto("/recommendations/albums");
  await expect(page.locator("#candidateList")).toContainText("Browser Album");
  await page.goto("/recommendations/lucky");
  await expect(page.locator("#candidateList")).not.toContainText("Browser Album");
});