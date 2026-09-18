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
- WebS3 delete, overwrite and multipart transfer are not complete.
  No claim of full storage administration is made.
- Memcached TLS live acceptance and broader real-model mutation acceptance.
  Real-model metadata/navigation turns are covered by the later milestone below.
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

## WebS3 workspace and AI Native milestone — 2026-09-15

Implemented on `feature/storage-agent-workspace` and deployed to staging:

- Downloads use authenticated, owner-scoped HTTP and are bounded to 16 MiB.
  Responses are attachments with no-store/nosniff headers; partial upstream
  content is discarded on error. Bearer session tokens are not used in page URLs.
- Create-only uploads require the independent `webs3.files.upload` feature and
  a live authorized session. Normal users default to denied. The browser asks
  for confirmation, preserves exact keys, rejects oversized files and supports
  cancellation. S3 conditional writes reject existing keys without overwriting.
- Upload/download audit records contain operation metadata, not object bodies
  or credentials. Permissions are checked again for every request; hiding UI
  is not authorization. Revoked access prevents new operations.
- Connection recovery, bounded waits, transfer feedback and retry states are
  available. Success notices expire. Navigation stays fixed while object lists
  scroll. Agent uses a docked desktop panel and a mobile overlay.
- Agent receives the current bucket, prefix and selected key before each user
  turn. Failed context synchronization blocks inference. This navigation is
  explicitly untrusted and cannot expand the session's bucket scope.
- S3 exposes only `data.schema`: bucket/object metadata, exact prefixes and
  opaque pagination. No query/write/download tool or automatic file-content
  access is exposed to the model. Existing shared Markdown, conversation,
  session ownership and disclosure machinery is reused.

Verification: race tests passed for web, agent packages, IAM and objectaccess;
frontend production build passed. Browser fixture tests passed Chinese/English,
light/dark, desktop/mobile, upload conflict, download, context-before-inference,
context failure, pagination, reconnect and independent scrolling. Desktop and
mobile screenshots were inspected, including the Agent panel. These tests use
HTTP fixtures, not a real inference provider or the deployed MinIO service.

### Deployed acceptance

- Real connector-backed MinIO checks passed: create-only upload, binary/Unicode
  byte round-trip, existing-object conflict without overwrite, missing-object
  errors, bucket isolation, unauthenticated denial, audit redaction and closed
  connection rejection.
- Real-model tests passed for browser-context discovery and selected-object
  metadata. An initial browser test exposed ambiguity in the schema tool path:
  the model substituted a bucket name for the literal `"bucket"` marker. Tool
  results now provide an exact `current_listing_path`; malformed read-only
  arguments return a recoverable error and `retry_path`. Permission and
  connection errors still stop execution. Regression tests cover both cases.
- Deployed browser upload/download, Agent context, hashed session URL,
  Chinese/English UI, light/dark and desktop/mobile checks passed. Temporary
  objects and connections were removed, Agent test sessions archived, and the
  demo access restored to its original stopped state. No password reset or
  unrelated connector change was performed.
- All 17 selected frontend/protocol scripts passed. The SQL protocol assertion
  was updated for WebSFTP and removal of unsupported native RDP/VNC listeners.
  `LIAISON_E2E=1 go test -race ./test/e2e -timeout 12m` passed with real local
  Manager/Frontier processes, not skipped tests. Related backend race suites
  also passed. This does not mean every protocol's live-service E2E ran.
- The deployed binary checksum was verified and rollback retained. Disk space
  remains low; additional capacity is needed before further large deployments.

## Database / Cache navigation handoff — deployed, 2026-09-15

- WebData now replaces the attached user's navigation hint before each Agent
  message: database/schema and selected table, collection, index or Redis key.
  Memcached shares only the explicitly entered key, or an empty selection.
- The shared Agent panel displays the selection and explains that editor drafts,
  result rows and cached values are not automatically shared. Failed context
  synchronization blocks the turn and preserves the message for retry.
- The new context endpoint requires access-AI permission, session ownership and
  an active access. Requests are size/time bounded and reject unknown fields.
  Navigation never changes the actual connection database or grants permission.
- Empty-path schema responses expose the hint as untrusted data alongside the
  actual current database. The model instruction asks for a fresh schema check
  each resource-specific turn, not reuse of the previous selection. This is a
  navigation handoff, not automatic injection of query results or file contents.
