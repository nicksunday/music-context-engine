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