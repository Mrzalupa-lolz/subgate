## 4. Реестр требований и доказательств

**DONE** — весь указанный проверяемый объём подтверждён. **PARTIAL** — часть реализации есть, но остаются требования или отсутствует behavioral verification. **STUB** — схема/поле/неподключённая заготовка под обещанную функцию. **MISSING** — нет требуемой реализации/quality gate. Для reference/non-goal строк статус описывает наличие соответствующей возможности в SubGate; он не разрешает её реализацию.

В evidence ниже имена Test* имеют PASS в текущем `go test -json ./...`. У PARTIAL тест подтверждает **только описанную часть**, а не все условия исходной цитаты. «Тест отсутствует; не проверено» не заменяется словом «готово». `FuzzTransform` здесь выполнял seed cases; fuzz campaign не запускалась. `TestLiveStatsService` — SKIP, не DONE.

### Проверенные части (не повышают целые F1–F20 до DONE)

| Проверенный объём | Строки требований | Реализация / выполненный тест |
|---|---|---|
| Opaque upstream passthrough, bounded persistent stale, authoritative 4xx, coalescing | F-075…F-081 | Gateway.HandleRequest; TestOpaquePassthroughAndPersistentFallback, TestExpiredSnapshotNeverFallsBack, TestConcurrentRequestsCoalesceAndReleaseIdleLocks, TestClearDoesNotResurrectInFlightSnapshot |
| Atomic users/nodes snapshot с last-good при failed fetch | F-043…F-047 | Server.syncPanel; TestPanelSyncCommitsCompleteSnapshotAndPreservesLastGood, TestPanelSyncRejectsOversizedTotalWithoutErasingSnapshot |
| HMAC/raw body/replay/invalidation | F-045 | Server.webhook; TestWebhookHMACReplayAndSnapshotInvalidation |
| Ручной pin, tier fallback, unknown entries preservation | F-085…F-090 | transformSubscription/selectTier; TestPerRequestRoutingKeepsUnmappedProtocolsAndHealthyFallback, TestLeastPingRejectsStaleSamplesAndUsesFallback |
| Bounded append/prepend/remove/JSON Pointer и atomic audited injection CRUD | F-091 | ApplyInjections/mutateFeature; TestApplyInjectionsURIAppendPrependRemove, TestApplyInjectionsJSONPointerAndEndpointPatch, TestInjectionAuditFailureRollsBackMutation |
| Quota accounting/periods/timezones/restart/replay/inbound overlap | F-092 | quotaUsage/Manager; TestCalendarQuotaTimezones, TestQuotaTelemetryUsesCommittedUsageAndSurvivesRestart, TestNodeQuotaDistinguishesOverlappingAndAdjacentIntervals; enforcement исключения ноды этим не доказан |
| Trusted proxy chain, session/CSRF/viewer restrictions, Secure cookie | F-071/F-103/F-181 | clientIP/authenticate; TestTrustedProxyChainCannotBeSpoofed, TestAuthenticationCSRFAndRoleEnforcement, TestSecureCookieTracksPublicEndpoint |
| One-use enrollment, node identity, revocation, encrypted spool restart/corruption bounds | F-107 | enroll/agentAuth/revokeAgent/persistSpool; TestEnrollmentOneTimeIdentityRevocation, TestEncryptedSpoolReplaysSameBatchAfterFailedSendAndRestart, TestSpoolRejectsCorruptionWrongTokenAndNode |
| QueryStats reset=false, rates/counter reset без spike на gRPC fixture | F-111 | StatsClient.queryLocked/QueryStats; TestRealGRPCStatsContractDeltaAndCounterReset |
| Encrypted event ingestion + batch replay + audited read, unambiguous alias purge, escaped CSV | F-136/F-155…F-157 | telemetry/queryActivity/purgeActivity; TestTelemetryIdempotencyEncryptionAndReadAudit, TestActivityPanelLinksResolveXrayIdentitiesAndPurge, TestCSVExportEscapesSpreadsheetFormula |
| Локальный MMDB/prefix lookup и last-good reload | F-067/F-069/F-073 | Engine.Lookup/loadMMDB/LoadPrefixes; TestPrefixLongestMatchAndLastGoodReload, TestMMDBLookupReloadAndSourceSwitch |
| Consistent WAL encrypted backup и decrypt/reopen integrity на fixture | F-121 | DB.Backup/createBackup; TestBackupRestoresCommittedWALData, TestEncryptedBackupRestoresRealDatabase; полноценный CLI safe restore отдельно не проверен |
| Block-check target/TLS и YouTube URL/redirect restrictions | F-096/F-098 | blockCheck/youtubeCheck; TestBlockCheckRestrictsTargetToSyncedNodeAndHonorsTLS, TestYouTubeAPIDoesNotFollowRedirectToInternalService; DPI/region classification этим не доказана |
| Запуск Core и embedded HTTP assets/demo API | F-101/F-165 | Ручной subprocess: -demo -database :memory: -listen 127.0.0.1:<free>; GET health/version/assets, login и nodes/users/settings/live/activity = 200. БД, ключи и реальная панель не использовались; браузер не проверено |

