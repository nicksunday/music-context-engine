const { test, expect } = require("@playwright/test");

test("successful empty recommendation attempts remain inspectable through the trace API", async ({ request }) => {
  const batches = await request.get("/api/batches?mode=song");
  expect(batches.ok()).toBeTruthy();
  const payload = await batches.json();
  const emptyBatch = payload.batches.find((batch) => batch.prompt === "empty browser trace");
  expect(emptyBatch).toBeTruthy();

  const traceResponse = await request.get(`/api/trace?id=${emptyBatch.id}`);
  expect(traceResponse.ok()).toBeTruthy();
  const trace = await traceResponse.json();
  expect(trace.available).toBe(true);
  expect(trace.trace.shortfall_reason).toBe("discovery_pool_exhausted");
});