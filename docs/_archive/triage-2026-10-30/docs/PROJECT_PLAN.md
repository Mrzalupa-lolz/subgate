# Единый план SubGate

Порядок учитывает зависимости; один пункт — один узкий проверяемый результат на15–40 минут.
Если fixture/implementation не помещается, разделить этот пункт до начала, не объявлять весь workflow DONE.
[x] — только указанный узкий scope с наблюдённым PASS; это не готовность всей inventory feature.
Новые имена Test* ниже — требуемые будущие assertions, не evidence. go test exit0 без точного pass/с skipped children не принимается.
Статусы36 Part B rows: [FEATURE_STATUS](FEATURE_STATUS.md); правила/источники: [DECISIONS](DECISIONS.md).
Начать1.4: текущий B04. После каждого пункта пересмотреть только затронутые evidence/IDs и короткий STATUS.
Excluded: Part A/email/S3/SubGate self-update/автосанкции F21/connector/CRM rebuild, см. DECISIONS.
Legacy R01–R60/TOKEN_PLAN сохранены в архиве; метки в пунктах обеспечивают traceability без второго плана.

## Этап 1 — Фундамент, health regressions и безопасные границы

Область: [registry/quality.md](registry/quality.md).

- [ ] 1.1 Owner contract — одна password-only учётка, duplicate accounts fail closed — проверено: TestSingleOwnerPasswordLogin и TestOwnerPasswordOnlyConfiguration PASS (R10/R11)
- [ ] 1.2 Panel default read-only — отказ non-GET до сериализации — проверено: TestPanelReadOnlyBeforeSerialization и TestPanelReadOnlyRejectsBeforeSending PASS (R01)
- [ ] 1.3 Docs navigation — единый INDEX,12 доменов, исходники сохранены — проверено: python3 scripts/check-docs.py PASS (TOKEN_PLAN)
- [ ] 1.4 Maintenance other-node fixture: сохранить synchronized node до scheduled probe — критерий готовности: go test -json ./core/internal/server -run ^TestMaintenanceSuppression$: точный pass без skip/fail; B04 (R13/R14)
- [ ] 1.5 Slow maintenance fixture: сохранить node до quorum read — критерий готовности: go test -json ./core/internal/server -run ^TestMaintenanceSuppressesWindowEntirelyInsideSlowProbe$: точный pass без skip/fail; B04 (R13/R14)
- [ ] 1.6 Slow quarantine fixture: проверить синхронизированную node и commit timestamp — критерий готовности: go test -json ./core/internal/server -run ^TestQuarantineMinimumAndBackoffStartAfterSlowProbe$: точный pass без skip/fail; B04 (R13/R14)
- [ ] 1.7 Cancellation/rollback fixture: проверить node persistence и state rollback — критерий готовности: go test -json ./core/internal/server -run ^TestScheduledHealthUnknownCancellationAndAtomicFailure$: точный pass без skip/fail; B04 (R13/R14)
- [ ] 1.8 Incomplete-endpoint fixture: сохранить node и не допустить false quarantine — критерий готовности: go test -json ./core/internal/server -run ^TestScheduledHealthIncompleteEndpointsCannotQuarantine$: точный pass без skip/fail; B04 (R13/R14)
- [ ] 1.9 Закрыть baseline regression gate — критерий готовности: go test -count=1 ./... и make check PASS; закрыть B04 (R59)
- [ ] 1.10 Document security/data flows — короткий threat model для node/token/feed/log injection — критерий готовности: в registry/security и activity перечислены assets/trust boundaries/retention и mitigations, secrets scan diff PASS (R59)

## Этап 2 — Panel contract и webhook freshness

Область: [registry/panel-ui.md](registry/panel-ui.md).

- [ ] 2.1 Read-contract3.4.5 nullable/unknown fields — проверено: TestPanelSpecReadContract PASS (R02–R04/R08)
- [ ] 2.2 Generated users DTO integrity без потери int64 — проверено: TestGeneratedPanelUsersIntegrity PASS (R02–R04/R08)
- [ ] 2.3 Startup version diagnostics mismatch/unavailable — проверено: TestPanelVersionMismatch PASS (R02–R04/R08)
- [ ] 2.4 user.updated синхронно обновляет state/cache — проверено: TestWebhookRefreshesUserBeforeDeadline PASS (R02–R04/R08)
- [ ] 2.5 Generated nodes projection — один GET reader без строгого enum writer — критерий готовности: новый TestGeneratedPanelNodesContract: nullable/unknown/large counters + python3 scripts/generate-panel.py --check PASS (R03)
- [ ] 2.6 Generated hosts projection — один GET reader без строгого enum writer — критерий готовности: новый TestGeneratedPanelHostsContract: nullable/unknown/large counters + python3 scripts/generate-panel.py --check PASS (R03)
- [ ] 2.7 Webhook user.created — native payload/atomic replay projection — критерий готовности: fixture user.created: <1s state/cache freshness, duplicate/stale rollback assertions PASS (R08)
- [ ] 2.8 Webhook user.deleted — native payload/atomic replay projection — критерий готовности: fixture user.deleted: <1s state/cache freshness, duplicate/stale rollback assertions PASS (R08)
- [ ] 2.9 Webhook user.disabled — native payload/atomic replay projection — критерий готовности: fixture user.disabled: <1s state/cache freshness, duplicate/stale rollback assertions PASS (R08)
- [ ] 2.10 Webhook user.enabled — native payload/atomic replay projection — критерий готовности: fixture user.enabled: <1s state/cache freshness, duplicate/stale rollback assertions PASS (R08)
- [ ] 2.11 Webhook node.updated — native payload/atomic replay projection — критерий готовности: fixture node.updated: <1s state/cache freshness, duplicate/stale rollback assertions PASS (R08)
- [ ] 2.12 Webhook host.updated — native payload/atomic replay projection — критерий готовности: fixture host.updated: <1s state/cache freshness, duplicate/stale rollback assertions PASS (R08)
- [ ] 2.13 Webhook fan-out — один configured destination, подпись нового raw body — критерий готовности: fixture delivery/HMAC/retry/idempotency и SSRF deny test PASS (R08/F14)
- [ ] 2.14 Webhook deployment evidence — критерий готовности: test panel3.4.5 synthetic user.updated меняет next subscription<1s без production writes; записать non-skipped assertions (R08)

## Этап 3 — Gateway/cache/fail-open и mirror

Область: [registry/gateway-routing.md](registry/gateway-routing.md).

