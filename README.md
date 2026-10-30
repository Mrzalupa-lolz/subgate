# SubGate

SubGate — шлюз подписок и консоль управления VPN-инфраструктурой на базе Remnawave. Core синхронизирует данные панели, применяет правила выдачи подписок и принимает телеметрию; Agent собирает статистику Xray на нодах и отправляет её в Core через исходящие HTTP(S)-соединения.

## Стек технологий

- Go; в CI и Docker-сборке используется Go 1.26. В `go.mod` указана директива `go 1.25.0`.
- HTTP API: `chi/v5`, JSON; поток live-данных: Server-Sent Events (SSE).
- SQLite через `modernc.org/sqlite`, режим WAL. Сборка бинарников не требует CGO.
- HTML, CSS и vanilla JavaScript. Интерфейс включён в Core через `go:embed`, отдельной frontend-сборки нет.
- gRPC/Protobuf для Xray StatsService, QUIC/HTTP3 для транспортных проверок, MaxMind DB для локальных IP-данных.
- Docker Compose для развёртывания Core и Agent.

## Архитектура

```text
Браузер ────────────────> Core HTTP API ─────────> SQLite
                             │
                             └──> Remnawave API

VPN-клиент ─────────────> Core gateway ──────────> Upstream подписок

Agent на ноде ──────────> Core: heartbeat, телеметрия, результаты проверок
    ├──> локальный Xray StatsService
    ├──> локальный access.log
    └──> транспортные проверки нод
```

Agent не открывает входящий HTTP-сервер. Токен Remnawave используется только Core; Agent получает отдельный отзываемый токен, привязанный к ноде.

| Каталог                                                                                         | Содержимое                                                                                                                           |
| ------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `core/cmd/`                                                                                          | Запуск Core, флаги, offline-восстановление резервной копии                                              |
| `core/internal/server/`                                                                              | Маршруты API, авторизация, синхронизация, приём телеметрии, интеграция модулей |
| `core/internal/panel/`                                                                               | Клиент Remnawave и сгенерированный контракт пользователей                                           |
| `core/internal/gateway/`, `core/internal/routing/`                                                 | Кеш подписок, выбор нод и преобразование ответов                                                      |
| `core/internal/health/`, `core/internal/quota/`, `core/internal/ipintel/`                        | Доступность, квоты и IP/ASN-данные                                                                                      |
| `core/internal/db/`, `core/internal/tsdb/`, `core/internal/activity/`, `core/internal/notify/` | Хранилище, обработка телеметрии, журнал и доставка уведомлений                           |
| `agent/cmd/`, `agent/internal/`                                                                    | Запуск, регистрация Agent, сбор и доставка данных                                                          |
| `pkg/wire/`                                                                                          | Общие структуры обмена Core и Agent                                                                                       |
| `pkg/xray/`                                                                                          | Клиент StatsService и чтение журналов Xray                                                                                |
| `pkg/healthprobe/`                                                                                   | Общие транспортные проверки Core и Agent                                                                             |
| `web/`                                                                                               | Встроенная веб-консоль                                                                                                     |
| `deploy/`                                                                                            | Dockerfile, Compose, systemd units, установщик и шаблоны конфигурации                                            |
| `test/`                                                                                              | Дополнительные сценарии discovery и проверки fallback                                                           |
| `scripts/`                                                                                           | Генерация клиента панели и проверки окружения                                                          |

Каталога `api/` нет: HTTP-маршруты определены в [server.go](core/internal/server/server.go), структуры обмена — в [wire.go](pkg/wire/wire.go).

## Ключевые функции и API

Пути в таблицах ниже указаны относительно `/api/v1`, кроме явно обозначенных публичных маршрутов. API консоли, кроме входа, требует сессию владельца: cookie `subgate_session` либо `Authorization: Bearer <session-token>`. При изменениях через cookie нужны допустимый `Origin` и заголовок `X-SubGate-CSRF: 1`.