- Backend web/Agent race suites and frontend production build passed. Browser
  fixtures passed 20 protocol/language/theme combinations covering MySQL,
  MongoDB, Elasticsearch, Redis and Memcached, with desktop/mobile screenshots,
  selection replacement and failed-sync retry. Existing S3 and Memcached UI
  regressions also passed. Screenshots were inspected.
- Deployed with a verified binary checksum, health check and retained rollback.
  Real-service/model acceptance passed 16 turns across MySQL, PostgreSQL,
  MongoDB, Redis, Memcached and Elasticsearch, including selection replacement
  and clearing. Tests requested metadata only, not queries or cached values.
  Temporary connections were closed, Agent sessions archived, and demo access
  states restored. Other SQL variants and OpenSearch reuse WebData but have not
  been separately verified against live services for this change.

## Explicit query-result analysis — deployed, 2026-09-15

- Result tables offer Analyze result only when access-AI is available. A local,
  editable preview samples up to 20 rows and 10 fields per object, bounds nested
  data and long values, and enforces a 12 KiB UTF-8 limit. Common secret field
  names are masked; this is not complete sensitive-data detection. Users must
  review other sensitive content before sharing it with the configured model.
- Add to Agent draft preserves existing text and does not send automatically.
  The user must send the draft explicitly; sent content is saved in the Agent
  conversation. No original SQL/editor draft is automatically attached, and the
  snapshot is explicitly distinguished from the current navigation selection.
- No new query-execution authority is introduced. Analysis requests say the
  snippet is data, not instructions, and ask not to run additional commands.
  Existing backend permissions and confirmation requirements remain in force;
  prompt wording is not treated as authorization.
- Frontend production build and four Chinese/English, light/dark browser
  combinations passed, including desktop/mobile screenshots, cancellation,
  bounded previews, secret masking and preservation of existing drafts.
- Deployed browser acceptance passed using a synthetic MySQL SELECT: preview
  masked the test password, adding the draft sent nothing, manual send produced
  a real-model analysis of the supplied total, and no additional query or
  approval was issued. The demo state and temporary sessions were cleaned up.

## Explicit operation-draft review — deployed, 2026-09-15

- WebData's query-assistance toolbar now offers Review draft beside Complete
  draft, under the existing access-AI permission. It snapshots the current
  editor into a local editable preview without executing it or requesting a
  completion. Memcached's structured-operation overview has no raw-editor
  review entry.
- Preview edits do not change the editor. Users must remove secrets and other
  sensitive text themselves; arbitrary SQL/command redaction is not claimed.
  The serialized snapshot is limited to 12 KiB without silently truncating a
  statement. Cancel sends nothing. Add to Agent draft preserves existing chat
  text, requires manual send and reuses context-sync failure handling.
- The review prompt asks for intent, scope, data-loss/permission/performance
  risks and improvements, with high-risk findings first and uncertainty stated.
  It requests no execution/query tools. This is advice, not safety approval or
  a new restricted-tool mode: existing server authorization and confirmations
  still govern all Agent actions.
- Build and 16 browser combinations passed across MySQL, MongoDB, Redis and
  Elasticsearch, Chinese/English and light/dark. Tests cover empty/oversized
  drafts, cancellation, editor isolation, preserved chat text, manual send,
  absence of execute/completion requests and permission-sensitive visibility.
  Desktop/mobile screenshots were inspected; four result-sharing regressions
  also passed.
- Real deployed browser/model acceptance identified missing-WHERE/full-table
  deletion risk in a fictional table draft. The test did not execute SQL or
  approve any operation. Temporary sessions were cleaned up and the private
  demo access restored. This frontend-only deployment retains rollback.

## Reviewed code handoff and home toolbar — deployed, 2026-09-15

- Finished assistant code blocks in connected WebData editor workspaces offer
  Preview in editor. User/tool messages, streaming output, busy/disconnected
  sessions and non-editor workspaces do not expose the action. Preview shows
  the current editor alongside editable proposed code. Replacement requires
  explicit confirmation and does not execute or authorize any query.
- Confirmation closes the Agent panel and its route so the updated editor is
  visible immediately, including on mobile.
- Preview content is bounded to 12 KiB and rejects terminal control characters.
  Changes to the editor after preview block replacement; reconnect/disconnect
  clears the pending suggestion. No code-language detection or safety guarantee
  is inferred from a model's code block.
- Four language/theme handoff browser tests passed with desktop/mobile
  screenshots, cancellation, existing-draft conflict and input-limit checks.
  Sixteen draft-review and four S3 UI regressions passed after the final changes.
  The initial draft-review run timed out during development edits; rerunning
  with stable sources passed. Real-model deployed acceptance generated a code
  block, previewed it, and replaced the editor without executing SQL. Test
  sessions were removed/archived and the demo access state restored.