- [ ] 3.1 GET/HEAD metadata isolation — проверено: TestSubscriptionHEADAndMetadata PASS (R05–R08)
- [ ] 3.2 Transformer panic/deadline300ms bypass — проверено: TestTransformPanicAndDeadlineFailOpen PASS (R05–R08)
- [ ] 3.3 Bounded32 transform workers/context — проверено: TestTransformWorkersBoundedAndContextCancelled PASS (R05–R08)
- [ ] 3.4 SWR generation/CAS freshness — проверено: TestLateSWRRefreshCannotUndoAuthoritativeForeground PASS (R05–R08)
- [ ] 3.5 Runtime policy version cache namespace — критерий готовности: изменение policy version из store меняет cache identity, stale old version не resurrects PASS (R05–R08/F1a)
- [ ] 3.6 Caddy fail-open route — критерий готовности: killed-Core synthetic Caddy GET/HEAD preserves query/metadata без credential forwarding PASS (R05–R08/F1a)
- [ ] 3.7 Nginx fallback fresh evidence — критерий готовности: TestKilledCoreSubscriptionFallback non-skipped PASS на synthetic origin PASS (R05–R08/F1a)
- [ ] 3.8 DB-locked gateway bypass — критерий готовности: fixture DB locked возвращает exact upstream без5xx при живом upstream PASS (R05–R08/F1a)
- [ ] 3.9 Panel outage/expired snapshot matrix — критерий готовности: fresh/stale/expired/error/authoritative4xx matrix с byte/header assertions PASS (R05–R08/F1a)
- [ ] 3.10 Configurable subscription aliases — критерий готовности: custom prefix GET/HEAD и invalid path tests без открытия console/auth PASS (R05–R08/F1a)
- [ ] 3.11 Mirror explicit mode — критерий готовности: client→mirror→origin GET/HEAD/query/metadata fixture без credential leak PASS (R05–R08/F1a)
- [ ] 3.12 Standalone mirror subpage — критерий готовности: fixture standalone page/API routing сохраняет panel templates/response rules PASS (R05–R08/F1a)
- [ ] 3.13 Panel-side mirror mode — критерий готовности: fixture panel-side upstream/path settings плюс fallback на last-good PASS (R05–R08/F1a)
- [ ] 3.14 Real-fixture rules-off diff: xray-json — критерий готовности: очищенный fixture xray-json: raw unknown fields/entries/numbers сохраняются, no-empty assertions PASS (R59)
- [ ] 3.15 Real-fixture rules-off diff: uri-list/base64 — критерий готовности: очищенный fixture uri-list/base64: raw unknown fields/entries/numbers сохраняются, no-empty assertions PASS (R59)
- [ ] 3.16 Real-fixture rules-off diff: mihomo-yaml — критерий готовности: очищенный fixture mihomo-yaml: raw unknown fields/entries/numbers сохраняются, no-empty assertions PASS (R59)
- [ ] 3.17 Real-fixture rules-off diff: singbox-json — критерий готовности: очищенный fixture singbox-json: raw unknown fields/entries/numbers сохраняются, no-empty assertions PASS (R59)

## Этап 4 — Health, transport и manual controls

Область: [registry/health.md](registry/health.md).

- [ ] 4.1 N/M minimum+backoff engine — проверено: TestQuarantineThresholdsMinimumTimeAndBackoff PASS (R13/R14)
- [ ] 4.2 Authenticated quorum votes/revocation — проверено: TestProbeQuorumRevocationAndTransitionAreSerialized PASS (R13/R14)
- [ ] 4.3 Committed quarantine filters cache-hit subscription — проверено: TestQuarantineUpdatesCachedSubscriptionWithoutPanelRefetch PASS (R13/R14)
- [ ] 4.4 generic configured probe — ограниченный reachability scope — проверено: TestUnknownProtocolGenericHandling PASS; B04 полную health функцию не закрывает (R15)
- [ ] 4.5 WS configured probe — ограниченный reachability scope — проверено: TestHTTPTransportProbe PASS; B04 полную health функцию не закрывает (R15)
- [ ] 4.6 HTTPUpgrade configured probe — ограниченный reachability scope — проверено: TestHTTPUpgradeTransportProbe PASS; B04 полную health функцию не закрывает (R15)
- [ ] 4.7 gRPC configured probe — ограниченный reachability scope — проверено: TestGRPCTransportProbe PASS; B04 полную health функцию не закрывает (R15)
- [ ] 4.8 XHTTP configured probe — ограниченный reachability scope — проверено: TestXHTTPConfiguredPathProbe PASS; B04 полную health функцию не закрывает (R15)
- [ ] 4.9 XHTTP/H3 configured probe — ограниченный reachability scope — проверено: TestQUICHandshakeProbe PASS; B04 полную health функцию не закрывает (R15)
- [ ] 4.10 Live synthetic Xray26.7.28 WS — критерий готовности: positive + wrong path/Host/service/cert cases WS, все leaf tests PASS без SKIP (R15/B06)
- [ ] 4.11 Live synthetic Xray26.7.28 HTTPUpgrade — критерий готовности: positive + wrong path/Host/service/cert cases HTTPUpgrade, все leaf tests PASS без SKIP (R15/B06)
- [ ] 4.12 Live synthetic Xray26.7.28 gRPC — критерий готовности: positive + wrong path/Host/service/cert cases gRPC, все leaf tests PASS без SKIP (R15/B06)
- [ ] 4.13 Live synthetic Xray26.7.28 XHTTP — критерий готовности: positive + wrong path/Host/service/cert cases XHTTP, все leaf tests PASS без SKIP (R15/B06)
- [ ] 4.14 Live synthetic Xray26.7.28 XHTTP/H3 — критерий готовности: positive + wrong path/Host/service/cert cases XHTTP/H3, все leaf tests PASS без SKIP (R15/B06)
- [ ] 4.15 REALITY unauthenticated probe descriptor — критерий готовности: positive/negative fake-server result: acceptance либо honest unverified, TCP не считается REALITY PASS (R13–R15)
- [ ] 4.16 REALITY shared probe — критерий готовности: bounded context/socket/certificate/target fixture без proxy credentials PASS (R13–R15)
- [ ] 4.17 REALITY Core/manual/Agent consumers — критерий готовности: report/quorum/latency tests принимают только новый verified level PASS (R13–R15)
- [ ] 4.18 REALITY real-Xray fixture — критерий готовности: isolated version-pinned valid/invalid REALITY transport cases без SKIP PASS (R13–R15)
- [ ] 4.19 UDP transport capability matrix — критерий готовности: каждый WG/mKCP/Hysteria/legacy transport имеет honest unverified reason, никаких false failure PASS (R13–R15)
- [ ] 4.20 Credentialed probe opt-in — критерий готовности: synthetic probe-user secrets не сохраняются/не попадают в reports, explicit enable PASS (R13–R15)
- [ ] 4.21 Manual force quarantine — критерий готовности: owner API+atomic audit+persist/restart+gateway exclusion tests PASS (R13–R15)
- [ ] 4.22 Manual force healthy/expiry — критерий готовности: override expiry/failure/restart/quorum interaction tests PASS (R13–R15)
- [ ] 4.23 Health transition notification — критерий готовности: один category healthy→quarantine→recovery даёт deduped critical+recovery fixture PASS (R13–R15)
- [ ] 4.24 Process/log health signal — критерий готовности: один safe Xray process/log source влияет на diagnostic с bounded freshness PASS (R13–R15)
- [ ] 4.25 Regional quorum deployment — критерий готовности: две независимые synthetic vantage дают согласованные fresh votes, revoked/stale не влияют PASS (R13–R15)

## Этап 5 — Policy layouts, balancers и pins

Область: [registry/gateway-routing.md](registry/gateway-routing.md).