### Каталог evidence

Ссылки E-* в каждой F-строке раскрывают файлы/функции, имена тестов и способ проверки. Полные Test* выполнялись общей командой `go test -json ./...`; повторные targeted runs не требовались.

<a id="e-activity"></a>

**E-activity.** Файлы/функции: core/internal/server/telemetry.go: telemetry/queryActivity/readActivity/exportActivity; activity_access.go: recordActivityAccess/parseActivityFilters/allowActivityRead/purgeActivity; core/internal/server/identity.go: activityIdentities; core/internal/server/sync.go: background; core/internal/activity/journal.go: отдельный Journal.

Тест / проверка: R11 TestOwnerActivityAccess (owner read/export data, purge, encrypted backup, anonymous401/cookie CSRF403, no role field) RED→GREEN; make check/security review PASS2026-10-14; TestTelemetryIdempotencyEncryptionAndReadAudit; TestActivityPanelLinksResolveXrayIdentitiesAndPurge; TestActivityIdentitiesDoNotExpandAmbiguousAliases; TestCSVExportEscapesSpreadsheetFormula.

Ограничения: Runtime хранит AES-GCM JSON по событию в SQLite. Journal создан, методы не вызываются. Нет agent coalescing/chatter/dictionaries/columnar segments/bloom/WAL journal ack/tier compaction/disk quota/legal hold/capture switches/quality/categories/session scrubber. q/group не поддерживаются и отклоняются; API export JSON/CSV, не NDJSON; permissions/TOTP отменены Q6; R12 final read/export audit хранит encrypted normalized range/filters/limit/format/count и возвращает details владельцу; shared60/min, invalid400/audit failure500; TestActivityReadExportAuditFilters/SharedRateLimit/InvalidFiltersAreNotSuccessfulReads/AuditIntegrityAndQueryFailure/ReadBudgetRecovery PASS. R12 key rollover: current encrypt + до8 previous decrypt, legacy activity/audit/cache/backup read и new current-key writes — TestActivityKeyRotation PASS; private regular32-byte keys, FIFO/symlink rejection PASS. Автоматическое перешифрование/retirement и bulk alerts отсутствуют. Domain substring по-прежнему после SQL LIMIT.

<a id="e-agent"></a>

**E-agent.** Файлы/функции: agent/internal/agent/agent.go: Start/collectSamples/deliveryLoop/persistSpool; agent/internal/enrollment/enrollment.go: Enroll; core/internal/server/telemetry.go: enroll/agentAuth/heartbeat/revokeAgent.

Тест / проверка: R09 TestEnrollmentAndSyncAuditFailures (issuance/enroll/manual-sync rollback when audit fails, retry/success/failure outcomes); TestBackgroundSyncDoesNotRequireAdminAudit (non-demo synthetic snapshot commits despite abort audit trigger); Linux make check PASS2026-10-14, read-only security review без findings. TestEnrollmentOneTimeIdentityRevocation; TestEncryptedSpoolReplaysSameBatchAfterFailedSendAndRestart; TestTelemetryBatchBoundsAndExplicitOverflowCount; TestRevokedCredentialPreservesPendingForReenrollment; TestTelemetryRejectsExpiredOrFutureSendTime.

