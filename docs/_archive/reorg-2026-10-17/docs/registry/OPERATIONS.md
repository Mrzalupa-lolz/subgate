## 6. Команды проверки и эксплуатации

Сводная команда для **Linux + make + C compiler для race** уже есть:

```sh
make check
```

Текущий target делает test, test-race, vet и static build двух бинарников. Не добавлялся и не менялся. Он **не** запускает smoke, frontend lint, fuzz campaign, Docker, live Xray или performance thresholds.

Независимые проверки, воспроизведённые здесь:

```sh
go build ./...
go vet ./...
go test ./...
go test -cover ./...
node --check web/app.js
```

Windows PowerShell, если default build cache недоступен:

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.gocache'
go build ./...
if ($LASTEXITCODE -ne 0) { throw 'build failed' }
go vet ./...
if ($LASTEXITCODE -ne 0) { throw 'vet failed' }
go test ./...
if ($LASTEXITCODE -ne 0) { throw 'tests failed' }
```

| Проверка 2026-10-13 | Результат / предел доказательства |
|---|---|
| go build ./... | PASS, exit 0; Go 1.26.2 windows/amd64 |
| go vet ./... | PASS, exit 0 |
| go test -json ./... (тот же suite go test ./...) | PASS, exit 0; 76 обычных Test* верхнего уровня; FuzzTransform seed run PASS; live StatsService test SKIP |
| go test -cover ./... | PASS тесты, но quality target 70% core / 80% skill не достигнут повсеместно: gateway88.3%, ipintel72.7%, panel70.8%, db70.5%, config70.1%, quota76.7%, routing68.3%, server57.2%, health40.8%, tier46.8%, agent47.2%, xray39.2%; ряд utility packages 0% |
| node --check web/app.js | PASS синтаксис; TypeScript здесь нет, browser behavior не проверено |
| Core demo :memory: HTTP smoke | PASS health/version/assets/login и GET nodes/users/settings/live/activity; synthetic state, процесс остановлен |
| go test -race ./... | НЕ ВЫПОЛНЕН: exit1 до тестов, «-race requires cgo; enable cgo by setting CGO_ENABLED=1»; C compiler не найден |
| make check / Linux static build / installer / Docker / CI / browser E2E / live panel / live Xray / fuzz campaign / load/chaos/perf | **Не проверено** в этой сессии. make/docker/gcc отсутствуют в найденных командах; существующие ИИ-отчёты не подставлялись вместо результата |
| Source grep Docker socket / unsafe TLS / command execution | В проверенных core/agent/pkg/deploy Go/YAML/sh не найдено Docker socket или InsecureSkipVerify; это ограниченный поиск, не полный security review. Реальные секретные файлы не открывались |
| Diff | Внесены документы и ignore cache; Go/test/module/OpenAPI/deploy/Makefile не изменены этим аудитом; pre-existing changes сохранены |

Для roadmap-гейтов можно определить Linux helper (не production script):

```sh
require_test() {
  go test -json "$1" -run "^$2$" | python3 -c '
import json,sys
events=[json.loads(line) for line in sys.stdin if line.strip()]
assert not any(e.get("Action")=="fail" for e in events)
assert any(e.get("Action")=="pass" and e.get("Test")==sys.argv[1] for e in events), "required test absent/skipped"
' "$2"
}
```

Сборка и локальный старт (перенесено из BUILD/START и сверено с flags/Makefile):

```sh
go mod download
make build                 # bin/subgate-core и bin/subgate-agent, CGO_ENABLED=0
make test
make test-race
make vet
make bench                 # пока нет Benchmark*; не доказательство budgets
make smoke                 # Linux executable E2E, Python3, synthetic stats failure
go run ./core/cmd -demo -database :memory: -listen 127.0.0.1:8080
# public demo fixture: admin / subgate-demo
curl --fail http://127.0.0.1:8080/health
```

Cross-build примеры из BUILD (инструкции, не выполненный результат этой сессии):

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/subgate-core ./core/cmd
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/subgate-agent ./agent/cmd
make docker
```

