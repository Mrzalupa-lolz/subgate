# SubGate

Самостоятельный Go-проект: subscription gateway, Core, node Agent, встроенная консоль и SQLite. Цель — заменить remnawave-admin. Интеграции — Remnawave и VPN-ноды (коннектор remnawave-admin не входит).

**Версия 0.2.0** — идентификатор сборки, не production-релиз. Требования: [docs/PROJECT_PLAN.md](docs/PROJECT_PLAN.md), [docs/api.md](docs/api.md); правила: [AGENTS.md](AGENTS.md); статус: [STATUS.md](STATUS.md).

Проверены части: opaque proxy с cache/stale fallback, users/nodes sync, HMAC invalidation, policy/tier/pin, quota accounting, encrypted Agent spool, Xray gRPC counters, activity query/export/purge, local MMDB, encrypted backups. Gaps: automatic quarantine не подключён, exhausted quota возвращает origin, columnar journal и full log pipeline отсутствуют. Детали: PROJECT_PLAN.md.

## Быстрый старт

**Требования:** Go 1.25+, Linux — production цель.

```sh
go mod download
go build ./...
go test ./...
make build                 # bin/subgate-core, bin/subgate-agent (Linux/make/CGO)
make check                 # tests + race + vet + build
```

Demo (локально, :memory:):

```sh
go run ./core/cmd -demo -database :memory: -listen 127.0.0.1:8080
curl http://127.0.0.1:8080/health
```

Открыть `http://127.0.0.1:8080/`, login `admin` / `subgate-demo`.

## Deployment (Linux)

Env: `SUBGATE_LISTEN`, `SUBGATE_DATABASE_URL`, `SUBGATE_PANEL_URL`, `SUBGATE_PANEL_TOKEN`, `SUBGATE_ADMIN_USERNAME`, `SUBGATE_ADMIN_PASSWORD`. Panel token только Core.

```sh
make build
sudo bash deploy/install.sh --bin-dir ./bin --core
sudo bash deploy/install.sh --bin-dir ./bin --agent
```

Installer: локальные бинарники, systemd units, службы остановлены без `--start`. Настроить env, enroll Agent, проверить права перед start. Docker: см. `deploy/compose.yaml`. Core требует HTTPS reverse proxy; [nginx example](../../../deploy/nginx.conf.example) (без fail-open fallback при остановке Core). Agent outbound-only, требует enrollment token из консоли/API.

## Восстановление

Бэкап: DB + `subgate.key` вместе. Activity входит в backup (вопреки желаемому exclusion).

```sh
subgate-core -restore /path/to/backup.sgb -key-file /path/to/subgate.key -output /path/to/recovered.db
```

Output должен быть новым. CLI не реализует transaction-safe restore; проверяй отдельную копию перед заменой state.

## Архив документации

`~/SubGate-docs-archive-2026-10-14/`: OVERVIEW, EVIDENCE, activity-journal/storage, subgate-ai-prompt-v4, subgate-feature-inventory (исходные требования владельца).

## English

SubGate is an independent Go control plane and subscription gateway for Remnawave and VPN nodes. Current code identifies as 0.2.0; full product readiness unverified. Core embeds vanilla HTML/CSS/JS and uses SQLite; Agent reads local Xray counters without reset. See [PROJECT_PLAN.md](docs/PROJECT_PLAN.md) for tested subsets and gaps. No remnawave-admin connector in scope.
