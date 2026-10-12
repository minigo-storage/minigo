# Minigo Console

An embedded React/TypeScript console for Minigo, built with Vite and shadcn/ui. The compiled files are embedded in the Go binary by `assets.go`; no Node service runs in production.

## Development

```sh
npm ci
npm run build
npm test
npm run dev
```

Development binds to `0.0.0.0:5173` and proxies API traffic to the local Minigo Console on port `19001`. For a complete review, use the embedded Console in the root project's Docker Compose deployment. Its server handles encrypted session cookies, API authorization, SSO callback routes, and reverse-proxy subpaths.

## Source and dependencies

- `src/minigo` contains the active frontend. Upstream `src/screens`, generated API types, and historical tests remain as reference and are excluded from this application's TypeScript build.
- `src/minigo/components/ui` contains actual shadcn/ui New York registry components, with the upstream MIT license and source URLs retained. Styling is customized through CSS variables and the application stylesheet.
- Runtime packages are React, React DOM, two Radix primitives (dialog and slot), Lucide icons, and the three small shadcn class helpers. Routing and API requests use browser APIs.
- `package-lock.json` is the dependency lock used by this application. The historical Yarn setup and upstream test harness are reference material only.
- Credentials are never written to browser storage. Only the light/dark appearance preference is stored locally. Application secrets are shown once after creation and held only in page memory.

## Verification

`npm run build` checks TypeScript and produces `build/`. `npm test` verifies reverse-proxy URL handling, object key encoding, permission matching, cookie/CSRF-compatible requests, upload boundaries, and explicit API error handling. Browser integration checks belong to `tests-minigo/` and use an actual Minigo backend; no mock storage records are rendered in the application.

The server's S3 and mc interfaces remain independent of the frontend. The web console includes storage browsing and uploads, object download/ZIP download, delete and version restore, metadata/tags/sharing, rewind views, bucket versioning/lifecycle/replication, identities, groups, policies, access keys, and server configuration. Advanced lifecycle, replication, and IAM policy documents use explicit JSON editors with confirmations. Destructive operations require a confirmation and remain subject to server authorization and object retention policies.

Large browser ZIP downloads buffer an archive in browser memory, as the original console did; use mc for very large transfers. Object lists initially request 500 entries and progressively increase the limit when Load more is selected. SSO integration requires a configured identity provider. Anonymous access is limited by server bucket policy.