- [ ] 5.1 Policy: panel tag tier membership — критерий готовности: positive/negative first-match fixture panel tag tier membership; fallback сохраняет неизвестные entries; behavioural PASS (R16)
- [ ] 5.2 Policy: squad membership — критерий готовности: positive/negative first-match fixture squad membership; fallback сохраняет неизвестные entries; behavioural PASS (R16)
- [ ] 5.3 Policy: network_type — критерий готовности: positive/negative first-match fixture network_type; fallback сохраняет неизвестные entries; behavioural PASS (R16)
- [ ] 5.4 Policy: operator — критерий готовности: positive/negative first-match fixture operator; fallback сохраняет неизвестные entries; behavioural PASS (R16)
- [ ] 5.5 Policy: client app — критерий готовности: positive/negative first-match fixture client app; fallback сохраняет неизвестные entries; behavioural PASS (R16)
- [ ] 5.6 Policy: plan — критерий готовности: positive/negative first-match fixture plan; fallback сохраняет неизвестные entries; behavioural PASS (R16)
- [ ] 5.7 Policy: mandatory default policy — критерий готовности: positive/negative first-match fixture mandatory default policy; fallback сохраняет неизвестные entries; behavioural PASS (R16)
- [ ] 5.8 Policy: transport-aware layout — критерий готовности: positive/negative first-match fixture transport-aware layout; fallback сохраняет неизвестные entries; behavioural PASS (R16)
- [ ] 5.9 xray-json random balancer rewrite — критерий готовности: fixture xray-json/random: selector/outbounds/observatory и unknown references round-trip PASS (R17)
- [ ] 5.10 xray-json roundRobin balancer rewrite — критерий готовности: fixture xray-json/roundRobin: selector/outbounds/observatory и unknown references round-trip PASS (R17)
- [ ] 5.11 xray-json leastPing balancer rewrite — критерий готовности: fixture xray-json/leastPing: selector/outbounds/observatory и unknown references round-trip PASS (R17)
- [ ] 5.12 xray-json leastLoad balancer rewrite — критерий готовности: fixture xray-json/leastLoad: selector/outbounds/observatory и unknown references round-trip PASS (R17)
- [ ] 5.13 mihomo-yaml random balancer rewrite — критерий готовности: fixture mihomo-yaml/random: selector/outbounds/observatory и unknown references round-trip PASS (R17)
- [ ] 5.14 mihomo-yaml roundRobin balancer rewrite — критерий готовности: fixture mihomo-yaml/roundRobin: selector/outbounds/observatory и unknown references round-trip PASS (R17)
- [ ] 5.15 mihomo-yaml leastPing balancer rewrite — критерий готовности: fixture mihomo-yaml/leastPing: selector/outbounds/observatory и unknown references round-trip PASS (R17)
- [ ] 5.16 mihomo-yaml leastLoad balancer rewrite — критерий готовности: fixture mihomo-yaml/leastLoad: selector/outbounds/observatory и unknown references round-trip PASS (R17)
- [ ] 5.17 singbox-json random balancer rewrite — критерий готовности: fixture singbox-json/random: selector/outbounds/observatory и unknown references round-trip PASS (R17)
- [ ] 5.18 singbox-json roundRobin balancer rewrite — критерий готовности: fixture singbox-json/roundRobin: selector/outbounds/observatory и unknown references round-trip PASS (R17)
- [ ] 5.19 singbox-json leastPing balancer rewrite — критерий готовности: fixture singbox-json/leastPing: selector/outbounds/observatory и unknown references round-trip PASS (R17)
- [ ] 5.20 singbox-json leastLoad balancer rewrite — критерий готовности: fixture singbox-json/leastLoad: selector/outbounds/observatory и unknown references round-trip PASS (R17)
- [ ] 5.21 Initial weighted rendezvous pin — критерий готовности: stable same-user assignment; weighted distribution и node removal deterministic fixture PASS (R18)
- [ ] 5.22 Persist pin/restart — критерий готовности: same choice после restart, event re-pin только quarantine/removal/quota/manual PASS (R18)
- [ ] 5.23 Sticky-return hysteresis — критерий готовности: fake-clock transient recovery не дёргает pin до deadline PASS (R18)
- [ ] 5.24 Pinned-user capacity — критерий готовности: max-pinned overflow выбирает eligible fallback, no-empty PASS (R18)
- [ ] 5.25 Ordered-fallback output — критерий готовности: ordered node list сохраняет unknown entries и healthy priority PASS (R18)
- [ ] 5.26 Balancer-preferred output — критерий готовности: selected pin и balancer fallback не конфликтуют PASS (R18)
- [ ] 5.27 Rebalance dry-run — критерий готовности: без mutation выводит before/after+change budget PASS (R18)
- [ ] 5.28 Apply pin change budget — критерий готовности: concurrent apply не превышает budget, atomic audit PASS (R18)
- [ ] 5.29 Exit-change metric — критерий готовности: ровно один exit-change/user/day на committed transition PASS (R18)

## Этап 6 — Quotas, injections, merge и geo

Область: [registry/subscription.md](registry/subscription.md).

- [ ] 6.1 Quota accounting — durable replay/timezone/alias safety — проверено: TestQuotaTelemetryUsesCommittedUsageAndSurvivesRestart и TestNodeQuotaUsesInboundFallbackWithoutDoubleCountingUsers PASS (R19)
- [ ] 6.2 Quota exhausted host stub — критерий готовности: TestQuotaExcludesExhaustedNode: убрать host, country flag+точный текст stub, eligible fallback; no counter reset PASS (R19)

- [!] 6.3 All-exhausted panel stub — evidence прежде собственной policy — блокер: B02: на test panel synthetic exhausted user получить fixture; нет stub → зафиксировать вопрос владельцу; [BLOCKED](BLOCKED.md) (R19/B02)

- [ ] 6.4 Panel usage source — один per-node reader — критерий готовности: exact OpenAPI request/response + fail-open Agent fallback fixture PASS (R20)
- [ ] 6.5 Quota 80% warning — критерий готовности: fake usage crossing 80% один alert/period с reset rollover и replay dedupe PASS (R20)
- [ ] 6.6 Quota 90% warning — критерий готовности: fake usage crossing 90% один alert/period с reset rollover и replay dedupe PASS (R20)
- [ ] 6.7 Injection user scope — критерий готовности: один positive/negative workflow user scope, unknown passthrough и atomic audit assertions PASS (R21)
- [ ] 6.8 Injection policy scope — критерий готовности: один positive/negative workflow policy scope, unknown passthrough и atomic audit assertions PASS (R21)
- [ ] 6.9 Injection squad scope — критерий готовности: один positive/negative workflow squad scope, unknown passthrough и atomic audit assertions PASS (R21)
- [ ] 6.10 Injection format scope — критерий готовности: один positive/negative workflow format scope, unknown passthrough и atomic audit assertions PASS (R21)
- [ ] 6.11 Injection blocked/expired/over-limit gate — критерий готовности: один positive/negative workflow blocked/expired/over-limit gate, unknown passthrough и atomic audit assertions PASS (R21)
- [ ] 6.12 Injection client-core compatibility matrix — критерий готовности: один positive/negative workflow client-core compatibility matrix, unknown passthrough и atomic audit assertions PASS (R21)
- [ ] 6.13 Injection sandbox templates — критерий готовности: один positive/negative workflow sandbox templates, unknown passthrough и atomic audit assertions PASS (R21)
- [ ] 6.14 Injection RFC6902 patch — критерий готовности: один positive/negative workflow RFC6902 patch, unknown passthrough и atomic audit assertions PASS (R21)
- [ ] 6.15 Injection JSON Merge Patch — критерий готовности: один positive/negative workflow JSON Merge Patch, unknown passthrough и atomic audit assertions PASS (R21)
- [ ] 6.16 Injection dry-run diff — критерий готовности: один positive/negative workflow dry-run diff, unknown passthrough и atomic audit assertions PASS (R21)
- [ ] 6.17 Injection version history — критерий готовности: один positive/negative workflow version history, unknown passthrough и atomic audit assertions PASS (R21)
- [ ] 6.18 Injection one-click rollback — критерий готовности: один positive/negative workflow one-click rollback, unknown passthrough и atomic audit assertions PASS (R21)
- [ ] 6.19 WG/AWG pool: allocate unique key per subscriber/device — критерий готовности: fixture allocate unique key per subscriber/device с collision/concurrency/failure rollback PASS (R22)
- [ ] 6.20 WG/AWG pool: revoke and never assign active key twice — критерий готовности: fixture revoke and never assign active key twice с collision/concurrency/failure rollback PASS (R22)
- [ ] 6.21 WG/AWG pool: atomic restart/retry lifecycle — критерий готовности: fixture atomic restart/retry lifecycle с collision/concurrency/failure rollback PASS (R22)
- [ ] 6.22 Merge two subscriptions — критерий готовности: две bounded synthetic upstream responses: metadata/unknown formats/no-empty/failure fallback fixture PASS (R22)
- [ ] 6.23 Geo unknown allow/deny/grace — критерий готовности: fixture unknown allow/deny/grace IPv4/IPv6/unknown/low-confidence; errors fail-open отдельно от intentional deny PASS (R23)
- [ ] 6.24 Geo explicit priority — критерий готовности: fixture explicit priority IPv4/IPv6/unknown/low-confidence; errors fail-open отдельно от intentional deny PASS (R23)
- [ ] 6.25 Geo per-user exemption — критерий готовности: fixture per-user exemption IPv4/IPv6/unknown/low-confidence; errors fail-open отдельно от intentional deny PASS (R23)
- [ ] 6.26 Geo per-policy scope — критерий готовности: fixture per-policy scope IPv4/IPv6/unknown/low-confidence; errors fail-open отдельно от intentional deny PASS (R23)
- [ ] 6.27 Geo 403 response — критерий готовности: fixture 403 response IPv4/IPv6/unknown/low-confidence; errors fail-open отдельно от intentional deny PASS (R23)
- [ ] 6.28 Geo 404-mimic response — критерий готовности: fixture 404-mimic response IPv4/IPv6/unknown/low-confidence; errors fail-open отдельно от intentional deny PASS (R23)
- [ ] 6.29 Geo decoy response — критерий готовности: fixture decoy response IPv4/IPv6/unknown/low-confidence; errors fail-open отдельно от intentional deny PASS (R23)
- [ ] 6.30 Geo intentional empty-block response — критерий готовности: fixture intentional empty-block response IPv4/IPv6/unknown/low-confidence; errors fail-open отдельно от intentional deny PASS (R23)

