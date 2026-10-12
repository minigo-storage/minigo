import test from "node:test";
import assert from "node:assert/strict";
global.document = { baseURI: "https://storage.example.test/team/console/" };
global.window = {
  location: { origin: "https://storage.example.test" },
  dispatchEvent() {},
};
const { url, api, can, setAnonymous } = await import("../src/minigo/api.ts");
test("API paths preserve reverse proxy subpaths and encode Unicode object keys exactly", () => {
  const target = new URL(
    url("/buckets/reports/objects", {
      prefix: "日本語/space + # & %.txt",
      version_id: "a+/=",
    }),
  );
  assert.equal(target.pathname, "/team/console/api/v1/buckets/reports/objects");
  assert.equal(target.searchParams.get("prefix"), "日本語/space + # & %.txt");
  assert.equal(target.searchParams.get("version_id"), "a+/=");
});
test("permission rendering respects action and resource scopes", () => {
  const session = {
    permissions: {
      "arn:aws:s3:::reports/*": ["s3:GetObject"],
      "arn:aws:s3:::reports": ["s3:ListBucket"],
      "console-ui": ["admin:ListUsers"],
    },
  };
  assert.equal(can(session, "s3:GetObject", "reports/2026.csv"), true);
  assert.equal(can(session, "s3:PutObject", "reports/2026.csv"), false);
  assert.equal(can(session, "s3:GetObject", "private/2026.csv"), false);
  assert.equal(can(session, "admin:DeleteUser"), false);
  assert.equal(can(session, "admin:ListUsers"), true);
  assert.equal(
    can(
      { permissions: { "arn:aws:s3:::*": ["s3:Get*"] } },
      "s3:GetObject",
      "reports/file",
    ),
    true,
  );
});
test("resource literals are escaped before matching policy wildcards", () => {
  assert.equal(
    can(
      { permissions: { "arn:aws:s3:::release.v1/*": ["s3:GetObject"] } },
      "s3:GetObject",
      "releaseXv1/file",
    ),
    false,
  );
});
test("mutations use cookie authentication and JSON content type, including DELETE", async () => {
  let request;
  global.fetch = async (u, options) => {
    request = { u, options };
    return new Response("", { status: 200 });
  };
  await api("/minigo/users/alice", "DELETE");
  assert.equal(request.options.credentials, "same-origin");
  assert.equal(request.options.headers["Content-Type"], "application/json");
  assert.equal(request.options.cache, "no-store");
  assert.equal(request.options.body, undefined);
});
test("uploads preserve browser multipart boundaries and public browser header", async () => {
  let request;
  global.fetch = async (u, options) => {
    request = options;
    return new Response("", { status: 200 });
  };
  setAnonymous(true);
  const form = new FormData();
  form.append("4", new Blob(["data"]), "a.txt");
  await api("/buckets/public/objects/upload", "POST", form, {
    prefix: "folder/a.txt",
  });
  assert.equal(request.headers["Content-Type"], undefined);
  assert.equal(request.headers["X-Anonymous"], "1");
  assert.equal(request.body, form);
  setAnonymous(false);
});
test("authorization errors are surfaced instead of rendering success", async () => {
  global.fetch = async () =>
    new Response(JSON.stringify({ message: "Access denied" }), { status: 403 });
  await assert.rejects(
    api("/minigo/users"),
    (e) => e.status === 403 && e.message === "Access denied",
  );
});

test("missing configuration codes remain distinguishable from a missing bucket", async () => {
  global.fetch = async () =>
    new Response(
      JSON.stringify({
        code: "NoSuchLifecycleConfiguration",
        message: "Not found",
      }),
      { status: 404 },
    );
  await assert.rejects(
    api("/minigo/buckets/example/lifecycle"),
    (e) => e.status === 404 && e.code === "NoSuchLifecycleConfiguration",
  );
  global.fetch = async () =>
    new Response(
      JSON.stringify({ code: "NoSuchBucket", message: "Not found" }),
      { status: 404 },
    );
  await assert.rejects(
    api("/minigo/buckets/example/lifecycle"),
    (e) => e.code === "NoSuchBucket",
  );
});
