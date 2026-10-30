# Отчёт реорганизации SubGate
Ветка docs/reorg. Baseline текущего рабочего дерева сохранён коммитом dd91b66 до изменений документации.
Дата аудита17.10.2026; запуск начат16.10.2026. Это docs-аудит, runtime feature development не выполнялась.

## Результат
12 файлов registry по4–6KiB; отдельный docs/QUESTIONS.md; AGENTS58 строк, INDEX35 строк.
Единственный план:17 этапов/386 пунктов, завершённые узкие части отмечены[x], новые blockers[!], остальное с assertions/criteria.
Длительность15–40min — оценка узкого прохода; перед большой fixture задачей агент обязан разделить её, не растягивать scope.
Старые289 audit IDs не равны36 независимым функциям; полные формулировки/ID/source lines сохранены в архиве.
FEATURE_STATUS покрывает36 Part B и37 Part A references, а также разбирает составные F2–F4/F6–F13.

Part B: DONE0, PARTIAL11, MISSING23, EXCLUDED2. Это не процент готовности.
Работают проверенные части: HEAD/metadata, transformer panic/deadline, SWR/coalescing, readonly panel/users/version,
owner/CSRF/audit, encrypted spool/usage, TCP/TLS/HTTP transports включая synthetic HTTP3.
Нет полного mirror/merge/pool/grace/HWID/presets, origins/maps, abuse7 analysers, protection/optimizer/catalogue27/19,
текстового Xray editor, support/PWA/plugins; quota stub/compact activity/safe restore/real-load тоже не завершены.
Детали с кодом и именами существующих тестов — [FEATURE_STATUS](FEATURE_STATUS.md).

## Слияния, разбиение, архив
Panel+UI+webhooks→panel-ui; Agent+deployment+fleet+optimizer→nodes-fleet.
Auth+geo+anti-scan+abuse+node protection→security; health45KiB разделён на health и transports.
Speed+metrics+diagnostics+drops→telemetry; logs+activity+storage→activity; IP+origins→ip-origins.
Gateway+routing+relay→gateway-routing; quotas+injections+devices+lifecycle→subscription.
Notifications+ops+backups+automation+support+plugins→ops; quality+performance+workflow→quality.
PROMPT/TOKEN_PLAN/token_report и competing ROADMAP/legacy INDEX перенесены в archive.
docs/adr и docs/runbooks на старте не содержали md; недоступные исторические файлы внешнего архива не читались.
Ранее удалённые4 markdown восстановлены только из git parent dd91b66^, не из запрещённого внешнего архива.

Оригиналы всех60 доступных markdown сохранены с путями/bytes/SHA256 в
[archive manifest](_archive/reorg-2026-10-17/manifest.json) и
[original-markdown.zip](_archive/reorg-2026-10-17/original-markdown.zip).
Видимые archive md имеют 4 исправления ссылок, ZIP сохраняет точные bytes.
Добавлен scripts/check-docs.py: ссылки/anchors,36+37 ID,архив SHA256,навигация/размеры/skills symlink.
Ни один source requirements/OpenAPI/runtime исходник не изменён; git не очищался/reset/force не применялся.
ARCHIVE_INDEX отделён от original README; originals доступны и как md, token_report сохранён в archive явно.
Новые справочники не повторяют устаревшие claims; потерянные прежним аудитом внешние файлы не восстановлены догадкой.
Skills source .agents/skills уже был единственным; добавлен .claude/skills→../.agents/skills. Codex source не перемещён.

## Размер
Метод: сумма st_size обычных файлов docs рекурсивно, без docs/_archive; каталогные blocks du не используются.
До: 405048 байт. После: 234522 байт. Снижение: 170526 байт (42.1%).
Рост archive ожидаем и исключён из метрики; originals/manifest позволяют обратимый review.
Крупный новый план читается по одному этапу; полный план не загружается на каждой итерации.
Автоматически проверить: python3 scripts/check-docs.py.

## Проверки
| Проверка | Результат | Предел |
|---|---|---|
| Markdown links/anchors + INDEX coverage | PASS | Root/docs включая архивные md; URL availability внешних сайтов не проверялась |
| Archive SHA256/bytes | PASS | Все60 originals в ZIP + неизменность v4/inventory; missing-link/anchor negative checks PASS |
|5 функций через AGENTS→INDEX | PASS5/5 | После уже предоставленного AGENTS: INDEX + один domain,2 открытия |
| go build ./... | PASS | Компиляция не доказывает features |
| go vet ./... | PASS | Статическая проверка Go |
| make lint | PASS | gofmt, node --check, bash -n |
| python3 scripts/generate-panel.py --check | PASS | Проверка drift generated client |
| go test ./... | FAIL | Baseline health regression, до docs mutations |
| go test -json -cover ./... | FAIL |176 top-level pass/5fail/7skip; ещё13skipped live descendants |
| go test -race ./... | FAIL | Health regressions; make check не дошёл до race, запущен отдельно |
| make check | FAIL | Остановился на test target |
| Security/diff review | PASS для docs change scope | No production calls/secret reads/runtime source edits; не полный product security audit |

