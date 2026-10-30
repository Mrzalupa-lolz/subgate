# P0 Discovery Scripts

## Purpose

Gather real data from Remnawave + Xray deployments to verify assumptions and populate golden corpora.

## Scripts

### 1. `check-xray-stats.sh` – Verify Xray gRPC StatsService

Tests whether Xray exposes StatsService and captures the format of user counters.

```bash
./check-xray-stats.sh <node-ip> [port]
```

**Validates:**
- gRPC endpoint reachability
- Per-user counter format: `user>>>{email}>>>traffic>>>uplink|downlink`
- Inbound-level fallback: `inbound>>>tag>>>traffic>>>uplink|downlink`
- Connection state API

### 2. `capture-xray-logs.sh` – Golden Log Corpus

Captures Xray logs at all levels (debug, info, warning, error) and access log from a real running node.

```bash
./capture-xray-logs.sh <node-ip> <duration_seconds> <output_dir>
```

**Outputs:**
- `access.log` – access log with all fields
- `error-debug.log` – error + debug level
- `xray-version.txt` – Xray version reported
- `corpus-summary.json` – line counts per level, field presence indicators

### 3. `capture-xray-config.sh` – Protocol & Transport Samples

Dumps the running Xray config and documents protocol/transport combinations in use.

```bash
./capture-xray-config.sh <node-ip> <output_file>
```

**Validates:**
- Protocol support (VLESS, VMess, Trojan, Shadowsocks, Hysteria, WireGuard, etc.)
- Transport support (TCP, WebSocket, gRPC, HTTP/2, XHTTP, QUIC, mKCP, etc.)
- Security modes (TLS, REALITY, XTLS, none)
- Access log configuration (email field present?)
- Inbound sniffing enabled?

### 4. `fetch-panel-openapi.sh` – Panel API Contract

Retrieves the OpenAPI spec from the Remnawave panel.

```bash
./fetch-panel-openapi.sh <panel-url> <api-token> [output_file]
```

**Validates:**
- Reachable endpoints (which require API token scope)
- Webhook signature scheme
- Subscription endpoints format
- User, node, host, quota, bandwidth-stats structure

### 5. `check-ip-sources.sh` – IP Intelligence Availability

Verifies data source availability and downloads sample data.

```bash
./check-ip-sources.sh [output_dir]
```

**Checks:**
- IPtoASN PDDL data (URL, format, freshness)
- RIPE NCC delegated-extended file (daily update)
- DB-IP Lite availability (CC BY 4.0 verification)
- MaxMind GeoLite2 account status
- BGP.tools User-Agent compliance

---

## Running P0 Discovery

Configure SSH access to your node using `ssh-agent` or `~/.ssh/config`.
The scripts use your standard SSH settings. Replace the example address
`192.0.2.10` below with your node address:

```bash
# 1. Verify Xray stats path
./check-xray-stats.sh 192.0.2.10

# 2. Capture golden corpus (1 hour)
mkdir -p corpora/xray-logs
./capture-xray-logs.sh 192.0.2.10 3600 corpora/xray-logs

# 3. Document Xray config
./capture-xray-config.sh 192.0.2.10 corpora/xray-config.json

# 4. Check panel API (if accessible)
./fetch-panel-openapi.sh https://panel.example.com <token> corpora/panel-openapi.json

# 5. Verify IP data sources
./check-ip-sources.sh corpora/ip-sources

# 6. Generate corpus summary
ls corpora/ > corpora/MANIFEST.md
```

---

## Expected Outputs

After running all scripts, populate:

- `docs/xray-stats.md` – gRPC address, per-user counter layout
- `docs/xray-logs.md` – per-level format matrix, real examples
- `docs/protocol-matrix.md` – protocols × transports in use, per-user stat capability
- `docs/api-mapping.md` – panel endpoints reachable with token, per-feature scope list
- `docs/ipintel.md` – verified data source URLs, build pipeline
- `test/corpora/` – golden samples for unit tests and benchmarks

---

## Corpus Format

### Xray Logs Corpus

```
corpora/xray-logs/
├── access.log                    # Raw access log (sample 1h)
├── error-debug.log               # Error + debug combined
├── error-info.log
├── corpus-summary.json           # {
                                  #   "lines_total": 50000,
                                  #   "lines_per_level": {"debug": 20000, ...},
                                  #   "access_log_fields_present": ["email", "user", ...],
                                  #   "protocols": ["vless", "vmess", ...],
                                  #   "transports": ["ws", "grpc", ...],
                                  #   "xray_version": "1.8.x"
                                  # }
└── per-protocol-samples/         # grouped by detected protocol
    ├── vless.log
    ├── vmess.log
    └── ...
```

### Xray Config Corpus

```
corpora/
├── xray-config.json              # Full Xray config (anonymized IPs/emails)
├── protocol-matrix.md            # Table: protocol × transport × security
└── per-protocol-examples/
    ├── vless-reality-grpc.json
    ├── vmess-ws.json
    └── ...
```

### IP Intelligence Corpus

```
corpora/ip-sources/
├── iptoas.sample.txt             # Sample IPtoASN records
├── ripe-delegated.sample.txt     # Sample RIR delegation
├── db-ip-lite.sample.mmdb        # (if available)
├── sources-metadata.json         # {
                                  #   "iptoas": {"url": "...", "license": "PDDL", "freshness": "daily"},
                                  #   ...
                                  # }
└── build-log.txt                 # MMDB build process & sizes
```

---

## P0 Blocking Questions (To Answer During Discovery)

- [ ] What is the exact Xray gRPC endpoint and port?
- [ ] Are per-user stats available for all protocols or only some?
- [ ] What fields are in the access log for each protocol?
- [ ] Is the panel API accessible? What endpoints are reachable with an API token?
- [ ] Which IP data sources are available / accessible?
- [ ] What is the event volume estimate (events/sec at peak)?
- [ ] Parquet vs custom – which is faster for queries?