### Подписки и маршрутизация

Публичные маршруты поддерживают GET и HEAD:

```text
/sub/{shortUuid}
/sub/{shortUuid}/{clientType}
/{shortUuid}
/{shortUuid}/{clientType}
```

Gateway передаёт запрос upstream подписок, кеширует исходный ответ и применяет правила при выдаче. Поддерживаются фоновое обновление устаревшего кеша и fallback на сохранённый ответ возрастом менее пяти минут. Полученный от upstream ответ 4xx инвалидирует соответствующий кеш и возвращается клиенту.

Преобразование подписки ограничено 300 мс. При ошибке, панике или превышении времени Core возвращает исходный ответ upstream. Неизвестные форматы и неоднозначно сопоставленные адреса сохраняются.

- **Группы нод (`tiers`)**: `random`, `roundRobin`, `leastLoad`, `leastPing`, минимальное число доступных нод и резервная группа. `leastPing` использует успешные подтверждённые измерения не старше десяти минут.
- **Политики (`policies`)**: выбор группы по стране, ASN и подстроке User-Agent; меньший `priority` проверяется раньше.
- **Закрепления (`pins`)**: привязка пользователя к доступной ноде имеет приоритет перед политиками.
- **Карантин**: распознанные адреса карантинных нод исключаются, если возможно построить корректную подписку; иначе сохраняется исходный ответ.
- **Квоты (`quotas`)**: учёт байтов по данным Agent для пользователя или ноды, календарные периоды и скользящие 30 дней. При исчерпании квоты выбранной ноды текущая реализация возвращает исходную подписку; блокировка VPN-соединений этим механизмом не выполняется.
- **Инъекции (`injections`)**: `append`, `prepend`, `patch`, `remove` для поддерживаемых URI/base64, JSON и YAML. В текущем пути выдачи применяются после успешного выбора ноды через pin или политику.

| Методы | Путь         | Назначение                                        |
| ------------ | ---------------- | ----------------------------------------------------------- |
| GET          | `/{kind}`      | Список правил                                   |
| PUT, DELETE  | `/{kind}/{id}` | Сохранение или удаление правила |

Здесь `kind`: `tiers`, `policies`, `pins`, `geo-rules`, `alerts`, `quotas`, `injections`.

### Remnawave, ноды и пользователи

Core синхронизирует ноды, пользователей и hosts при запуске и затем периодически; интервал по умолчанию — одна минута. Локальные данные используются в консоли и маршрутизации. Сгенерированная проекция API пользователей привязана к спецификации Remnawave 3.4.5.

| Методы | Путь                         | Назначение                                      |
| ------------ | -------------------------------- | --------------------------------------------------------- |
| GET          | `/nodes`, `/nodes/{id}`      | Список и карточка ноды                 |
| GET          | `/users`, `/users/{id}`      | Список и карточка пользователя |
| POST         | `/panel/sync`                  | Принудительная синхронизация   |
| POST         | `/nodes/{id}/actions/{action}` | `enable`, `disable`, `restart`, `reset-traffic`   |
| POST         | `/users/{id}/actions/{action}` | `enable`, `disable`, `reset-traffic`                |

Запись в Remnawave по умолчанию запрещена (`panel.allow_writes: false`). Действия записи проверяются только на отдельной тестовой панели с явно включённым `allow_writes`; production-панель используется только для GET. Веб-консоль запрашивает подтверждение опасных действий, операции фиксируются в аудите.

Публичный `POST /webhook/panel` проверяет HMAC-SHA256 и временное окно, отслеживает повторную доставку. Событие `user.updated` обновляет локальную запись пользователя; прочие типы событий инвалидируют кеш, а состояние сверяется периодической синхронизацией. Путь и ключ webhook задаются в конфигурации.

### Доступность нод

