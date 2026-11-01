# Operations, backups и автоматизация
О чём: panel actions, Telegram, recovery runbook, F22/F33/F36.
Когда читать: операторские действия, уведомления, backup/restore или optional workflows.

## Panel operations F17
Код: core/internal/server/api.go:nodeAction, panel/client.go:Action, web/app.js.
Node enable/disable/reset/restart entrypoints есть, но generic Action не гарантирует точный endpoint/body/id для каждой операции.
Write gate default GET-only; рабочие production credentials этим аудитом не использовались.
Нет complete revoke/hosts bulk workflow, global search/deep links, гарантированного final outcome audit и confirmations всего каталога.
Одна owner учётка имеет полный доступ; не восстанавливать RBAC из старого roadmap.
Сначала сверить OpenAPI и test-panel fixture; не считать наличие POST route разрешением писать в production.

Управление пользователями передано middleware; прежние handlers/UI в [архиве](../../_archive/middleware-overlap-2026-10-31/README.md).

## Telegram F14
notify.Telegram Send и Dispatcher dedupe/cooldown/quiet hours/test endpoint есть.
AllowedUserIDs сейчас используются как ChatIDs, не авторизация bot commands; нет polling/commands/buttons/topics/RU-EN/proxy workflow.
Нет automatic health/drop/quota/bandwidth/log transition producers и recovery messages.
При отсутствии sender Dispatcher возвращает nil; success test endpoint не доказывает delivery.
Нужны per-category health/drops/youtube/16-20/request-observations/bandwidth/log/system topics, allowlisted /status/node/user/top/ip.
Для сигналов нарушений нужны ссылки на журнал и проверяемые признаки. Реализован дополнительный Linux worker с nDPI, длительными окнами перегрузки и временным TC/TBF shaper. Автоматика выключена до настройки протоколов/лимита/срока и точной атрибуции выделенного пути аккаунта. TTL обслуживает независимый systemd timer. См. [NODE_MONITORING.md](../NODE_MONITORING.md).
Email/S3 не делать; TG test delivery в этом docs-аудите не отправлялась.

## Backup F18 и recovery
Локальный encrypted .sgb snapshot через SQLite WAL проверен TestEncryptedBackupRestoresRealDatabase и TestBackupRestoresCommittedWALData.
Snapshot сейчас включает activity, хотя F18 требует default exclusion.
core/internal/backup/remote.go содержит utility remote stores/Prune, но они не runtime schedule или проверенный TG pipeline.
Нет schedule/count-age rotation, panel backup, Telegram auto-split, pre-migration snapshot hooks.
S3 utility в коде не разрешает интеграцию: S3 исключён D03, runtime не подключать.

Сохранять DB и соответствующий private subgate.key вместе по смыслу, физически key отдельно от encrypted artifact.
Не копировать active SQLite без WAL snapshot; не менять DB под running Core.
CLI restore только новый output:
    subgate-core -restore /secure-backups/EXAMPLE.sgb -key-file /secure-keys/subgate.key -output /private-recovery/subgate.db

Вернуть original key рядом с restored DB/в configured key path; проверить isolated копию до cutover.
CLI пока не выполняет integrity_check/version gate/transaction-safe cutover/полный rollback; signature/header check не равен safe restore.
Перед ручным update нужны snapshot+key+old executable; old binary на migrated DB не гарантирует rollback.
Автоматическое обновление бинарника SubGate исключено; разрешённые data updates не должны обходить integrity/licence controls.
Activity backup/export отдельные sensitivity/retention правила — [activity](activity.md).

## Automation/support/plugins
F22 builder predicates/schedules/templates/execution logs отсутствуют; начать validated rule evaluation без actions.
Триггер не может обходить D01 signal-only, URL SSRF, owner confirmation или запрет production writes.
F33 optional tickets/SLA/templates/attachments/customer card/quiet-hours и support-event API отсутствуют; TG warnings допустимы, email исключён.
F33b full mail server невозможен в согласованном scope; это не техническая невозможность SMTP.
Billing/CRM/Bedolaga/referral/promo/payments не строить: non-goal и вне разрешённых предметных интеграций.
F36 sandbox plugin system later/optional: нет loader/API/capability enforcement; сначала deny network/filesystem и bounded host API.
Решения optional не подменять обязательным v1 workload, B05. План хранится только в [PROJECT_PLAN](../PROJECT_PLAN.md).
