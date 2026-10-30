# Работа с SubGate
SubGate — самостоятельный продукт владельца вокруг Remnawave и VPN-нод.
Go Core + outbound Agent, SQLite, встроенный vanilla JS UI; Linux, скорость, надёжность, минимум слоёв.
Цель — заменить remnawave-admin; его коннектор запрещён.

## Навигация и источники
Старт: [docs/INDEX.md](docs/INDEX.md) → текущая задача [плана](docs/PROJECT_PLAN.md).
Требования: неизменяемые inventory Part B и prompt v4; согласованные уточнения владельца — docs/DECISIONS.md.
Реализацию доказывают код + assertions/PASS; FEATURE_STATUS хранит статусы, STATUS — короткий журнал.
Не читай весь registry/код: один домен и source/tests текущей задачи.

| Если делаешь | Найди через docs/INDEX.md |
|---|---|
| Panel API, sync, UI, wizard, PWA | registry/panel-ui.md; remnawave-api-summary.md |
| Auth, geo, anti-scan, F21/F23 | registry/security.md |
| Health/quorum/maintenance | registry/health.md |
| Xray probes, REALITY/HTTP3, editor F27 | registry/transports.md |
| Speed, TSDB, drops, diagnostics | registry/telemetry.md |
| Xray logs, activity, retention | registry/activity.md |
| IP/ASN, origins/maps | registry/ip-origins.md |
| Gateway, tiers, pins, relay | registry/gateway-routing.md |
| Quotas/injections/grace/HWID/presets | registry/subscription.md |
| Agent/install/fleet/optimizer | registry/nodes-fleet.md |
| Operations/Telegram/backups/automation | registry/ops.md |
| DONE/CI/perf/chaos/verification | registry/quality.md |

## Команды
    go build ./...
    go vet ./...
    go test ./...
    make check
    python3 scripts/check-docs.py
    go run ./core/cmd -demo -database :memory: -listen 127.0.0.1:8080
make build создаёт bin/subgate-core и bin/subgate-agent; demo admin/subgate-demo — публичный fixture.
make check: test/race/vet/lint/static build, Linux+CGO+C compiler.
go test -v → tmp/test.out, make check → tmp/make.out; показывай summary и первые10 строк ошибок.
Логи >50 строк: head20+tail10, остальное в tmp; старые tmp не читать.

## Автономный цикл
Бери один пункт плана на15–40 минут, проверь assertions, обнови evidence/статус и переходи к следующему.
Не останавливайся и не спрашивай о рутинных решениях; новые блокеры — docs/BLOCKED.md, решения — docs/DECISIONS.md.
При блокере продолжай независимые задачи; не выдумывай продуктовую политику/разрешение на внешние действия.
DONE/[x] только с нужными assertions и PASS либо воспроизведённой ручной проверкой; SKIP не PASS.
Go: gofmt, простые явные слои, bounded resources, fail-open, unknown passthrough, контекст/deadline сетевых операций.
Перед commit обязательны build/vet/test, для релиза полный make check; FAIL записывать честно.
Не stage/commit без явного поручения; сохраняй чужие изменения; без reset/clean/force checkout/restore.
Ничего не удаляй безвозвратно: убранную документацию переноси в docs/_archive/.

## Границы и безопасность
Одна owner учётка login/password, без ролей/2FA. Xray editor текстовый в v1.
Production panel только GET; writes проверять на отдельной test panel; подтверждение risky operation и audit обязательны.
Panel token только Core; Agent outbound-only/revocable token; Xray StatsService всегда reset=false.
Не читать/выводить secrets, .env*, keys, существующие DB/backup; только synthetic state.
Без Docker socket/чужих DB/FIFO sink/payload/URL inspection; destinations только host:port.
Не читай remnawave-openapi.json целиком: jq по конкретному .paths["/api/..."] или компоненту.
Не читай subgate-ai-prompt-v4.md целиком: rg заголовков и нужный диапазон; исходные требования не менять.
Не читать ~/SubGate-docs-archive-2026-10-14/ и docs/_archive/ без отдельной исторической задачи.
Telegram/IP-ASN/Xray data разрешены; email/S3/самообновление SubGate/автосанкции по F21 исключены.
