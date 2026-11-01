# Feature inventory: what every project does, and how SubGate merges it

Sources read (README level, October 2026): `Case211/remnawave-admin` (v5.1.1), `Mrvibecodic/remnawave-subscription-middleware`, `DigneZzZ/remnawave-scripts`, `legiz-ru/my-remnawave`, `dotX12/traffic-guard` (+ `DonMatteoVPN/TrafficGuard-auto`, see note), `jestivald/node-accelerator`, `Case211/node-diagnostic`, `DonMatteoVPN/Reshala-Remnawave-Bedolaga`, `saveksme/multitest` (+ `censorcheck`), `oximeter-cloud/xray-monitor`, `remnawave/panel`.

Notes on reliability of this inventory:
- `TrafficGuard-auto` returns 404 when fetched; only a search snippet was available.
- `remnawave/panel`'s README only points to docs.rw (a JavaScript site that could not be read), so panel capabilities were taken from third-party listings (MCP servers, Terraform provider, backend commits). Verify against the panel's OpenAPI in P0.
- `remnawave-admin` moved fast: the release page I read earlier was stale (2.18.x); the current README describes v5.1.x. Many ideas I proposed earlier already exist there.

---

## Part A. Per-project functions

### 1. Case211/remnawave-admin (web + Telegram bot + agent; Python/FastAPI, React, PostgreSQL; AGPL-3.0 since 2.16)
- **Bot:** users/nodes/hosts management, bulk ops, templates/snippets/tokens, billing nodes, health/stats/bandwidth commands, roles, topic-routed notifications.
- **Web panel:** dashboard, users/nodes/hosts + bulk, violations view with IP lookup and inline actions, geo map with per-city detail, analytics (top users, trends, per-node traffic, P2P/torrent), blocked-IP management, audit log, API keys, 6 themes, RU/EN.
- **Automation hub:** rule builder, schedules, templates, execution logs, alert engine, traffic monitor.
- **Anti-abuse:** 7 analyzers (temporal, geographic with impossible travel, ASN, behaviour profile, devices, HWID, User-Agent), scoring → actions (warn / soft-block / temp-block / hard-block / speed cut), dedupe window + escalation, delayed measures with re-check, CGNAT buffer, 60+ Russian agglomerations, speed-consumption monitoring, P2P/torrent detection, customer warnings with templates, node-side eBPF speed shaper.
- **Node agent:** collects Xray log data, WebSocket command channel (remote terminal, script catalogue), install one-liner.
- **Monitoring:** Prometheus `/metrics` (30+ gauges), 5 Grafana dashboards, sync watchdog, fleet monitoring including non-Remnawave servers.
- **Xray editor:** visual inbound/outbound/routing/DNS/reverse editor, balancers + observatory UI, WARP generator, protocol presets, link import/generator, geo viewer, topology graph, diagnostics.
- **Backups:** create/restore/upload/send to Telegram/rotation/safe restore.
- **Built-in mail server:** MX delivery, DKIM/SPF/DMARC, inbound SMTP, spam score, bounce stop-list, DMARC report parsing, queue.
- **Support desk:** tickets with SLA, templates, attachments, customer card, quiet-hours autoreply, external support-events API.
- **Bedolaga integration:** CRM dashboard, clients, subscriptions, referrals, promo codes, marketing analytics, client cabinet.
- **Mobile:** Android app (widget, push, biometrics, multiple panel profiles).
- **Platform:** RBAC without privilege escalation, TOTP, Telegram login, IP allowlist, secret path, plugin manager + plugin API, realtime settings, API v2/v3 + HMAC webhooks, MaxMind GeoLite2 (or ip-api fallback).