## Этап 7 — IP data и origins

Область: [registry/ip-origins.md](registry/ip-origins.md).

- [ ] 7.1 L1 IPtoASN/BGP source contract — критерий готовности: URL/format/license/checksum/update terms documented; один очищенный parser positive+bad input fixture PASS (R24/R53)
- [ ] 7.2 L2 RIR source contract — критерий готовности: URL/format/license/checksum/update terms documented; один очищенный parser positive+bad input fixture PASS (R24/R53)
- [ ] 7.3 L3 DB-IP source contract — критерий готовности: URL/format/license/checksum/update terms documented; один очищенный parser positive+bad input fixture PASS (R24/R53)
- [ ] 7.4 L3 IPinfo source contract — критерий готовности: URL/format/license/checksum/update terms documented; один очищенный parser positive+bad input fixture PASS (R24/R53)
- [ ] 7.5 L3 MaxMind source contract — критерий готовности: URL/format/license/checksum/update terms documented; один очищенный parser positive+bad input fixture PASS (R24/R53)
- [ ] 7.6 L4 ASN classification source contract — критерий готовности: URL/format/license/checksum/update terms documented; один очищенный parser positive+bad input fixture PASS (R24/R53)
- [ ] 7.7 L5 threat lists source contract — критерий готовности: URL/format/license/checksum/update terms documented; один очищенный parser positive+bad input fixture PASS (R24/R53)
- [ ] 7.8 L1 last-good refresh — критерий готовности: TestL1FeedLastGood: bad/download timeout не меняет active dataset PASS (R24)
- [ ] 7.9 L1+L2 unknown-geo fallback — критерий готовности: TestRIRUnknownGeoFallback ASN+country medium confidence без geo PASS (R24)
- [ ] 7.10 Merged MMDB atomic swap — критерий готовности: TestMergedMMDBAtomicReload + concurrent lookup zero errors PASS (R24)
- [ ] 7.11 IP curated ASN override — критерий готовности: fixture curated ASN override: positive/invalid/last-good/provenance assertions PASS (R25)
- [ ] 7.12 IP CSV import/export — критерий готовности: fixture CSV import/export: positive/invalid/last-good/provenance assertions PASS (R25)
- [ ] 7.13 IP RU mobile seeds needs-review — критерий готовности: fixture RU mobile seeds needs-review: positive/invalid/last-good/provenance assertions PASS (R25)
- [ ] 7.14 IP heuristics suggestion-only — критерий готовности: fixture heuristics suggestion-only: positive/invalid/last-good/provenance assertions PASS (R25)
- [ ] 7.15 IP source dates/confidence attribution — критерий готовности: fixture source dates/confidence attribution: positive/invalid/last-good/provenance assertions PASS (R25)
- [ ] 7.16 IP stale configurable alert — критерий готовности: fixture stale configurable alert: positive/invalid/last-good/provenance assertions PASS (R25)
- [ ] 7.17 IP bulk lookup — критерий готовности: fixture bulk lookup: positive/invalid/last-good/provenance assertions PASS (R25)
- [ ] 7.18 IP CLI lookup — критерий готовности: fixture CLI lookup: positive/invalid/last-good/provenance assertions PASS (R25)
- [ ] 7.19 IP coverage report — критерий готовности: fixture coverage report: positive/invalid/last-good/provenance assertions PASS (R25)
- [ ] 7.20 trusted CDN ranges/real-IP — критерий готовности: fixture trusted CDN ranges/real-IP; proxy spoof/no-external-lookup/privacy retention assertions PASS (R26/R27)
- [ ] 7.21 IPv6 /64 grouping — критерий готовности: fixture IPv6 /64 grouping; proxy spoof/no-external-lookup/privacy retention assertions PASS (R26/R27)
- [ ] 7.22 CGNAT many-users hint — критерий готовности: fixture CGNAT many-users hint; proxy spoof/no-external-lookup/privacy retention assertions PASS (R26/R27)
- [ ] 7.23 request-origin aggregation — критерий готовности: fixture request-origin aggregation; proxy spoof/no-external-lookup/privacy retention assertions PASS (R26/R27)
- [ ] 7.24 connection-origin enrichment — критерий готовности: fixture connection-origin enrichment; proxy spoof/no-external-lookup/privacy retention assertions PASS (R26/R27)
- [ ] 7.25 per-user distinct origins history — критерий готовности: fixture per-user distinct origins history; proxy spoof/no-external-lookup/privacy retention assertions PASS (R26/R27)
- [ ] 7.26 origin retention30d/365d — критерий готовности: fixture origin retention30d/365d; proxy spoof/no-external-lookup/privacy retention assertions PASS (R26/R27)
- [ ] 7.27 IP truncation/hash/no-IP — критерий готовности: fixture IP truncation/hash/no-IP; proxy spoof/no-external-lookup/privacy retention assertions PASS (R26/R27)
- [ ] 7.28 bundled offline origin map — критерий готовности: fixture bundled offline origin map; proxy spoof/no-external-lookup/privacy retention assertions PASS (R26/R27)

## Этап 8 — Logs и bounded collection

Область: [registry/activity.md](registry/activity.md).

