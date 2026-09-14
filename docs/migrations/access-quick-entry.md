# Access quick entry

## Change

- Browser access creation includes the current user's protocol-specific connection fields. Different accounts use separate accesses targeting the same application.
- Browser access opens directly. Connection-manager routes are compatibility redirects, not list pages. Missing legacy configuration is completed in Edit access.
- Edit access preserves a saved password when left blank. Account identity is fixed after creation; create another access for a different username. Legacy multi-account records are retained, with the most recently used profile selected; this is a UI workflow change, not a destructive data migration or removal of compatibility APIs.
- Every access type shows its actions directly in a fixed right-hand column. Cells do not wrap; narrow screens scroll the table horizontally.
- Save password is optional. With it disabled, only account metadata is persisted; starting a new session prompts for a temporary password. Disabling it for an existing account clears the stored secret. Passwords never enter URLs or browser persistence. An empty database password remains distinct from an unsaved password.
- Desktop credential POST reuses existing encrypted, user-scoped credential storage. SSH/SFTP and WebData reuse their credential APIs. Credentials are never included in the shared access object or navigation URL.

## Failure handling

Access and credential creation are separate requests. If credential storage fails, the form retains the newly created access ID and retries only credential storage. Canceling leaves an incomplete access; Open takes the user to Edit access to finish configuring it. There is no separate connection-creation page.

## Rollout / rollback

Standard application update; no new schema changes beyond the accompanying SFTP profile migration. Deploy the manager and frontend together. Retain the previous binary/assets or image for rollback. Check login, access listing, personal credential isolation, and direct WebSSH/SFTP/database/desktop entry. If these checks fail, restore the previous application revision; do not delete user data.

## Validation

- `go test -race ./pkg/liaison/manager/controlplane ./pkg/liaison/manager/web`
- `npm run build --prefix web`
- `web/e2e/access-ui.cjs`: isolated all-type menus, protocol fields, partial failure/retry, direct routes, Chinese/English and light/dark/mobile.
- `web/e2e/access-auth.cjs`, `web/e2e/websftp-ui.cjs`: temporary-password prompts, no automatic connection before submission, safe retry and no persistent browser secrets.
- Staging: create temporary accesses, save/retrieve credentials without plaintext disclosure, open an existing real file session, then remove only the test accesses.
