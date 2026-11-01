# PROMPT v4: Build "SubGate" — the single-pane control plane for Remnawave

> Scope 2026-11-01 (latest owner clarification): SubGate automates Remnawave/Xray traffic monitoring, explainable anomaly review and temporary shaping of high-confidence confirmed offenders. Subscription serving/HTTP subscription monitoring/user administration belong to middleware. External DDoS/firewall (F23) excluded. F25 restored; owner will choose rate/TTL later. Optional metadata-only nDPI collector approved. P2P or high usage alone never authorizes sanctions. Current implementation and gaps: docs/PROJECT_PLAN.md.

> Working name: `subgate` (rename freely). Paste the whole document to the AI coding agent and feed it **phase by phase** (section 19). This version supersedes v1–v3.

---

## Historical reference

The detailed v4 sections below contain superseded subscription features and speculative capacities. They are retained for traceability, not instructions to rebuild them. For current scope, priorities, UI and acceptance use docs/PROJECT_PLAN.md and docs/MONITORING_VALIDATION.md. Do not implement F1–F5 subscription paths, Users/pins pages or HTTP subscription monitoring from this historical text.

## 0. Role and mission

You are a principal-level engineer (Go, Linux networking, Xray-core, observability, IP intelligence, secure self-hosted systems) with a good eye for UI craft. Build **SubGate**: one self-hosted product around a **Remnawave** panel that gives the operator, in **one website brought up by one script**:

1. a **subscription gateway** (middleware between users and the panel) that balances, pins, quarantines, filters and customises subscriptions per user;
2. **fleet monitoring**: health, quarantine, block detection, alerts;
3. **real-time per-user bandwidth**, per second;
4. **IP intelligence**: a precise "who is connecting from where" layer (ASN, operator type, country, city) used everywhere;
5. **universal protocol support**, a **full Xray log pipeline** (all levels, including `debug`) and a **user activity journal** — where each user went, viewable for any moment in time, stored compactly;
6. **traffic monitoring and investigation** (request anomalies, IP origins, drop detection) and an **operations console** with notifications.

The operator must not need Grafana, Prometheus, Redis, PostgreSQL, a separate bot, or ten scattered scripts. **Everything is built in.** Integrations with Remnawave and `remnawave-admin` are first-class, but SubGate must be fully useful without `remnawave-admin`.

## 1. Product principles

1. **One place.** One binary/image, one port, one website. Embedded UI, time-series store, Telegram bot, IP-intelligence builder, backups, self-update, agent distribution.
2. **Fast and exact.** Performance budgets (section 11) are requirements verified by benchmarks in CI.
3. **Fail-open on the critical path.** If SubGate misbehaves, users still get a working subscription.
4. **Read-only toward accounting.** Never disturb Remnawave's own traffic accounting or Xray's behaviour (see F16, F19).
5. **Unknown = untouched / never crash on unknown input.** New protocols, new log formats, new panel fields must degrade gracefully, never panic.
6. **Everything auditable; sensitive data access-controlled, encrypted and minimised in size; secrets never leaked.**
7. **Tolerant reader, strict writer** toward the panel API.

## 2. Operator environment (kept deliberately generic)

Remnawave 3.x panel (verify exact version) managing Xray nodes on Linux servers; users mostly in a censoring region (RU) including mobile/"LTE whitelist" conditions; the operator may also run `Case211/remnawave-admin` (optional integration, section 4.2). **Do not hard-code anything about the current server list or protocols**: SubGate must handle *any* protocol/transport Xray supports (section 6), including ones not deployed today. Fill during P0:

```
PANEL_VERSION=<<verify>>  CLIENT_APPS=<<FILL>>  SUB_FORMATS=<<FILL>>  USERS=<<FILL>>  NODES=<<FILL>>
```

## 3. Platform, language and stack

**Platform: Linux only** (amd64 + arm64; kernel ≥ 5.10; systemd). No Windows/macOS code paths, no portability shims — use Linux-native facilities freely (inotify, netlink, journald, epoll). Build tags `linux`. Developers may build on other OSes only via containers.

**Language: Go (1.24+) for Core and Agent.** Record in `docs/adr/0001-language.md`:
- The workload is I/O-bound with huge concurrency *and* CPU-heavy inner loops (per-second parsing of tens of thousands of counters, log-line parsing at up to 100k lines/s, JSON rewriting for thousands of subscription requests/s). Go handles both with goroutines, a low-latency GC and single static binaries.
- **Python is rejected for these hot paths**: it is interpreted and GIL-bound, typically an order of magnitude or more slower than Go on CPU-bound loops (parsing, aggregation, JSON patching), needs a runtime and virtualenv on every node, and consumes far more RAM per agent. (Python is fine for glue/scripts; it is simply the wrong tool for a per-second, per-node, per-user pipeline.) The sibling project `remnawave-admin` is Python; SubGate talks to it over HTTP only.
- Rust rejected for v1 (negligible gain for I/O-bound paths, slower iteration); revisit only if a benchmark proves a hot path cannot meet its budget.

**Libraries (choose from this list; record deviations in ADRs):**

| Concern | Choice | Notes |
|---|---|---|
| Remnawave API client | **generated with `ogen`** from the OpenAPI spec of the exact panel version | zero-reflection decoding; do not depend on the stale community Go SDK (reference only). Regenerate on panel upgrade (CI drift job); cross-check nullability/enums with `@remnawave/backend-contract` |
| HTTP | `net/http` + `chi`; tuned `Transport` (HTTP/2, pools); `x/sync/singleflight` | no heavyweight frameworks |
| JSON | `tidwall/gjson`+`sjson` for surgical edits of Xray JSON; `bytedance/sonic` or `goccy/go-json` for full decodes | benchmark and pick |
| Cache | `maypok86/otter` v2 or `dgraph-io/ristretto` | TTL + stale-while-revalidate |
| Storage | SQLite WAL via `modernc.org/sqlite`; single-writer goroutine with batching; `sqlc`; embedded `goose` migrations | |
| Time series | built-in TSDB (section 10) | |
| Activity store (F20) | pure-Go columnar segments: `parquet-go/parquet-go` or a custom format; `klauspost/compress/zstd`; `golang.org/x/net/publicsuffix`; a bloom-filter library | no CGO |
| Xray stats API | gRPC with **minimal stubs generated from Xray's `.proto`** (do not import all of xray-core) | `google.golang.org/grpc` |
| Probes | `quic-go` (QUIC/Hysteria2 reachability), `crypto/tls`, raw sockets via `x/sys/unix` | |
| Realtime to browser | `coder/websocket`, compact binary/JSON diff frames | 1 Hz |
| Agent↔Core | bidirectional gRPC stream or WebSocket + protobuf + zstd, outbound-only | choose by benchmark (ADR) |
| Log tailing | `fsnotify`/raw inotify, journald via `go-systemd/sdjournal` or `journalctl -o json -f` subprocess | no Docker socket |
| IP intelligence | `oschwald/maxminddb-golang/v2` (read), `maxmind/mmdbwriter` (build) | section 7 |
| TLS/ACME | `caddyserver/certmagic` (standalone mode) | |
| Telegram | `go-telegram/bot` or `telebot` | |
| Auth/crypto | argon2id, `pquerna/otp`, optional `go-webauthn`, AES-256-GCM or `age` for secrets | |
| Logging/metrics | `log/slog` JSON; optional `/metrics` via `prometheus/client_golang` | |
| Runtime | `automaxprocs`, `automemlimit`, `sync.Pool` on hot paths, pprof behind auth | |
| Frontend | **Preact + TypeScript + Vite** (small bundle), hand-written CSS with design tokens (Tailwind optional), TanStack Table/Virtual, **uPlot** for streaming charts, `d3-geo` + bundled TopoJSON for maps, i18next (RU/EN) | embedded via `go:embed`; section 9 |

