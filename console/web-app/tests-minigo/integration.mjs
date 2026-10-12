// Runs against a real disposable Minigo deployment. Creates and removes its own fixtures.
// MINIGO_UI_URL=http://127.0.0.1:19001 MINIO_ROOT_USER=... MINIO_ROOT_PASSWORD=... node tests-minigo/integration.mjs
import { chromium } from "@playwright/test";
import assert from "node:assert/strict";
import { mkdir, readFile } from "node:fs/promises";
const origin = process.env.MINIGO_UI_URL || "http://127.0.0.1:19001";
if (!process.env.MINIO_ROOT_USER || !process.env.MINIO_ROOT_PASSWORD)
  throw new Error("Set credentials for a disposable integration deployment.");
const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({
  viewport: { width: 1440, height: 1000 },
  acceptDownloads: true,
});
const page = await context.newPage();
if (process.env.MINIGO_UI_SOURCE) {
  await page.route("**/*", async (route) => {
    const requestURL = new URL(route.request().url());
    if (
      requestURL.origin === new URL(origin).origin &&
      !requestURL.pathname.startsWith("/api/")
    ) {
      const response = await route.fetch({
        url:
          process.env.MINIGO_UI_SOURCE +
          requestURL.pathname +
          requestURL.search,
      });
      await route.fulfill({ response });
    } else {
      const response = await route.fetch();
      await route.fulfill({ response });
    }
  });
}
page.on("requestfailed", (r) =>
  console.log(
    "Request failure",
    new URL(r.url()).pathname,
    r.failure()?.errorText,
  ),
);
const errors = [];
page.on("pageerror", (e) => errors.push(e.message));
const bucket = `ui-test-${Date.now()}`;
await mkdir("/private/tmp/minigo-ui-evidence", { recursive: true });
async function click(name) {
  await page.getByRole("button", { name, exact: true }).click();
}
async function submit(name) {
  await page
    .getByRole("dialog")
    .getByRole("button", { name, exact: true })
    .click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
}
async function request(path, method = "GET", body) {
  return page.evaluate(
    async ({ path, method, body }) => {
      const base = new URL(document.baseURI).pathname;
      const r = await fetch(`${base}api/v1${path}`, {
        method,
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
      const text = await r.text();
      return { status: r.status, body: text ? JSON.parse(text) : {} };
    },
    { path, method, body },
  );
}
try {
  await page.goto(`${origin}/login`);
  await page
    .getByLabel("Username or access key")
    .fill(process.env.MINIO_ROOT_USER);
  await page
    .getByLabel("Password or secret key")
    .fill(process.env.MINIO_ROOT_PASSWORD);
  await click("Sign in");
  await page
    .getByRole("heading", { name: "Storage overview", exact: true })
    .waitFor();
  await page.screenshot({
    path: "/private/tmp/minigo-ui-evidence/overview-desktop.png",
    fullPage: true,
  });
  await page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("button", { name: "Buckets", exact: true })
    .click();
  await click("Create bucket");
  await page.getByLabel("Bucket name").fill(bucket);
  await submit("Create bucket");
  await page.getByRole("heading", { name: bucket, exact: true }).waitFor();
  const name = "Unicode 日本語 + #%.txt",
    content = "Minigo console compatibility fixture\n";
  await page
    .locator("input[type=file]")
    .first()
    .setInputFiles({
      name,
      mimeType: "text/plain",
      buffer: Buffer.from(content),
    });
  await submit("Upload");
  await page.getByRole("button", { name, exact: true }).waitFor();
  const downloadPromise = page.waitForEvent("download");
  await page
    .getByRole("link", { name: `Download ${name}`, exact: true })
    .click();
  const download = await downloadPromise;
  assert.equal(await readFile(await download.path(), "utf8"), content);
  await page.getByRole("button", { name, exact: true }).click();
  await page.getByRole("heading", { name: "Metadata", exact: true }).waitFor();
  await page.getByRole("button", { name: "Preview text", exact: true }).click();
  await page.getByText(content.trim(), { exact: true }).waitFor();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Close", exact: true })
    .click();
  await click("Settings");
  await page.getByText("Not enabled", { exact: true }).waitFor();
  await click("Edit configuration");
  await page.getByLabel("Versioning status").selectOption("Enabled");
  await page.getByLabel(`Type ${bucket} to confirm`).fill(bucket);
  await submit("Save changes");
  await page.getByText("Protected", { exact: true }).waitFor();
  await page.getByRole("tab", { name: "Lifecycle", exact: true }).click();
  await page.getByText("No lifecycle rules", { exact: true }).waitFor();
  assert.equal(
    await page
      .getByRole("button", { name: "Edit configuration", exact: true })
      .isEnabled(),
    true,
    "An absent lifecycle must remain editable",
  );
  for (const [label, title] of [
    ["Users", "Users"],
    ["Groups", "Groups"],
    ["Policies", "Policies"],
    ["Access keys", "Access keys"],
    ["Deployment", "Deployment"],
  ]) {
    await page
      .getByRole("navigation", { name: "Main navigation" })
      .getByRole("button", { name: label, exact: true })
      .click();
    await page.getByRole("heading", { name: title, exact: true }).waitFor();
    await page.waitForTimeout(150);
    assert.equal(
      await page.getByRole("alert").count(),
      0,
      `${label} should load without an API error`,
    );
  }
  await page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("button", { name: "Overview", exact: true })
    .click();
  await page.setViewportSize({ width: 390, height: 844 });
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth > innerWidth,
    ),
    false,
    "Mobile overflow",
  );
  await page.screenshot({
    path: "/private/tmp/minigo-ui-evidence/overview-mobile.png",
    fullPage: true,
  });
  await click("Open navigation");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Buckets", exact: true })
    .click();
  await page.getByRole("heading", { name: "Buckets", exact: true }).waitFor();
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth > innerWidth,
    ),
    false,
    "Mobile bucket table overflow",
  );
  await page.screenshot({
    path: "/private/tmp/minigo-ui-evidence/buckets-mobile.png",
    fullPage: true,
  });
  assert.deepEqual(errors, []);
  console.log(
    "PASS real UI login, bucket create, Unicode upload/download, metadata/text preview, versioning, admin screens, mobile navigation and overflow checks",
  );
} catch (e) {
  await page.screenshot({
    path: "/private/tmp/minigo-ui-evidence/failure.png",
    fullPage: true,
  });
  console.log(
    "Failure page",
    (await page.locator("body").innerText()).slice(-2500),
  );
  throw e;
} finally {
  try {
    await request(
      `/buckets/${bucket}/objects?prefix=${encodeURIComponent("Unicode 日本語 + #%.txt")}&all_versions=true`,
      "DELETE",
    );
    await request(`/minigo/buckets/${bucket}`, "DELETE");
  } catch {}
  await browser.close();
}