Config: `SUBGATE_LISTEN`, `SUBGATE_DATABASE_URL`, `SUBGATE_PANEL_URL`, `SUBGATE_PANEL_TOKEN`, `SUBGATE_SUBSCRIPTION_URL`, `SUBGATE_PUBLIC_URL`, `SUBGATE_TRUSTED_PROXIES`, `SUBGATE_ADMIN_USERNAME`, `SUBGATE_ADMIN_PASSWORD`, `SUBGATE_BACKUP_PATH`. Bootstrap password используется только при пустой admin table; смена env не меняет пароль существующего admin. Optional `SUBGATE_JWT_SECRET` compatibility field, sessions opaque и не JWT. При `-config FILE` YAML defaults+file; env не накладывается сверху. Webhook key/retention/sync/IP path/TLS и Telegram задаются YAML. Наличие поля config не гарантирует implemented behavior.

IP data: `ipintel.data_path` — local JSON prefix dataset, MMDB или directory *.mmdb; manual `/api/v1/ipintel/reload`, bad dataset оставляет last-good. City approximate, source licence обязан проверить оператор; DB-IP attribution предусмотрена web-кодом, browser display не проверено. Никакие внешние feeds здесь не скачивались.

Deployment инструкции (не выполнялись):

```sh
cp deploy/.env.example deploy/.env
chmod 600 deploy/.env
# Вручную настроить свои URL/token/admin password, не выводить файл.
docker compose --env-file deploy/.env -f deploy/compose.yaml up -d --build
# TLS profile: DNS, SUBGATE_DOMAIN, SUBGATE_PUBLIC_URL, свободные 80/443
docker compose --env-file deploy/.env -f deploy/compose.yaml --profile tls up -d
sudo bash deploy/install.sh --bin-dir ./bin --core
sudo bash deploy/install.sh --bin-dir ./bin --agent
```

Installer требует native Linux/root/systemd, сохраняет existing env, службы не запускает без `--start`; placeholders и unenrolled agent отвергает при старте. `--checksums FILE` optional; signatures/downloader отсутствуют. Службы пишут в private `/var/lib/subgate[-agent]`. Контейнер Core UID/GID10001, bind mount ownership подготовить отдельно. При смене путей поправить systemd ReadWritePaths и mount ownership. Не включать Core hostname/public route одновременно с subscription hostname без продуманного ограничения console — встроенного hostname split/IP allowlist ещё нет.

Agent:

```sh
subgate-agent enroll -core-addr https://subgate.example.com -node-id NODE_UUID \
  -enrollment-token ONE_TIME_TOKEN -token-file /var/lib/subgate-agent/token
subgate-agent -core-addr https://subgate.example.com -node-id NODE_UUID \
  -token-file /var/lib/subgate-agent/token -stats-addr auto \
  -access-log /var/log/remnanode/access.log -spool-file /var/lib/subgate-agent/pending.sgb
```

`auto` выбирает единственный abstract `xtls-api-*` из `/proc/net/unix`, иначе loopback10085; разрешены `unix:@name`/`unix:/absolute/path`. Логи нужны с email и narrow group/ACL read/traverse. Agent начинает с EOF; прошлые logs не импортирует. В контейнере loopback/Unix namespace должен соответствовать Xray; host networking либо явная namespace configuration. Не давать Docker group/socket. Для proxy client-IP trust задавать точные контролируемые CIDR, не широкие public ranges.

Backup/restore и update (сведения из прежнего runbook, не выполнялись на данных владельца): API superadmin создаёт `.sgb` encrypted WAL snapshot. Вместе с состоянием сохранять original `subgate.key` приватно и отдельно от encrypted artifact. CLI восстанавливает только в **новый** output:

```sh
subgate-core -restore /secure-backups/EXAMPLE.sgb \
  -key-file /secure-keys/subgate.key -output /private-recovery/subgate.db
```

Затем нужен original key рядом с restored DB и isolated validation. Нельзя копировать активную SQLite без WAL snapshot, менять DB под running Core или запускать old binary на migrated DB как гарантированный rollback. Перед update сохранить consistency snapshot/key/old executable; rollback — восстановление пары DB/key после остановки процесса и проверок на отдельной копии. Restore CLI сейчас не выполняет SQLite integrity_check/version gate/transaction-safe cutover, поэтому не считать safe restore завершённым. Автоматических schedules/remote rotation/panel backups нет.

