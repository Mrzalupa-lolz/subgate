# Контракт Remnawave

Опорная схема — [remnawave-openapi.json](../remnawave-openapi.json), версия 3.4.5. Core использует API-токен, по умолчанию разрешает только GET, запрещает redirects и ограничивает ответ 16 MiB. Наличие метода в OpenAPI не означает, что соответствующая функция подключена в SubGate.

## Используемые методы

| Метод и путь панели | Назначение | Минимальный scope |
| --- | --- | --- |
| `GET /api/users` | Синхронизация аккаунтов: start/size, response.users/total, UUID/id, shortUuid, username, status и traffic counters | `users:list` |
| `GET /api/users/{userId}` | Reader отдельного аккаунта; userId в схеме числовой | `users:by-id` |
| `GET /api/users/by-short-uuid/{shortUuid}` | Reader по short UUID | `users:by-short-uuid` |
| `GET /api/nodes` | Ноды, адреса, состояния и configProfile.activeInbounds | `nodes:list` |
| `GET /api/hosts` | Связь хостов с нодами | `hosts:list` |
| `GET /api/system/stats/recap` | Версия панели из response.version | Проверить доступ токена на целевой панели |
| `POST /api/nodes/{uuid}/actions/enable` | Включение ноды через node action | `nodes:enable` |
| `POST /api/nodes/{uuid}/actions/disable` | Отключение ноды через node action | `nodes:disable` |
| `POST /api/nodes/{uuid}/actions/restart` | Перезапуск ноды через node action | `nodes:restart` |

Имена scopes ранее наблюдались на отдельной тестовой панели 3.4.5 (2026-10-14). Это не свежая проверка токена рабочей панели. Оператор создаёт scoped key в Remnawave; Core не получает пароль администратора и не создаёт ключи через admin-only `/api/tokens`.

В HTTP API Core также есть node action `reset-traffic`; точный контракт и права проверяйте отдельно на тестовой панели. Node actions используют общий клиент: наличие route не подтверждает завершённый workflow, итоговый аудит и проверку каждой операции. Доступ на запись требует явного `panel.allow_writes`; development-проверки записи выполняются отдельно от production.

Пользовательские mutations и выдача подписок переданы middleware. Панельные `/api/sub/...`, HWID, request history, squads и billing не являются подключёнными функциями мониторинга SubGate.

## Синхронизация и версия

Снимок users/nodes/hosts публикуется атомарно; неудачный sync сохраняет последний успешный. HMAC вебхука проверяется по исходному body, replay marker и обновление пользователя сохраняются вместе. Применение `user.updated` покрыто тестами; для остальных типов нужны отдельные projection-проверки.

При старте Core запрашивает recap с бюджетом 3 секунды и сравнивает версию с `generated.SpecVersion`. `/api/v1/system/status` содержит `panel_version={expected,actual,state}`: `not_checked`, `compatible`, `mismatch`, `unavailable`. Ошибка или отсутствие scope не раскрывают response body и не останавливают мониторинг.

## Сгенерированный reader

Генератор — ogen v1.24.0. В runtime подключён generated response decoder `GET /api/users`; HTTP-клиент и другие readers остаются ручными. Полное покрытие OpenAPI и zero-reflection не заявляются.

Projection содержит start/size и response200. Nested optional filters и same-type error oneOf не вошли из-за ограничений генератора; HTTP errors обрабатывает ручной Request. Unknown enums читаются без writer Validate. Отдельная нормализация исходных JSON-байтов сохраняет точные int64 counters и приоритет UUID. Отсутствие обоих идентификаторов отвергается; present optional metadata проверяются на тип/формат. Исходная схема не меняется.

```bash
python3 scripts/generate-panel.py --check
go test ./core/internal/panel -count=1
```

Проверки `TestGeneratedPanelUsersContract`, `TestGeneratedPanelUsersIntegrity`, `TestPanelReadOnlyRejectsBeforeSending`, `TestPanelVersionMismatch` проверяют части контракта. Live-сценарии и границы сохранённых результатов описаны в [DEVELOPMENT.md](DEVELOPMENT.md).

## Будущий Xray-редактор

Редактор пока не реализован. Для него нужны `GET /api/config-profiles`, `GET /api/config-profiles/{uuid}`, при необходимости computed-config и `PATCH /api/config-profiles` с UUID в body. Это не `PATCH /api/config-profiles/{uuid}`. Scopes списка/обновления — `config-profiles:list` и `config-profiles:update`.

Перед реализацией извлеките точный path и DTO из схемы. Computed-config не заменяет проверку конфига. Нужны подтверждение применения, итоговый аудит, version conflict/rollback и проверка записи в отдельной панели.

```bash
jq '.paths["/api/config-profiles"].patch' remnawave-openapi.json
jq '.components.schemas.UpdateConfigProfileBodyDto' remnawave-openapi.json
```