Core и Agent используют общий пакет проверок TCP/TLS, WebSocket, HTTPUpgrade, gRPC и XHTTP, включая поддерживаемые HTTP3-проверки. Результат описывает подтверждённый уровень доступности транспорта, а не полноценную VPN-сессию. Неподдерживаемые сочетания возвращают `unverified`; TCP-доступность REALITY не доказывает успешную REALITY-аутентификацию.

Фоновые проверки учитывают кворум, пороги ошибок и восстановления, минимальное время карантина, backoff и окна обслуживания.

| Методы | Путь                         | Назначение                                                                 |
| ------------ | -------------------------------- | ------------------------------------------------------------------------------------ |
| POST         | `/nodes/{id}/probe`            | Ручная проверка ноды                                               |
| GET          | `/health/probes`               | Сохранённые результаты                                          |
| POST         | `/nodes/{id}/block-check`      | TCP/TLS-диагностика синхронизированного endpoint       |
| GET          | `/blocks`                      | Результаты диагностики                                          |
| POST, GET    | `/youtube/check`, `/youtube` | Запуск YouTube-проверки и сохранённые результаты |

YouTube-проверка выполняется с Core; переданный `node_id` не направляет запрос через соответствующую VPN-ноду.

### Agent, трафик и журнал

Agent опрашивает Xray StatsService с `reset=false`, вычисляет скорость и приращения счётчиков, читает локальный access.log и отправляет пакеты в Core. Сбор выполняется раз в секунду, heartbeat — раз в 30 секунд. Для перебоев связи предусмотрена ограниченная очередь и зашифрованный spool; Core распознаёт повторные пакеты.

| Методы | Путь                                      | Назначение                                                             |
| ------------ | --------------------------------------------- | -------------------------------------------------------------------------------- |
| GET          | `/agents`                                   | Состояние и диагностика Agent                               |
| POST         | `/agents/enrollment-tokens`                 | Одноразовый токен регистрации, срок 15 минут |
| DELETE       | `/agents/{id}`                              | Отзыв доступа Agent                                                  |
| GET          | `/live`, `/live/stream`, `/live/export` | Текущие скорости, SSE и CSV-экспорт                       |
| GET          | `/traffic`, `/users/{id}/traffic`         | История трафика                                                    |
| GET          | `/activity`, `/activity/export`           | Поиск и экспорт событий соединений                 |
| DELETE       | `/activity/{user}`                          | Удаление сохранённых событий пользователя  |

Отдельные маршруты Agent: `POST /agents/enroll`, `POST /agents/heartbeat`, `POST /agents/telemetry`, `GET /agents/health-targets`, `PUT /agents/health-results`. Регистрация использует одноразовый токен, остальные запросы — Bearer-токен Agent.

События активности хранятся зашифрованными; чтение и экспорт фиксируются в аудите. Назначение соединения представлено как host:port. Срок хранения активности по умолчанию — 14 дней, данных скорости — 24 часа.

### Доступ и эксплуатация

- Одна учётная запись владельца, вход по логину и паролю, ограничение попыток входа. Роли и 2FA не предоставляются.
- Геоправила подписок по CIDR, стране, ASN и типу сети; локальные JSON-prefix/MMDB-источники и перезагрузка IP-данных.
- Анализ подозрительных запросов подписок, задержка или отказ по сигналам anti-scan.
- Зашифрованные локальные снимки SQLite `.sgb` и offline-восстановление. Для восстановления нужен исходный ключ шифрования.
- Telegram-доставка через модуль уведомлений и тестовый endpoint. Реестр `alerts` хранит заметки; автоматические условия и отправка из этого реестра не подключены.