### 2. Mrvibecodic/remnawave-subscription-middleware (PHP; beta; closest relative of SubGate)
- RU-mirror proxy for subscriptions (+ "main subscription" mode serving `/api/sub` and drawing its own page).
- Grace squad for expired users (move to limited squad, restore on payment, snapshot table).
- HWID device block with placeholder config; HWID from User-Agent; `CUSTOM_SUB_PREFIX` support.
- Request log (who/when/client/decision/size/device model+OS) and **client-version catalogue** (GitHub/App Store/Google Play/F-Droid sources; flags outdated clients).
- Per-app response rules (headers by UA/OS).
- **Extra configs by squad:** WireGuard / AmneziaWG / VLESS (raw, ws, httpupgrade, grpc, h2, xhttp, mKCP, QUIC, Reality, Vision) with a **per-client-core compatibility matrix**; nothing injected for blocked/expired/over-limit users; built on the panel's subscription template.
- WG/AWG config pool (one key per subscriber/device). Merge of two subscriptions into one link.
- Webhook "tee" (fan-out with re-signing). Masking login page. Support chat widget on the stub page (Telegram + webhook).
- **Encrypted subscription channel** (own clients only): rotating path label, key derived from the sub URL, fixed-size ciphertext responses.
- System tab with request graph (real updates vs scanners) and anomaly-spike detector. DB migrate SQLite↔MySQL, purge, vacuum. Self-update from GitHub with rollback.
- Minimal API-token scope tables (and a paste-JSON scope list); panel 2.x/3.x id handling; shortUuid-length scanner filter.

### 3. DigneZzZ/remnawave-scripts (Bash; MIT)
Panel/node/Selfsteal/WARP-Tor/NetBird installers; SHORT_UUID method picker; Caddy with MFA portal; standalone subscription-page; safe update with snapshot and migrations; PG upgrade helper; Telegram backups with retention; node: Xray-core update incl. pre-release, live Xray logs, log rotation, auto-restart, mass provisioning, version pinning; Selfsteal (11 masking templates, anti-fingerprint); WTM (WARP as WireGuard outbound, Tor); jsDelivr mirror fallback; self-updating scripts. Warning found: node ≥ 2.8 ignores the legacy `XTLS_API_PORT`.

### 4. legiz-ru/my-remnawave (configs)
Per-platform client catalogues (multiapp / hwid), custom subscription-page templates, subscription templates for mihomo/stash/sing-box/xray-json, Happ/INCY routing presets, XHTTP-behind-nginx recipe (unix socket, `xmux`, `downloadSettings`), Prizrak-Box response rules with `pxa/pxd-template` headers and placeholders.

### 5. dotX12/traffic-guard (Go; MIT) + TrafficGuard-auto
iptables/ipset blocking of scanner and government-network lists (multiple URLs, `file://`), persistence, UFW integration, optional aggregated CSV log with whois ASN/NETNAME, per-IP log rate limit, clean `uninstall`. TrafficGuard-auto: one-command installer with a log/statistics viewer (lists: government_networks, antiscanner, skipa).

### 6. jestivald/node-accelerator (Bash; MIT) — the best node-side spec
- **Optimizer:** XanMod BBRv3, tier-aware sysctl, RPS/RFS/XPS, zram, MSS clamp, FD limits, NIC tuning, hourly log-rotation timer.
- **Protect:** own nftables table (never flushes ruleset), strict/open/skip modes, AntiScan with ban-once, flag-drop, anti-spoof, per-IP SYN/UDP/connlimit, UDP bulk ports for Hysteria2/TUIC, node-port auto-detect + panel auto-whitelist, conntrack phantom-eviction, blocklists (Spamhaus DROP, FireHOL L1, Tor), **fleet auto-sync of whitelists**, CrowdSec, SSH anti-lockout, safety timer, dry-run, rollback.
- **Diagnose:** kernel/BBR/conntrack/NIC/PSI/UDP errors, remnanode container state, node-port vs firewall desync, TLS cert expiry, whitelist consistency, retransmit-cause analysis, disk/log sensors, JSON output.
- **Forensics (`na-report`):** drops by reason, timeline, top IPs with verdict, top ASN, per-IP deep verdict.
- Signed releases (minisign) and pinned tags.

