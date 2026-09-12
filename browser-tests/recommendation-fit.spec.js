const { test, expect } = require("@playwright/test");

test("restores album and song fit feedback and presents failed saves", async ({ page }) => {
  await page.goto("/recommendations/albums");
  await expect(page.locator("#candidateList")).toContainText("Browser Album");
  await expect(page.locator(".prompt-fit")).toContainText("Met my request");
  await expect(page.locator(".prompt-fit-button.selected")).toHaveText("Met my request");

  await page.reload();
  await expect(page.locator(".prompt-fit-button.selected")).toHaveText("Met my request");

  await page.goto("/recommendations/songs");
  await expect(page.locator("#candidateList")).toContainText("Browser Song");
  await expect(page.locator(".prompt-fit")).toContainText("Missed my request");
  await expect(page.locator(".prompt-fit-button.selected")).toHaveText("Missed my request");

  await page.route("**/api/prompt-fit", async route => {
    if (route.request().method() === "POST") {
      await route.fulfill({ status: 400, contentType: "application/json", body: JSON.stringify({ error: "simulated save failure" }) });
      return;
    }
    await route.continue();
  });
  await page.locator(".prompt-fit-button", { hasText: "Met my request" }).click();
  await expect(page.locator(".message.error")).toContainText("simulated save failure");
  await expect(page.locator(".prompt-fit-button.selected")).toHaveText("Missed my request");
});
test("song ratings show saved selection and retain it when request fit rerenders", async ({ page }) => {
  await page.goto("/recommendations/songs");
  const row = page.locator(".song-row").first();
  const liked = row.getByRole("button", { name: "Liked", exact: true });
  const disliked = row.getByRole("button", { name: "Disliked", exact: true });
  await page.route("**/api/feedback", route => route.fulfill({
    status: 200, contentType: "application/json", body: "{}",
  }));
  await liked.click();
  await expect(liked).toHaveAttribute("aria-pressed", "true");
  await expect(disliked).toHaveAttribute("aria-pressed", "false");
  expect(await liked.evaluate(button => getComputedStyle(button).backgroundColor))
    .not.toBe(await disliked.evaluate(button => getComputedStyle(button).backgroundColor));
  expect(await liked.evaluate(button => getComputedStyle(button, "::before").content)).toContain("✓");

  await row.getByRole("button", { name: "Met my request", exact: true }).click();
  await expect(row.getByRole("button", { name: "Met my request", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(liked).toHaveAttribute("aria-pressed", "true");

  await page.route("**/api/feedback", route => route.fulfill({
    status: 400, contentType: "application/json", body: JSON.stringify({ error: "rating save failed" }),
  }));
  await disliked.click();
  await expect(page.locator(".message.error")).toContainText("rating save failed");
  await expect(liked).toHaveAttribute("aria-pressed", "true");
  await expect(disliked).toHaveAttribute("aria-pressed", "false");
  await expect(disliked).toBeEnabled();
});
