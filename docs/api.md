# Core HTTP API (кодовая версия 0.2.0)

Справочник сверён с текущими `Server.routes` и handlers 2026-10-17. Это описание существующего API, не заявление о готовности всех требований. Доказательства проверенных частей и ограничения — [PROJECT_PLAN.md](PROJECT_PLAN.md). Полный runtime каждого endpoint отдельно не проверен.

Responses are JSON unless stated otherwise. Errors use `{"error":"message"}` with a suitable HTTP status. The API prefix is `/api/v1`. The console uses these same endpoints.

## Authentication

`POST /api/v1/auth/login` takes `{"username":"admin","password":"..."}` and sets a `subgate_session` cookie (`HttpOnly`, `SameSite=Strict`, `Secure` when serving HTTPS or `SUBGATE_PUBLIC_URL` is HTTPS). It returns `{"id":1,"username":"admin"}`. Login is rate limited by the resolved client IP (default five attempts per minute).

All administrative endpoints require an unexpired session. Cookie-authenticated mutations additionally require `X-SubGate-CSRF: 1`; an `Origin` header, if supplied, must match `SUBGATE_PUBLIC_URL`, or the request scheme/host when no public URL is configured. A session value can also be sent as `Authorization: Bearer SESSION`; this is an existing session credential rather than a standalone API key. Q6 defines a single owner with full access, authenticated with username and password only. Stored legacy roles do not restrict access. Multiple existing accounts block startup, login and session authorization; no accounts are deleted automatically. Enabling legacy TOTP/WebAuthn configuration is rejected.

Activity reads (`activity.read`) and exports (`activity.export`) record final success/failure, owner, normalized user/node/domain/time range, effective limit/format and returned row count. Filter details are AES-GCM encrypted in SQLite and exposed as `details` only by the authenticated `/audit` endpoint; legacy rows have null details. Both routes share one owner budget of 60 requests per minute, independent of sessions/IP; exhaustion returns 429 with `Retry-After`. Duplicate, unknown or oversized filters and reversed ranges return 400. CSV is accepted only by `/activity/export`. An unavailable audit denies data with 500. Sessions are hashed in SQLite. No roles, groups or separate superadmin permissions are exposed. Backup creation/download and activity purge require the same authenticated owner session; cookie mutations require CSRF.

## Public endpoints

| Method | Path | Behavior |
|---|---|---|
| GET | `/health` | `{"status":"ok","database":true}`; 503 if DB health fails |
| GET | `/version` | Code version (0.2.0) and demo flag |
| GET | `/metrics` | Public text metrics exporter |
| GET | `/`, `/console` | Embedded console |
| GET/HEAD | `/sub/{shortUuid}` or `/sub/{shortUuid}/{clientType}` | Subscription response |
| GET/HEAD | `/{shortUuid}` or `/{shortUuid}/{clientType}` | Legacy subscription aliases |
| POST | `/api/v1/auth/login` | Login |
| POST | `/api/v1/agents/enroll` | Consume one-time enrollment token |
| POST | `/webhook/panel` | Optional signed invalidation webhook; YAML can change path |

GET/HEAD aliases сохраняют method/raw query и Accept-Language/UA/HWID/X-Ver-Os/X-Device-* metadata; credentials/hop headers исключены.
HEAD не запускает body transforms; response announce/announce-url сохраняются. Exact cache variant и fail-open scope: [gateway-routing](registry/gateway-routing.md).

Subscription responses expose `X-Cache-Status: hit`, `miss`, or `stale`; stale responses also carry an HTTP Warning. The origin body is preserved unless an applicable mapped policy transforms it. `X-SubGate-Policy` describes policy, passthrough or bypass handling. Unknown representations and endpoints with ambiguous node mappings are retained. Cache identity includes request query, client and device headers. Upstream 4xx responses are authoritative and remove the matching snapshot.

## Authenticated administrative endpoints

