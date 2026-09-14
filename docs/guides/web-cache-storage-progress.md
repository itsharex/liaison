# Web Cache / Storage implementation

## Scope

Only network services reachable through a registered connector are in scope.
SQLite and native database listeners are not part of this release.

- Cache: existing WebRedis, then WebMemcached.
- Storage: WebS3 for S3-compatible object services.
- Preserve saved connections, user isolation, access revocation and audit rules.

## Implemented and verified

- WebRedis moved from Database to Cache. Legacy `access_type=webredis` and
  `category=database&access_type=webredis` links still select Redis.
- Access-page fixture E2E passed Chinese/English and light/dark variants,
  including mobile overflow, existing connection forms and old navigation.
- `cacheaccess` implements bounded Memcached basic-text stats/get/set/delete
  on an owned connection. It does not dial an endpoint itself. Write operations
  require caller authorization; raw command strings are not accepted.
- Tests cover framing, binary values, absent keys, request injection, oversized
  and truncated responses, write denial and cancellation, including `-race`.
- WebData has an internal Memcached executor, JSON command contract and
  read/write classification. Each command opens a fresh control-plane stream;
  no caller-provided target address is accepted. Memcached application
  registration and WebMemcached access are enabled in Cache.
- Connection creation exposes only applicable TLS options; basic-text has no
  username/password or SASL. The workspace prepares stats/get/set/delete JSON
  drafts, requires confirmation for mutations and has no fake database tree.
- Audit previews retain only the operation, not keys or values. Connection
  profiles and Agent handles remain user-scoped; closed handles are invalidated.
- Per-operation timeout uses context cancellation and stream Close. Real
  connector streams exposed a panic in their deadline API; no socket deadline
  methods are used by this adapter. Cancellation regression tests cover this.
- On 2026-09-14, deployed WebMemcached with a loopback-only real Memcached
  demo. Live acceptance passed stats, set/get/delete, binary value round-trip,
  absent keys, forbidden flush_all, and the deployed browser workspace.
- Browser fixture acceptance passed Chinese/English, light/dark, desktop/mobile
  layouts, no password prompt, stats and mutation confirmation/cancellation.
  Existing access navigation/forms tests passed all four language/theme variants.
- Full race tests passed for cacheaccess, web, controlplane, agent executor,
  agent runtime and entry. Frontend production build passed.
- Native MySQL/PostgreSQL listener creation is explicitly rejected; the
  unpublished gateway is not wired into the running entry.

## Not yet complete

- WebS3 production readiness beyond the staging read-only milestone below.
- WebS3 upload/download/delete and Agent integration are not exposed.
- Memcached TLS live acceptance and real-model Agent conversation acceptance.
  Tool registration/binding is covered; no claim of real-model E2E is made.
  Inline draft assistance is intentionally not offered for Memcached yet.

## WebS3 adapter milestone (initial implementation)

- Added `objectaccess`, a path-style S3 adapter using AWS SDK for Go v2
  (`service/s3 v1.105.2`, Apache-2.0). No handwritten signer or default AWS
  credential chain. Credentials are supplied explicitly by the future session
  layer; no credentials are loaded from instance metadata or the local machine.
- Transport takes a no-address `Dial(ctx)` callback, validates the HTTP authority,
  disables pooling/retries/redirect following, and uses only that callback for
  HTTP and HTTPS. The caller must reauthorize and open the registered connector
  application on each call. The session milestone below wires this to control-plane.
- Bucket and object pagination, prefix browsing, URL-encoded key decoding,
  create-only uploads, bounded downloads and single-object deletion. Maximum
  page size 200, metadata response 2 MiB, object transfer 16 MiB. Read-only
  sessions cannot upload/delete. Object keys are never filesystem-normalized.
- Tests cover signatures/Host, special keys, opaque pagination, binary transport,
  read-only rejection, redirect isolation, safe errors, cancellation without
  connector deadline methods, response bounds and malformed list responses.
- `go test -race` passed for objectaccess, cacheaccess, web and controlplane.
  These are adapter/HTTP-fixture tests, not real S3-service or browser E2E.
- The initial Go 1.24.13 scan blocked deployment because it reported reachable
  vulnerabilities. Toolchain/dependency remediation is recorded below.

## WebS3 read-only session and browser milestone

- Added Storage → WebS3 and application type S3. Access creation accepts Access
  Key, write-only Secret Key, Region, optional bucket and TLS configuration.
  Saved secrets reuse encrypted WebData credentials; unsaved secrets are prompted
  for at entry and cleared from component state after connection.
- User-scoped WebData sessions route every new S3 connection through control-plane
  authorization as the session owner. Target changes require reconnection.
  A configured bucket cannot be escaped by passing another bucket in a command.
- Bucket listing, opaque object pagination, exact prefix navigation, object size,
  modified time and ETag are implemented. Prefix navigation is not a filesystem
  tree. No upload, download or delete UI/API is exposed by this session milestone.
- Strict JSON command allowlist rejects mutations and unknown fields. Unit tests
  cover bucket scoping and closed sessions; race tests pass for web, controlplane
  and objectaccess.
- Browser fixture tests pass in Chinese/English, light/dark, desktop/mobile:
  listing, pagination, navigating back then forward, exact repeated-slash/Unicode
  prefixes, empty/error/retry and page overflow. Screenshots were inspected.
  Fixtures do not prove real MinIO compatibility or deployment readiness.

## Staging acceptance — 2026-09-14

- Deployed the read-only WebS3 workspace with a loopback-only real MinIO demo,
  reached through the existing staging connector. Open Access → Storage → WebS3
  → WebS3 Demo. Demo objects use temporary container storage, not durable storage.
- Real-service acceptance passed: saved credentials, 206 root entries paginated
  as 200 + 6, Unicode prefix navigation, object metadata, configured-bucket
  isolation, mutation rejection and unauthenticated denial.
- Deployed-browser tests passed in Chinese/English, light/dark and desktop/mobile.
  Checked list → open → pagination → folder → metadata, contextual return,
  unsaved-secret prompting, wrong-secret rejection and successful retry without
  saving the secret. Screenshots were inspected.
- Fixed an intermittent connector response truncation: do not send
  `Connection: close` before consuming the response. Each request still uses a
  private transport and fresh authorized dial; close its idle connection only
  after the SDK closes the response body. Regression tests assert this contract.
- Updated Go builds to 1.26.8 and upgraded affected dependencies including SSH,
  database drivers, gRPC, compression and OpenTelemetry. This is not a claim that
  all security warnings are resolved: binary scanning still requires assessment
  of GO-2026-5932 (obsolete OpenPGP) and GO-2026-5471 (Kratos confused deputy),
  for which the advisory database gives no patched version.
- Backend race regression covers manager and entry packages; frontend production
  build passed. These results do not constitute every protocol's live E2E suite
  or production security certification.
- Deployment retains rollback. Removed only two unused old installer archives
  after verifying local backup checksums; disk capacity remains constrained.

Never return Secret Access Key or session token in target/credential responses.

Do not show unimplemented protocol options. Memcached basic text does not imply
SASL support. Do not expose `flush_all`, arbitrary commands or a fake full key
catalog. S3 credentials must never reach the browser, redirects must not bypass
the connector, and object keys must not be normalized as local filesystem paths.

Protocol references:
[Memcached basic text](https://docs.memcached.org/protocols/basic/),
[S3 ListObjectsV2](https://docs.aws.amazon.com/AmazonS3/latest/API/API_ListObjectsV2.html).
