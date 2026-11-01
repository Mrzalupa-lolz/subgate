> Актуальное уточнение 2026-11-01: четыре оставшихся health/maintenance FAIL исправлены в synthetic fixtures; полный suite прошёл. Описания FAIL ниже — история аудита. Health не влияет на подписки.

# Health engine и quarantine
О чём: scheduler, quorum, maintenance, persisted health и состояние мониторинга нод.
Когда читать: health transitions, Agent votes, quarantine/recovery и расписание проб.

## Реализация и текущая проверка
Код: core/internal/server/health_schedule.go, health_quorum.go, health_maintenance.go; core/internal/health/.
Scheduler независим от panel poll; defaults30s interval/8s node timeout, максимум4 workers/16 endpoints node.
Interval10ms..24h, timeout10ms..30s. Probe+engine state commit вместе, restart восстанавливает state.
N/M defaults3/2, диапазон1..1000; min quarantine1m, backoff30s..15m capped exponential.
Границы backoff/min hold отсчитываются после slow probe/commit decision.
Первый успех переводит quarantined в recovering; исключение сохраняется до M consecutive successes + min hold.
Неудача recovery сбрасывает successes и возвращает quarantine.
Мониторинг и расписание Agent читают committed snapshot под lock; rolled-back transition не должен менять опубликованное состояние.

История аудита: в прежнем baseline B04 тесты maintenance/slow probe/incomplete получали `sql: no rows in result set`; подробности сохранены в [REORG_REPORT](../REORG_REPORT.md). Этот результат относится к предыдущему состоянию.
В правке 2026-11-01 исправлены synthetic fixtures: проверяемые ноды теперь присутствуют в synced registry. Полный Linux go test ./... прошёл; текущие проверки — [MONITORING_VALIDATION](../MONITORING_VALIDATION.md).

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
После окончания probes возобновляются; maintenance сохраняет наблюдаемое состояние и подавляет новые проверки.
Текущий synthetic suite проверяет эти правила; live transport acceptance остаётся отдельным gate.

## Потребители состояния

После удаления gateway health/quarantine остаются внутренними состояниями мониторинга и расписания проб. Они не меняют подписки и не выполняют санкций. Нет manual force-quarantine/healthy workflow, transition critical/recovery producers, log-derived health и full process checks.

## Пробы
Reachability уровни, HTTP/3 и unknown defaults вынесены в [transports](transports.md).
TCP не доказывает TLS, транспорт не доказывает proxy-user authentication, unverified не равно failure.
Для новой пробы нужны bounded deadline, positive/negative fixture, Core/manual/Agent/report/quorum consumer tests.
Полная transport/security матрица и real-Xray non-skipped run остаются release gates.
