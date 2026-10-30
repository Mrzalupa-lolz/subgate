# Core HTTP API (кодовая версия 0.2.0)

Справочник сверён с текущими `Server.routes` и handlers 2026-10-13. Это описание существующего API, не заявление о готовности всех требований. Доказательства проверенных частей и ограничения — [PROJECT_PLAN.md](PROJECT_PLAN.md). Полный runtime каждого endpoint отдельно не проверен.

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

Subscription routes register GET only; HEAD is not registered. Upstream requests always use GET. Forwarded request headers are `Accept`, `User-Agent`, `X-Hwid`, `X-Device-Os`, `X-Device-Model`, `X-Ver-Os`, `X-Device-Type`; no device-header wildcard or `Accept-Language` forwarding. Response passthrough uses content-type, subscription-userinfo, profile-title, profile-update-interval, support-url, profile-web-page-url and content-disposition. Additional headers are not preserved automatically.

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
| POST | `/backups` | Superadmin creates encrypted `.sgb` snapshot |
| GET | `/backups/{name}` | Superadmin downloads encrypted backup bytes |
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

Tier strategies are `random`, `roundRobin`, `leastPing`, and `leastLoad`. Healthy nodes currently mean panel status `online`; `leastPing` requires a recorded successful probe. A fallback must reference an existing tier and cannot form a cycle.

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

See [`../pkg/wire/wire.go`](../../../../pkg/wire/wire.go) for the exact wire fields. Agent never resets Xray counters. It does not offer a remote shell or arbitrary command ingress.

Activity queries, exports, and purge accept a synchronized panel ID, username, email, or short UUID. Core expands only aliases belonging unambiguously to one panel user; unknown Xray labels remain searchable by their original value. Stored telemetry retains the Xray label for encryption and diagnostics. Shared aliases are excluded when expanding a panel ID.

Agent's encrypted pending spool defaults to `data/pending.sgb` (`-spool-file`). It retains the same batch ID across retries/restarts, limits each batch to 5000 samples/1000 events/2 MiB, and bounds pending telemetry to 16 MiB. Permanent validation 4xx responses discard the rejected batch and count lost items; 408/429/5xx and network errors retain it for retry. 401/403 stop the Agent and preserve pending telemetry. Re-enrollment changes the encryption key, so archive/remove the old spool before starting with the new token. Diagnostics report discarded or expired telemetry; logs waiting in the in-memory channel are not yet durable.

Subscription GET/HEAD routes preserve method and raw query. Forwarded metadata includes Accept-Language, User-Agent, X-Hwid, X-Ver-Os and X-Device-* (all values); Cookie/Authorization and Connection-nominated headers are excluded. Cached variants include method and exact metadata bytes. HEAD returns metadata/length without running body transforms; announce/announce-url pass through with the existing subscription header list.

Subscription body transforms have a 300 ms context budget. Panic, timeout or 32-worker capacity exhaustion returns the original upstream bytes/headers with `X-SubGate-Policy: bypass`. Late transform results cannot modify the response or raw cache. Noncooperative transforms retain a bounded worker slot until they finish.

Storage encryption key rollover (restart required): `storage.encryption_key_file` / `SUBGATE_STORAGE_KEY_FILE` selects the active key; `storage.previous_encryption_key_files` / comma-separated `SUBGATE_PREVIOUS_STORAGE_KEY_FILES` supplies up to eight previous keys for reads. Each file must be a private regular file with exactly 32 raw bytes; group/other permissions and symlinks are rejected. The default generated `subgate.key` follows the same checks.

To roll over, stop Core, generate a fresh private key outside the repository, point the active key setting to it and include the original key in the previous-key list, then restart. New activity, encrypted audit details, gateway snapshots and local backups use the active key; old ciphertext retains its original format and can be read with a configured previous key. This does not re-encrypt existing data: retain each old key until all dependent data expires or is deliberately removed. Retain the key used for an archived backup separately; CLI restore still takes that backup's original key. Losing all matching keys makes recovery impossible. Restore compatibility via CLI is not verified by the rollover test.

