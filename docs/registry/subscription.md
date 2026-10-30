# Quotas, injections, devices и lifecycle
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

## Injections F5
Append/prepend/patch/remove, uri/json/yaml detection, JSON Pointer path, max content256KiB, validation/CRUD+atomic audit есть.
TestApplyInjectionsURIAppendPrependRemove/Base64AndDisabled/JSONPointerAndEndpointPatch/PreservesUnknownAndEnforcesLimits,
TestInjectionAuditFailureRollsBackMutation PASS.
Runtime применяет правила только после selected policy/pin node; passthrough без selection не получает injections.
Нет full group/user/policy/format scopes, status gating blocked/expired/over-limit, client-core compat matrix,
RFC6902/Merge Patch полного контракта, sandbox templates, version/dry-run diff/rollback.
Поддержать WG/AWG/VLESS variants raw/ws/httpupgrade/grpc/h2/xhttp/mKCP/QUIC/REALITY/Vision, opaque future entries.
Опираться на subscription template панели; не вносить unsupported transport клиенту Happ/Incy/mihomo.

F5b WG/AWG pool per subscriber/device пока нет: one-key lifecycle, non-reuse/collision, revoke и cleanup нужны.
F5c merge двух upstream subscriptions отсутствует; требуется bounded fetch, preserving unknown/metadata, отказ без пустой подписки.

## Grace/devices/request catalogue
F29: expired→limited squad snapshot, restore-on-payment; runtime и тестов нет.
Только test panel writes, snapshot сохраняет исходные squads и выдерживает replay/partial failure; запрет возобновить оплаченный статус по догадке.
F30 HWID blocked stub, derivation from UA, cards, CUSTOM_SUB_PREFIX — не реализованы.
Текущее forwarding x-hwid и device-aware cache не являются HWID policy.
Panel-native HWID limit/history читать через exact API, не дублировать панель.
Request log who/when/client/decision/bytes/model+OS и version catalogue/outdated flags не реализованы.
Источники GitHub/App Store/Google Play/F-Droid из inventory сначала проверить terms; ограничить downloads/SSRF и last-good.
F31 per-app headers/UA/OS rules, Prizrak-Box pxa/pxd-template placeholders, Happ/Incy routing,
client/subpage template library отсутствуют; не путать upstream header passthrough с response-rule engine.

## Scope и проверки
Encrypted channel rotating path/key derived from sub URL/fixed-size ciphertext optional own-client-only; отсутствует, B05.
Decoy/masking page и Telegram support widget из Part B также отсутствуют; Core login screen не доказывает decoy.
Любая mutation должна иметь exact shape validation, audit final outcome и adversarial fixtures.
Функции шире CRUD оцениваются PARTIAL до behavioural workflow и test evidence, строки inventory — [FEATURE_STATUS](../FEATURE_STATUS.md).