- Fixed Home's history/avatar overlap: the fixed-position global user control
  now lives beside history/new-chat actions in normal toolbar flow. The logo
  stays above navigation; other pages retain their global user controls.
  Eight home/session language/theme cases passed at desktop/mobile sizes with
  long account labels, and four deployed Home combinations passed menu clicks,
  screenshot inspection and non-overlap assertions.
- Frontend build passed; deployment retained the previous frontend for rollback.

## SSH reviewed input handoff — 2026-09-15

- Completed side-panel assistant code can be previewed, edited and explicitly
  inserted into a verified empty single-line shell prompt. No newline, readline
  replacement shortcut or execution request is sent. Existing input is never
  overwritten. The Agent panel closes after successful insertion.
- Prompt/input changes invalidate the preview. Running commands, alternate
  screen applications, unknown/wrapped prompts, multiline/control characters
  and inputs longer than 4096 characters are rejected. This relies on the
  existing OSC 633 integration, not universal detection of arbitrary shells.
- Context labels now distinguish current input plus Shell conversation from
  additional directory/command/output sharing. Copy explicitly says Shell
  history is separate from the side-panel Agent, and Analyze shares output.
- Xterm browser integration passed no-Enter, replay/stale-input, execution and
  alternate-screen rejection checks. Four Chinese/English light/dark full-page
  fixture tests passed desktop/mobile screenshots and exact socket input checks.
  These use simulated transport, not a real remote shell or model invocation.
- Production frontend build passed. No backend, connector or schema changes.

## Selected sharing and conversation isolation — 2026-09-15

- Result analysis now offers explicit row and column selection, with paged row
  browsing, a redacted preview and an editable sample. Defaults are the first
  20 rows and 10 columns across the result. Selection never issues another query.
  Projecting away a secret's key/name column cannot bypass value masking.
- SSH can share the current xterm selection through an editable preview into
  the side-panel Agent draft. It does not read unselected terminal output or
  send automatically. Users must remove sensitive text themselves. Selection
  is bounded to 12 KiB and cleared when the connection or capability changes.
- Closing/reopening one Agent conversation preserves its unsent draft. Switching
  to a different conversation on the same connection clears the draft, references
  and transient response state. Stale session-creation responses cannot replace
  the active conversation. Sending waits for session restoration to finish.
- Reloaded SSH history does not silently create a new SSH connection and cannot
  insert commands into a disconnected terminal. Unsent side-panel drafts are
  not durable across browser reloads; this is not cross-device draft recovery.
- Live acceptance passed real-model SSH code preview and exact insertion without
  Enter, selected-output draft handoff, and refresh/history without reconnect.
  Real MySQL result analysis passed redaction, explicit send and no extra query.
  Six live database/cache/search protocols passed 16 metadata/navigation turns.
  Home history/avatar layout passed both languages and themes at desktop/mobile.
- All 26 suites in `web/e2e/run-fixtures.cjs` passed the final serial run, including
  the 16 draft-review protocol/language/theme cases and gated session-restoration
  check. Earlier runs exposed a transient send-readiness issue and a timing-based
  test assertion; the final run used stable sources and the corrected checks.
  This
  runner deliberately distinguishes fixtures from external live integrations;
  Oracle, SQL Server, ClickHouse, OpenSearch, provider gateways and TLS variants
  have not all been rerun against live services for this milestone.
- Full Go race tests, process-backed IAM E2E, Go build/vet and frontend build
  passed during acceptance. Deployment uses a separate frontend release directory
  with rollback and does not replace the unrelated local Cloud connector.

Next: redesign the permission model after this acceptance milestone. Keep the
current owner/access checks and independent upload capability in force until a
separate policy/migration design is approved. Broader external-protocol live
coverage and durable cross-reload side-panel drafts remain explicitly separate.
Keep future storage
mutations behind separate authorization and explicit confirmation.

Never return Secret Access Key or session token in target/credential responses.

Do not show unimplemented protocol options. Memcached basic text does not imply
SASL support. Do not expose `flush_all`, arbitrary commands or a fake full key
catalog. S3 credentials must never reach the browser, redirects must not bypass
the connector, and object keys must not be normalized as local filesystem paths.

Protocol references:
[Memcached basic text](https://docs.memcached.org/protocols/basic/),
[S3 ListObjectsV2](https://docs.aws.amazon.com/AmazonS3/latest/API/API_ListObjectsV2.html).