Ограничения: Outbound HTTP JSON, не stream/protobuf+zstd. Нет downloadable signed one-liner, probe subcommand, backoff/auto-update/mTLS/nft plugins; heartbeat heap Alloc не RSS, нет CPU/disk/NIC/conntrack/retransmits/process metrics.

<a id="e-antiscan"></a>

**E-antiscan.** Файлы/функции: core/internal/antiscan/antiscan.go: AnalyzeRequest/AnalyzeHandshakeFailures/ApplyTarpit; core/internal/server/server.go: subscription handler.

Тест / проверка: целевого теста не найдено; runtime поведение не проверено.

Ограничения: UA deny подключён, не проверено. AnalyzeRequest не вызывает ApplyTarpit, поэтому escalation не включает delay. Нет node handshake pipeline, subnet/ASN correlation, config/UI allowlist, firewall export/Telegram topic; hardcoded UA deny включает curl/wget.

<a id="e-auth"></a>

**E-auth.** Файлы/функции: core/internal/server/auth.go: login/authenticate/validOrigin/bootstrapAdmin; core/internal/config/config.go: AuthConfig.

Тест / проверка: R10 TestSingleOwnerPasswordLogin, TestOwnerPasswordOnlyConfiguration RED→GREEN; TestAuthenticationCSRFAndOwnerEnforcement; TestSecureCookieTracksPublicEndpoint; Linux make check PASS2026-10-14, read-only security review без findings. Coverage server61.6%, config70.9%, ниже80%.

Ограничения: Q6 отменяет TOTP/WebAuthn/роли/группы: одна password-only учётка с полным доступом. Legacy extra accounts блокируют startup/login/session без удаления; stored role игнорируется. R11 убрал role из principal/login/me и operational checks/UI; TestOwnerActivityAccess PASS, anonymous401/CSRF403 сохранены. PBKDF2 вместо argon2id; отдельные API keys/lockout-duration не реализованы; secrets/TLS/full activity audit не доказаны полностью.

<a id="e-backup"></a>

**E-backup.** Файлы/функции: core/internal/db/db.go: Backup; core/internal/server/api.go: createBackup/downloadBackup; core/cmd/main.go: restoreBackup; core/internal/backup/remote.go: отдельные Store/Prune.

Тест / проверка: TestBackupRestoresCommittedWALData; TestEncryptedBackupRestoresRealDatabase.

Ограничения: Локальный encrypted WAL snapshot доказан. Snapshot включает activity, вопреки default exclusion. CLI restore не проверен этим тестом: только signature/header, не integrity/transaction/version-safe restore. Нет schedules/rotation/panel backup/TG split/S3 SigV4/runtime remote destinations.

<a id="e-block"></a>

**E-block.** Файлы/функции: core/internal/blocks/detector.go: Probe/Classify; core/internal/server/features.go: blockCheck.

Тест / проверка: TestBlockCheckRestrictsTargetToSyncedNodeAndHonorsTLS.

Ограничения: Проверяется target restriction/TLS, не DPI detection. Core читает сокет без запроса, EOF до 64KiB может дать ok. Нет regional probe binary/proxy/vantage matrix, xray client transfer, alert/quarantine actions; 16–20 signature не проверена.

<a id="e-deploy"></a>

**E-deploy.** Файлы/функции: deploy/install.sh; deploy/Dockerfile.core; deploy/Dockerfile.agent; deploy/compose*.yaml; deploy/subgate-*.service; deploy/Caddyfile; deploy/nginx.conf.example; .github/workflows/ci.yml.

Тест / проверка: целевого теста не найдено; runtime поведение не проверено.

Ограничения: Файлы существуют; installation/Docker/Linux multiarch/CI run не проверено. Installer local-only/checksum optional/version 0.2.0, без signature/download/wizard/bootstrap token; Caddy external ACME, Core TLS static cert; fallback absent.

<a id="e-drop"></a>

**E-drop.** Файлы/функции: core/internal/db/schema.sql: baselines (только схема); core/internal/server/sync.go: background; web/app.js: alerts registry.

Тест / проверка: целевого теста не найдено; runtime поведение не проверено.

Ограничения: Нет detector runtime: median/MAD/EWMA, learning, T-minute evaluation, maintenance/context/recovery; таблица baselines не реализация.