## 4. Integration contracts

### 4.1 Remnawave panel (mandatory)
- **Auth:** panel **API token**, minimum scopes (document the scope list per feature). Never store admin passwords. Some endpoints are unreachable with API tokens — design only against reachable ones and degrade gracefully with a deep link to the panel.
- **Spec first:** obtain the OpenAPI spec for the exact panel version (panel's docs endpoint or the matching `remnawave/backend` release tag). If unobtainable, **stop and ask**. Never invent endpoints/fields.
- **Subscription upstream:** `GET /api/sub/{shortUuid}` and `/api/sub/{shortUuid}/{clientType}` plus the separate subscription-page app; support both. Preserve panel templates and response rules; transform *after* them.
- **Webhooks (primary sync):** verify the **HMAC signature** against the raw body, replay protection, idempotency; sub-second cache invalidation; reconciliation loop every 5 min (jittered) repairs missed events.
- **Panel-native concepts:** internal/external squads, **tags on hosts and nodes** (drive tiers/policies from tags such as `tier:1`, `lte`, `whitelist`, `udp`), config profiles, HWID limits, consumption multipliers, bandwidth-stats endpoints, subscription request history.
- **Actions through the API** (RBAC + audit + confirmation): enable/disable user, reset traffic, revoke subscription, node restart/enable/disable, host enable/disable/bulk. Heavy config editing stays in the panel (deep link).
- Compatibility matrix in docs; startup check compares panel version with the generated client version.

### 4.2 `remnawave-admin` (optional connector)
- Known from its public release notes (current line v2.18.x): Python/FastAPI + React + PostgreSQL (+ optional Redis), node-agent/collector, Prometheus `/metrics` with Bearer token, REST API v2 and v3 `violations:read`, outbound webhooks, violation/HWID/torrent analyzers, Xray editor, backups, plugin API; **AGPL-3.0 since 2.16.0** (≤ 2.15.x MIT).
- **Rules:** communicate **only over documented HTTP interfaces**; never read its PostgreSQL schema; never copy its code; treat its data as untrusted.
- **Connector features (toggleable):** show violation/HWID data in SubGate user detail; forward SubGate alerts to its webhook; optionally expose per-user live speed to it; deep links both ways; shared-secret + IP allowlist.
- Coexistence: all SubGate ports configurable (its collector/agents use 8081 by default), no Docker-socket sharing.
- Not in v1: re-implementing its violation detectors or Xray editor.

### 4.3 Xray (via agents)
Per-user live counters come from Xray's gRPC `StatsService`; logs come from Xray's log files/journal. Discover in P0 how the node runtime exposes the Xray API (address/port, user/online stats enabled?), how the Xray `email` maps to a Remnawave user, where logs live and at what level. Document in `docs/xray-stats.md` and `docs/xray-logs.md`. Missing prerequisite ⇒ precise diagnostic in the UI, never silent failure.

## 5. Architecture

```
 Clients ─HTTPS─► [reverse proxy | built-in ACME] ─► SubGate Core ─► Remnawave panel (REST + webhooks)
                         ▲ fail-open fallback               │ ▲
                                                            │ └─ remnawave-admin (optional, HTTP only)
 Operator ─HTTPS─► embedded web console                      │
                                                             ▼
 Node Agents (outbound-only) ──► per-second user stats, Xray logs, health, region checks
 Probe Agents in the censorship region ──► reachability + "16–20" block checks
```
Core = gateway + panel sync + health + policy/tier/pin engine + quotas + IP-intel + TSDB + log analytics + request observations + notifier + bot + admin API + console + agent registry/distribution + backups + updater. Agent/Probe = same binary (`subgate agent|probe`). Deployment modes: **A) behind an existing reverse proxy** (recommended, with fail-open fallback) and **B) standalone with built-in ACME**. The panel API token lives **only on Core**; agents use their own revocable tokens.

## 6. Universal protocol and transport support (hard requirement)

SubGate must work **stably with every protocol, transport and security mode Xray-core supports — including those not deployed today and those added in future Xray releases.**