| Method | Path | Parameters or payload |
|---|---|---|
| GET | `/auth/me` | Current principal |
| POST | `/auth/logout` | End session |
| GET | `/system/status` | Counts, last sync/error, database/cache/intelligence status; `panel_version` (`expected`, `actual`, `state`: not_checked/compatible/mismatch/unavailable) |
| GET | `/nodes`, `/users` | `q`, `offset`, `limit` (default 500, max 1000) |
| GET | `/nodes/{id}`, `/users/{id}` | Cached object by panel UUID |
| POST | `/nodes/{id}/actions/{action}` | enable, disable, restart, reset-traffic |
| POST | `/users/{id}/actions/{action}` | enable, disable, reset-traffic |
| POST | `/nodes/{id}/probe` | Real TCP/TLS reachability probe; UDP is not verified by a TCP probe |
| GET | `/health/probes` | Latest recorded probe objects |
| POST | `/panel/sync` | Fetch and atomically publish a complete panel snapshot |
| GET | `/live` | Independent user/node/scope rows; `user`, `node`, `scope`, `window_seconds` (default 15, range 1–60) |
| GET | `/live/stream` | SSE full snapshots each second, same filters; not WebSocket diff |
| GET | `/live/export` | Same filters; `format=csv`, otherwise JSON snapshot |
| GET | `/traffic` | `minutes` (default 60, max 1440), `from`, `to`, `user`, `node`; ten-second buckets per user/node/scope |
| GET | `/users/{id}/traffic` | History with unambiguous user-identity alias resolution |
| GET | `/activity` | `user`, `node`, `from`, `to`, `domain`, `limit` (default 100, max 1000) |
| GET | `/activity/export` | Same filters, max 10000; `format=csv`, otherwise JSON array |
| DELETE | `/activity/{user}` | Owner deletes that user's activity |
| GET | `/agents` | Enrollment identity, heartbeat age/status, diagnostic errors |
| POST | `/agents/enrollment-tokens` | `{"node_id":"PANEL_NODE_UUID"}`; expires in 15 minutes |
| DELETE | `/agents/{id}` | Revoke node Agent token |
| GET | `/settings` | Effective non-secret configuration |
| GET | `/audit` | `limit` (default 100, max 1000) |
| GET | `/ipintel/lookup` | `ip=...`; unloaded addresses remain unknown |
| POST | `/ipintel/reload` | Reload the configured local dataset atomically; keep the last good dataset if validation fails |
| GET | `/backups` | Backup file metadata |
| POST | `/backups` | Owner creates encrypted `.sgb` snapshot |
| GET | `/backups/{name}` | Owner downloads encrypted backup bytes |
| GET/PUT/DELETE | `/injections[/{id}]` | List, replace or delete; see payload below |
| GET/PUT/DELETE | `/quotas[/{id}]` | List, replace or delete; see payload below |
| GET | `/blocks`, `/youtube` | Saved diagnostic results |
| POST | `/nodes/{id}/block-check` | Synced inbound TCP/TLS diagnostic from Core |
| POST | `/youtube/check` | Allowlisted YouTube HTTP diagnostic from Core |
| POST | `/notifications/test` | Manual notifier dispatch; no sender configured may still return success |

GET for feature collections uses `/injections` or `/quotas`; PUT/DELETE require `/{id}`. Settings have GET only; no versioned hot-reload API. User revoke, host actions/bulk, logs live-tail and first-run wizard endpoints are absent.

Live rows expose EMA3/10, recent average/peak/p95, delta bytes, 24-hour usage totals, interval/age, gap/warming/coarse fields. Top-level fields: `data`, `generated_at`, `sample_interval_ms`, `window_seconds`. The in-memory distribution uses at most 12 samples, not a guaranteed one-minute window. History `from`/`to` accept RFC3339 or Unix seconds/milliseconds; the ordered range is at most 31 days and reads at most 200000 rows. Raw speed is cleaned after 24 hours, so a 31-day request does not guarantee 31 days of data. Buckets are computed during reads, not durable rollup tiers.

List responses normally use `{"data":[],"total":N,"offset":0}` for object lists or `{"data":[]}` for telemetry/audit/backup lists. Activity `from`/`to` accept RFC3339 timestamps, and filtering by `domain` is a case-insensitive destination substring over the bounded result set. Activity is not a complete browsing record: attribution requires Xray's user email and depends on logging and collection availability.

## Injections and quotas

