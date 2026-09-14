# Native LLM and database access

## Current implementation

Anthropic upstreams accept an additional `POST /api/v1/ai/accesses/{id}/v1/messages`
endpoint. Use a **Liaison** API key in `x-api-key` or Bearer authorization, not both.
The existing OpenAI endpoint remains unchanged. Native Messages requests preserve
system messages, tool blocks and thinking payloads; unsupported beta headers fail
explicitly. Model aliases, key ownership, quotas and revocation are shared.
The endpoint does not translate native Messages to OpenAI or Ollama upstreams.

JSON and SSE are handled separately. Streaming success requires `message_stop`;
truncation is not success. Provider errors are sanitized. Cache-read and cache-write
tokens count toward native Messages input usage. Missing usage is not zero usage.

Reference: [Anthropic streaming specification](https://platform.claude.com/docs/en/build-with-claude/streaming).

## Database implementation boundary

The `dbgateway` package contains bounded MySQL/PostgreSQL negotiation and a
bidirectional TLS-preserving relay. It validates the upstream's TLS capability,
rejects plaintext startup, limits negotiation to ten seconds, and closes both
connections on cancellation or termination. Certificate-verified TLS round-trips
are tested with in-process peers; these are not real database authentication E2E.
The entry layer now dispatches native MySQL/PostgreSQL to dedicated listeners,
not the generic TCP runtime. Listeners route through the registered connector,
send only the stored application target, and apply source policy before dialing.
Persisted source rules are loaded before listening; lookup failures fail closed.
Disabling/deleting access closes pending negotiation and active relay sessions.
Source policy is rechecked every second. The gateway limits native sessions to
1024 overall, connector dialing to ten seconds, directional IO inactivity to
15 minutes and total connection lifetime to 24 hours. These are initial internal
limits, not configurable UI settings.
MySQL and PostgreSQL creation options intentionally remain disabled; WebMySQL and
WebPostgreSQL continue to use the existing browser access implementation.

Native access must keep database authentication at the upstream database, preserve
end-to-end TLS and never synthesize successful authentication or TLS negotiation.
Connector tunnel encryption alone does not encrypt a public client-to-gateway hop.
TLS passthrough cannot supply SQL-level audit; expose connection/traffic metadata
only, without claiming statement inspection. Database account authentication does
not by itself establish a Liaison user identity.

References: [MySQL TLS](https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_basic_tls.html),
[PostgreSQL startup and SSL messages](https://www.postgresql.org/docs/17/protocol-message-formats.html).

## Remaining before release

- Consumer workspace now advertises `external_protocols`; the detail page offers
  capability-driven OpenAI/Anthropic curl examples and protocol logos. Separate
  access-type tabs still need a consistent multi-protocol resource model.
- Native Messages has been deployed to staging. Connector-backed protocol-fixture
  E2E passed JSON/SSE, aliases, missing/conflicting credentials, model scope,
  token quota, revocation and the real UI. Temporary app/access were removed.
  This verifies transport, not real model quality or native database login.
  Real-model tool round-trips and expired-key native endpoint coverage remain.
- Listener routing, source denial before connector dialing, policy changes,
  cancellation during dialing/negotiation, port reuse and capacity rejection
  have local tests. Full control-plane lifecycle and real connector-backed
  database authentication E2E still need verification before deployment.
- Verify MySQL and PostgreSQL native clients with valid/invalid database accounts,
  certificate verification and TLS downgrade refusal before enabling UI options.
