# Контур подписок передан middleware

Актуальное решение владельца 2026-11-01: SubGate не выдаёт подписки и не выполняет subscription routing, tiers/policies/pins, transforms, mirror или relay. Исходники и специализированные тесты сохранены в [архиве](../../_archive/monitoring-scope-2026-11-01/README.md).

Ноды и их доступность по-прежнему наблюдаются; health/quarantine — внутреннее состояние мониторинга, оно не меняет выдачу подписок. Учёт квот остаётся аналитикой трафика.

[Распределение функций](../middleware-integration.md), [план мониторинга](../PROJECT_PLAN.md), [health](health.md).
