# Матрица Remnawave 3.4.5 и план generated client

Источник: schema `remnawave-openapi.json` + наблюдённый GET /api/tokens/scopes на отдельной test panel 3.4.5 (2026-10-14). Имена scopes не выводились из предположений. Production GET-only default; panel.allow_writes включать только по разрешению записи.

| Область | Метод/endpoint по схеме | Минимальный scope | Ограничение |
|---|---|---|---|
| users | `GET /api/users` | `users:list` | read-only |
| users | `GET /api/users/by-short-uuid/{shortUuid}` | `users:by-short-uuid` | read-only |
| users | `GET /api/users/{userId}` | `users:by-id` | read-only |
| users | `POST /api/users/{userId}/actions/revoke` | `users:revoke-subscription` | test panel only; явное подтверждение операции и audit |
| users | `POST /api/users/{userId}/actions/disable` | `users:disable` | test panel only; явное подтверждение операции и audit |
| users | `POST /api/users/{userId}/actions/enable` | `users:enable` | test panel only; явное подтверждение операции и audit |
| users | `POST /api/users/{userId}/actions/reset-traffic` | `users:reset-traffic` | test panel only; явное подтверждение операции и audit |
| nodes | `GET /api/nodes` | `nodes:list` | read-only |
| nodes | `POST /api/nodes/{uuid}/actions/enable` | `nodes:enable` | test panel only; явное подтверждение операции и audit |
| nodes | `POST /api/nodes/{uuid}/actions/disable` | `nodes:disable` | test panel only; явное подтверждение операции и audit |
| nodes | `POST /api/nodes/{uuid}/actions/restart` | `nodes:restart` | test panel only; явное подтверждение операции и audit |
| config-profiles | `GET /api/config-profiles` | `config-profiles:list` | read-only |
| config-profiles | `PATCH /api/config-profiles` | `config-profiles:update` | test panel only; явное подтверждение операции и audit |
| hosts | `PATCH /api/hosts` | `hosts:update` | test panel only; явное подтверждение операции и audit |
| hosts | `GET /api/hosts` | `hosts:list` | read-only |
| hosts | `POST /api/hosts/bulk/disable` | `hosts:bulk-disable` | test panel only; явное подтверждение операции и audit |
| hosts | `POST /api/hosts/bulk/enable` | `hosts:bulk-enable` | test panel only; явное подтверждение операции и audit |

Admin-only /api/tokens (создание ключа) не доступен Core через API-token: оператор создаёт scoped key в панели. Test provisioning использует временный admin JWT с browser header; Core не получает пароль администратора панели.

Генерация: pinned ogen v1.24.0. R03 первый проход — GET /api/users и его response DTOs, выделенные программно без изменения схемы; checked-in генерация и drift-check. Остальные endpoints добавлять по одному после tests. R04 version: GET /api/system/stats/recap (response.version). Не считать DTO/генератор полным внедрением panel-native workflow.

R03 reader projection: только start/size и response200. Ogen v1.24.0 не поддержал nested optional filters и same-type error oneOf; ошибки HTTP остаются в Request. У user required остаются shortUuid/username/status/trafficLimitBytes/expireAt/userTraffic, у userTraffic — usedTrafficBytes. Остальные metadata optional, но present types/formats декодируются generated DTO. id optional только для совместимости старого UUID identity; отсутствие обоих ID отвергается. Unknown enums читаются без writer Validate. Исходный OpenAPI не изменяется.

Нормализация по исходным байтам через encoding/json сохраняет точные int64 traffic counters и UUID precedence независимо от metadata. Поэтому zero-reflection всего клиента пока не достигнут: F-023 PARTIAL. Generated HTTP client пока не используется (strict enum writer validation); подключён generated response decoder GET /api/users.

Проверено 2026-10-14: TestGeneratedPanelUsersContract и TestGeneratedPanelUsersIntegrity PASS, четыре security findings воспроизведены RED и исправлены GREEN; drift-check PASS; make check PASS. Opt-in TestLivePanelReadContract PASS на отдельной synthetic panel3.4.5; обычный offline run SKIP этого opt-in теста не считается live PASS. Coverage handwritten panel package80.6%. Остальные generated endpoints — следующие проходы R03, полного покрытия нет.

| Компонент | Версия контракта | Проверка |
|---|---|---|
| Panel / generated users reader | 3.4.5 | TestLivePanelReadContract PASS; generated.SpecVersion из info.version, drift PASS |
| Test remnanode | 3.4.2 | test GET /api/nodes сообщил версию |
| Test Xray | 26.7.28 | test GET /api/nodes сообщил версию; live StatsService отдельно не закрыт |

R04: при старте GET /api/system/stats/recap с бюджетом3s сравнивает response.version с generated.SpecVersion. Несовпадение предупреждается в журнале; ошибка/нет scope показываются как unavailable без данных ответа. GET /api/v1/system/status возвращает panel_version={expected,actual,state}; state: not_checked, compatible, mismatch, unavailable. Непроверенные версии не считаются совместимыми. Диагностика не запрещает sync/subscriptions. TestPanelVersionMismatch RED→GREEN и make check PASS2026-10-14; security review без findings.