### 7. Case211/node-diagnostic (Bash + eBPF; MIT)
27 diagnostic checks (system, network, throughput, **service reachability for 19 services**, **IP reputation**, Xray/remnanode, open-ports audit), `optimize` with namespaced drop-ins and rollback, generate-only `protect` with auto-revert timer, `bbr3`, **eBPF per-client-IP shaper** (EDT) with whitelist and dynamic penalty, installers (node, Selfsteal, NetBird, monitoring).

### 8. DonMatteoVPN/Reshala-Remnawave-Bedolaga (Bash TUI; ideas)
Skynet fleet manager (mass commands, auto-capture), per-user traffic shaper, dashboard with Speedtest and "how many users can this node carry", UFW/Fail2Ban/sysctl security wizard, Docker deep clean, Remnawave lifecycle. Roadmap: Telegram control, alerting, web panel, auto-response.

### 9. saveksme/multitest (+ censorcheck)
Launcher for ipregion, censorcheck (DNS/IP/HTTP(S)/DPI/regional layers; ~90 % accuracy; regional caveat; needs TLS listener on 443), iperf3 to Russian servers, YABS, IP.Check.Place, IPQuality, bench.sh, sysbench; BBR+Cake and IPv6-off utilities.

### 10. oximeter-cloud/xray-monitor (TypeScript; MIT)
Edge agents → ingest key → central engine; **multi-hop route resolution** across relays; subtractive direct/tunneled traffic accounting; destination categories (Google/YouTube, Meta, Telegram, CDN…); per-user destination drill-down, live stream, timeline; ~85 MB RAM for 140 users.

### 11. remnawave/panel (NestJS; AGPL-3.0)
Users, nodes, hosts, internal/external squads, config profiles with tags, snippets, subscription templates/settings/response rules/page configs, request history, HWID limits, API tokens with scopes, passkeys/OAuth2, infra billing, bandwidth stats, **node plugins (torrent blocker) and shared ASN/IP lists (3.3+)**, node integrations (3.3+).

---

## Part B. The merged catalogue (union) → SubGate

Legend: **v4** = already in prompt v4; **NEW** = added in prompt v5; **INT** = consumed through the panel's own feature instead of duplicated.

