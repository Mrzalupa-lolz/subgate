# Реестр: quotas

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 8. Functional requirements / F6 — Per-node traffic quotas

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-092 | Quota = (node, scope) × bytes × period (day/week/month, calendar or rolling, time zone). Source: panel per-user/per-node usage with fallback to agent accounting (F16). Over quota ⇒ node removed from **that user's** next subscription; warnings at 80/90 %. Document that enforcement happens at refresh time.<br>Источник: [v4:168](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-quota](EVIDENCE.md#e-quota); TestQuotaTelemetryUsesCommittedUsageAndSurvivesRestart; TestCalendarQuotaTimezones; TestNodeQuotaUsesInboundFallbackWithoutDoubleCountingUsers; TestQuotaUsesSampleTimeAcrossPeriodsAndFailsOpenOnStorageError; TestInjectionAndQuotaVerticalSlice | Учёт/периоды/таймзоны/replay есть. Ключевой пробел: quota-exhausted возвращает исходную подписку с исчерпанной нодой. Нет panel usage source, 80/90% alerts, quota-aware выбор следующей ноды; rolling фиксирован 30 d; identity history/usage retention отсутствуют. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-264 | Per-node quotas, geo-block, subscriber map, 16–20, drops, YouTube region, anti-scan, hot settings, console — F6–F13 (v4)<br>Источник: [inventory:94](../../subgate-feature-inventory.md) | **PARTIAL** | [E-quota](EVIDENCE.md#e-quota); TestQuotaTelemetryUsesCommittedUsageAndSurvivesRestart; TestCalendarQuotaTimezones; TestNodeQuotaUsesInboundFallbackWithoutDoubleCountingUsers; TestQuotaUsesSampleTimeAcrossPeriodsAndFailsOpenOnStorageError; TestInjectionAndQuotaVerticalSlice | Учёт/периоды/таймзоны/replay есть. Ключевой пробел: quota-exhausted возвращает исходную подписку с исчерпанной нодой. Нет panel usage source, 80/90% alerts, quota-aware выбор следующей ноды; rolling фиксирован 30 d; identity history/usage retention отсутствуют. |