- **Scope (non-exhaustive, data-driven):** protocols VLESS, VMess, Trojan, Shadowsocks (incl. 2022 AEAD), SOCKS, HTTP, Dokodemo-door, WireGuard, Hysteria/Hysteria2, Freedom, Blackhole, DNS, Loopback, and any others Xray adds; transports raw/TCP, WebSocket, gRPC, HTTP/2, HTTPUpgrade, XHTTP (SplitHTTP), mKCP, QUIC, Hysteria; security none / TLS / REALITY / XTLS flows (Vision etc.), fingerprints, ECH, mux, and whatever Xray adds. Subscription URI schemes: `vless://`, `vmess://`, `trojan://`, `ss://`, `hysteria2://`/`hy2://`, `wireguard://`, `socks://`, `http(s)://`, plus sing-box/Mihomo-only types (e.g. tuic, anytls) as **pass-through**.
- **Design rule: opaque-by-default.** The transformer treats outbounds/entries as opaque objects identified by `tag`, `protocol`, address/port and remark; it never whitelists protocols for passthrough. Protocol-specific knowledge lives in a small **protocol descriptor registry** (probe type, per-user-stats capability, port semantics) with a **generic default** for unknown protocols (generic TCP/UDP reachability or "unprobed" — never an error).
- **Health probes per transport:** TCP connect; TLS handshake with SNI; REALITY (unauthenticated client probe); WebSocket/gRPC/XHTTP/HTTPUpgrade (HTTP-level request to the configured path); UDP/QUIC/Hysteria2 (QUIC Initial via `quic-go`); mKCP and WireGuard (UDP reachability heuristics; full handshake only if the operator supplies a probe credential) — mark what cannot be verified as `unverifiable` rather than guessing.
- **Per-user stats capability matrix** (`docs/protocol-matrix.md`): for each protocol, whether Xray exposes per-user counters; if not, fall back to **inbound-level counters** (`inbound>>>tag>>>traffic`) clearly labelled "not per-user".
- **Golden corpus & real-Xray tests:** generate a corpus of configs covering every protocol × transport × security combination Xray supports (from Xray's own documentation/schema); run **real xray-core in CI containers** against them to verify parsing, rewriting, probes and stats. **Fuzz** every parser (`go test -fuzz`) — arbitrary input must never panic.
- Acceptance: adding a never-seen protocol to a config profile requires **no SubGate code change** for passthrough, balancing and health (generic), and produces a visible "unknown protocol (generic handling)" badge.

## 7. IP intelligence (ASN, operator type, geography) — "who connects from where"

**Goal:** for any client IP (v4/v6), return in-process, in microseconds: ASN, AS name/organisation, **network type** (`mobile`, `home-isp`, `hosting`, `vpn/proxy`, `cdn`, `edu`, `gov`, `unknown`), country, region, city, approximate coordinates, time zone, **confidence** and **provenance**.

**Data layers (Core downloads and refreshes them itself; verify current URLs, formats and licences in P0):**
1. **L1 – origin ASN from BGP (authoritative for ASN, fresh for newly announced prefixes):** IPtoASN (public-domain PDDL data, daily) and/or tables derived from RIPE RIS / RouteViews; optionally the BGP.tools table (requires a compliant User-Agent; check its terms).
2. **L2 – registration country from RIR delegation files** (`delegated-extended` from RIPE NCC, ARIN, APNIC, LACNIC, AFRINIC; public, daily). Covers **newly allocated blocks** that geolocation databases don't know yet — this closes the "new territories" gap for IP-origin analytics.
3. **L3 – geolocation:** **DB-IP Lite** City + ASN (CC BY 4.0, monthly; **a visible link-back "IP Geolocation by DB-IP" is mandatory on pages displaying its results**), **IPinfo Lite** (free token, country + ASN, daily; the publisher's resource page cites CC BY-SA 4.0 — verify share-alike implications before redistributing any derived file), optional **MaxMind GeoLite2** City/ASN (free account + licence key, EULA, twice-weekly updates). Support any MMDB v2 file the operator supplies.
4. **L4 – network type classification:** seed from public datasets (e.g. `ipverse/as-metadata` categories, Stanford **ASdb** research dataset — latest snapshot 2026-03 — BGP.tools tags, PeeringDB) **plus operator-curated overrides in the UI**. Public datasets lump mobile carriers into `isp`, so **the mobile-operator list must be curated**: ship seed lists for the key countries (RU first; major mobile operators resolved by organisation name through RIR/BGP data, each marked `needs review`). Heuristics (AS name contains mobile/LTE/GSM/cellular, CGNAT density = many distinct users per IP, reverse-DNS patterns) produce **suggestions only**, never auto-applied.

**Build and serve:** Core merges the layers into one internal **MMDB** (`maxmind/mmdbwriter`), built off-path and **atomically hot-swapped**; memory-mapped reads. Precedence: ASN from L1; country from geo DBs unless they contradict L2/AS-country with no multinational explanation (then lower confidence); unknown prefixes fall back to L1+L2. Result carries `confidence: high|medium|low`, `sources[]`, DB build dates. Keep last-good on failed refresh; verify checksums; alert when a source is stale > N days; allow "country+ASN only" lite mode to save memory.

**Honest precision:** ASN origin is precise; country is usually right; **city is approximate** — mobile carriers often geolocate to a gateway hub, not the subscriber. The UI labels precision level and never shows a street-level pin.

**Client-IP quality rules:** trusted-proxy chain only; IPv4-mapped IPv6 normalisation; IPv6 grouped by /64 for per-user logic; **CDN-fronted traffic** (XHTTP via CDN) shows CDN IPs — detect CDN ASNs and use configured real-IP headers (`CF-Connecting-IP` etc.) only from trusted CDN ranges, else mark "via CDN"; show **CGNAT hints** (many users per IP).

**Uses:** IP-origin analysis (F7), policy conditions/tiers (F3: ASN class, operator, country), subscriber map (F8), per-user location history, request observations (F12), **connection-origin analytics from node logs (F19)**, optional impossible-travel / sharing hints (suggestions only).

**Interfaces:** `GET /api/v1/ipintel/lookup?ip=` (+ bulk), CLI `subgate ipintel lookup <ip>`, UI page "IP intelligence" with a lookup box, source/licence/build-date panel, curated-ASN editor (tags, import/export CSV).

**Acceptance:** ≤ 5 µs p99 per lookup in-process; golden IP corpus tests; a prefix present in RIR files and BGP but absent from geo DBs still yields ASN + country with `confidence: medium`; a coverage report shows the % of real seen client IPs resolved with high confidence; attribution links present; hot-swap during load causes zero failed lookups.

## 8. Functional requirements

### F1 — Subscription gateway
- Routes `GET|HEAD /{shortUuid}` and `/sub/{shortUuid}` (configurable). Forward method/query/relevant headers (`User-Agent`, `x-hwid`, `x-device-*`, `Accept-Language`); resolve client IP via the trusted-proxy list only.
- Pass through upstream headers (`subscription-userinfo`, `profile-update-interval`, `profile-title`, `content-disposition`, `support-url`, announce…) unless a rule modifies them.
- Adapters `Parse → NormalizedModel → Serialize`: `xray-json` and `uri-list` first; `mihomo-yaml`, `singbox-json` in P4. Unknown ⇒ byte-for-byte passthrough.
- Pipeline of pure stages: `geo-gate → quarantine filter → tier/policy ordering → pin → quota filter → injections → header edits`.
- Cache by (shortUuid, format, policy-version), TTL + stale-while-revalidate, `singleflight` to the panel, webhook invalidation.
- **Fail-open:** panic recovery; transformer budget 300 ms; DB/panel failure ⇒ last-good or unmodified upstream; never 5xx if upstream is fine.
- Acceptance: with all rules off, output equals upstream (diff test over real fixtures); killing the process still serves subscriptions via the shipped reverse-proxy fallback.

### F2 — Health engine and quarantine
- Checks per host/transport as in section 6, plus panel node status, agent-local checks (Xray process, ports), optional end-to-end proxied check with a dedicated probe user.
- Quarantine after N consecutive failures, restore after M successes; min quarantine time; exponential back-off; **quorum** across vantage points; manual force-quarantine/healthy and maintenance windows.
- Degradation: next tier → panic mode (serve everything + critical alert). One alert per transition (dedupe + cooldown). Log-derived signals (F19) feed health.

### F3 — Tiers, policies, transport-aware selection
- Tiers 1..N per policy; strategies map to Xray balancers (`random`, `roundRobin`, `leastPing`, `leastLoad`) with editable `burstObservatory`; tier k+1 engages when tier k has `< min_healthy` healthy nodes (or via `fallbackTag`). Rewrite `routing.balancers[].selector`, outbounds, observatory.
- Tier membership from **panel tags**, squads, or explicit mapping. Policies select layouts by squad/tag/country/**network type & operator (IP-intel)**/User-Agent/app/plan; first match wins; default mandatory.
- Layouts may order/exclude entries by transport (any protocol per section 6).
- Acceptance: a mobile-operator client and a home-ISP client of the same user receive different layouts; losing all tier-1 nodes yields tier-2, never an empty subscription.

### F4 — Pinning
- Persist `user → node` (or ordered list); initial choice by **rendezvous hashing** over healthy nodes of the top eligible tier, capacity-weighted; re-pin only on quarantine/removal/quota/operator action; configurable sticky-return hysteresis.
- Output modes `single`, `ordered-fallback`, `balancer-preferred`; per-node max pinned users; rebalance with dry-run and change budget; metric "exit changes per user per day".

### F5 — Custom injections
- Rules scoped to all/group/user/policy/format: append/prepend raw entries (Xray outbound JSON, URI lines, Mihomo block, sing-box outbound), JSON Merge Patch / RFC 6902 on `xray-json`, remove by tag. Sandboxed templating (**no code execution**). Validation before activation, **dry-run per user with diff**, versioning, one-click rollback.

### F6 — Per-node traffic quotas
- Quota = (node, scope) × bytes × period (day/week/month, calendar or rolling, time zone). Source: panel per-user/per-node usage with fallback to agent accounting (F16). Over quota ⇒ node removed from **that user's** next subscription; warnings at 80/90 %. Document that enforcement happens at refresh time.

### F7 — IP-origin analysis
- Country/ASN/network-type/CIDR are context for investigation, not access restrictions. Unknown IPs remain unknown; no geo deny or decoy responses.

### F8 — "Who is where": maps and per-user origins
- Two views: (a) **subscription-request origins** and (b) **connection origins from node access logs (F19)** — the latter is the accurate one. Aggregations by country/city/ASN/operator/network type over time, CGNAT hints, per-user history (last N distinct ASN/country/city with timestamps), filters by operator.
- Bundled map data only (no external tile servers; never leak user IPs). Retention is configurable per tier (defaults: raw 30 d, aggregates 365 d); IP truncation (/24, /48), salted hashing and a global **no-IP mode** are optional switches (off by default).

### F9 — Block check "16–20"
- Detect DPI that stalls/kills connections after ~16–20 KiB, per inbound, from inside the censorship region. **Default raw-TLS mode:** connect with the inbound's SNI and read ≥ 64 KiB, record bytes at first stall/reset. **Advanced mode:** real proxied transfer via a spawned xray-core client with a probe user. Verdict `ok|suspect|blocked` (default signature: stall/reset within 14–24 KiB). Matrix per (vantage × node × inbound); always alerts; quarantine/demote only if enabled. Run via `subgate probe` in-region or an operator-supplied SOCKS5/HTTP proxy.

### F10 — Traffic-drop detection
- Metrics: online users and traffic per node/global, subscription requests/min. Seasonal baseline (hour-of-week median + MAD, EWMA fallback). Alert below baseline × (1 − threshold) for T minutes; maintenance suppression; context (quarantines, blocks); dedupe, cooldown, recovery message; visible "learning" state.

### F11 — YouTube region scan
- Every N hours per node the agent determines the region YouTube sees for the node egress (locale/consent handled; no API keys). If it becomes a forbidden region (default `RU`): dedicated alert, stop re-checking that node until reset/cooldown, optional policy action.

### F12 — Internal Xray anomaly observations (scope 2026-11-01)
- Analyze internal VPN connection logs and node telemetry; HTTP subscription requests belong entirely to middleware.
- Show measurements, thresholds, time windows, source quality and context for review. High usage, P2P and many connections alone do not establish abuse.
- Future high-confidence major violations may trigger temporary shaper F25 after exact attribution, repeated independent confirmation and owner-selected rate/TTL. Other cases go to owner review. External DDoS/firewall protection remains excluded.

### F13 — Web console, settings, updates
- Pages: **Live**, Dashboard, Nodes & mapping, Users & pins, Policies & tiers, Injections, Quotas, **IP intelligence**, Geo & maps, Blocks matrix, **Logs**, **User activity (F20)**, Alerts, Request observations, Operations, Backups, Settings, Audit, System.
- All runtime settings in a **versioned config store** with validation, atomic hot-reload and rollback.
- Auth: local admins + mandatory TOTP, optional WebAuthn, roles `superadmin/admin/viewer` with **no privilege escalation**, HttpOnly cookies + CSRF, login rate-limit/lockout, API keys.
- **First-run wizard:** admin bootstrap → panel URL + token test → node discovery → IP-intel sources (keys) → Telegram → "Add node" (generates the agent install command).
- **One-click update:** signed releases, minimal-privilege updater (no raw Docker socket in the main container), previous version kept for rollback, DB backup before migrations.

### F14 — Notifications and Telegram bot
- Built-in bot with per-category topics (health, drops, youtube, 16-20, request observations, bandwidth, logs, system), webhooks, optional email. Severity, dedupe, cooldown, quiet hours, recovery messages, test button, RU/EN templates, proxy support. Commands (allow-listed Telegram IDs only): `/status`, `/node`, `/user`, `/top`, `/ip <addr>` (IP-intel lookup), investigation links.

### F15 — Agent and Probe
- Enrollment via one-time token from the UI; install one-liner served by Core, version-pinned, checksum **and signature** verified; systemd unit; outbound-only; batching, back-off, replay protection, clock-skew tolerance; revocable per-agent tokens; optional auto-update.
- Telemetry: heartbeat, CPU/RAM/disk, NIC throughput/drops, conntrack, TCP retransmits, Xray/remnanode state (read-only, least privilege, **no Docker socket**), per-user stats (F16), logs (F19).

### F16 — Real-time per-user bandwidth ("who uses how much speed this very second")
- **Source:** Xray `StatsService` per-user uplink/downlink counters (`user>>>{email}>>>traffic>>>uplink|downlink`), sampled by the Agent; where a protocol has no per-user counters, inbound-level fallback (section 6). Fallback of last resort (labelled "coarse"): panel bandwidth-stats API.
- **Hard rule — read-only:** query with `reset=false` **always**. Panel/node accounting consumes these counters; SubGate must never reset them. Handle externally-reset counters (negative delta ⇒ new value is the delta since reset) and Xray restarts without spikes.
- **Computation:** `speed_bps = Δbytes × 8 / Δt` with Δt from a **monotonic clock** around each successful read. Provide instantaneous (1 s), EMA 3 s/10 s, 1-min average, peak, p95. Align node series to wall-clock second buckets (±500 ms) when aggregating; tolerate NTP skew.
- **Cost control:** 1 s sampling by default; measure query cost each tick; if > 25 % of the interval, adapt (2 s → 5 s) and mark resolution in the UI; optional "live mode" (1 s only on watched nodes). Decide strategy by P0 benchmark (ADR).
- **Transport:** sparse per-second frames (non-zero deltas only), protobuf + zstd, back-pressure with drop-oldest and explicit **gap markers** (never fake zeros).
- **Storage tiers:** raw 1 s in an in-memory sparse ring (default 5 min, up to 15, hard memory cap, default ≤ 64 MB); 10 s rollups 24 h; 1 min 30 d; 1 h 1 y (SQLite, batched, time-partitioned).
- **UI:** virtualised top-talkers table, per-user detail with per-node breakdown, 1 s uPlot chart + zoomable history, node throughput vs configured NIC capacity (saturation %), burst detection, CSV export, WebSocket diff frames at 1 Hz.
- **Alerts:** user above X Mbit/s for Y min, node saturation > 85 %, abnormal burst; optional forward to `remnawave-admin`.
- **Semantics (documented in UI):** payload bytes counted by Xray, not wire bytes; devices of one user are summed.
- **Acceptance:** with `iperf3` through a test node the reported speed is within ±5 % at 1 s resolution; integration test proves no counter reset ever occurs and panel accounting is unchanged; load test 20 nodes × 5 000 online users without gaps on a 4 vCPU Core; resets/restarts produce no phantom spikes.

### F17 — Operations console
- Quick actions via the panel API (user enable/disable/reset traffic/revoke, node restart/enable/disable, host enable/disable/bulk) with RBAC, confirmations for destructive actions, full audit; global search over users/nodes/hosts; deep links into the panel.

### F18 — Backups
- Scheduled encrypted backups of SubGate state (SQLite snapshot + settings) to local disk, Telegram (auto-split large files) and optional S3; rotation by count/age; **safe restore** (single transaction, abort-and-rollback on any error, path-traversal-safe, superadmin only). The activity journal (F20) is excluded from backups by default (size, sensitivity); optional encrypted per-user/per-range export.

### F19 — Xray log pipeline (all levels, including `debug`)

**Goal:** ingest, parse, analyse and display Xray logs **at every verbosity level** (`debug`, `info`, `warning`, `error`, plus the access log and DNS log), stably, at any volume, without ever affecting Xray.

- **Hard rules:**
  1. **Never block or slow Xray.** Tail log *files* (inotify) or the journal; **never use a FIFO/pipe as Xray's log sink by default** (a slow reader would stall the writer). If the agent falls behind it skips ahead and records a gap; it never applies back-pressure to Xray.
  2. **Never crash on unknown input.** Parsers are tolerant state machines/regexes validated against a **golden log corpus captured from real Xray runs for every supported Xray version and every log level** (collect in P0/P1); unknown lines are counted as `unparsed` and kept in a bounded sample; very long lines are truncated; partial writes handled; **fuzz the parsers**.
  3. **Handle log rotation correctly:** rename, `copytruncate`, inode change, deleted-and-recreated files, journald vacuum — no lost lines, no duplicates beyond a documented bound.
- **Sources:** log files named in Xray's `log.access`/`log.error`/`dnsLog` settings, journald, and the Docker `json-file` log path mounted read-only. **No Docker socket.**
- **Volume handling:** target ≥ 100 000 lines/s per agent core at `debug` with bounded memory; per-class rate limits and adaptive sampling beyond budget; explicit drop counters shown in the UI; per-node ingestion budget configurable.
- **Extracted signals (aggregated in the agent):** line counts per level; error/warning classes (dial/DNS failures, timeouts, handshake/auth failures, inbound/outbound failures, benign-noise filter e.g. cancelled contexts); per-inbound/outbound-tag counters; **connection events with client IP, user, inbound/outbound tag and result**; failed-handshake bursts by source IP/ASN (feeds F12).
- **Connection origins:** client IPs seen in node access logs are enriched with IP intelligence (section 7) to power the accurate "who is where" views (F8): per user, per node, per ASN/operator/country/city, CGNAT hints.
- **Destinations and privacy switches:** destination hosts/IPs are captured and journaled per **F20** (enabled by default; switchable globally / per node / per user group). Client IPs follow the retention options of F8. The **Logs** page offers an ephemeral **live tail** (any level, per node) that is not persisted, permission-gated and audited. **Raw log text is never persisted** — only parsed and coalesced events (F20) and bounded error-line samples if the operator enables them.
- **Log-level awareness:** show each node's configured log level; warn that `debug` is verbose and costly for Xray itself; offer to change it only by proposing a config-profile change through the panel API with explicit confirmation (Xray needs a restart for this). SubGate works fully at any level.
- **Acceptance:** replay of the golden corpus at 100k lines/s completes without growth in memory beyond budget; rotation torture test (rename/copytruncate/recreate under load) loses no lines beyond the documented bound; Xray throughput is unaffected in an A/B test with the agent attached; unparsed ratio on the corpus is ≤ 0.5 % for supported versions and unknown lines never panic.

### F20 — User activity journal: "where did this user go, at any moment" — with compact storage

**Goal:** for every user, a searchable timeline of destinations (host/domain, port, protocol, node, inbound/outbound tag, client IP/ASN), viewable for any moment in time, stored so compactly that disk usage stays predictable and bounded.

**Capture (decision of the operator: enabled by default; switchable globally / per node / per user group, with a visible "activity journaling is ON" indicator in Settings):**
- **Source:** the Xray access log via the F19 pipeline. It requires the access log to be enabled with the user `email` field, and inbound `sniffing` to be enabled to recover domains for connections addressed by IP. **P0 must verify, per protocol/transport/Xray version, what the access log really contains** (domain vs IP, user, client IP) and the Core shows a per-node **capture-quality indicator** (% of events with user / with domain / with client IP). If something is missing, SubGate may *propose* enabling access log/sniffing via a config-profile change through the panel API (restart required, explicit confirmation).
- **Honest scope:** only destination `host:port` metadata per connection — never URLs, page content or payloads (TLS hides them). Clients that resolve DNS locally, or use ECH/DoH, produce **IP-only** destinations: enrich them with the owning ASN/organisation (IP intelligence L1, section 7), cached reverse DNS, and domain hints from the node's DNS log where available, and label each record with `resolved_by` so the operator knows how the name was obtained.
- **Event model (normalised in the agent):** `ts`, `user`, `node`, `inbound`, `outbound`, `src_ip`, `dst_host | dst_ip`, `dst_port`, `net` (tcp/udp), `result` (accepted/rejected/failed).

**Storage optimisation pipeline (the core of this feature):**
1. **Parse and discard raw text immediately.** Raw log lines are never persisted anywhere (a CI grep test enforces this).
2. **Coalesce in the agent.** Key = (user, destination, port, net, inbound/outbound, client IP) per time bucket (default 10 s) → one record with `count`, `first_ts`, `last_ts`. Browsers open many parallel connections to the same host, so this alone removes most volume.
3. **Chatter control.** Per-user per-minute cap on *distinct* destination rows (default 300). Overflow is collapsed into one summary row (`distinct_destinations`, `top_ports`, `p2p_suspected`) — this protects against torrent swarms and scan floods that would otherwise explode storage. Optional ignore-lists (OS telemetry, captive-portal checks, NTP) with per-rule hit counters. **Nothing is dropped silently:** every cap/filter exposes counters in the UI.
4. **Dictionary encoding.** Global string tables for hosts, registrable domains (eTLD+1 via the public-suffix list), users, nodes, tags, ASNs → integer IDs. Core owns the dictionaries; agents send per-batch local dictionaries to keep the wire small. Garbage-collect orphaned strings (reference counts) after purges and evictions.
5. **Columnar, immutable segments.** Time-partitioned (hour segments merged into day segments), **sorted by (user_id, ts)** for maximum compression and fast per-user queries. Columns: timestamp (delta-of-delta), user (run-length), destination (dictionary + delta), port (bit-packed), small enums `net/inbound/outbound/result` (bit-packed), `count` (varint), source ASN / client-IP id (dictionary). Each column zstd-compressed; per-segment footer with min/max time, **user range index, user bloom filter and destination bloom filter**; files written atomically and never modified. Implementation must be **pure Go, no CGO**: choose between `parquet-go` (dictionary + zstd) and a custom compact format by P0 benchmark (ADR). Embedded DuckDB, ClickHouse or any external database are rejected (they break the single-binary principle) unless the benchmark proves it necessary.
6. **Tiered retention with automatic compaction (all configurable):**
   - **Hot** — full-resolution coalesced events, default 14 days.
   - **Warm** — re-compacted to per-minute records per (user, registrable domain) with counts, first/last seen and a bounded list of top hostnames, default 90 days.
   - **Cold** — per-day summaries per (user, domain) (days visited, counts, first/last), default 365 days — tiny.
   - Optional per-user **legal hold** to keep hot resolution longer.
   - Re-compaction runs off-peak at low I/O and CPU priority, time-boxed, with a higher zstd level (≈15–19); the hot tier uses a cheaper level (≈3).
7. **Disk budget.** The operator sets a quota (e.g. 30 GB). The store enforces it: compact earlier, and if still over budget evict the oldest data tier by tier (cold last), with a warning *before* eviction. The UI shows GB/day, bytes/event, compression ratio versus raw text, and the **projected retention at the current rate**. Agents keep a bounded on-disk **spool** (default 512 MB ring) for Core outages.
8. **Crash-safe write path.** Batched, append-only, one writer per partition, configurable fsync policy; incoming batches go through a small WAL and are replayed on start; the Core acks batches explicitly and the agent replays unacked ones from its spool.
9. **Targets (measure; deviate only via ADR):** ≤ 8 bytes/event average compressed in the hot tier after coalescing; compression ratio ≥ 30× versus raw access-log text; ≥ 100 000 events/s sustained ingest on a 4 vCPU Core; compaction capped at 25 % CPU.

**Query and UI ("User activity" page):**
- Pick a user and a date/time range → a **timeline of visits grouped into sessions** (idle gap 5 min, configurable) with domain/ASN-organisation labels (no external favicon requests), top destinations per period, filters by node / client IP / ASN / operator, and a **time scrubber** that shows precisely what the user was connected to at second T (±window) on the hot tier.
- **Reverse search:** "who visited X" across users; "first seen" for a domain; period comparison; CSV/JSON/NDJSON export (permission-gated and audited).
- Segment pruning by time → user range → bloom filters. **Targets:** one user-day on the hot tier p95 ≤ 300 ms (20 nodes × 5 000 online users); one month on the warm tier ≤ 1.5 s; cross-user domain search ≤ 3 s per day of hot data.
- API: `GET /api/v1/activity?user=&from=&to=&q=&group=session|raw`, with streaming NDJSON export.

**Security of the journal (it will be the most sensitive dataset the product holds):**
- Encrypted at rest (per-segment AEAD, key from env/file, key rotation); a separate permission `activity:read` (and `activity:export`), granted to superadmin only by default.
- **Every query and export is audit-logged** (who, whose history, time range, filters); optional TOTP re-authentication before opening a user's history; rate limits plus an alert on bulk reads; the journal is excluded from the general API token scopes.
- Per-user **purge/erasure** command across all tiers and dictionaries (audited); scheduled retention enforcement.
- Backups (F18) exclude the journal by default (size and sensitivity); optional encrypted per-user/per-range export.

**Acceptance:**
- Golden-corpus replay shows ≥ 30× reduction versus raw text and reports actual bytes/event per tier.
- A user-day query meets the latency target; the scrubber is exact to the second on the hot tier.
- A crash during writes loses at most the last un-acked batch, which the agent replays.
- Disk-quota test: fill to budget → warning → oldest data evicted tier by tier → no corruption.
- Purge test: after erasure, the user's data and orphaned dictionary strings are gone from every tier.
- Grep test: no raw log line is ever persisted.

## 9. UI/UX and visual design brief

Design is **not the top priority, but the result must look polished and feel instant.** You are expected to hand-write beautiful, modern **HTML, CSS and JavaScript** (compiled into the embedded bundle).

- **Process:** in P1 first deliver a **static prototype in plain HTML/CSS/JS with realistic mock data** for: Login, **Live** (per-user speed), Dashboard, Nodes, User detail, IP-intelligence map, Logs, **User activity timeline**. Get it reviewed, then implement it in Preact. Keep one design-token file (CSS variables) as the single source of truth.
- **Look and feel:** dark-first with a refined light theme; calm, high-contrast, distinctive (not a generic admin template); one strong accent colour plus semantic status colours (ok/warn/critical/quarantine); subtle depth (soft glass or gradient accents are fine if legibility stays perfect); consistent 4/8-px spacing, rounded cards, restrained shadows; tabular numerals and a monospace face for numbers/IPs; self-hosted variable fonts with a solid system-font fallback.
- **Live page:** animated speed bars/sparklines at 60 fps via `requestAnimationFrame`, smooth number transitions, colour-coded saturation, instant sorting without layout jank; virtualised tables for 10 000+ rows.
- **Navigation/UX:** left nav + top bar, **command palette (Ctrl/Cmd+K)** with global search (users/nodes/IPs), keyboard shortcuts, toasts, skeleton loaders, meaningful empty states, inline help on risky settings, destructive-action confirmations, diff views for rule changes.
- **Responsive:** fully usable on a phone (the operator will check alerts there); touch targets ≥ 44 px.
- **Accessibility:** WCAG AA contrast, focus rings, ARIA labels for icon buttons, text labels in addition to colour for statuses, `prefers-reduced-motion` and `prefers-color-scheme` respected.
- **Constraints:** no external CDNs, fonts or tile servers (strict CSP); initial JS ≤ 250 KB gzipped, lazy-load heavy pages (map, charts); icons as an inline SVG sprite; attribution footer for data sources (DB-IP etc.); i18n RU/EN.

## 10. Data and storage

SQLite (WAL) for configuration/state/rollups; in-memory structures for hot paths. Tables (refine in the design doc): `nodes`, `hosts`, `host_node_mappings`, `health_state`, `health_checks`, `quarantine_events`, `tiers`, `policies`, `policy_conditions`, `users_cache`, `pins`, `injections(+versions)`, `quotas`, `usage_snapshots`, `geo_rules`, `asn_overrides`, `ipintel_sources`, `sub_requests` (retention-limited), `sub_request_aggregates`, `conn_origin_aggregates`, `scanner_events`, `bans`, `block_check_results`, `baselines`, `alerts`, `agents`, `agent_tokens`, `probes`, `speed_rollup_10s|1m|1h`, `log_signal_aggregates`, `log_samples` (bounded, optional), `activity_segments` (index of on-disk columnar segment files — the data itself lives in segment files, not SQLite), `activity_dictionaries`, `activity_retention_state`, `settings(+versions)`, `admins`, `api_keys`, `audit_log`. Hot-path indexes (`shortUuid`, time ranges), partial indexes for open states, **no casts that defeat indexes**, batched upserts (never per-row loops), time-partitioned or rolling tables for high-volume series, scheduled retention jobs.

## 11. Performance budgets (verified by CI benchmarks; change only via ADR)

- Gateway: cache-hit p99 ≤ 5 ms in-process; fresh transform p99 ≤ 25 ms excluding upstream; ≥ 10 000 req/s cache hits on 2 vCPU; panel change visible ≤ 1 s after webhook.
- IP-intel lookup ≤ 5 µs p99; hot-swap with zero failed lookups.
- Agent @ 1 s sampling with 5 000 online users: ≤ 1.5 % of one core, ≤ 40 MB RSS; log pipeline ≥ 100 000 lines/s per core at `debug` within a fixed memory cap.
- Core: ≥ 200 000 user-samples/s ingestion; UI diff stream ≤ 100 KB/s per viewer; ≤ 16 ms/frame rendering 10 000-row virtualised table; ≤ 512 MB RSS at 20 nodes × 5 000 online users (raw rings capped).
- Activity journal (F20): ≥ 100 000 events/s ingest on 4 vCPU, ≤ 8 B/event in the hot tier, ≥ 30× reduction vs raw text, user-day query p95 ≤ 300 ms, compaction ≤ 25 % CPU, bounded disk (quota enforced).
- `make bench` with reproducible synthetic generators, checked-in pprof profiles, regression thresholds, published results table.

## 12. Security and privacy

- Secrets encrypted at rest, never logged or re-displayed; TLS everywhere; optional mTLS for agents; never log a full `shortUuid` (mask to first 4 + last 2).
- **Trusted proxies only** for client-IP headers; SSRF protection on every operator-supplied URL (deny private ranges unless allowed) including data-source downloaders and webhooks.
- No privilege escalation between admin roles; restore/import/upload superadmin-only; path-traversal-safe files; WebSocket origin checks; CSRF protection.
- Admin console not exposed on the public subscription hostname unless configured; IP allowlist option.
- Least-privilege containers (non-root, read-only FS, dropped caps); **no Docker socket for Core or Agents**; dependency pinning, vulnerability scan, SBOM, signed releases.
- **Sensitive-data handling:** the activity journal (F20) is the most sensitive dataset the product holds — encrypted at rest, separate `activity:read`/`activity:export` permissions, audited reads and exports, optional TOTP re-auth, purge/erasure across all tiers, retention enforcement. Document data flows and every retention setting in `docs/data-handling.md` so the operator can align them with the user agreement.
- `docs/threat-model.md`: compromised node, stolen agent token, malicious injection, scanner floods, panel-token leak, malicious webhook, hostile `remnawave-admin` response, poisoned IP-intel/ASN dataset, log-injection (hostile client crafting log lines), insider abuse or theft of the activity journal.

## 13. Reliability

Graceful shutdown, bounded queues, request timeouts, circuit breaker toward the panel with stale-cache fallback, idempotent migrations with a re-run guard, automatic pre-migration backup, safe restore, systemd/Docker auto-restart; chaos tests (DB locked, panel down, Core killed, agent flapping, clock skew, data-source download failure, log flood, log rotation storms).

## 14. Testing and quality gates

- Unit tests per transformer stage with real fixtures; property tests for round-trip fidelity; tolerant-decoding tests (nulls, unknown enums); **fuzzing of every parser** (subscription formats, Xray configs, log lines, IP-intel feeds).
- Panel contract tests (record/replay) + **OpenAPI drift CI job**; webhook-signature tests.
- **Real xray-core in CI** for the protocol corpus, stats and log corpora (section 6, F16, F19).
- Health engine with fake clock/nodes; F16 tests (resets, restarts, skew, gaps, adaptive sampling, accuracy vs `iperf3`); IP-intel golden corpus and hot-swap test.
- **Activity store (F20):** golden-corpus compression ratio, crash-recovery and WAL replay, disk-quota eviction, purge/erasure across tiers and dictionaries, query-latency benchmarks, grep test proving no raw log text is persisted.
- E2E in Docker Compose: fake panel + Core + agents + reverse-proxy fallback.
- CI: `go vet`, `staticcheck`, `golangci-lint`, `-race`, frontend lint/typecheck, licence check, coverage ≥ 70 % on core packages, benchmark regression gates.

## 15. Deployment (Linux)

- `install.sh` (version-pinned, checksum + signature verified, idempotent): installs Docker or the static binary + systemd unit, generates secrets, prints the URL and a one-time bootstrap token; the **browser wizard** does the rest. Multi-arch images (amd64/arm64).
- Reverse-proxy examples (nginx/Caddy) **with fail-open fallback**; standalone mode with built-in ACME; all ports configurable.
- Docs: quick start, upgrade/rollback, backup/restore, runbooks per alert, troubleshooting, panel-version compatibility matrix, data-source licences.

## 16. Engineering lessons to apply (from public release notes of sibling projects)

Null-safe decoding of optional/boolean fields; panel enums are UPPERCASE on write; surface the panel's real error `message` instead of generic 500s; cardinality-safe metrics (route templates, never raw URLs/UUIDs as labels); trusted-proxy handling; privilege-escalation checks; path-traversal-safe and single-transaction restore; idempotent migrations with a guard; partial indexes and no index-defeating casts; batch upserts instead of N inserts; refuse malformed SQL fragments at build time; reverse-proxy docs that distinguish standard vs split modes and warn about 502 pitfalls; no Docker socket for agents.

## 17. Non-goals

Billing, sales bot, replacing the Remnawave panel UI, re-implementing `remnawave-admin` detectors or Xray editor, Windows/macOS support, inspecting user traffic content (payloads, URLs, page bodies — only destination host:port metadata is journaled), multi-tenant SaaS in v1.

## 18. Reference projects (study; do not copy code)

**Notation:** every name written as `owner/repo` anywhere in this document (sections 4, 6, 18) is a **GitHub repository**. Pull it from `https://github.com/<owner>/<repo>` (shallow clone or raw files). For the panel API contract use the `remnawave/backend` release tag that matches the operator's panel version. Pin the exact commit/tag you studied in the relevant ADR, and respect each repository's licence.

Repositories (study; do not copy code):
- https://github.com/DigneZzZ/remnawave-scripts — panel/node lifecycle scripts, backups, Selfsteal, WARP/Tor
- https://github.com/legiz-ru/my-remnawave — subscription-page configs, client templates, response rules
- https://github.com/dotX12/traffic-guard — reference only; firewall features excluded
- https://github.com/jestivald/node-accelerator — reference only; protection features excluded
- https://github.com/Case211/node-diagnostic — node diagnostics with JSON output and reachability checks
- https://github.com/saveksme/multitest — wrapper around region/DPI/IP-quality tools
- https://github.com/oximeter-cloud/xray-monitor — central collector + edge agents pattern
- https://github.com/DonMatteoVPN/Reshala-Remnawave-Bedolaga — fleet-management ideas only
- https://github.com/Case211/remnawave-admin — integration target (**AGPL-3.0 from 2.16.0**; integrate over HTTP only, copy nothing)
- https://github.com/remnawave/panel and https://github.com/remnawave/backend — API contract source (AGPL-3.0); docs at https://docs.rw
- https://github.com/remnawave/python-sdk and https://github.com/Jolymmiles/remnawave-api-go — community SDKs, reference for endpoint coverage and webhook signatures only
- https://github.com/ipverse/as-metadata — AS category/metadata dataset (IP intelligence, section 7)

**Data sources** (verify terms and URLs in P0): IPtoASN, RIPE RIS/RouteViews, RIR delegated-extended files, DB-IP Lite, IPinfo Lite, MaxMind GeoLite2, Stanford ASdb, BGP.tools, PeeringDB. Check every licence before reuse; default to clean re-implementation.

## 19. Delivery phases (stop at each gate and report)

- **P0 – Discovery, benchmarks, design:** panel OpenAPI + webhook contract; Xray stats path on a real node; **capture golden Xray log corpus (all levels, several versions)** and protocol/transport corpus; verify IP-intel sources, formats, licences and build a prototype merged MMDB; benchmark `QueryStats` at 1 s with 1k/5k/10k users and log parsing at 100k lines/s; **benchmark the activity-journal storage engine (pure-Go Parquet vs custom segments) on the golden corpus and verify what the access log really contains per protocol (domains, emails, sniffing)**; produce `docs/design.md`, `docs/api-mapping.md`, `docs/xray-stats.md`, `docs/xray-logs.md`, `docs/protocol-matrix.md`, `docs/ipintel.md`, `ASSUMPTIONS.md`, ADRs. Ask blocking questions **here**.
- **P1 – Gateway MVP + design prototype:** F1, panel client (ogen), webhooks, node mapping, auth + minimal console, wizard skeleton, fail-open + fallback configs, **IP-intel builder/lookup (section 7)**, the static HTML/CSS/JS prototype (section 9).
- **P2 – Live core:** agent enrollment, **F16 end-to-end**, built-in TSDB, Live page, **F19 log pipeline** with connection origins, **F20 capture, coalescing, hot-tier segment store and basic timeline API**.
- **P3 – Fleet logic:** F2, F3, F4, F14, universal protocol corpus tests in CI.
- **P4 – Monitoring & investigation:** F5, F6, F7, F8, F10, F11, F12, F17, F18, **F20 tiered compaction, disk budget, full Activity UI, exports, purge**, `remnawave-admin` connector, mihomo + sing-box adapters, F9 + probe.
- **P5 – Hardening & release:** load/chaos tests, threat model, updater, signed releases, docs, benchmark report.

## 20. Working method (rules for you)

1. Begin each phase with a short plan and file list; end with what was verified and what remains.
2. Never invent panel endpoints, Xray config keys, log formats, dataset URLs or licence terms — verify against specs, real runs and publisher pages, and cite the source in an ADR or comment. If the panel contract, the Xray stats/log path or a data-source licence cannot be verified, **stop and ask**.
3. Missing information ⇒ safest default, logged in `ASSUMPTIONS.md`.
4. Small, reviewable commits; benchmarks and tests land with the feature.
5. No features beyond this spec — propose them in `docs/ideas.md`.
6. State limitations honestly in docs/UI (quota enforcement at refresh time, city-level geolocation is approximate, coarse-resolution fallbacks, `unverifiable` probes).

## 21. Deliverables

Monorepo `core/`, `agent/`, `web/` (prototype + app), `deploy/`, `docs/`; generated panel client + generator config; migrations; OpenAPI for the admin API; Docker images + compose; installer scripts; reverse-proxy fallback snippets; golden corpora (configs, logs, IPs) with generators; benchmark suite and report; tests and CI; `README.md` (RU + EN), `CHANGELOG.md`, `ASSUMPTIONS.md`, ADRs, `docs/threat-model.md`, `docs/data-handling.md`, runbooks, release/signing guide.

**Begin with P0. Write no feature code until the design, API mapping, Xray stats/log verification and IP-intel source verification are delivered and all blocking questions are answered.**
