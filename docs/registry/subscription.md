# Quotas и функции подписки в middleware
О чём: F5/F6/F29–F31, пользовательские подписки и panel-native gates.
Когда читать: exhausted node, extra configs, WG pool, HWID/client rules, grace squad.

## Quotas F6
Код: core/internal/quota/quota.go, server/features.go:quotaUsage, routing.go.
Calendar day/week/month с timezone, week Monday; rolling фиксирован30d.
Rule scope per_user/per_node, positive limit, grace>=0, persisted telemetry usage/replay/correction и transaction rollback есть.
Inbound fallback не double-count user intervals; alias ambiguity не даёт списать usage чужому user.
Storage error fail-open; reset counters/менять Remnawave accounting запрещено.
TestQuotaTelemetryUsesCommittedUsageAndSurvivesRestart/OfflineReplayPreservesSampleTimestamp/CorrectionRemovesPreviousUsage,
TestNodeQuotaUsesInboundFallbackWithoutDoubleCountingUsers, calendar/timezone/overflow tests PASS.
Но selected exhausted node → exact original с quota-exhausted; host не удаляется.
Q4 требует заменить конкретный exhausted host на «<флаг> У ВАС ЗАКОНЧИЛСЯ ТРАФИК», eligible fallback проверить.
All exhausted должна обрабатывать сама Remnawave; B02 требует проверить её stub на test panel, свою fallback policy не придумывать.
Нет panel per-user/per-node usage source, 80/90% warning transitions, scope priority/usage cleanup и configurable rolling window.
Enforcement только при refresh, это не data-plane cut-off.

## Функции, переданные middleware (2026-10-31)

F5: инъекции append/prepend/patch/remove и xray-profile, их CRUD и UI вынесены из активного кода.
Оригиналы и тесты: [архив](../../_archive/middleware-overlap-2026-10-31/README.md).
Существующие записи SQLite сохранены, но больше не читаются трансформером.
Произвольный Xray JSON и JSON Pointer не имеют точного аналога в PHP middleware; конфиги переносить вручную.

F5b WG/AWG pool, F5c merge, F29 grace, F30 HWID/request log/client versions,
F31 per-app response rules, subscription-page, encrypted channel и support widget обслуживает middleware.
В SubGate эти возможности не были реализованы; повторно планировать их реализацию не требуется.
Forwarding HWID/device metadata сохраняется для совместимости; оно не реализует HWID policy.

Сравнение по исходникам и порядок подключения: [middleware-integration](../middleware-integration.md).

## Scope и проверки
Encrypted channel, decoy/masking page и support widget относятся к middleware.
Любая mutation должна иметь exact shape validation, audit final outcome и adversarial fixtures.
Функции шире CRUD оцениваются PARTIAL до behavioural workflow и test evidence, историческая сверка inventory — [архив FEATURE_STATUS](../_archive/triage-2026-10-30/docs/FEATURE_STATUS.md).
