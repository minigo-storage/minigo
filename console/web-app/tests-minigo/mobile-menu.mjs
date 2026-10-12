// Presentation-only regression for the shadcn dialog drawer with Tailwind v4.
import { chromium } from "@playwright/test";
import assert from "node:assert/strict";
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
await page.route("**/api/v1/**", (route) => {
  const path = new URL(route.request().url()).pathname;
  return route.fulfill({
    json: path.endsWith("/session")
      ? {
          status: "ok",
          permissions: {
            "console-ui": ["admin:*"],
            "arn:aws:s3:::*": ["s3:*"],
          },
        }
      : path.endsWith("/server")
        ? { mode: "online", servers: [], buckets: 0, objects: 0, usageBytes: 0 }
        : { buckets: [] },
  });
});
await page.goto("http://127.0.0.1:5173/overview");
await page
  .getByRole("button", { name: "Open navigation", exact: true })
  .click();
const drawer = page.getByRole("dialog");
const box = await drawer.boundingBox();
assert.equal(box.x, 0);
assert.equal(box.y, 0);
await drawer.getByRole("button", { name: "Buckets", exact: true }).click();
await page.getByRole("heading", { name: "Buckets", exact: true }).waitFor();
await drawer.waitFor({ state: "hidden" });
assert.equal(
  await page.evaluate(() => document.documentElement.scrollWidth > innerWidth),
  false,
);
await browser.close();
console.log(
  "PASS 390px drawer stays at viewport origin and navigates to Buckets",
);