- [ ] 8.1 access-log golden Xray26.7.28 — критерий готовности: очищенный corpus/fixture access-log golden Xray26.7.28: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.2 error/debug/info/warning classifier integration — критерий готовности: очищенный corpus/fixture error/debug/info/warning classifier integration: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.3 DNS source — критерий готовности: очищенный corpus/fixture DNS source: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.4 journald source — критерий готовности: очищенный corpus/fixture journald source: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.5 docker-json-file read-only source — критерий готовности: очищенный corpus/fixture docker-json-file read-only source: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.6 unparsed bounded samples — критерий готовности: очищенный corpus/fixture unparsed bounded samples: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.7 per-level/tag signal counters — критерий готовности: очищенный corpus/fixture per-level/tag signal counters: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.8 oversize-line truncate/recover — критерий готовности: очищенный corpus/fixture oversize-line truncate/recover: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.9 channel overflow skip-ahead+gap — критерий готовности: очищенный corpus/fixture channel overflow skip-ahead+gap: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.10 rename rotation — критерий готовности: очищенный corpus/fixture rename rotation: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.11 copytruncate rotation — критерий готовности: очищенный corpus/fixture copytruncate rotation: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.12 delete/recreate rotation — критерий готовности: очищенный corpus/fixture delete/recreate rotation: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.13 partial write handling — критерий готовности: очищенный corpus/fixture partial write handling: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.14 audited ephemeral live-tail — критерий готовности: очищенный corpus/fixture audited ephemeral live-tail: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.15 configured log level/capture-quality — критерий готовности: очищенный corpus/fixture configured log level/capture-quality: correct metadata, bounded memory/no raw persistence PASS (R28/R29)
- [ ] 8.16 Log parser fuzz seed regression — критерий готовности: unknown input не panic; воспроизведённый corpus seed suite PASS (R59)
- [ ] 8.17 Log pipeline flood acceptance — критерий готовности: >=100k lines/s/Agent core, unparsed<=0.5%, RSS bound/gap counters записаны; без влияния на Xray PASS (R28/R60)

## Этап 9 — Activity: компактное хранение и queries

Область: [registry/activity.md](registry/activity.md).

- [ ] 9.1 Owner query/export audit и shared60/min — проверено: TestActivityReadExportAuditFilters/SharedRateLimit/AuditIntegrityAndQueryFailure PASS (R11/R12)
- [ ] 9.2 Private key rollover restart/read old/new ciphertext — проверено: TestActivityKeyRotation и TestStorageKeyRejectsFIFOWithoutBlocking PASS (R12)
- [ ] 9.3 Capture switches — критерий готовности: global/node/user ON default+indicator, exclude option не создаёт auth groups PASS (R30–R35)
- [ ] 9.4 Runtime retention Q6 — критерий готовности: 7d hot/90d deletion defaults/UI,50% deployment-free budget сохранён PASS (R30–R35)
- [ ] 9.5 Agent coalesce10s — критерий готовности: same metadata key count/first/last bucket, no cross-node mixing PASS (R30–R35)
- [ ] 9.6 Chatter distinct-destination cap — критерий готовности: overflow summary/counters не теряет counts и hard bounds PASS (R30–R35)
- [ ] 9.7 Segment benchmark input — критерий готовности: reproducible synthetic/golden event generator с realistic cardinality PASS (R30–R35)
- [ ] 9.8 Parquet/custom benchmark decision — критерий готовности: непустые bytes/event+throughput/query measurements и rationale PASS (R30–R35)
- [ ] 9.9 Immutable segment writer/reader — критерий готовности: TestSegmentRoundTrip sorted(user,ts), interrupted-write atomicity PASS (R30–R35)
- [ ] 9.10 Dictionary encryption — критерий готовности: TestSegmentEncryption без plaintext domain/IP PASS (R30–R35)
- [ ] 9.11 Bloom/user index pruning — критерий готовности: TestSegmentPruning без false negatives PASS (R30–R35)
- [ ] 9.12 Segment WAL/ACK replay — критерий готовности: TestCrashReplayUnackedBatch exactly-once committed metadata PASS (R30–R35)
- [ ] 9.13 7d hot→compressed archive — критерий готовности: compactor preserves count/first/last/AEAD, concurrent query consistency PASS (R30–R35)
- [ ] 9.14 90d archive deletion — критерий готовности: fake clock удаляет segments/dictionaries/index entries без replay resurrection PASS (R30–R35)
- [ ] 9.15 Disk quota eviction — критерий готовности: TestDiskQuotaEviction50% deployment-free cap, explicit gaps/fail-open PASS (R30–R35)
- [ ] 9.16 Purge all tiers/dictionaries — критерий готовности: TestPurgeAllTiers не оставляет reverse index/backup export remnants PASS (R30–R35)
- [ ] 9.17 Session timeline — критерий готовности: TestActivitySessionWindow idle gap5min/configurable и secondT±window PASS (R30–R35)
- [ ] 9.18 NDJSON endpoint export — критерий готовности: TestActivityNDJSONExport limits/audit/escaping/pagination PASS (R30–R35)
- [ ] 9.19 Destination categories — критерий готовности: TestDestinationCategories versioned source/provenance без payload inspection PASS (R30–R35)
- [ ] 9.20 IP-only resolved_by enrichment — критерий готовности: ASN/org/rDNS/DNS hint source labels, ECH/DoH honest IP-only PASS (R30–R35)
- [ ] 9.21 Domain reverse search/first seen — критерий готовности: период/пользователи/filters query корректен с indexes PASS (R30–R35)
- [ ] 9.22 Period comparison query — критерий готовности: bounded same-user periods/categories count fixture PASS (R30–R35)

- [!] 9.23 Legal hold policy — блокер: B07: не реализовывать retention>90d или auth groups без решения владельца; независимая Q6 реализация продолжается; [BLOCKED](BLOCKED.md) (R34)

- [ ] 9.24 Compression acceptance — критерий готовности: <=8B/event и>=30x raw reduction на reproducible corpus measured PASS (R31/R60)
- [ ] 9.25 Ingest budget — критерий готовности: >=100k events/s4vCPU и compaction<=25%CPU measured PASS (R31/R60)
- [ ] 9.26 Activity query budgets — критерий готовности: hot user-day p95<=300ms,warm month<=1.5s,domain/day<=3s measured PASS (R31/R60)

## Этап 10 — Live telemetry, diagnostics и alerts

Область: [registry/telemetry.md](registry/telemetry.md).

