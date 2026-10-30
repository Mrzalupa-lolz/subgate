# Телеметрия, метрики и диагностика
О чём: live speed F16, TSDB, drops F10, block F9, YouTube F11 и F24 checks.
Когда читать: stats accuracy, sampling/rollups, diagnostics или exporter.

## Live speed
Код: pkg/xray/stats.go, agent/internal/agent/agent.go, core/internal/tsdb/, server/live.go, server/telemetry.go.
gRPC QueryStats с reset=false всегда; external counter reset даёт new value delta, restart warm-up исключает spikes.
Скорость delta bytes*8/monotonic interval; user label из Xray связывается с panel identity без двусмысленных aliases.
При отсутствии user counters используется inbound scope; payload bytes не равны wire traffic, устройства одного user суммируются.
TestRealGRPCStatsContractDeltaAndCounterReset PASS использует настоящий gRPC transport с fake service,
TestLiveStatsService SKIP — неизменность реального panel accounting этим не доказана.
LiveStore/SSE snapshots1Hz, EMA3/10, peak/p95, recent average, history10s buckets и CSV существуют.
Average основан на12 samples, не полной минуте; window1..60s, freshness/gap/warming честно видны.
SQLite speed очищается после24h; запрошенный31-day range не означает сохранённые31 days.
Rollups сейчас вычисляются на read, durable10s24h/1m30d/1h1y tiers отсутствуют.
Нет adaptive1→2→5s при стоимости query>25%, watched mode/coarse panel fallback, sparse diff stream, hard byte cap64MiB.
Стек HTTP JSON/SSE сохраняется по D07; переход на protobuf/zstd/WebSocket возможен только после измерений.

## Acceptance и performance
Нужны iperf3 real node accuracy ±5% при1s, неизменный panel accounting, skew buckets±500ms,
20 nodes×5000 users на4vCPU без скрытых gaps; нельзя считать fake gRPC достаточным доказательством.
Agent RSS/CPU/disk/NIC/conntrack/TCP retransmit collection ещё нет; heap Alloc не RSS.
Burst/user-speed/saturation>85% transitions, recovery и NIC capacity в UI отсутствуют.

## Drops, блокировки, YouTube
Drop baselines table/utility не runtime: нет hour-of-week median+MAD/EWMA learning, T-minute suppression/recovery и maintenance context.
Block detector TCP/TLS читает socket с Core, target ограничен synced node/port, arbitrary SSRF не разрешён.
EOF до64KiB может выглядеть ok; не доказан реальный DPI stall14..24KiB.
Требуются in-region vantage×node×inbound matrix, >=64KiB transfer, bytes-at-stall/reset, suspect/blocked verdict,
SOCKS5/HTTP proxy или dedicated Probe, optional real Xray client/user и alerts; quarantine/demote только opt-in.
YouTube check тоже идёт с Core, node_id — label, не egress. HTTPS443 allowlist + redirect validation тестированы.
Нужны per-node Agent schedule, locale/consent handling, forbidden-region default RU, stop/reset/cooldown и transition alerts.

## Metrics и F24
Public /metrics text exporter с HTTP requests/errors/duration подключён; custom fleet30+ gauges/watchdog/Grafana dashboards ещё нет.
Metrics optional export, Grafana/Prometheus не обязательные runtime dependencies.
Каталог F24 должен описать27 read-only checks и19 service reachability из public descriptions;
точные названия/пороговые значения сначала сверить, не придумывать неописанные checks.
Из явно требуемого: kernel/BBR/conntrack/NIC/PSI/UDP errors, process/container state без Docker socket,
node ports/firewall desync, cert expiry, whitelist consistency, retransmit causes, disk/log sensors,
IP reputation/open ports, DNS/IP/HTTP(S)/DPI/regional layers, iperf3 RU/YABS/sysbench reports.
Каждый check отдельно: JSON output, timeout, missing-prerequisite diagnostic, fake-host test; live host change не требуется.
Advisor/optimizer flags/dry-run/rollback описаны в [nodes-fleet](nodes-fleet.md), на этой стадии отсутствуют.