| Методы | Путь                                      | Назначение                                                               |
| ------------ | --------------------------------------------- | ---------------------------------------------------------------------------------- |
| POST         | `/auth/login`, `/auth/logout`             | Вход и выход                                                             |
| GET          | `/auth/me`                                  | Текущий владелец                                                    |
| GET          | `/system/status`, `/settings`, `/audit` | Состояние, конфигурация без секретов и аудит |
| GET, POST    | `/ipintel/lookup`, `/ipintel/reload`      | Поиск IP и перезагрузка локального источника  |
| GET, POST    | `/backups`                                  | Список и создание резервных копий                     |
| GET          | `/backups/{name}`                           | Скачивание копии                                                    |
| POST         | `/notifications/test`                       | Тестовая отправка уведомления                           |

Публичные служебные маршруты: `GET /health` проверяет доступность БД, `GET /version` возвращает версию, `GET /metrics` — метрики в формате Prometheus. Консоль доступна по `/` и `/console`.

## Установка через Docker

Команды ниже выполняются в Bash на Linux. На центральном сервере нужны Git, Docker Engine и плагин Docker Compose v2; на VPN-нодах — Docker Engine, Compose v2 и curl. Инструкция установки Docker: https://docs.docker.com/engine/install/.

Core запускается на центральном сервере за HTTPS-прокси Caddy или Nginx. На каждой VPN-ноде запускается отдельный Agent из готового образа `ghcr.io/mrzalupa-lolz/subgate-agent:latest`. Go на серверах устанавливать не нужно: Core собирается внутри Docker, агент скачивается из GitHub Packages (GHCR).

### Центральный сервер: подготовка

1. Создайте DNS-запись `A` для `subgate.example.com`, указывающую на центральный сервер. Если есть `AAAA`, она тоже должна вести на этот сервер.
2. Разрешите входящие TCP-порты 80 и 443. Core должен иметь исходящий доступ к API Remnawave и upstream подписок; ноды — HTTPS-доступ к Core.
3. Получите исходники и создайте конфигурацию:

```bash
git clone https://github.com/Mrzalupa-lolz/subgate.git
cd subgate
cp deploy/.env.example deploy/.env
chmod 600 deploy/.env
nano deploy/.env
```

Заполните `deploy/.env`, заменив домены и секреты своими значениями:

```dotenv
SUBGATE_PANEL_URL=https://panel.example.com
SUBGATE_PANEL_TOKEN=replace-with-remnawave-api-token
SUBGATE_SUBSCRIPTION_URL=https://subscriptions.example.com/api/sub
SUBGATE_PUBLIC_URL=https://subgate.example.com
SUBGATE_DOMAIN=subgate.example.com
SUBGATE_ADMIN_USERNAME=admin
SUBGATE_ADMIN_PASSWORD=replace-with-a-long-unique-password
SUBGATE_TRUSTED_PROXIES=
```

`SUBGATE_SUBSCRIPTION_URL` — базовый адрес upstream до `/{shortUuid}/{clientType}`, без идентификатора пользователя. Он должен вести к исходному сервису подписок, иначе получится петля запросов. Токен Remnawave хранится только на центральном сервере. Пароль владельца — не короче 12 символов; он применяется при создании первой учётной записи. Изменение переменной после установки не меняет пароль существующего владельца.

Core публикует порт только на `127.0.0.1:8080`. БД, ключ `subgate.key` и резервные копии находятся в именованном volume `subgate_core-data`. Миграции выполняются при запуске. `.env` с секретами не добавляйте в Git.

Выберите один из следующих вариантов HTTPS-прокси.

### Вариант 1: Caddy в Docker

Подходит для сервера, где порты 80 и 443 свободны. В Compose уже есть Caddy с автоматическим получением и продлением сертификата:

```bash
# Выполняйте из каталога subgate.
docker compose --env-file deploy/.env -f deploy/compose.yaml --profile tls up -d --build
docker compose --env-file deploy/.env -f deploy/compose.yaml --profile tls ps
docker compose --env-file deploy/.env -f deploy/compose.yaml --profile tls logs --tail=100 core caddy
```

Откройте `https://subgate.example.com` и войдите с данными из `.env`. Caddy использует [deploy/Caddyfile](deploy/Caddyfile), обращается к `core:8080` по сети Docker и хранит сертификаты в отдельных volumes. Для HTTP/3 дополнительно разрешите UDP-порт 443.