<a id="e-gateway"></a>

**E-gateway.** Файлы/функции: core/internal/gateway/gateway.go: Gateway.HandleRequest/serve; core/internal/server/server.go: routes; deploy/nginx.conf.example: subscription-only reverse-proxy fallback.

Тест / проверка: R08 TestStaleSnapshotServesWhileRefreshRunsInBackground; TestPolicyVersionSeparatesCacheEntries; TestSWRRefreshCoalescesAndClearPreventsResurrection; TestLateSWRRefreshCannotUndoAuthoritativeForeground; TestAuthoritative4xxWithTruncatedBodyInvalidatesSnapshot;  TestOpaquePassthroughAndPersistentFallback; TestDeviceIsolationAndInvalidIdentity; TestExpiredSnapshotNeverFallsBack; TestConcurrentRequestsCoalesceAndReleaseIdleLocks; R07 TestKilledCoreSubscriptionFallback (opt-in Docker nginx integration, PASS 2026-10-14); `make check` PASS; R07 security re-review без findings.

Ограничения: R08 реализует TTL30s/SWR5min с максимум32 background refresh, CAS/generation и отдельным namespace setter; runtime policy-store version binding, full policy fail-open matrix и Caddy fallback не закрыты. Nginx fallback ограничен `/sub/<id>[/<clientType>]`, GET/HEAD; при остановке Core сохраняет method/query, очищает client/proxy credentials, проверяет TLS origin и не перехватывает API/console. Fallback bypasses Core policy checks by design; panel remains authoritative. R07 использует synthetic origin и pinned nginx digest; production TLS/certificate deployment отдельно не проверены.

<a id="e-geo"></a>

**E-geo.** Файлы/функции: core/internal/server/api.go: geoDenied/validateObject; core/internal/server/clientip.go: clientIP; core/internal/geoblock/geoblock.go: отдельный Engine.

Тест / проверка: TestTrustedProxyChainCannotBeSpoofed (только IP trust, не geo enforcement).

Ограничения: CIDR/country/ASN/network_type ветви есть, geo enforcement не проверено отдельно. Нет user exemptions/policy scope/unknown grace/decoy/response switch и source L1+L2; порядок по object ID вместо priority; utility Engine не подключён.

<a id="e-health"></a>

**E-health.** Файлы/функции: core/internal/server/health_schedule.go: initializeHealth/startBackground/scheduledHealth/scheduledNodeProbe; core/internal/server/probes.go: probeNode/probeInbound; core/internal/health/health.go: Engine.RecordProbeResult/ProbeNode.

Тест / проверка: TestProbeRequiresTLSHandshakeAndMarksUDPUnverified; TestQuarantineRemainsAndRecovers; TestTCPDoesNotCountAsTLS.

Ограничения: R13 Core запускает independent scheduled TCP/TLS cycle (default30s/8s,4 workers/16 endpoints), state+probe atomic, restart restore и failure rollback. TestScheduledHealthTransitions/UnknownCancellationAndAtomicFailure/IncompleteEndpointsCannotQuarantine PASS; неполный coverage без success unverified. R13 configurable quorum1..64 (default1): fresh90s Core+token-bound Agent votes, latest source replace, revoked/rotated/stale exclude; consensus+transition atomic. TestProbeQuorum/VotesCannotRegressOrMultiply/RevocationAndTransitionAreSerialized и TestAgentReportsActualHealthProbes/CompletedProbesFromPartialCycle PASS. Agent outbound4worker/30s cycle, partial completed reports + unfinished-target cursor. M/min-time/backoff, maintenance, routing exclusion, recovery alerts и credentialed transports ещё отсутствуют; TCP/TLS reachability не доказывает transport authentication.

<a id="e-injection"></a>

**E-injection.** Файлы/функции: core/internal/routing/injection.go: ApplyInjections; core/internal/server/features.go: putInjection/injectionRules/mutateFeature; core/internal/server/routing.go: transformSubscription.

Тест / проверка: TestApplyInjectionsURIAppendPrependRemove; TestApplyInjectionsJSONPointerAndEndpointPatch; TestApplyInjectionsPreservesUnknownAndEnforcesLimits; TestInjectionAuditFailureRollsBackMutation; TestInjectionAndQuotaVerticalSlice.