- [ ] 10.1 Xray reset=false/delta fixture — проверено: TestRealGRPCStatsContractDeltaAndCounterReset PASS (R36)
- [ ] 10.2 Sampling cost measurement — критерий готовности: per-tick wall/monotonic cost visible без PII PASS (R36–R40/R58)
- [ ] 10.3 Adaptive1/2/5s — критерий готовности: >25% query-cost переключает resolution с hysteresis и UI field PASS (R36–R40/R58)
- [ ] 10.4 Sparse samples + explicit gaps — критерий готовности: drop/restart/warmup не fake zeros, idempotent replay PASS (R36–R40/R58)
- [ ] 10.5 Raw ring byte cap — критерий готовности: TestRawRingMemoryCap<=64MiB default на high-cardinality fixture PASS (R36–R40/R58)
- [ ] 10.6 Durable10s24h rollup — критерий готовности: TestTenSecondRollup restart/window boundaries PASS (R36–R40/R58)
- [ ] 10.7 Durable1m30d rollup — критерий готовности: read after24h возвращает actual persisted data PASS (R36–R40/R58)
- [ ] 10.8 Durable1h1y rollup — критерий готовности: retention jobs/test timezone/skew не double-count PASS (R36–R40/R58)
- [ ] 10.9 Full minute average — критерий готовности: 60s weighted average/EMA/peak/p95 без12-sample shortcut PASS (R36–R40/R58)
- [ ] 10.10 Panel coarse fallback — критерий готовности: exact bandwidth endpoint fixture и explicit coarse label PASS (R36–R40/R58)
- [ ] 10.11 Node NIC capacity/saturation — критерий готовности: counter delta/capacity integer overflow и percent fixtures PASS (R36–R40/R58)
- [ ] 10.12 User-speed threshold alert — критерий готовности: X Mbit/Y min suppression/recovery/dedupe fake clock PASS (R36–R40/R58)
- [ ] 10.13 Burst and saturation alert — критерий готовности: >85% transitions/no repeated per-tick spam PASS (R36–R40/R58)
- [ ] 10.14 Regional>=64KiB transfer — критерий готовности: stall14..24KiB suspect/blocked vs EOF/truncated error, node/vantage/inbound saved PASS (R36–R40/R58)
- [ ] 10.15 Regional proxy execution — критерий готовности: explicit SOCKS5/HTTP proxy transport fixture не Core-only label PASS (R36–R40/R58)
- [ ] 10.16 Advanced Xray transfer — критерий готовности: dedicated synthetic probe-user real transfer не resets accounting PASS (R36–R40/R58)
- [ ] 10.17 YouTube node egress — критерий готовности: Agent request test без Core egress substitution PASS (R36–R40/R58)
- [ ] 10.18 YouTube schedule/cooldown — критерий готовности: forbidden RU→stop/reset/recovery fake clock PASS (R36–R40/R58)
- [ ] 10.19 Drop MAD/EWMA learning — критерий готовности: seasonal baseline не alert до learning threshold PASS (R36–R40/R58)
- [ ] 10.20 Drop T-minute/recovery — критерий готовности: maintenance/context/dedupe/recovery message fixture PASS (R36–R40/R58)
- [ ] 10.21 Metrics HTTP semantics — критерий готовности: /metrics content type/counters label bounds behavioral request PASS (R36–R40/R58)
- [ ] 10.22 Fleet exporter first gauge — критерий готовности: one online-node gauge и stale sync watchdog fixture PASS (R36–R40/R58)
- [ ] 10.23 Grafana bundled dashboard — критерий готовности: dashboard JSON с existing metrics + import validation PASS (R36–R40/R58)
- [ ] 10.24 Agent host metric RSS — критерий готовности: fake /proc/sysfs reader RSS, unavailable diagnostic/bounded sample fixture PASS (R36)
- [ ] 10.25 Agent host metric CPU — критерий готовности: fake /proc/sysfs reader CPU, unavailable diagnostic/bounded sample fixture PASS (R36)
- [ ] 10.26 Agent host metric disk — критерий готовности: fake /proc/sysfs reader disk, unavailable diagnostic/bounded sample fixture PASS (R36)
- [ ] 10.27 Agent host metric NIC throughput/drops — критерий готовности: fake /proc/sysfs reader NIC throughput/drops, unavailable diagnostic/bounded sample fixture PASS (R36)
- [ ] 10.28 Agent host metric conntrack — критерий готовности: fake /proc/sysfs reader conntrack, unavailable diagnostic/bounded sample fixture PASS (R36)
- [ ] 10.29 Agent host metric TCP retransmits — критерий готовности: fake /proc/sysfs reader TCP retransmits, unavailable diagnostic/bounded sample fixture PASS (R36)
- [ ] 10.30 Agent host metric Xray process state — критерий готовности: fake /proc/sysfs reader Xray process state, unavailable diagnostic/bounded sample fixture PASS (R36)
- [ ] 10.31 Real accounting unchanged — критерий готовности: TestLiveStatsService + panel before/after counters без reset, все leaf cases PASS; synthetic isolated environment, B06 (R60)
- [ ] 10.32 Speed accuracy iperf — критерий готовности: 1s node transfer measured±5%,reset/restart не spikes; synthetic isolated environment, B06 (R60)
- [ ] 10.33 Clock skew aggregation — критерий готовности: ±500ms wall buckets и NTP skew fixtures explicit gap; synthetic isolated environment, B06 (R60)
- [ ] 10.34 20×5000 users load — критерий готовности: 4vCPU Core>=200k samples/s,RSS<=512MiB,no hidden gaps; synthetic isolated environment, B06 (R60)

## Этап 11 — Anti-scan и explainable abuse

Область: [registry/security.md](registry/security.md).

- [ ] 11.1 Anti-scan tarpit activation — критерий готовности: fixture tarpit activation; legitimate client/fail-open/positive-negative assertions PASS (R41)
- [ ] 11.2 Anti-scan request allowlist config/UI — критерий готовности: fixture request allowlist config/UI; legitimate client/fail-open/positive-negative assertions PASS (R41)
- [ ] 11.3 Anti-scan masked enumeration/rate events — критерий готовности: fixture masked enumeration/rate events; legitimate client/fail-open/positive-negative assertions PASS (R41)
- [ ] 11.4 Anti-scan node handshake burst corpus — критерий готовности: fixture node handshake burst corpus; legitimate client/fail-open/positive-negative assertions PASS (R41)
- [ ] 11.5 Anti-scan IP/subnet/ASN/UA correlation — критерий готовности: fixture IP/subnet/ASN/UA correlation; legitimate client/fail-open/positive-negative assertions PASS (R41)
- [ ] 11.6 Anti-scan dedicated TG topic — критерий готовности: fixture dedicated TG topic; legitimate client/fail-open/positive-negative assertions PASS (R41)
- [ ] 11.7 Anti-scan proxy/firewall generate-only export — критерий готовности: fixture proxy/firewall generate-only export; legitimate client/fail-open/positive-negative assertions PASS (R41)
- [ ] 11.8 F21 temporal analyser — критерий готовности: positive/negative synthetic temporal, concrete observations+thresholds in signal, no automatic action PASS (R51)
- [ ] 11.9 F21 geographic/impossible-travel analyser — критерий готовности: positive/negative synthetic geographic/impossible-travel, concrete observations+thresholds in signal, no automatic action PASS (R51)
- [ ] 11.10 F21 ASN analyser — критерий готовности: positive/negative synthetic ASN, concrete observations+thresholds in signal, no automatic action PASS (R51)
- [ ] 11.11 F21 behaviour analyser — критерий готовности: positive/negative synthetic behaviour, concrete observations+thresholds in signal, no automatic action PASS (R51)
- [ ] 11.12 F21 devices analyser — критерий готовности: positive/negative synthetic devices, concrete observations+thresholds in signal, no automatic action PASS (R51)
- [ ] 11.13 F21 HWID analyser — критерий готовности: positive/negative synthetic HWID, concrete observations+thresholds in signal, no automatic action PASS (R51)
- [ ] 11.14 F21 User-Agent analyser — критерий готовности: positive/negative synthetic User-Agent, concrete observations+thresholds in signal, no automatic action PASS (R51)
- [ ] 11.15 F21 scoring/dedupe — критерий готовности: fixture scoring/dedupe включает explainable evidence, false-positive handling, no-ban invariant PASS (R51)
- [ ] 11.16 F21 delayed re-check — критерий готовности: fixture delayed re-check включает explainable evidence, false-positive handling, no-ban invariant PASS (R51)
- [ ] 11.17 F21 CGNAT buffer/agglomerations — критерий готовности: fixture CGNAT buffer/agglomerations включает explainable evidence, false-positive handling, no-ban invariant PASS (R51)
- [ ] 11.18 F21 P2P/torrent panel report — критерий готовности: fixture P2P/torrent panel report включает explainable evidence, false-positive handling, no-ban invariant PASS (R51)
- [ ] 11.19 F21 customer warning template — критерий готовности: fixture customer warning template включает explainable evidence, false-positive handling, no-ban invariant PASS (R51)