### Вариант 2: Nginx на центральном сервере

Если Nginx уже обслуживает сервер, добавьте в него отдельный домен SubGate. Core запускайте без профиля Caddy:

```bash
docker compose --env-file deploy/.env -f deploy/compose.yaml up -d --build core
curl -fsS http://127.0.0.1:8080/health
```

Ниже пример для Ubuntu/Debian с Nginx и Certbot. Если они ещё не установлены:

```bash
sudo apt-get update
sudo apt-get install -y nginx certbot python3-certbot-nginx
```

Создайте `/etc/nginx/sites-available/subgate`, заменив домен:

```nginx
server {
    listen 80;
    listen [::]:80;
    server_name subgate.example.com;
    client_max_body_size 2m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_buffering off;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
```

Включите сайт и выпустите сертификат. Порт 80 должен быть доступен извне, а DNS уже указывать на сервер:

```bash
sudo ln -s /etc/nginx/sites-available/subgate /etc/nginx/sites-enabled/subgate
sudo nginx -t
sudo systemctl reload nginx
sudo certbot --nginx -d subgate.example.com --redirect
sudo nginx -t
sudo certbot renew --dry-run
```

Certbot добавит HTTPS и перенаправление с HTTP. Проверьте, что включён системный таймер или cron для автоматического продления сертификатов. `proxy_buffering off` позволяет сразу получать события SSE в консоли. Откройте `https://subgate.example.com` и войдите с данными из `.env`.

В [deploy/nginx.conf.example](deploy/nginx.conf.example) есть дополнительный вариант с fallback `/sub/...` на исходный сервис подписок при ошибках Core 502/503/504. Для него замените домены и пути сертификатов. При fallback подписка выдаётся напрямую, без применения правил Core; консоль и API через fallback не обслуживаются.

`SUBGATE_TRUSTED_PROXIES` задаёт CIDR только тех прокси, которыми вы управляете. Пустое значение отключает доверие к пересланному IP клиента. Для географических правил по реальному IP укажите адрес непосредственного прокси, который Core видит в Docker-сети; это не обязательно `127.0.0.1` даже при Nginx на хосте. Для Caddy это адрес контейнера Caddy. Закрепите адрес или используйте отдельную сеть только для доверенных сервисов, прежде чем доверять её CIDR.

### Подключение VPN-нод: готовый образ GHCR

На ноде уже должен работать Remnanode/Xray. Включите Xray StatsService на локальном адресе и запись `access.log` с email пользователя; файл должен быть доступен агенту для чтения. Агент использует сеть хоста (`network_mode: host`) для доступа к локальному StatsService и не открывает входящих HTTP-портов.

1. В Core дождитесь синхронизации ноды из Remnawave.
2. Откройте **«Агенты → Подключить агент»**, выберите ноду и создайте одноразовый токен.
3. Выполните предложенную интерфейсом команду на выбранной Linux-ноде. Токен действует 15 минут.

Можно вызвать тот же установщик вручную. Подставьте HTTPS-адрес Core и UUID ноды из Remnawave; токен установщик запросит интерактивно:

```bash
installer=$(mktemp)
curl -fsSL https://raw.githubusercontent.com/Mrzalupa-lolz/subgate/main/deploy/install-agent.sh -o "$installer"
sudo bash "$installer" \
  --core-url https://subgate.example.com \
  --node-id UUID-НОДЫ \
  --image ghcr.io/mrzalupa-lolz/subgate-agent:latest
```

Установщик скачает готовый образ для amd64/arm64, создаст `/opt/subgate-agent`, проверит чтение журнала, зарегистрирует агент и запустит Docker Compose. На ноде не нужны исходники проекта и сборка. Образ должен быть публичным в GitHub Packages, чтобы скачиваться без авторизации.

