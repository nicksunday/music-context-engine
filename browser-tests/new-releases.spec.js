const { test, expect } = require("@playwright/test");

test("new releases has its own page with listening links and no recommendation session", async ({ page }) => {
  await page.route("**/api/rail", route => route.fulfill({ json: { new_releases: [
    { artist: "Gloryhammer", album: "New Album", release_date: "2026-10-15", release_type: "Album", match_reason: "Explore power metal — adjacent to folk metal in your liked music." },
    { artist: "Liked Artist", album: "Another Album", release_type: "Album" },
  ], release_error: "Some new release lookups failed; showing partial results." } }));
  await page.goto("/recommendations");
  await page.getByRole("link", { name: /New Releases New albums/ }).click();
  await expect(page.locator("#pageTitle")).toHaveText("New Releases");
  await expect(page.locator("#promptForm")).toBeHidden();
  await expect(page.locator(".workbench")).toBeHidden();
  await expect(page.locator("#sessionList")).toBeHidden();
  await expect(page.locator("#releaseStatus")).toContainText("partial results");
  const release = page.locator(".release-card").first();
  await expect(release).toContainText("Gloryhammer");
  await expect(release).toContainText("Album");
  await expect(release).toContainText("adjacent to folk metal");
  await expect(release).toHaveAttribute("href", "/api/releases/open?artist=Gloryhammer&album=New+Album");
  await expect(release).toHaveAttribute("target", "_blank");
  await page.reload();
  await expect(page.locator("#pageTitle")).toHaveText("New Releases");
  await expect(page.locator(".release-card")).toHaveCount(2);
});

test("new release errors and empty results appear on the page", async ({ page }) => {
  await page.route("**/api/rail", route => route.fulfill({ json: { new_releases: [] } }));
  await page.goto("/recommendations/releases");
  await expect(page.locator("#releaseStatus")).toContainText("No new releases");
  await page.route("**/api/rail", route => route.fulfill({ status: 500, json: { error: "Release feed unavailable" } }));
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page.locator("#releaseStatus")).toHaveText("Release feed unavailable");
});
