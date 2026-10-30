# Проверка, budgets и работа агента
О чём: acceptance gates, Linux CI, историческая evidence и экономия контекста.
Когда читать: DONE, performance/chaos, verification или подготовка релиза.

## DONE
Feature DONE только при assertions полного согласованного поведения и наблюдённом PASS.
Build, schema/CRUD, utility без runtime call, skipped live test и прежний ИИ-отчёт не являются доказательством функции.
Текущие inventory rows проверены по исходникам, [FEATURE_STATUS](../FEATURE_STATUS.md); текущие check результаты — [REORG_REPORT](../REORG_REPORT.md).
Старые289 F-xxx не равны36 Part B features: там смешаны v4 requirements, references и scope statements.
Исходные формулировки/line IDs сохранены в originals ZIP архива, без переноса устаревших assertions в новый статус.

## Команды
    go build ./...
    go vet ./...
    go test ./...
    make check
    python3 scripts/check-docs.py
    python3 scripts/generate-panel.py --check

make check: test/test-race/vet/gofmt+node --check+bash -n/static build.
Linux+CGO+C compiler нужны race, static executables сами CGO_ENABLED=0.
Frontend TypeScript отсутствует, не запускать фиктивный type-check.
CI .github/workflows/ci.yml содержит race/vet/generation/install syntax/static build/executable smoke/container build.
Наличие CI YAML не доказывает observed CI run; нет full real-Xray/fuzz/perf/signing/license/SBOM gates.
make bench без Benchmark functions не доказывает budget; проверить непустые measurements.
test/discovery scripts оставить рядом с fixtures, не превращать их инструкции в evidence.

## Требования приёмки
Golden real fixtures/properties round-trip, fuzz всех subscription/config/log/feed parsers; unknown никогда panic.
Real-Xray protocol×transport×security matrix с positive/negative probes/stats/log capture без пропущенных cases.
Прочитать только конкретные test files и requirements; secrets/production DB/raw logs не использовать.
При exact test gate необходимо проверить событие pass для имени и отсутствие skip/fail у любых descendants.
go test -run nonexistent может завершиться0; parent pass с all children skipped тоже не live PASS.
B04: текущие health failures; не выдавать make check FAIL за исправленную функцию.

Performance v4 §11: gateway cache-hit p99<=5ms, fresh transform p99<=25ms excluding upstream,10k cache hits/s2vCPU.
Agent5000 users@1s<=1.5% одного core/40MB RSS; logs>=100k lines/s/core bounded memory.
Core>=200k samples/s,UI diff<=100KB/s/viewer,10k virtual rows<=16ms/frame,RSS<=512MB20nodes×5000users.
UI §9: initial JS<=250KiB gzip. Это целевые budgets, измеренного production результата нет.
Activity/telemetry/IP lookup budgets находятся в своих domain docs, чтобы не читать весь registry.
Load/chaos нужны DB locked/corrupt, Core killed fallback, panel stale/unavailable, clock skew,
feed failure/atomic reload, log flood/rotation, reset/restart, iperf accuracy ±5% и unchanged panel accounting.
Linux amd64/arm64 installer/Compose/runtime permissions, no-Docker-socket, TLS/SSRF, vuln/SBOM/licence/signature checks отдельные gates.

## Контекст и история
docs/INDEX — единственная точка входа. На задачу читать один domain и только указанные source/tests.
Оригинальные v4/inventory не менять, v4 целиком не читать на каждом старте; rg headings и нужный диапазон.
Не загружать OpenAPI целиком, go.sum, tmp, все skills или archive. Для schema только выбранный jq path/component.
Skills один source .agents/skills, применять по задаче; в baseline .claude/.codex duplicates не было.
TOKEN_PLAN сохранён в архиве: его советы удалить OpenAPI/requirements/skills и эвристики auto-compact не приняты.
Расход96.4M, cached71.5M, security reviewer25M — историческая метрика, обещания экономии60–88% не измеренный результат.
Security review ограничивать текущим diff и названными dependencies; docs-аудит не требует повторять полный source review.
Не создавать commits автоматически без поручения; конкретное текущее reorg-поручение явно разрешило series commits.