Каталог логов определяется по стандартным путям или bind mount контейнера Remnanode, GID — по группе `access.log`. Для нестандартной установки добавьте параметры:

```bash
# Добавьте к команде установки при необходимости:
--log-dir /actual/xray/logs --log-gid 1234 --stats-addr 127.0.0.1:61000
```

Здесь каталог должен содержать `access.log`, GID и порт замените своими. Группа должна иметь права чтения файла и прохода по каталогу; установщик права логов не меняет. В режиме `auto` агент ищет StatsService локально; если обнаружение не сработало, укажите фактический loopback-адрес Xray. Не публикуйте StatsService в интернет.

Проверьте контейнер и логи:

```bash
cd /opt/subgate-agent
sudo docker compose ps
sudo docker compose logs --tail=100 -f agent
```

В Core проверьте свежий heartbeat и диагностику источников. Работающий контейнер сам по себе не подтверждает доступность StatsService или чтение журнала. Конфигурация хранится в `/opt/subgate-agent/.env`, токен и очередь отправки — в volume `subgate-agent_agent-data`. Одноразовый токен установщик в `.env` не записывает.

### Обновление агента на ноде

После публикации нового образа выполните на каждой ноде:

```bash
sudo bash /opt/subgate-agent/install-agent.sh --update
```

Команда скачивает актуальный образ выбранного тега и пересоздаёт контейнер при изменении образа. Регистрация, настройки, токен и очередь сохраняются; новый одноразовый токен не нужен. Если установлен `latest`, будет скачан свежий `latest`. Сам по себе этот тег контейнеры не обновляет, а `docker compose restart` новый образ не скачивает.

То же обновление вручную через Compose:

```bash
cd /opt/subgate-agent
sudo docker compose pull agent
sudo docker compose up -d --wait agent
sudo docker compose logs --tail=100 agent
```

Для перехода на конкретную опубликованную версию или возврата на `latest`:

```bash
sudo bash /opt/subgate-agent/install-agent.sh --update \
  --image ghcr.io/mrzalupa-lolz/subgate-agent:0.2.1

sudo bash /opt/subgate-agent/install-agent.sh --update \
  --image ghcr.io/mrzalupa-lolz/subgate-agent:latest
```

`0.2.1` — пример: используйте существующий тег из Packages. Выбранный тег сохраняется для последующих обновлений. При установке в другой каталог передайте также `--install-dir /ваш/каталог`. Обновление образа не обновляет сохранённую копию установщика и Compose-файл; изменения этих файлов применяются отдельно после проверки.

Не удаляйте volume с токеном: `docker compose down` сохраняет его, `docker compose down -v` удаляет. После обновления проверьте версию, heartbeat и диагностику в Core.

### Публикация новой версии образа агента

Чтобы обновить сам пакет `ghcr.io/mrzalupa-lolz/subgate-agent`, сначала отправьте изменения кода в GitHub. Затем откройте **Actions → Publish agent image → Run workflow**, выберите нужную ветку и укажите новую версию без `v`, например `0.2.1`. Альтернатива — тег на нужном коммите:

```bash
git tag agent-v0.2.1
git push origin agent-v0.2.1
```

Workflow проверит агент и опубликует образы `ghcr.io/mrzalupa-lolz/subgate-agent:0.2.1` и `ghcr.io/mrzalupa-lolz/subgate-agent:latest` для amd64/arm64. Обычный push в `main` этот workflow не запускает. Используйте новую версию для каждого выпуска. Отдельный PAT для публикации не нужен: workflow использует `GITHUB_TOKEN` с правом `packages: write`.

В настройках пакета GHCR установите видимость **Public** для скачивания без логина; при необходимости разрешите репозиторию доступ в **Manage Actions access**. Дождитесь успешной публикации, затем выполните обновление на нодах — публикация пакета сама по себе их не перезапускает.

### Обновление центрального сервера