## Этап 12 — Операции, Telegram и backup

Область: [registry/ops.md](registry/ops.md).

- [ ] 12.1 Telegram health topic/recovery — критерий готовности: fake BotAPI fixture health topic/recovery, unauthorised chat denied, no secrets/log echo PASS (R42)
- [ ] 12.2 Telegram allowlisted /status — критерий готовности: fake BotAPI fixture allowlisted /status, unauthorised chat denied, no secrets/log echo PASS (R42)
- [ ] 12.3 Telegram allowlisted /node — критерий готовности: fake BotAPI fixture allowlisted /node, unauthorised chat denied, no secrets/log echo PASS (R42)
- [ ] 12.4 Telegram allowlisted /user — критерий готовности: fake BotAPI fixture allowlisted /user, unauthorised chat denied, no secrets/log echo PASS (R42)
- [ ] 12.5 Telegram allowlisted /top — критерий готовности: fake BotAPI fixture allowlisted /top, unauthorised chat denied, no secrets/log echo PASS (R42)
- [ ] 12.6 Telegram allowlisted /ip — критерий готовности: fake BotAPI fixture allowlisted /ip, unauthorised chat denied, no secrets/log echo PASS (R42)
- [ ] 12.7 Telegram quiet hours/severity — критерий готовности: fake BotAPI fixture quiet hours/severity, unauthorised chat denied, no secrets/log echo PASS (R42)
- [ ] 12.8 Telegram RU/EN templates — критерий готовности: fake BotAPI fixture RU/EN templates, unauthorised chat denied, no secrets/log echo PASS (R42)
- [ ] 12.9 Telegram proxy delivery — критерий готовности: fake BotAPI fixture proxy delivery, unauthorised chat denied, no secrets/log echo PASS (R42)
- [ ] 12.10 Confirmed user revoke — критерий готовности: exact OpenAPI method/id/body test, explicit owner confirmation+final audit success/failure+read-only denial PASS (R43)
- [ ] 12.11 Confirmed user enable/disable/reset — критерий готовности: exact OpenAPI method/id/body test, explicit owner confirmation+final audit success/failure+read-only denial PASS (R43)
- [ ] 12.12 Confirmed node enable/disable/restart — критерий готовности: exact OpenAPI method/id/body test, explicit owner confirmation+final audit success/failure+read-only denial PASS (R43)
- [ ] 12.13 Confirmed host enable/disable — критерий готовности: exact OpenAPI method/id/body test, explicit owner confirmation+final audit success/failure+read-only denial PASS (R43)
- [ ] 12.14 Confirmed hosts bulk — критерий готовности: exact OpenAPI method/id/body test, explicit owner confirmation+final audit success/failure+read-only denial PASS (R43)
- [ ] 12.15 Global users/nodes/hosts search/deep links — критерий готовности: bounded search/no secret URL/keyboard browser fixture PASS (R43)
- [ ] 12.16 Local encrypted WAL backup — проверено: TestEncryptedBackupRestoresRealDatabase PASS (R44)
- [ ] 12.17 Backup activity exclusion — критерий готовности: TestBackupExcludesActivity decrypt synthetic snapshot: no journal rows/keys, rest of state consistent PASS (R44/R45)
- [ ] 12.18 Backup schedule — критерий готовности: fake clock scheduled local artifact exactly once/retry audit PASS (R44/R45)
- [ ] 12.19 Count+age rotation — критерий готовности: synthetic private artifacts prune only eligible backups PASS (R44/R45)
- [ ] 12.20 Restore corruption/path rejection — критерий готовности: new output only, traversal/symlink/auth failure never changes running DB PASS (R44/R45)
- [ ] 12.21 Restore schema/version gate — критерий готовности: TestRestoreRejectsVersionAndCorruption integrity_check/migration compatibility PASS (R44/R45)
- [ ] 12.22 Restore atomic cutover plan — критерий готовности: failure rollback/temp file permissions/offline operator steps fixture PASS (R44/R45)
- [ ] 12.23 Telegram multipart backup — критерий готовности: TestTelegramSplitBackup reconstructs same encrypted artifact with bounded memory PASS (R44/R45)
- [ ] 12.24 Panel backup contract — критерий готовности: read-only OpenAPI-supported snapshot source first; if absent precise blocker без PG schema read PASS (R44/R45)
- [ ] 12.25 Pre-migration snapshot — критерий готовности: synthetic schema upgrade failure сохраняет original DB/key/artifact PASS (R44/R45)

## Этап 14 — Settings, wizard, UI и установка

Область: [registry/panel-ui.md](registry/panel-ui.md).

- [ ] 14.1 Versioned settings validation — критерий готовности: unknown/invalid fields reject; secrets never returned PASS (R46/R47/R58)
- [ ] 14.2 Atomic settings hot-reload — критерий готовности: one config version applied all readers, failure leaves last-good PASS (R46/R47/R58)
- [ ] 14.3 Settings rollback — критерий готовности: TestSettingsRollback audit+config restored PASS (R46/R47/R58)
- [ ] 14.4 Wizard owner bootstrap — критерий готовности: first-run only single-owner password setup, repeat cannot create second account PASS (R46/R47/R58)
- [ ] 14.5 Wizard panel test — критерий готовности: TestWizardPanelConnection read-only contract feedback without token echo PASS (R46/R47/R58)
- [ ] 14.6 Wizard node discovery — критерий готовности: synced nodes scope errors shown and retry safe PASS (R46/R47/R58)
- [ ] 14.7 Wizard IP sources — критерий готовности: licences/keys private, test lookup/attribution PASS (R46/R47/R58)
- [ ] 14.8 Wizard Telegram — критерий готовности: fake delivery config/test no real outbound during fixture PASS (R46/R47/R58)
- [ ] 14.9 Wizard Agent enrollment — критерий готовности: one-time command docs private token,15min expiry PASS (R46/R47/R58)
- [ ] 14.10 Live virtualization — критерий готовности: TestLiveVirtualizedRows10k rows <=16ms/frame PASS (R46/R47/R58)
- [ ] 14.11 Activity scrubber UI — критерий готовности: TestActivityScrubber shows secondT windows/session gaps PASS (R46/R47/R58)
- [ ] 14.12 Offline origin map — критерий готовности: no external tiles/fonts/IP requests PASS (R46/R47/R58)
- [ ] 14.13 Logs live-tail screen — критерий готовности: audited ephemeral display/no raw persistence PASS (R46/R47/R58)
- [ ] 14.14 Blocks matrix screen — критерий готовности: vantage/node/inbound result labels accurate PASS (R46/R47/R58)
- [ ] 14.15 Injections screen — критерий готовности: scope/dry-run/diff/version fixture PASS (R46/R47/R58)
- [ ] 14.16 Quotas screen — критерий готовности: period/flags/stub/warnings fixture PASS (R46/R47/R58)
- [ ] 14.17 Anti-scan screen — критерий готовности: allowlist/signals/evidence/correlation fixture PASS (R46/R47/R58)
- [ ] 14.18 Operations screen — критерий готовности: confirmed actions/errors/audit fixture PASS (R46/R47/R58)
- [ ] 14.19 Locale switch — критерий готовности: TestLocaleSwitch RU/EN persists PASS (R46/R47/R58)
- [ ] 14.20 Light/dark theme — критерий готовности: contrast/focus/no FOUC fixture PASS (R46/R47/R58)
- [ ] 14.21 Keyboard/reduced motion/mobile — критерий готовности: TestKeyboardAndReducedMotion,44px targets,focus/ARIA/mobile assertions PASS (R46/R47/R58)