| # | Function | Sources | SubGate ID |
|---|---|---|---|
| 1 | Subscription proxy, fail-open, caching | middleware, my-remnawave | F1 (v4) |
| 2 | RU-mirror mode (client → mirror → origin), standalone vs panel-side | middleware | F1a **NEW** |
| 3 | Balancing, tiers, pinning, quarantine, health | XBM, (none of the repos) | F2–F4 (v4) |
| 4 | Injections incl. WG/AWG/VLESS extra configs with core-compat matrix and status gating | middleware | F5 extended **NEW** |
| 5 | WG/AWG config pool per subscriber/device | middleware | F5b **NEW** |
| 6 | Merge of two subscriptions | middleware | F5c **NEW** |
| 7 | Grace squad / soft expiry with snapshot-restore | middleware | F29 **NEW** |
| 8 | HWID block stub, HWID from User-Agent, device cards | middleware, admin, panel | F30 **NEW** |
| 9 | Request log + client-version catalogue + outdated flag | middleware, panel (history API) | F30 **NEW** |
| 10 | Per-app response rules & preset library (Prizrak-Box, Happ routing, client catalogues, subpage templates) | middleware, my-remnawave | F31 **NEW** |
| 11 | Webhook fan-out with re-signing | middleware | F14 extended **NEW** |
| 12 | Masking/decoy page + support chat widget | middleware | F7/F12 + F33 **NEW** |
| 13 | Encrypted subscription channel | middleware | optional, own-client only (listed in ideas) |
| 14 | Per-node quotas, geo-block, subscriber map, 16–20, drops, YouTube region, anti-scan, hot settings, console | XBM | F6–F13 (v4) |
| 15 | Real-time per-user speed (per second) | (new) | F16 (v4) |
| 16 | Activity journal with compact storage + destination categories | xray-monitor, admin | F20 (v4) + categories **NEW** |
| 17 | Multi-hop relay topology and subtractive traffic accounting | xray-monitor | F32 **NEW** |
| 18 | Abuse engine: 7 analyzers, scoring, escalation ladder, delayed measures, CGNAT buffer, P2P/torrent | admin, panel (torrent blocker) | F21 **NEW** |
| 19 | Rules/automation engine (builder, schedules, templates, logs) | admin, Reshala roadmap | F22 **NEW** |
| 20 | Node protection (nftables, ban-once, per-IP limits, UDP bulk, ctguard, blocklists, fleet whitelist sync, safety timer) | node-accelerator, traffic-guard(+auto), node-diagnostic | F23 **NEW** |
| 21 | Threat lists (scanner, government, skipa, Spamhaus DROP, FireHOL, Tor) as IP-intel tags | traffic-guard, node-accelerator | §7 L5 **NEW** |
| 22 | Node optimizer/advisor with dry-run + rollback (BBR/cake, RPS/RFS/XPS, NIC, limits, MSS, zram, journald, XanMod) | node-accelerator, node-diagnostic, multitest | F24 **NEW** |
| 23 | Diagnostic catalogue: 27 checks, 19-service reachability, IP reputation, open-port audit, cert expiry, retransmit analysis | node-diagnostic, node-accelerator, multitest | F24 **NEW** |
| 24 | Blocking radar: DNS/IP/HTTP(S)/DPI/regional, iperf3 to RU servers, benchmarks | multitest/censorcheck | F9 extended + F24 **NEW** |
| 25 | Per-client-IP eBPF shaper with dynamic penalty | node-diagnostic, Reshala, admin | F25 **NEW** (optional) |
| 26 | Fleet ops: remote terminal, script catalogue, mass commands, auto-capture, Docker hygiene, node/Xray-core lifecycle, log rotation, auto-restart, WARP/Tor outbound | admin, Reshala, remnawave-scripts | F26 **NEW** (optional) |
| 27 | Xray config toolkit: linter, generators (XHTTP via nginx, Selfsteal/Reality dest, WARP), balancer/observatory editor, snippets, import/export of links | admin, my-remnawave, remnawave-scripts | F27 **NEW** |
| 28 | Backups of SubGate **and the panel**, scheduled, Telegram, safe restore with version check, pre-update snapshots | scripts, admin, middleware | F18 extended **NEW** |
| 29 | Prometheus `/metrics` + Grafana dashboards + sync watchdog (optional exports) | admin | F35 **NEW** |
| 30 | Support desk (tickets, SLA, templates), customer warnings (TG/email), outbound mail | admin, middleware | F33 **NEW** (optional) |
| 31 | Built-in full mail server (MX, DKIM/SPF/DMARC, inbound) | admin | F33b optional, **not recommended for v1** |
| 32 | Billing/CRM (Bedolaga), referrals, promo, payments | admin | integration only (non-goal to rebuild) |
| 33 | Android app / push | admin | PWA + Web Push instead **NEW** |
| 34 | Installers/lifecycle for panel & node (one-liner, idempotent, mirror fallback, pinned versions, signed releases) | scripts, node-accelerator | F15/installer + F26 **NEW** |
| 35 | Plugin system | admin | F36 **NEW** (sandboxed, later) |
| 36 | Panel-native features to consume, not duplicate: HWID limits, torrent-blocker reports, request history, bandwidth stats, shared ASN/IP lists, billing nodes | panel | INT **NEW** |

### Overlap decisions
- Where the panel already provides a capability (HWID limit, torrent blocker, request history), SubGate **reads and visualises** it, and **pushes** its own curated data (e.g. ASN lists) into the panel's shared lists where an API exists.
- Where `remnawave-admin` already provides a capability, SubGate re-implements it **from public descriptions only (clean-room; AGPL-3.0)**, so the operator can run SubGate alone. The optional connector remains for coexistence.
- Two modules are deliberately risky and flagged optional: the **full mail server** and the **eBPF shaper**.
