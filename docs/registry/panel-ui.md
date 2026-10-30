# Панель и консоль
О чём: Remnawave API, синхронизация, owner UI и настройка продукта.
Когда читать: panel contract, новые страницы, wizard, settings, PWA; код открывать по указанным путям.

## Контракт и реализация
Core — самостоятельный продукт, панель остаётся источником пользователей, подписок и panel accounting.
Код: core/internal/panel/client.go, inbound.go, version.go; core/internal/server/sync.go, panel_version.go; web/.
Актуальные методы/поля: [API summary](../remnawave-api-summary.md); scopes и generated projection: [panel contract](../panel-contract.md).
Из OpenAPI 3.4.5 используется generated users response decoder ogen1.24.0; HTTP и остальные readers ручные.
Нормализация исходных bytes сохраняет int64 counters и UUID precedence; full zero-reflection не достигнут.
Request не следует redirect, читает envelope response, ограничивает ответ 16 MiB, по умолчанию разрешает только GET.
Startup GET recap сравнивает версию с generated.SpecVersion, timeout3s; mismatch/unavailable видны в system/status и не останавливают подписки.
Sync публикует users/nodes/hosts атомарно; при ошибке остаётся last-good. Текущая минута reconciliation соответствует D08.
HMAC проверяется по raw body; replay marker и user update атомарны, старый webhook/full sync не перезаписывает новые данные.
Непосредственно применяется user.updated; прочим событиям ещё нужны отдельные projection tests. Fan-out/re-signing отсутствует.

## Проверенные части и границы
TestPanelSpecReadContract, TestGeneratedPanelUsersContract/Integrity, TestPanelVersionMismatch, TestPanelReadOnlyRejectsBeforeSending,
TestPanelSyncCommitsCompleteSnapshotAndPreservesLastGood, TestWebhookRefreshesUserBeforeDeadline имеют behavioral assertions и PASS этого аудита.
Live panel/subscription tests opt-in и в этом аудите SKIP; прежний synthetic PASS2026-10-14 — только историческая запись.
Scopes из test panel3.4.5 сохранены в panel-contract, не выдумывать новый scope по названию endpoint.
Нет полного panel-native каталога: HWID, torrent reports, request history, bandwidth, shared ASN/IP lists и billing nodes ещё не связаны с UI.
Чужие panel passwords Core не хранит; недоступный API должен давать понятную диагностику и deep link.

## Web UI
Встроены vanilla JS/CSS/HTML, SSE live, canvas history, поиск users/nodes, Agent enrollment, activity, IP lookup, pins/tiers/policies/geo forms.
Обычный JSON-редактор этих объектов не является Xray-редактором. Его требования находятся в [transports](transports.md).
Pages/fields проверены чтением web/app.js и Server.routes; node --check проверяет синтаксис, браузерное поведение отдельно не воспроизводилось.
Нет wizard, versioned settings mutations/rollback, отдельного Logs/live-tail, полной Blocks matrix, origin map и всего operations каталога.
Нужны Live/table virtualization10k+, RU/EN, light/dark, command palette/global search, keyboard, readable empty/loading/error states,
44px targets, WCAG AA, ARIA/focus, reduced motion, mobile layout, destructive confirmations и diff risky changes.
Не подменять продукт Preact/TypeScript ради соответствия старой таблице: D07 оставляет JS без измеренной причины менять стек.
Нет внешних CDN/fonts/tiles, данные карты и документация editor должны работать offline.
Требования UI: JS<=250KiB gzip, lazy heavy pages, единые CSS tokens, SVG icons, attribution DB-IP.
PWA и Web Push из Part B заменяют Android; сейчас manifest/service worker/push workflow отсутствуют.

## Продолжение
Задачи и зависимости только в [PROJECT_PLAN](../PROJECT_PLAN.md), статусы только в [FEATURE_STATUS](../FEATURE_STATUS.md).
Сначала исправить B04 и обеспечить безопасные confirmed test-panel mutations; затем settings/wizard/native features и browser gates.