Перед обновлением создайте резервную копию в консоли и сохраните её вместе с исходным `subgate.key` вне сервера. Для копирования ключа в защищённый каталог из корня репозитория:

```bash
umask 077
docker compose --env-file deploy/.env -f deploy/compose.yaml cp core:/data/subgate.key ./subgate.key
chmod 600 ./subgate.key
```

Ключ не добавляйте в Git. Затем получите изменения и пересоберите Core:

```bash
git pull --ff-only

# При Caddy:
docker compose --env-file deploy/.env -f deploy/compose.yaml --profile tls up -d --build

# При Nginx вместо предыдущей команды:
docker compose --env-file deploy/.env -f deploy/compose.yaml up -d --build core

curl -fsS https://subgate.example.com/health
docker compose --env-file deploy/.env -f deploy/compose.yaml logs --tail=100 core
```

Core в этом Compose собирается из исходников: одного `docker compose pull` для его обновления недостаточно. Именованный volume сохраняет данные при пересоздании контейнера. Не используйте `down -v`.

### Восстановление резервной копии через Docker

Из корня репозитория остановите Core. Копия из примера должна находиться в `/data/backups/backup.sgb` внутри volume, а выходной файл ещё не должен существовать:

```bash
docker compose --env-file deploy/.env -f deploy/compose.yaml stop core
docker compose --env-file deploy/.env -f deploy/compose.yaml run --rm --no-deps core \
  -restore /data/backups/backup.sgb \
  -key-file /data/subgate.key \
  -output /data/restored.db
```

Для запуска восстановленной БД замените `SUBGATE_DATABASE_URL` в секции `services.core.environment` файла `deploy/compose.yaml` на `file:/data/restored.db` и выполните соответствующую команду `up -d` для вашего прокси. Именно Compose задаёт этот путь поверх `.env`. Сохраните доступ к исходному ключу: файл `.sgb` его не содержит.

### Локальная демонстрация в Docker

Для знакомства без подключения Remnawave из корня репозитория:

```bash
docker compose -f deploy/compose.demo.yaml up --build -d
```

Откройте `http://127.0.0.1:8080` на этом же компьютере: логин `admin`, пароль `subgate-demo`. Данные demo хранятся в отдельном volume. Запускайте demo отдельно от production Core: оба используют локальный порт 8080.

## Тестирование

```bash
go build ./...
go vet ./...
go test ./...
make check
```

`make check` запускает обычные тесты, race detector, vet, проверку gofmt, синтаксиса `web/app.js` и `deploy/install.sh`, затем собирает бинарники. Полный набор предназначен для Linux с CGO и C-компилятором для race detector.

Дополнительные проверки:

```bash
python3 scripts/generate-panel.py --check  # Контракт панели и сгенерированный код
make smoke                              # Сборка и изолированная проверка бинарников
make bench                              # Go-бенчмарки
```

Большинство Go-тестов использует временные или in-memory БД и локальные тестовые серверы. Отдельные интеграционные сценарии требуют явного включения:

| Переменная                                               | Проверка                                                                                                                |
| ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------- |
| `SUBGATE_SYNTHETIC_PANEL_URL`, `SUBGATE_SYNTHETIC_PANEL_TOKEN` | Отдельная синтетическая панель; для подписок также `SUBGATE_SYNTHETIC_SHORT_UUID` |
| `SUBGATE_SYNTHETIC_XRAY_BINARY`                                  | Изолированный Xray для транспортных проверок                                                |
| `SUBGATE_TEST_XRAY_SERVER`                                       | Интеграция с Xray StatsService                                                                                       |
| `SUBGATE_RUN_NGINX_FALLBACK=1`                                   | Docker/Nginx-сценарий отказа Core                                                                                 |

Без нужного окружения эти сценарии пропускаются. Успешный `go test ./...` с пропусками не подтверждает проверку живой панели, Xray и Nginx fallback. Для внешних проверок используйте отдельную тестовую инфраструктуру.