Core automatically probes synchronized node inbounds independently of panel polling. YAML `health.probe_interval` defaults to `30s`, `health.probe_timeout` to `8s` per node; accepted ranges are `10ms..24h` and `10ms..30s`. One cycle runs at a time with at most four node workers and sixteen endpoints per node. `/api/v1/health/probes` returns the last probe with its health `status`. Probe and engine state commit together and restore on restart. UDP/credential-only probes and incomplete endpoint coverage without success remain unverified and do not count as node failures. A verified success currently means TCP/TLS reachability, not application authentication. YAML `health.failure_threshold` (default3) and `health.success_threshold` (default2), each1..1000, control consecutive N/M. `health.min_quarantine` defaults to1m; `health.backoff_base` to30s and `health.backoff_max` to15m, bounded10ms..24h with base<=max. Restart applies changes. A first success moves quarantine to recovering; both statuses remain excluded until M consecutive successes and the minimum deadline pass. A failure during recovery resets the success count and restores quarantine. Continued failures double the next-probe delay up to the cap; Core and Agent targets wait until that persisted deadline. Minimum hold and backoff start at the committed transition decision after the probe, not its start. Counters/deadlines restore after restart. Routing reads a committed health snapshot: it skips quarantined/recovering pins and tier members, takes eligible fallback tiers, and removes unambiguously mapped endpoints even without a routing policy. Unknown entries and ambiguous addresses remain untouched; unsafe transforms or exhausted fallback tiers return exact upstream bytes. Health changes also apply on cache hits. Manual overrides, critical/recovery alerts and full transport authentication are follow-up work.

YAML `health.quorum` is the number of agreeing vantage points required for a transition (`1..64`, default `1`). Core contributes one verified vote; each enrolled Agent contributes at most one per target. If both success and failure reach the configured threshold, success wins. Missing consensus preserves the previous state; `/api/v1/health/probes` exposes `quorum_met` and `vantages`, while `success`/`verified` describe Core's own latest observation.

Agent-token routes: `GET /api/v1/agents/health-targets` returns `{data:[{node_id,address,inbounds:[{protocol,network,security,port}]}]}` for synchronized nodes (max64 targets,16 endpoints/node); no panel credential is sent. `PUT /api/v1/agents/health-results` accepts `{results:[{node_id,timestamp,completed_at,success,verified}]}` (max64, no duplicate node IDs; verified `tcp`, `tls` or `unverified`). The source is derived from the Agent credential, not JSON. Per-source/target latest timestamps replace older evidence; timestamps/receive age expire after90s, future tolerance5s. Revoked or replaced credentials invalidate their saved votes. Unknown nodes, invalid verification and out-of-window timestamps return400; missing/revoked/changed auth401; persistence failure500. Vote validation and state transition commit in a shared transaction.

Agent.Start runs a health cycle every30s, four workers,8s/node and30s cycle budget. Completed results from a partial cycle are reported under a separate5s send deadline; canceled/unprocessed results are omitted, and the next cycle starts at unfinished targets to avoid starvation. This proves TCP/TLS reachability only; credentialed transport handshakes and independent regional deployment remain unverified. Fleets beyond64 targets return503, and nodes beyond16 inbounds are omitted from Agent targets rather than reporting false failure.

YAML `health.maintenance_windows` accepts up to128 windows `{node_id, from, until}` with absolute RFC3339 timestamps and a nonempty node ID; `from < until` is required. Restart Core to apply changes. Windows are `[from,until)`; overlaps form a union for each node. During maintenance Core does not dial the node, manual probes return409, and Agent targets omit it. `/api/v1/health/probes` exposes `maintenance:true`; existing health state and counters remain unchanged. Core observations overlapping a window up to the transition decision and Agent observations overlapping a window are ignored. Agent `completed_at` must be at least `timestamp` and no more than5s in the future; legacy reports without it conservatively use server receive time as the observation end. After the window ends regular probes resume. Maintenance suppresses health checks and transitions; it does not by itself remove the node from subscriptions.

Health observations expose `generic:true` for an unknown protocol. The descriptor names/aliases follow Xray26.7.28. Unknown protocols require an explicit supported transport; unknown/missing transports or unknown security remain `unverified`. Recognized TCP-based transports currently prove only TCP reachability, or a verified TLS handshake when `security=tls`; they do not prove WS/gRPC/XHTTP/HTTPUpgrade or REALITY acceptance. Manual node probes also persist `unverified` when any endpoint is unverified or more than16 endpoints prevent complete coverage, unless an endpoint succeeds. Unknown protocol/transport/security strings are preserved in observations. The UI shows a generic-handling label; browser behavior is not yet verified.

