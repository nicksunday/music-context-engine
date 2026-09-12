const { test, expect } = require("@playwright/test");

for (const width of [1920, 1440, 1280, 1024, 821, 768, 390]) {
  test(`song controls remain readable at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 800 });
    await page.goto("/recommendations/songs");
    const row = page.locator(".song-row").first();
    await expect(row).toContainText("Browser Song");
    const layout = await row.evaluate(element => {
      const bounds = selector => element.querySelector(selector).getBoundingClientRect().toJSON();
      return {
        identity: bounds(".song-identity"),
        actions: bounds(".song-actions"),
        fit: bounds(".prompt-fit"),
      };
    });
    expect(layout.identity.width).toBeGreaterThan(180);
    expect(layout.fit.y).toBeGreaterThanOrEqual(layout.identity.bottom);
    expect(layout.fit.y).toBeGreaterThanOrEqual(layout.actions.bottom);
    const overflow = await page.evaluate(() => {
      return [...document.querySelectorAll(".workspace, .composer, .results-panel, .song-row, .song-actions, .prompt-fit")]
        .filter(element => element.scrollWidth > element.clientWidth + 1)
        .map(element => element.className);
    });
    expect(overflow).toEqual([]);
    if (width <= 1400) {
      const results = await page.locator(".results-panel").boundingBox();
      const sidecar = await page.locator(".sidecar").boundingBox();
      expect(sidecar.y).toBeGreaterThanOrEqual(results.y + results.height);
    }
    await page.locator(".prompt-fit-notes").first().scrollIntoViewIfNeeded();
    await expect(page.locator(".prompt-fit-notes").first()).toBeInViewport();
  });
}

for (const width of [1920, 1440, 1280, 390]) {
  test(`full song batch scrolls to the last feedback controls at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 800 });
    await page.route("**/api/batch/latest?mode=song", async route => {
      const response = await route.fetch();
      const payload = await response.json();
      const candidate = payload.batch.candidates[0];
      payload.batch.candidates = Array.from({ length: 10 }, (_, index) => ({
        ...candidate, id: `scroll-song-${index}`, song: `Scroll Song ${index + 1}`,
      }));
      await route.fulfill({ response, json: payload });
    });
    await page.goto("/recommendations/songs");
    const last = page.locator(".song-row").last();
    await expect(last).toContainText("Scroll Song 10");
    if (width > 1400) {
      const panel = page.locator(".results-panel");
      const bounds = await panel.boundingBox();
      await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + 60);
      await page.mouse.wheel(0, 4000);
      await expect.poll(() => panel.evaluate(element => element.scrollTop)).toBeGreaterThan(0);
    }
    await last.locator(".prompt-fit-notes").scrollIntoViewIfNeeded();
    await expect(last.locator(".prompt-fit-notes")).toBeInViewport();
  });
}