Coverage: gateway87.8%,panel78.2%,server68.2%,health45.3%,healthprobe83.2%,agent52.0%,routing68.3%,
quota76.7%,ipintel72.7%,db70.5%,config75.5%,xray39.2%. Coverage target80% всего продукта не достигнут.
Parent live gRPC/XHTTP pass с all children SKIP не выдаётся за transport acceptance.
Отдельная race-проверка упала на тех же5 tests; сообщений DATA RACE не обнаружено.
7 top-level skips: live panel,live subscription,WS,HTTPUpgrade,H3,StatsService,nginx fallback.
5 failing tests: TestMaintenanceSuppression,TestMaintenanceSuppressesWindowEntirelyInsideSlowProbe,
TestQuarantineMinimumAndBackoffStartAfterSlowProbe,TestScheduledHealthUnknownCancellationAndAtomicFailure,
TestScheduledHealthIncompleteEndpointsCannotQuarantine; sql:no rows in result set.
Это записано в [B04](BLOCKED.md#b04-воспроизводимые-тесты); текущие исторические health PASS не подменяют этот FAIL.
Логи созданы только в /tmp/subgate-reorg-20261016/checks; старый repo tmp и внешние archives не читались.
Sandbox bwrap не стартовал, host execution после approval; исходный fail не скрыт.

| Sample inventory | Через INDEX | Результат |
|---|---|---|
|#1 subscriptions/cache|registry/gateway-routing.md|PASS: paths/cache/forwarding/code/tests |
|#5 WG/AWG pool|registry/subscription.md|PASS: отсутствующий workflow/requirements/next task |
|#16 activity/categories|registry/activity.md|PASS: runtime rows vs unconnected utility/retention |
|#21 threat lists|registry/ip-origins.md|PASS: L5/terms/provenance/last-good |
|#27 Xray editor|registry/transports.md|PASS: v1 text editor/highlight/completion/hover/version/offline |

## Решения агента и внимание владельца
D11 закрыл B01 как уже решённый запрет connector, не запросил повторное разрешение.
D13/D14 выбрали12 доменов/36-row реестр/ZIP originals, единственный INDEX/план; full Q1–Q10 дословно в DECISIONS.
D15 оставил optional/later задачи зависимыми от scope (B05), не исключил тихо из исходных требований.
HTTP3 найден в baseline code и synthetic tests: старое «ещё не реализован» исправлено; fresh live H3 не заявлен.
Не применены старые TOKEN_PLAN советы удалить OpenAPI/источники/skills или обещать неизмеренную token-экономию.
S3/email/self-update/abuse sanctions/RBAC/TOTP не перенесены обратно в активный план.
Внимание человека позже: B02 если panel не даёт all-exhausted stub; B05 optional scope/CRM граница; B07 legal hold90d/group conflict.
B04 исправляется агентом самостоятельно следующим этапом; B06 требует non-skipped synthetic/live/browser/load evidence.
Реорганизация проверена отдельно, но продукт и make check остаются NOT READY. Непроверенные функции не повышены до DONE.

## Аудит всех исходных markdown
Размеры до изменений; «архив» означает сохранение exact original, не безвозвратное удаление.
| Файл | Байт | Суть | Решение | Причина |
|---|---:|---|---|---|
| AGENTS.md | 2924 | Правила агента, чтение/команды/запреты | оставить; original в архиве | Актуализировать один canonical документ без потери original |
| CLAUDE.md | 208 | Указатель на правила | оставить: короткая ссылка AGENTS | Единые правила для обоих агентов |
| PROMPT.md | 5915 | Дублирующий автономный цикл с разрешением baseline/commits | архив; влить применимое в AGENTS/quality/план | Конкурирующие циклы/спорные рекомендации/историческая метрика |
| README.md | 3375 | Обзор, quick start, install/recovery, прежние gaps | оставить; original в архиве | Актуализировать один canonical документ без потери original |
| STATUS.md | 998 | Короткий статус R13–R15 и transport evidence | оставить; original в архиве | Актуализировать один canonical документ без потери original |
| codex-reorg-prompt.md | 7554 | Поручение текущей реорганизации | оставить | Авторские требования/поручение, не редактировать |
| docs/BLOCKED.md | 1562 | B01/B02/B03 и пояснение Q6 | оставить; original в архиве | Актуализировать один canonical документ без потери original |
| docs/DECISIONS.md | 1548 | Краткие Q1–Q10/read-only решения | оставить; original в архиве | Актуализировать один canonical документ без потери original |
| docs/ENVIRONMENT.md | 416 | Локальная test-машина и старые resource estimates | оставить; original в архиве | Актуализировать один canonical документ без потери original |
| docs/PROJECT_PLAN.md | 19839 | Статусы289 строк, обзор R01–R60 и историю проходов | оставить; original в архиве | Актуализировать один canonical документ без потери original |
| docs/TOKEN_PLAN.md | 18221 | Оценки tokens и предложения docs/skills cleanup | архив; влить применимое в AGENTS/quality/план | Конкурирующие циклы/спорные рекомендации/историческая метрика |
| docs/api.md | 27404 | Core HTTP routes/semantics с противоречивыми appendices | оставить; original в архиве | Актуализировать один canonical документ без потери original |
| docs/panel-contract.md | 6341 | Scopes3.4.5, generated users projection и versions | оставить; original в архиве | Актуализировать один canonical документ без потери original |
| docs/registry/AUDIT_HISTORY.md | 10599 | Происхождение audit/connector removal и прежние archives | влить registry/quality.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/INDEX.md | 11269 | Legacy289 ID→область/status | архив; docs/INDEX + FEATURE_STATUS | Не читать289 reference rows на старте |
| docs/registry/OPERATIONS.md | 9310 | Commands/config/install/restore и старые verification results | влить registry/quality.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/QUESTIONS.md | 10847 | 10 вопросов и полные ответы владельца | разбить: ответы→DECISIONS, вопросы→QUESTIONS | Ответы не должны оставаться открытыми |
| docs/registry/REGISTRY_GUIDE.md | 1279 | Смысл legacy IDs/reference строк | влить registry/quality.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/ROADMAP.md | 19575 | R01–R60 по1–2ч с future Test names | влить PROJECT_PLAN | Единственный план, узкие задачи с criteria |
| docs/registry/abuse.md | 2140 | Реестр: abuse | влить registry/security.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/activity.md | 2864 | Реестр: activity (F20 journal + storage) | влить registry/activity.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/agents.md | 5059 | Реестр: agents | влить registry/nodes-fleet.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/anti-scan.md | 2786 | Реестр: anti-scan | влить registry/security.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/automation.md | 1788 | Реестр: automation | влить registry/ops.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/backups.md | 4589 | Реестр: backups | влить registry/ops.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/deployment.md | 7997 | Реестр: deployment | влить registry/nodes-fleet.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/devices.md | 2935 | Реестр: devices | влить registry/subscription.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/diagnostics.md | 8070 | Реестр: diagnostics | влить registry/telemetry.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/drop-detection.md | 1001 | Реестр: drop-detection | влить registry/telemetry.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/fleet-operations.md | 2037 | Реестр: fleet-operations | влить registry/nodes-fleet.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/gateway.md | 14925 | Реестр: gateway | влить registry/gateway-routing.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/geo-protection.md | 2069 | Реестр: geo-protection | влить registry/security.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/health.md | 44944 | Реестр: health | разбить health/transports | 45KiB повторяющихся R13–R15 описаний |
| docs/registry/injections.md | 5084 | Реестр: injections | влить registry/subscription.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/ip-intelligence.md | 12648 | Реестр: ip-intelligence | влить registry/ip-origins.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/metrics.md | 2864 | Реестр: metrics | влить registry/telemetry.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/node-optimization.md | 1830 | Реестр: node-optimization | влить registry/nodes-fleet.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/node-protection.md | 3732 | Реестр: node-protection | влить registry/security.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/notifications.md | 3366 | Реестр: notifications | влить registry/ops.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/origins.md | 2689 | Реестр: origins | влить registry/ip-origins.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/panel.md | 12983 | Реестр: panel | влить registry/panel-ui.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/performance.md | 5371 | Реестр: performance | влить registry/quality.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/plugins.md | 645 | Реестр: plugins | влить registry/ops.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/quality.md | 17906 | Реестр: quality | влить registry/quality.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/quotas.md | 2512 | Реестр: quotas | влить registry/subscription.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/relay-topology.md | 678 | Реестр: relay-topology | влить registry/gateway-routing.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/routing.md | 8440 | Реестр: routing | влить registry/gateway-routing.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/scope.md | 7944 | Реестр: scope | влить registry/quality.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/security.md | 7476 | Реестр: security | влить registry/security.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/storage.md | 13736 | Реестр: storage | влить registry/activity.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/subscription-lifecycle.md | 1752 | Реестр: subscription-lifecycle | влить registry/subscription.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/support.md | 4650 | Реестр: support | влить registry/ops.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/telemetry.md | 16288 | Реестр: telemetry | влить registry/telemetry.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/ui.md | 14548 | Реестр: ui | влить registry/panel-ui.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/webhooks.md | 3519 | Реестр: webhooks | влить registry/panel-ui.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/xray-logs.md | 9750 | Реестр: xray-logs | влить registry/activity.md | Общий домен/уникальные факты вместо повторных таблиц |
| docs/registry/xray-toolkit.md | 3223 | Реестр: xray-toolkit | влить registry/transports.md | Общий домен/уникальные факты вместо повторных таблиц |
| subgate-ai-prompt-v4.md | 54763 | Исходный полный контракт продукта | оставить | Авторские требования/поручение, не редактировать |
| subgate-feature-inventory.md | 14437 | Part A reference и36 Part B rows | оставить | Авторские требования/поручение, не редактировать |
| token_report.md | 3749 | Исторические token usage цифры | архив; влить применимое в AGENTS/quality/план | Конкурирующие циклы/спорные рекомендации/историческая метрика |