## Этап 15 — Grace, devices, presets и relay

Область: [registry/subscription.md](registry/subscription.md).git show 2c99ed5 -- core/internal/activity/segment/retention.go

- [ ] 15.1 Grace snapshot enter limited squad — критерий готовности: test-panel synthetic expired user preserve previous squads, retry safe PASS (R49/R50)
- [ ] 15.2 Grace restore on payment — критерий готовности: TestGraceSquadRestore idempotent webhook restoration and failure rollback PASS (R49/R50)
- [ ] 15.3 HWID limit/stub — критерий готовности: panel-native blocked device returns explicit stub; cache cannot cross devices PASS (R49/R50)
- [ ] 15.4 HWID from UA/device card — критерий готовности: unknown/malformed UA не выдумывает identity; TestHWIDDeviceCard PASS (R49/R50)
- [ ] 15.5 Request history projection — критерий готовности: exact panel history response/model+OS/decision/bytes retention PASS (R49/R50)
- [ ] 15.6 Client-version catalogue source — критерий готовности: one licensed bounded source+last-good update PASS (R49/R50)
- [ ] 15.7 Outdated-client flag — критерий готовности: TestOutdatedClientCatalogue semantic versions/unknown/no false stale PASS (R49/R50)
- [ ] 15.8 App response rules — критерий готовности: UA/OS precedence headers fixture, conflicting rules deterministic PASS (R49/R50)
- [ ] 15.9 Happ/Incy routing preset — критерий готовности: TestAppResponsePreset supported format/core/status gate PASS (R49/R50)
- [ ] 15.10 Subpage/client template library — критерий готовности: Prizrak-Box placeholders escaping/pxa/pxd rules; no unknown corruption PASS (R49/R50)
- [ ] 15.11 Relay topology one edge — критерий готовности: cycle rejection/read-only route graph fixture PASS (R49/R50)
- [ ] 15.12 Relay subtractive counters — критерий готовности: TestRelaySubtractiveAccounting no negative/double count/no reset PASS (R49/R50)
- [ ] 15.13 Multi-hop attribution — критерий готовности: two hops+direct branch counters invariant fixture PASS (R49/R50)

## Этап 16 — Protection, diagnostics и automation

Область: [registry/security.md](registry/security.md).

- [ ] 16.1 Threat-list import own tags — критерий готовности: TestThreatListImport IPv4/IPv6/allowlist/invalid source last-good PASS (R53/R54)
- [ ] 16.2 Generate own nft set — критерий готовности: TestOwnNftSetOnly never flush ruleset/foreign tables PASS (R53/R54)
- [ ] 16.3 Anti-lockout safety timer — критерий готовности: TestAntiLockoutRollback SSH/Core/panel whitelist, fixture timeout reverts PASS (R53/R54)
- [ ] 16.4 SYN/UDP/connection limits — критерий готовности: generate-only bounded own-chain plan positive/negative PASS (R53/R54)
- [ ] 16.5 UDP bulk ports/node discovery — критерий готовности: Hysteria2/TUIC/node-port+panel-whitelist fixtures PASS (R53/R54)
- [ ] 16.6 ctguard/ban-once modes — критерий готовности: strict/open/skip/repeated event does not double apply PASS (R53/R54)
- [ ] 16.7 Fleet whitelist plan — критерий готовности: TestFleetWhitelist version/conflict/revoke dry-run PASS (R53/R54)
- [ ] 16.8 Diagnostic catalogue27/19 manifest — критерий готовности: verify public source names/inputs/thresholds before check implementations PASS (R53/R54)
- [ ] 16.9 Read-only check kernel/BBR — критерий готовности: one fake-host kernel/BBR JSON/timeout/unavailable assertion PASS; расширять по одному service/check до27/19 (R54)
- [ ] 16.10 Read-only check conntrack — критерий готовности: one fake-host conntrack JSON/timeout/unavailable assertion PASS; расширять по одному service/check до27/19 (R54)
- [ ] 16.11 Read-only check NIC/PSI/UDP — критерий готовности: one fake-host NIC/PSI/UDP JSON/timeout/unavailable assertion PASS; расширять по одному service/check до27/19 (R54)
- [ ] 16.12 Read-only check Xray/remnanode process — критерий готовности: one fake-host Xray/remnanode process JSON/timeout/unavailable assertion PASS; расширять по одному service/check до27/19 (R54)
- [ ] 16.13 Read-only check open ports/firewall desync — критерий готовности: one fake-host open ports/firewall desync JSON/timeout/unavailable assertion PASS; расширять по одному service/check до27/19 (R54)
- [ ] 16.14 Read-only check certificate expiry — критерий готовности: one fake-host certificate expiry JSON/timeout/unavailable assertion PASS; расширять по одному service/check до27/19 (R54)
- [ ] 16.15 Read-only check whitelist consistency — критерий готовности: one fake-host whitelist consistency JSON/timeout/unavailable assertion PASS; расширять по одному service/check до27/19 (R54)
- [ ] 16.16 Read-only check retransmit causes — критерий готовности: one fake-host retransmit causes JSON/timeout/unavailable assertion PASS; расширять по одному service/check до27/19 (R54)
- [ ] 16.17 Read-only check disk/log sensors — критерий готовности: one fake-host disk/log sensors JSON/timeout/unavailable assertion PASS; расширять по одному service/check до27/19 (R54)
- [ ] 16.18 Read-only check IP reputation — критерий готовности: one fake-host IP reputation JSON/timeout/unavailable assertion PASS; расширять по одному service/check до27/19 (R54)
- [ ] 16.19 Read-only check service reachability — критерий готовности: one fake-host service reachability JSON/timeout/unavailable assertion PASS; расширять по одному service/check до27/19 (R54)
- [ ] 16.20 Advisor BBR/Cake/XanMod — критерий готовности: generate-only dry-run named drop-in BBR/Cake/XanMod; rollback leaves foreign config untouched PASS (R54)
- [ ] 16.21 Advisor RPS/RFS/XPS/NIC — критерий готовности: generate-only dry-run named drop-in RPS/RFS/XPS/NIC; rollback leaves foreign config untouched PASS (R54)
- [ ] 16.22 Advisor FD/MSS/zram/journald — критерий готовности: generate-only dry-run named drop-in FD/MSS/zram/journald; rollback leaves foreign config untouched PASS (R54)
- [ ] 16.23 Automation rule predicate — критерий готовности: one workflow rule predicate synthetic clock/validation, signal-only/owner-confirmation invariants PASS (R52)
- [ ] 16.24 Automation schedule — критерий готовности: one workflow schedule synthetic clock/validation, signal-only/owner-confirmation invariants PASS (R52)
- [ ] 16.25 Automation execution template — критерий готовности: one workflow execution template synthetic clock/validation, signal-only/owner-confirmation invariants PASS (R52)
- [ ] 16.26 Automation execution log — критерий готовности: one workflow execution log synthetic clock/validation, signal-only/owner-confirmation invariants PASS (R52)