Ограничения: Нет версии/rollback/dry-run/diff/templates/RFC6902; PUT не принимает scope/group/policy/user. Не применяются без выбранной ноды. Нет status/client-core gating, WG/AWG pool и merge subscriptions.

<a id="e-ip"></a>

**E-ip.** Файлы/функции: core/internal/ipintel/ipintel.go: Lookup/LoadPrefixes/loadMMDB/Health; core/internal/server/api.go: lookup; core/internal/server/ipintel.go: reloadIntel; web/app.js: IP page.

Тест / проверка: TestPrefixLongestMatchAndLastGoodReload; TestMMDBLookupReloadAndSourceSwitch.

Ограничения: Локальные JSON/MMDB/reload есть. Нет feed download/merge builder/checksums/L1-L5/scheduling/curated editor/CSV/CLI/bulk API/coverage report/<=5us benchmark; stale warning 45 d, не configurable N; hot-swap under load не проверено.

<a id="e-logs"></a>

**E-logs.** Файлы/функции: pkg/xray/logs.go: ParseLine/Tail; agent/internal/agent/agent.go: logsLoop; core/internal/logs/classifier.go: отдельные ParseLine/Aggregator.

Тест / проверка: целевого теста не найдено; runtime поведение не проверено.

Ограничения: Agent подключает только access-log, классификатор не импортирован. Нет all-level/DNS/journal/json-file pipeline/live tail/signals/unparsed counters/fuzz/golden/rotation torture; >1MiB прерывает Tail, channel send блокирует tailer, нет skip-ahead gap. Xray log sink не меняется.

<a id="e-metrics"></a>

**E-metrics.** Файлы/функции: core/internal/metrics/metrics.go: Registry/Counter/Histogram; core/internal/server/server.go: /metrics.

Тест / проверка: целевого теста не найдено; runtime поведение не проверено.

Ограничения: Public text exporter есть, не проверено запросом. Нет 30+ fleet gauges, Grafana dashboards/sync watchdog; сторонние observability tools не обязательная зависимость.

<a id="e-missing"></a>

**E-missing.** Файлы/функции: Проверено дерево core/agent/pkg/web/deploy и маршруты Server.routes; реализации/теста для указанного поведения не найдено.

Тест / проверка: целевого теста не найдено; runtime поведение не проверено.

Ограничения: Не реализовано; runtime не проверено.

<a id="e-notify"></a>

**E-notify.** Файлы/функции: core/internal/notify/notify.go: Telegram.Send/Dispatcher.Send; core/internal/server/features.go: notificationTest; core/internal/server/server.go: New.

Тест / проверка: целевого теста не найдено; runtime поведение не проверено.

Ограничения: Есть HTTP sender + dedupe/cooldown/quiet hours/test endpoint; доставка не проверено. Нет polling/commands/buttons/topics, RU/EN, proxy/email/webhook fanout/recovery/automatic event producers. AllowedUserIDs используются как ChatIDs, не command authorization.

<a id="e-origins"></a>

**E-origins.** Файлы/функции: pkg/wire/wire.go: Activity; pkg/xray/logs.go: ParseLine; core/internal/server/telemetry.go: telemetry/queryActivity; web/app.js: renderActivity.

Тест / проверка: R11 TestOwnerActivityAccess (owner read/export data, purge, encrypted backup, anonymous401/cookie CSRF403, no role field) RED→GREEN; make check/security review PASS2026-10-14; TestTelemetryIdempotencyEncryptionAndReadAudit; TestActivityPanelLinksResolveXrayIdentitiesAndPurge (только events/aliases).

Ограничения: Есть src_ip в activity. Нет sub-request origin pipeline, map/bundled geo, origin enrichment/aggregations/history/CGNAT, per-tier origin retention/no-IP/hash switches.

<a id="e-panel"></a>

**E-panel.** Файлы/функции: core/internal/panel/client.go: Client.Request/ListUsersContext/ListNodesContext/Action; core/internal/server/sync.go: syncPanel.

