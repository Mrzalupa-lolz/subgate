# Правила работы с SubGate

## Карта проекта

- **Код:** agent/, core/, pkg/, web/. НЕ читай целиком без задачи из roadmap.
- **Реестр и roadmap:** docs/PROJECT_PLAN.md (единственный source of truth).
- **API:** docs/api.md; текущий статус: STATUS.md.
- **remnawave-openapi.json:** НЕ читай целиком, только `jq '.paths["/api/..."]'` и grep по ключам.

## Что читать когда

- **Старт сессии:** STATUS.md (1 стр), PROJECT_PLAN.md (заголовки и текущий этап из roadmap).
- **Задача auth/API:** pkg/wire/, core/internal/server/auth*.go, docs/api.md.
- **Задача health/routing:** core/internal/server/health*.go, pkg/healthprobe/, docs/registry/health.md.
- **Задача panel:** core/internal/panel/, docs/api.md, docs/panel-contract.md.
- **Задача docs:** PROJECT_PLAN.md + только один файл области из docs/registry/, не весь registry.

## Ограничения output

- `go test -v`: перенаправь в tmp/test.out, выведи только summary (PASS/FAIL, coverage %).
- `make check`: только финальный статус (✓ или ✗ + первые 10 строк ошибок), полный лог в tmp/make.out.
- Длинные логи > 50 строк: head -20 + tail -10, остальное в tmp/.

## Запреты

- НЕ читай secrets, .env*, ключи, существующие DB/backup.
- НЕ читай весь registry (47 файлов): только PROJECT_PLAN.md + конкретный файл области текущего этапа.
- НЕ читай remnawave-openapi.json целиком (1.6 МБ): только jq/grep по ключам.
- НЕ читай архив ~/SubGate-docs-archive-2026-10-14/ и tmp/.
- НЕ применяй reset/clean/checkout --force/restore без поручения.
- НЕ commit/stage без явного указания.

## Проверка

- Минимум: `go build ./...`, `go vet ./...`, `go test ./...`.
- Полный цикл: `make check` (race, static, lint) — только на Linux с CGO.
- **DONE** только при зелёном тесте с нужными assertions и PASS либо ручной воспроизведённой проверке.
- SKIP не равно PASS. Зелёная сборка не равна готовой функции.

## Scope

- **Цель:** самостоятельный продукт владельца, заменяющий remnawave-admin.
- **Интеграции:** Remnawave (по remnawave-openapi.json) и VPN-ноды.
- **Коннектор remnawave-admin:** НЕ ДОБАВЛЯТЬ (исключён поручением).
- **Приоритет:** Go, скорость, надёжность, минимум слоёв, fail-open.
