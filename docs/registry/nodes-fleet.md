# Ноды, Agent и установка
О чём: outbound Agent, enrollment, локальная установка и опциональная fleet automation.
Когда читать: node lifecycle, installer, telemetry delivery или права процессов.

## Реализация
Код: agent/cmd/main.go, agent/internal/agent/, agent/internal/enrollment/, deploy/install.sh и deploy/*.service.
Agent — отдельный executable, HTTP JSON к Core; это осознанный сохранённый D07 стек, не незавершённый переход на gRPC.
Токен панели только Core; Agent получает revocable node token через одноразовый15min enrollment.
Перевыпуск токена меняет credential; node_id сверяется с ingress payload, hash токена хранится в Core.
Удалённый Core требует HTTPS; loopback HTTP пригоден только синтетическим проверкам.
Heartbeat сообщает версию, время, heap Alloc/uptime и stats/log errors; heap не равен RSS.
Сбор CPU/disk/NIC/drops/conntrack/retransmits/process state ещё не встроен.

Pending spool шифруется token-bound AES-GCM; node AAD исключает перенос на другую ноду.
Batch ID и sent_at переживают retry/restart; максимум16MiB pending, 5000 samples/1000 events/2MiB на batch.
408/429/5xx/network оставляют batch; постоянные validation4xx удаляют с lost counters;401/403 останавливают Agent и сохраняют pending.
Канал логов до spool ещё не durable. При re-enroll старый spool не расшифруется новым токеном: сохранить отдельно до принятого решения.
TestEncryptedSpoolReplaysSameBatchAfterFailedSendAndRestart, TestSpoolRejectsCorruptionWrongTokenAndNode,
TestTelemetryBatchBoundsAndExplicitOverflowCount, TestPermanentTelemetryRejectionDropsOnlyRejectedBatch проверены PASS.
Точные ingress limits и response semantics: [Core API](../api.md).

## Локальный запуск
    go run ./core/cmd -demo -database :memory: -listen 127.0.0.1:8080
    make build
    sudo bash deploy/install.sh --bin-dir ./bin --core
    sudo bash deploy/install.sh --bin-dir ./bin --agent

Demo: публичные synthetic admin/subgate-demo. Installer требует native Linux/root/systemd; не запускать его в данном docs-аудите.
Он сохраняет существующий env и без --start оставляет службы остановленными. Placeholder/un-enrolled конфиг не должен стартовать.
Бинарники локальные; checksums optional. Downloader/signed pinned one-liner/browser bootstrap token отсутствуют.
State — private /var/lib/subgate[-agent], permissions и ReadWritePaths должны соответствовать путям.
Core container UID/GID10001; bind-mount ownership готовится отдельно. Compose/systemd исходники не доказывают успешную установку.
make build создаёт CGO_ENABLED=0 executables; make check race требует CGO и C compiler.
Текущий sandbox не запускается (bwrap/loopback Operation not permitted); выполненные проверки использовали разрешённый host execution.

## Xray доступ
Stats адрес auto выбирает единственный abstract xtls-api-* из /proc/net/unix, иначе loopback10085.
Допустимы unix:@name и unix:/absolute/path. Namespace Xray/Agent должен совпадать; Docker socket/group не выдавать.
Access log read-only, narrow group/ACL read+traverse, Agent начинает с EOF; исторический импорт не заявлен.
Список transport capabilities и границы reachability — [transports](transports.md).

## Целевой lifecycle
Нужны idempotent panel/node one-liner, version pin, обязательные checksum+signature, mirror fallback, amd64/arm64 install checks.
Самообновление бинарников SubGate отменено D03; signed выпускаемые artifacts и обновление IP/Xray data разрешены.
Standalone Core ACME ещё нет; внешний Caddy ACME не является built-in ACME.
F26 optional: remote terminal, scripts/mass commands, auto-capture, Docker hygiene, node/Xray lifecycle, log rotation/auto-restart, WARP/Tor.
Ни shell endpoint, ни command channel сейчас не зарегистрированы. Не превращать optional fleet в prerequisite v1 без решения B05.
Для одной разрешённой операции нужны capability/confirmation/audit, dry-run, bounded timeout/output и rollback; без Docker socket у Core.
Optimizer F24: advisory first, namespaced drop-ins, dry-run/rollback для BBR/Cake/XanMod, RPS/RFS/XPS/NIC, MSS, FD limits, zram/journald.
Эти изменения хоста отсутствуют; read-only catalogue и проверка рекомендаций должны предшествовать применению.