Тест / проверка: TestRemnawaveV3Contract; TestPanelRejectsInvalidContractAndStatus; TestPanelSyncCommitsCompleteSnapshotAndPreservesLastGood.

Ограничения: Ручной клиент вместо ogen; нет проверки версии при старте, scopes matrix, полного panel-native каталога; тестовый HTTP fixture не доказывает совместимость живой панели.

<a id="e-perf"></a>

**E-perf.** Файлы/функции: Makefile: bench; .github/workflows/ci.yml; core/internal/tsdb/live.go; core/internal/server/live.go.

Тест / проверка: целевого теста не найдено; runtime поведение не проверено.

Ограничения: Benchmark/profile/report/regression gates отсутствуют (rg Benchmark не находит функций). Бюджеты из требований не измерены; make bench без benchmark functions не доказательство.

<a id="e-pin"></a>

**E-pin.** Файлы/функции: core/internal/server/routing.go: transformSubscription; core/internal/server/api.go: validateObject(pins).

Тест / проверка: TestPerRequestRoutingKeepsUnmappedProtocolsAndHealthyFallback.

Ограничения: Только ручной user->node pin с проверкой online. Нет weighted rendezvous, initial assignment, sticky return, списков/modes, pinned-user cap, rebalance/dry-run/change budget/exit metric.

<a id="e-quality"></a>

**E-quality.** Файлы/функции: существующие *_test.go; core/internal/routing/transform_test.go: FuzzTransform; .github/workflows/ci.yml; scripts/validate-linux.py.

Тест / проверка: 76 Test* PASS; FuzzTransform seed corpus PASS; TestLiveStatsService SKIP; go build/vet/test PASS.

Ограничения: Нет fuzz campaign всех parsers/golden protocol/log corpus/record-replay drift/real-Xray CI/Compose E2E; race не запущен: cgo disabled; coverage server57.2%, routing68.3%, agent47.2%; staticcheck/golangci/frontend typing/license/perf CI отсутствуют.

<a id="e-quota"></a>

**E-quota.** Файлы/функции: core/internal/quota/quota.go: Manager/Normalize; core/internal/server/features.go: quotaUsage/putQuota; core/internal/server/routing.go: transformSubscription.

Тест / проверка: TestQuotaTelemetryUsesCommittedUsageAndSurvivesRestart; TestCalendarQuotaTimezones; TestNodeQuotaUsesInboundFallbackWithoutDoubleCountingUsers; TestQuotaUsesSampleTimeAcrossPeriodsAndFailsOpenOnStorageError; TestInjectionAndQuotaVerticalSlice.

Ограничения: Учёт/периоды/таймзоны/replay есть. Ключевой пробел: quota-exhausted возвращает исходную подписку с исчерпанной нодой. Нет panel usage source, 80/90% alerts, quota-aware выбор следующей ноды; rolling фиксирован 30 d; identity history/usage retention отсутствуют.

<a id="e-routing"></a>

**E-routing.** Файлы/функции: core/internal/server/routing.go: transformSubscription/selectTier; core/internal/routing/transform.go: Transform; core/internal/server/api.go: validateObject.

Тест / проверка: TestPerRequestRoutingKeepsUnmappedProtocolsAndHealthyFallback; TestLeastPingRejectsStaleSamplesAndUsesFallback; TestJSONAndYAMLRepairSelectors; TestPolicyInvalidConditionsAndTierReferenceCannotBeDeleted.

Ограничения: Явные node_ids; условия только country/ASN/UA. Нет squad/tag/plan/operator/network-type layouts, burstObservatory, полноценного balancer strategy rewrite и automatic quarantine.

<a id="e-speed"></a>

**E-speed.** Файлы/функции: pkg/xray/stats.go: queryLocked/QueryStats/QueryInboundSpeeds/ComputeSpeed; agent/internal/agent/agent.go: collectSamples/Start; core/internal/tsdb/live.go: LiveStore; core/internal/server/live.go: live/traffic/liveStream.

Тест / проверка: TestRealGRPCStatsContractDeltaAndCounterReset; TestTelemetryIdempotencyEncryptionAndReadAudit; TestQuotaTelemetryOfflineReplayPreservesSampleTimestamp.