`GET /injections`, `PUT /injections/{id}`, and `DELETE /injections/{id}` manage bounded subscription mutations. A PUT accepts `rule_type` (`append`, `prepend`, `patch`, or `remove`), optional `format` (`uri-list`, `json`, or `yaml`), `content` (1 byte to 256 KiB), optional JSON Pointer `path`, and an optional `enabled` flag. Omitted `enabled` defaults to true; explicit false remains disabled. With no format, Core detects the representation. Unknown representations pass through unchanged. Injections currently run after a node is selected through a policy or pin; a passthrough subscription does not apply them.

`GET /quotas`, `PUT /quotas/{id}`, and `DELETE /quotas/{id}` manage `node_id`, `scope` (`per_user` or `per_node`), `user_id` for per-user rules, positive `limit_bytes`, optional nonnegative `grace_bytes`, `period` (`day`, `week`, `month`, or `rolling`; defaults to month), optional IANA `timezone` (UTC by default), and `enabled`. Rolling means the preceding 30 days; calendar weeks begin on Monday. A PUT returns the normalized rule and server-generated `updated_at`. Rule mutations and their audit records commit atomically. Quota checks use committed telemetry in SQLite, survive Core restarts and duplicate batch replay, and fail open when usage storage cannot be read. Per-node rules include inbound fallback counters when user counters are unavailable; overlapping inbound intervals are suppressed where user observations provide a more specific measurement. When the selected node is exhausted, the gateway retains the original subscription and reports `X-SubGate-Policy: quota-exhausted`. This is a routing decision, not a data-plane traffic cutoff.

`POST /nodes/{id}/block-check` accepts an optional synced inbound `port` (443 by default), optional `address` matching the synchronized node IP, and `tls`/`server_name`. Unknown nodes return 404, nodes without synced endpoints return 409, and mismatched targets return 400. The check runs from Core and is audited before network activity. `GET /blocks` lists saved results.

`POST /youtube/check` accepts `node_id`, optional `video_id`, and an optional endpoint restricted to HTTPS port 443 on `youtube.com`, `www.youtube.com`, `m.youtube.com`, or `youtu.be`; URL credentials and fragments are forbidden and every redirect is revalidated. Invalid endpoints return 400. Checks run from Core; `node_id` labels the result but does not move network execution to that node. `GET /youtube` lists saved results. A successful diagnostic request returns 200 with the probe result, including any network failure verdict.

Compatibility tightening: block checks no longer accept arbitrary addresses or unsynced ports, and the YouTube API no longer accepts arbitrary mirror URLs. The standalone YouTube utility can still use a mirror in trusted code. Quota rules now reject unknown timezones and integer overflow. Existing authentication, error envelopes, and endpoint paths are retained.

## Policies, tiers, pins and geo rules

Each of `tiers`, `policies`, `pins`, `geo-rules`, and `alerts` supports `GET /{kind}`, `PUT /{kind}/{id}`, and `DELETE /{kind}/{id}`. PUT replaces the JSON object and forces its `id` from the path. Create referenced tiers before policies; pins must reference synced users/nodes.

```json
{
  "name": "Europe",
  "enabled": true,
  "strategy": "leastLoad",
  "min_healthy": 1,
  "node_ids": ["NODE_UUID"],
  "fallback_tier_id": ""
}
```

Tier strategies are `random`, `roundRobin`, `leastPing`, and `leastLoad`. Eligible nodes have status `online` and no committed quarantine or recovery exclusion; `leastPing` requires a recorded successful probe. A fallback must reference an existing tier and cannot form a cycle.

```json
{
  "name": "Default routing",
  "enabled": true,
  "priority": 10,
  "tier_id": "europe",
  "conditions": {"countries": [], "asns": [], "user_agent": ""}
}
```

Policies are evaluated by ascending priority. A valid enabled user pin (`{"user_id":"UUID","node_id":"UUID","enabled":true}`) takes precedence. Country/ASN conditions use Core's client IP. Behind a proxy, configure explicit trusted proxy CIDRs before applying subscriber address rules. Unmapped endpoint entries are preserved to avoid corrupting unsupported protocols.

