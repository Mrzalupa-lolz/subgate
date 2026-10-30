# Health engine и quarantine
О чём: scheduler, quorum, maintenance, persisted health и применение к подпискам.
Когда читать: health transitions, Agent votes, quarantine/recovery и tier exclusions.

## Реализация и текущая проверка
Код: core/internal/server/health_schedule.go, health_quorum.go, health_maintenance.go; core/internal/health/.
Scheduler независим от panel poll; defaults30s interval/8s node timeout, максимум4 workers/16 endpoints node.
Interval10ms..24h, timeout10ms..30s. Probe+engine state commit вместе, restart восстанавливает state.
N/M defaults3/2, диапазон1..1000; min quarantine1m, backoff30s..15m capped exponential.
Границы backoff/min hold отсчитываются после slow probe/commit decision.
Первый успех переводит quarantined в recovering; исключение сохраняется до M consecutive successes + min hold.
Неудача recovery сбрасывает successes и возвращает quarantine.
Routing читает committed snapshot под lock; rolled-back transition не должен менять подписку.

ВАЖНО: текущий suite FAIL, B04. Пять tests, включая maintenance/slow probe/incomplete cases, получают sql: no rows in result set.
Исторический PASS2026-10-14 не отменяет текущую регрессию; health в целом PARTIAL.
TestScheduledHealthTransitions, TestProbeQuorum и TestQuarantineFiltersSubscription в текущем запуске PASS.
Точные результаты и отсутствие полного race gate — [REORG_REPORT](../REORG_REPORT.md).
Repair/re-run этих пяти regression cases — первая техническая задача плана.

## Quorum и Agent
health.quorum1..64, default1. Core даёт одно verified observation; каждый authenticated enrolled Agent — максимум один vote/target.
Vote latest source replacement, freshness90s, future5s; revoked/rotated tokens исключаются и при чтении, и при приёме.
Если success/failure одновременно набрали threshold, success выигрывает; отсутствие consensus не меняет state.
quorum_met/vantages отличаются от Core собственного success/verified.
Agent GET health-targets: максимум64 nodes/16 endpoints; >64 возвращает503, oversize node omitted.
PUT health-results: до64 результатов, no duplicate target, timestamp/completed_at window checks; source берётся из credential.
Legacy missing completed_at использует receive time, поэтому maintenance overlap не теряется.
Partial cycle отправляет completed probes под отдельным5s budget; cursor начинает следующий cycle с unfinished targets.
Не объявлять независимые региональные vantage работающими только по loopback fixtures.

## Maintenance
До128 YAML windows с node_id/from/until, from<until, RFC3339, интервалы [from,until), overlaps union.
Restart применяет настройки. Во время maintenance Core no-dial, manual probe409, Agent targets omit node.
Любой overlap полного observation с window должен игнорироваться; counters/state сохраняются.
После окончания probes возобновляются; maintenance само по себе не удаляет node из подписки.
Эти правила определены кодом/tests, но текущие failing regressions требуют исправления до DONE.

## Gateway consumers
Quarantined/recovering pins и tier members пропускаются, выбирается eligible fallback.
Без политики исключаются только однозначно mapped endpoints. Unknown formats/protocols и ambiguous addresses сохраняются.
При unsafe transform/exhausted tiers выдаются exact upstream bytes; cache hit заново применяет health.
Это fail-open, не доказательство полной политики critical/recovery alert или quota fallback.
Нет manual force-quarantine/healthy workflow, transition critical/recovery producers, log-derived health, full process checks.
Min-healthy, N/M/quorum и fallback тестировать fake clock/fixtures; не использовать production failure injection.

## Пробы
Reachability уровни, HTTP/3 и unknown defaults вынесены в [transports](transports.md).
TCP не доказывает TLS, транспорт не доказывает proxy-user authentication, unverified не равно failure.
Для новой пробы нужны bounded deadline, positive/negative fixture, Core/manual/Agent/report/quorum consumer tests.
Полная transport/security матрица и real-Xray non-skipped run остаются release gates.