Ограничения: 1s counters/EMA/history/CSV/SSE есть частично. EMA avg по 12 recent samples, не полная минута; нет adaptive sampling/watched mode/coarse panel fallback/WebSocket diff/uPlot/virtualization/rollup 30d+1y/NIC capacity/alerts. LiveStore не имеет hard byte cap; perf/iperf accuracy/skew не проверено; real Xray test skipped.

<a id="e-storage"></a>

**E-storage.** Файлы/функции: core/internal/db/db.go: New/BeginTx; core/internal/db/migrations.go: RunMigrations; core/internal/db/schema.sql; core/internal/server/server.go: initialize; core/internal/server/sync.go: background.

Тест / проверка: TestFailedTransactionReleasesWriter; TestBackupRestoresCommittedWALData; TestPanelSyncCommitsCompleteSnapshotAndPreservesLastGood.

Ограничения: modernc SQLite WAL и serialized mutex, не writer goroutine/sqlc/goose. Многие таблицы лишь схема; high-volume writes per-row loops, нет partitions/rollups/pre-migration backup; sg_usage/sg_speed_meta/diagnostics без cleanup; sg_speed 24h, sg_activity HotTTL.

<a id="e-ui"></a>

**E-ui.** Файлы/функции: web/embed.go; web/app.js; web/styles.css; core/internal/server/server.go: routes; core/internal/server/api.go: settings.

Тест / проверка: Ручной запуск Core -demo -database :memory:: GET /, /console, /app.js, /styles.css, demo login, GET /nodes,/users,/settings,/live,/activity = 200; node --check web/app.js PASS.

Ограничения: Vanilla JS/CSS, не Preact/TS/Vite. HTTP/синтаксис проверены; браузер UX, mobile, WCAG, performance не проверено. Нет maps/Logs/Blocks/Injections/Quotas/Anti-scan/Operations отдельных страниц, wizard, RU/EN, light theme, update; settings GET-only.

<a id="e-webhook"></a>

**E-webhook.** Файлы/функции: core/internal/server/sync.go: webhook/background.

Тест / проверка: R08 TestWebhookRefreshesUserBeforeDeadline (native full projected fields, <1s); TestWebhookDuringPanelFetchStaysFresh (valid REST snapshot и exact superseded assertion); TestWebhookRejectsMalformedUserWithoutMutation; TestWebhookRejectsPartialUserProjection; TestWebhookUnknownEventDoesNotUpsertUser; TestWebhookUserPersistenceRollsBackReplayMarker; TestWebhookPreservesCanonicalNumericIdentityAndEventOrder; TestWebhookOlderThanCompletedSyncCannotRegressUser; TestWebhookOlderThanCompletedSyncCannotResurrectDeletedUser; TestWebhookHMACReplayAndSnapshotInvalidation. Targeted gateway/server/panel suites и Linux `make check` PASS2026-10-14; final read-only security review без findings.

Ограничения: обновляется только user.updated. Unknown events сохраняют прежний invalidation-only behavior. Webhook contract проверен по remnawave/backend@3.4.5 libs/contract/models/webhook/webhook.schema.ts и src/queue/notifications/webhook-logger/webhook-logger.processor.ts (native camelCase, HMAC raw body). Marker+user+clock атомарны; numeric canonical ID сохраняется при совпадении native id/shortUuid. Watermark full-sync и per-user event timestamp требуют согласованных часов Core/panel; полевой SLA/live webhook deployment не проверены. Q8 задаёт polling1min; jitter/fan-out/re-signing ещё отсутствуют. Coverage ранее наблюдена gateway87.6%, panel66.2%, server60.7%, ниже skill target80% для двух последних; это не полная release readiness.

<a id="e-youtube"></a>

**E-youtube.** Файлы/функции: core/internal/youtube/check.go: Check/ParseRegion; core/internal/server/features.go: youtubeCheck.

Тест / проверка: TestYouTubeAPIRejectsUntrustedEndpoints; TestYouTubeAPIDoesNotFollowRedirectToInternalService (только URL restrictions).

Ограничения: Исполняется с Core; node_id только метка. Нет per-node egress scheduling, forbidden-region transitions, stop/cooldown, alerts/policy actions; parsing/region в живом YouTube не проверено.