Geo rules use `name`, `enabled`, `rule_type` (`country`, `asn`, `cidr`, `network_type`), `action` (`allow`, `deny`), and a nonempty string `values` array. The first matching rule in stored object order determines the decision; CIDR and operator-supplied intelligence rules are supported. Country matching requires intelligence stronger than `low` confidence.

## Agent contract

Enrollment request: `{"node_id":"UUID","token":"ONE_TIME_TOKEN"}`. A 201 response contains the durable node token; Agent stores it with mode 0600. Re-enrollment replaces the previous token. Agent ingress uses `Authorization: Bearer AGENT_TOKEN`, which is distinct from an administrator session.

`POST /api/v1/agents/heartbeat` accepts version, timestamp, memory/uptime, and stats/log error diagnostics. `POST /api/v1/agents/telemetry` accepts `batch_id`, `node_id`, speed `samples`, and activity `events`. The authenticated token must match the payload node. Duplicate batch IDs are safe to retry. Limits are 10000 samples and 1000 events per batch; timestamps must be within 24 hours in the past or five minutes in the future. Speed fields are nonnegative integer bits/s.

Optional `sent_at` is the batch creation time, retained through retry and restart. It has the same 24-hour-past/five-minute-future bounds; Core preserves original sample/event timestamps and does not shift them at delivery. Missing `sent_at` remains supported. Speed/usage observations use the millisecond `(node_id,user_id,timestamp)` key: repacking identical samples with a new batch ID replaces that observation, including corrections to zero or inbound scope. Repacking activity events with a new batch ID is not an event deduplication guarantee.

See [`../pkg/wire/wire.go`](../pkg/wire/wire.go) for the exact wire fields. Agent never resets Xray counters. It does not offer a remote shell or arbitrary command ingress.

Activity queries, exports, and purge accept a synchronized panel ID, username, email, or short UUID. Core expands only aliases belonging unambiguously to one panel user; unknown Xray labels remain searchable by their original value. Stored telemetry retains the Xray label for encryption and diagnostics. Shared aliases are excluded when expanding a panel ID.

Agent's encrypted pending spool defaults to `data/pending.sgb` (`-spool-file`). It retains the same batch ID across retries/restarts, limits each batch to 5000 samples/1000 events/2 MiB, and bounds pending telemetry to 16 MiB. Permanent validation 4xx responses discard the rejected batch and count lost items; 408/429/5xx and network errors retain it for retry. 401/403 stop the Agent and preserve pending telemetry. Re-enrollment changes the encryption key, so archive/remove the old spool before starting with the new token. Diagnostics report discarded or expired telemetry; logs waiting in the in-memory channel are not yet durable.

## Health, transport и ключи
Текущий health/quorum/maintenance контракт: [health](registry/health.md).
Sanitized inbound descriptors и verified levels (tcp/tls/ws/httpupgrade/grpc/xhttp-options/xhttp-h3-options):
[transports](registry/transports.md). Transport acceptance не доказывает proxy-user authentication.

Agent-token GET /api/v1/agents/health-targets возвращает synchronized targets (max64,16 inbounds/node).
PUT /api/v1/agents/health-results принимает max64 результатов без повторов target, source берётся из credential.
400 при invalid/unknown target/time/verification,401 при revoked/replaced/missing token,500 при persistence failure;
oversize fleet health-targets503. Свежесть90s/future5s, legacy completed_at использует receive time.
Core принимает только votes, применимые к текущему target; legacy TCP failures не дают quorum для H3-only.
Точные optional wire fields: [wire.go](../pkg/wire/wire.go).
Текущий suite имеет5health FAIL, см. [BLOCKED](BLOCKED.md#b04-воспроизводимые-тесты).

Storage rollover: storage.encryption_key_file/SUBGATE_STORAGE_KEY_FILE и previous_encryption_key_files/
SUBGATE_PREVIOUS_STORAGE_KEY_FILES (до8previous), restart required. Regular private32raw-byte key, no symlink/FIFO.
Старые данные не re-encrypt автоматически; restore принимает original backup key.
Retention/activity policy: [activity](registry/activity.md); audit/key safety: [security](registry/security.md).
