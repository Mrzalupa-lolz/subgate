# Учёт квот и передача подписок middleware

Актуальное решение владельца 2026-11-01: выдача подписок, каталог пользователей, HWID/UA запросов, grace, конфиги и клиентские страницы находятся в remnawave-subscription-middleware. SubGate их не дублирует. Gateway/tiers/policies/pins/requestmonitor перенесены в [архив](../../_archive/monitoring-scope-2026-11-01/README.md).

## Учёт квот по телеметрии

Код: core/internal/quota/quota.go, server/features.go:quotaUsage. Calendar day/week/month с timezone, week Monday; rolling фиксирован 30d. Scope per_user/per_node, positive limit, grace>=0, persisted telemetry usage/replay/correction и transaction rollback. Inbound fallback не double-count user intervals; alias ambiguity не даёт списать usage чужому user.

Квоты не меняют подписки и не выполняют data-plane cutoff. Прежнее требование заглушки exhausted host относится к удалённому контуру. Автоматическое временное ограничение подтверждённых нарушителей — отдельный будущий F25, не следствие исчерпания квоты.

Сохраняются тесты accounting, replay, correction, calendar/timezone/overflow. Нет panel per-user/per-node usage source, 80/90% warning transitions и configurable rolling window.

[Распределение функций](../middleware-integration.md), [план мониторинга](../PROJECT_PLAN.md).
