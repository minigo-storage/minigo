// Isolated presentation check only. Auth endpoints are mocked; this is not a backend integration test.
import { chromium } from "@playwright/test";
import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.route("**/api/v1/session", (r) =>
  r.fulfill({ status: 401, json: { message: "Unauthorized" } }),
);
await page.route("**/api/v1/login", (r) =>
  r.fulfill({ json: { loginStrategy: "form" } }),
);
await mkdir("/private/tmp/minigo-ui-evidence", { recursive: true });
for (const viewport of [
  { width: 1440, height: 1000 },
  { width: 390, height: 844 },
]) {
  await page.setViewportSize(viewport);
  await page.goto("http://127.0.0.1:5173/login");
  await page.getByRole("heading", { name: "Welcome back" }).waitFor();
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth > innerWidth,
    ),
    false,
    "Page must not overflow horizontally",
  );
  await page.screenshot({
    path: `/private/tmp/minigo-ui-evidence/login-${viewport.width}.png`,
    fullPage: true,
  });
  console.log(
    `PASS login ${viewport.width}px; accessible labels and no horizontal overflow`,
  );
}
await browser.close();
