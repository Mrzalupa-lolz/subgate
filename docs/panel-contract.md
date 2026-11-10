# Контракт Remnawave

Контракт проверен на Remnawave **3.4.8** (ноды remnanode 3.4.4, Xray 26.10.31). Core использует API-токен, по умолчанию разрешает только GET, запрещает redirects и ограничивает ответ 16 MiB. Зависимости от OpenAPI-схемы панели и генерируемого клиента нет: ответы читает рукописный валидатор.

## Используемые методы

| Метод и путь панели | Назначение | Минимальный scope |
| --- | --- | --- |
| `GET /api/users?start&size` | Синхронизация аккаунтов: response.users/total, числовой `id`, shortUuid, username, status и traffic counters | `users:list` |
| `GET /api/nodes` | Ноды, адреса, состояния (isConnected/isDisabled, usersOnline) и configProfile.activeInbounds | `nodes:list` |
| `GET /api/system/stats/recap` | Версия панели из response.version | Проверить доступ токена на целевой панели |
| `POST /api/nodes/{uuid}/actions/enable` | Включение ноды | `nodes:enable` |
| `POST /api/nodes/{uuid}/actions/disable` | Отключение ноды | `nodes:disable` |
| `POST /api/nodes/{uuid}/actions/restart` | Перезапуск ноды | `nodes:restart` |
| `POST /api/nodes/{uuid}/actions/reset-traffic` | Сброс счётчика трафика ноды | Проверить на тестовой панели |

`GET /api/hosts`, `GET /api/users/{id}` и `GET /api/users/by-short-uuid/{id}` больше не используются. Имена scopes ранее наблюдались на тестовой панели; это не свежая проверка токена рабочей панели. Оператор создаёт scoped key в Remnawave; Core не получает пароль администратора и не создаёт ключи через admin-only `/api/tokens`.

Запись (node actions) требует явного `panel.allow_writes`; без него Core отклоняет POST до отправки. Наличие route не подтверждает завершённый workflow и итоговый аудит каждой операции. Пользовательские mutations и выдача подписок переданы middleware. Панельные `/api/sub/...`, HWID, request history, squads и billing в SubGate не используются.

## Идентификаторы и безопасность данных

Аккаунты идентифицируются числовым `id`: в 3.x поля `uuid` у пользователя нет, но legacy `uuid` по-прежнему принимается. Отсутствие идентификатора отвергает пользователя; присутствующие optional-поля проверяются на тип и формат. Точные int64 counters сохраняются. `rawInbound` (ключи REALITY и т. п.) никогда не декодируется и не хранится.

## Синхронизация и версия

Снимок users/nodes публикуется атомарно раз в минуту; неудачный sync сохраняет последний успешный. Webhook-приёмника нет: SubGate опирается только на минутный опрос панели. Состояние ноды берётся из синхронизации, живость агента — из heartbeat.

При старте Core запрашивает recap с бюджетом 3 секунды и сравнивает версию с поддерживаемой серией. `/api/v1/system/status` содержит `panel_version={expected,actual,state}`: `not_checked`, `compatible` (любая 3.4.x), `mismatch` (другая major/minor), `unavailable`. Ошибка или отсутствие scope не раскрывают response body и не останавливают мониторинг.

## Рукописный валидатор и фикстуры

Ответы `GET /api/users` и `GET /api/nodes` декодирует [client.go](../core/internal/panel/client.go) с явной проверкой типов. Синтетические фикстуры 3.4.8: `core/internal/panel/testdata/users-3.4.8.json`, `nodes-3.4.8.json`. Они не содержат реальных данных; при обновлении панели их нужно сверять с живым ответом.

```bash
go test ./core/internal/panel -count=1
```

Тесты `contract_test.go` (`TestPanelFixtureReadContract`, `TestPanelUserValidation`, `TestPanelVersionSeries`), `TestPanelReadOnlyRejectsBeforeSending` проверяют контракт. Live-сценарии и границы сохранённых результатов описаны в [DEVELOPMENT.md](DEVELOPMENT.md).