For synchronized WS/websocket inbounds containing rawInbound, Core extracts only streamSettings.wsSettings path/Host (legacy headers.Host fallback) and TLS serverName. Xray26.7.28 method takes precedence over network; the ed early-data query parameter is removed and an empty configured path means /. Raw client settings, certificates, keys and arbitrary headers are discarded before persistence and never sent to Agents. Agent inbound descriptors may include http:{path,host,server_name}. Malformed or mismatched raw config becomes an invalid descriptor and remains unverified; unsupported paths with a remaining query/fragment, control characters, excessive length or invalid Host/port are unverified. Legacy inbounds without raw config retain their lower TCP/TLS reachability level.

With a valid configured descriptor, WS probes send a fresh RFC6455 GET upgrade to the synchronized node address; only matching 101/Upgrade/Connection/Sec-WebSocket-Accept proves verified:ws. Host and SNI do not change the destination. TLS uses normal certificate and hostname verification; HTTP Host keeps its port while inferred TLS SNI omits it. Redirects are never followed, no proxy environment is used, response headers are bounded to16KiB, and context/deadline closes the connection. No body or proxy credentials are sent. Manual probes, Agent reports, quorum and latency pins recognize ws; a successful upgrade proves transport acceptance, not proxy-user authentication. gRPC/XHTTP/HTTPUpgrade/QUIC/REALITY acceptance remains follow-up work.

Configured HTTPUpgrade inbounds also extract sanitized httpupgradeSettings path/Host and TLS serverName. An ed parameter is removed, empty path means /, and method overrides network. A Host key inside httpupgradeSettings.headers is invalid in Xray26.7.28 and yields unverified; arbitrary headers and raw credentials remain discarded. Core/manual/Agent perform the pinned HTTPUpgrade handshake: GET with Upgrade:websocket and Connection:Upgrade, without a WebSocket challenge. Matching101/Upgrade/Connection yields verified:httpupgrade; plain200, redirects and missing/wrong Upgrade fail. WebSocket still requires its independent RFC6455 challenge. The same destination restrictions,16KiB response bound, timeout/cancellation and certificate/hostname validation apply. Agent reports, quorum and latency pin consumers recognize httpupgrade. This proves transport acceptance only; gRPC/XHTTP/QUIC/REALITY acceptance and proxy authentication remain separate work.

Configured gRPC inbounds extract grpcSettings serviceName/multiMode/authority and TLS serverName into sanitized http:{path,host,server_name}. The method path mirrors Xray26.7.28 normal/custom service paths and Tun/TunMulti selection; unknown/invalid raw config stays unverified. Core/manual/Agent connect only to the synchronized node and send a five-byte gRPC envelope containing an empty protobuf Hunk, then close input, without proxy payload or credentials. Zero HTTP body does not complete the pinned Xray stream and is not used. HTTP2 is required; TLS requires h2 ALPN and normal certificate/hostname verification. No proxy or redirect is used. Headers are bounded16KiB and response body4KiB; malformed/compressed/incomplete envelopes or protobuf are rejected. Success requires HTTP200, application/grpc (or +subtype) and final grpc-status0; unknown service/method and stalled response fail. Verified:grpc is accepted by reports/quorum/latency pins. Empty service //Tun remains a path on the fixed origin, never a replacement destination. gRPC/REALITY remains unverified pending a REALITY probe; proxy authentication and XHTTP/QUIC remain separate work.

Configured XHTTP/splithttp inbounds extract only normalized literal path, Host and TLS serverName. Modes/placements and extra overrides follow Xray26.7.28; raw credentials/arbitrary headers are discarded. Core/manual/Agent send OPTIONS at that path without a session or proxy payload; HTTP200 with matching Xray CORS preflight headers yields verified:xhttp-options. This proves HTTP route acceptance only. Literal percent/hash characters are URL-escaped on the wire, preserving the server path. TLS with exactly alpn:[h3] listens on UDP and stays unverified until an HTTP/3 probe is implemented; it must not generate TCP failure votes. Invalid/mismatched configs remain unverified. Response headers are bounded16KiB, normal TLS certificate/name checks and3s socket/context deadline apply. Reports/quorum/latency pins recognize xhttp-options; QUIC/REALITY and tunnel authentication remain separate work.
