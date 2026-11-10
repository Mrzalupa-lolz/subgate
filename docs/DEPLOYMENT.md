# Установка и обслуживание

Команды выполняются в Bash на Linux. Один Core работает на центральном сервере, отдельный Agent — на каждой VPN-ноде.

## Установка через Docker


Установка выполняется в Bash на Linux: один core на центральном сервере и один агент на каждой VPN-ноде. Нужны Docker Engine, Docker Compose v2 с поддержкой `--wait` и curl; на центральном сервере также Git. [Установка Docker](https://docs.docker.com/engine/install/).

### 1. Центральный сервер

Создайте DNS A/AAAA для домена SubGate и откройте TCP 80/443. Если порты свободны, установщик запустит Caddy и получит HTTPS-сертификат:

```bash
git clone https://github.com/Mrzalupa-lolz/subgate.git
cd subgate
sudo bash deploy/install-core.sh
```

Если репозиторий станет приватным, `git clone` потребует доступа (токен или SSH-ключ); для нод это не нужно — установщик агента отдаёт Core.

Установщик спросит домен SubGate, URL Remnawave и API-токен панели. Пароль администратора создаётся автоматически. Настройки сохраняются в `deploy/.env` с правами 600. Core собирается в Docker, проверяет доступ к nodes/users панели, запускается и проверяет публичный HTTPS. Откройте показанный адрес. Учётные данные:

```bash
sudo sed -n '/^SUBGATE_ADMIN_/p' deploy/.env
```

Если `deploy/.env` уже существует, установщик использует его и не меняет секреты. Для установки без диалога заполните файл по [deploy/.env.example](../deploy/.env.example) или передайте `SUBGATE_DOMAIN`, `SUBGATE_PANEL_URL`, `SUBGATE_PANEL_TOKEN` через окружение root-процесса. Замените все примеры доменов и секретов.

Если HTTPS уже обслуживается вашим Nginx/Caddy, сначала настройте прокси по [инструкции](DEPLOYMENT.md), затем:

```bash
sudo bash deploy/install-core.sh --proxy external
```

Core доступен прокси на `127.0.0.1:8080`; при занятом порте задайте `SUBGATE_CORE_PORT` в `deploy/.env` и такой же upstream в прокси. Выбранный режим сохраняется для обновлений. Проверка настроек и API панели без запуска core:

```bash
sudo bash deploy/install-core.sh --check
```

### 2. Каждая VPN-нода

На ноде должен работать Remnanode/Xray с StatsService и `access.log`. В Core дождитесь синхронизации нод из Remnawave, откройте **«Агенты → Подключить агент»**, выберите ноду и выполните выданную команду на ней. Одноразовый токен действует 15 минут.

Можно запустить установщик вручную; он запросит токен:

```bash
installer=$(mktemp)
curl -fsSL https://subgate.example.com/install-agent.sh -o "$installer"
sudo bash "$installer" --core-url https://subgate.example.com --node-id UUID-НОДЫ
```

Установщик скачивается прямо с Core (`GET /install-agent.sh`, встроен в бинарник), а не с GitHub. Образ агента берётся из ghcr.io: пакет GHCR должен оставаться публичным, иначе на ноде нужен `docker login ghcr.io`.

Установщик скачает образ, определит каталог лога и группу чтения, проверит доступ к core, StatsService, логу и volume. Только затем зарегистрирует агент и дождётся принятого core heartbeat. Настройки — `/opt/subgate-agent/.env`, постоянный токен и очередь — в Docker volume. Агент не открывает входящих HTTP-портов и не получает токен Remnawave.

При нестандартных путях добавьте `--log-dir /каталог/с/access.log --log-gid GID --stats-addr ЛОКАЛЬНЫЙ-АДРЕС`. Режим `auto` поддерживает abstract Unix socket современных Remnanode. Установщик не меняет Xray и права логов; [разбор ошибок](DEPLOYMENT.md#если-установка-остановилась).

Если после сохранения настроек произошёл сбой, продолжите установку:

```bash
sudo bash /opt/subgate-agent/install-agent.sh --resume
```

Сохранённый постоянный токен используется повторно. Если регистрация не завершилась, установщик запросит новый одноразовый токен. Проверка и диагностика:

```bash
sudo bash /opt/subgate-agent/install-agent.sh --check
cd /opt/subgate-agent
sudo docker compose ps
sudo docker compose logs --tail=100 agent
```

В Core проверьте свежий heartbeat и отсутствие ошибок источников. Новый установщик требует образ агента с командами `check` и `health`; перед использованием изменений [опубликуйте образ из того же коммита](DEPLOYMENT.md#публикация-новой-версии-образа-агента).

### 3. Обновление

Перед обновлением core сохраните резервную копию и исходный `subgate.key` вне сервера; [команды резервирования и восстановления](DEPLOYMENT.md). Из каталога репозитория на центральном сервере:

```bash
git pull --ff-only
sudo bash deploy/install-core.sh --update
```

На каждой ноде:

```bash
sudo bash /opt/subgate-agent/install-agent.sh --update
```

Настройки, БД, ключ, токен и очередь сохраняются. Образ агента можно закрепить через `--image ghcr.io/mrzalupa-lolz/subgate-agent:ВЕРСИЯ`. Повторная регистрация для обновления не нужна. Установщик core не делает `git pull` самостоятельно; обновление агента не заменяет сохранённый установщик и Compose-файл. Не используйте `docker compose down -v` на рабочей установке.

[Ручная установка, Nginx, резервирование, восстановление и выпуск образов](DEPLOYMENT.md). [Дополнительный мониторинг ноды и worker](NODE_MONITORING.md).

## Ручная настройка Docker и HTTPS

### Центральный сервер: подготовка

1. Создайте DNS-запись `A` для `subgate.example.com`, указывающую на центральный сервер. Если есть `AAAA`, она тоже должна вести на этот сервер.
2. Разрешите входящие TCP-порты 80 и 443. Core должен иметь исходящий доступ к API Remnawave; ноды — HTTPS-доступ к Core.
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
SUBGATE_PUBLIC_URL=https://subgate.example.com
SUBGATE_DOMAIN=subgate.example.com
SUBGATE_ADMIN_USERNAME=admin
SUBGATE_ADMIN_PASSWORD=replace-with-a-long-unique-password
SUBGATE_TRUSTED_PROXIES=
# Необязательно:
SUBGATE_METRICS_TOKEN=
SUBGATE_TELEGRAM_TOKEN=
SUBGATE_TELEGRAM_CHAT_IDS=
SUBGATE_TELEGRAM_TIMEZONE=
```

`SUBGATE_METRICS_TOKEN` (YAML `http.metrics_token`) — bearer-токен для Prometheus: без него `/metrics` доступен только владельцу с сессией (`Authorization: Bearer ТОКЕН`). Telegram-уведомления настраиваются через `SUBGATE_TELEGRAM_TOKEN`, `SUBGATE_TELEGRAM_CHAT_IDS` (числовые id через запятую) и `SUBGATE_TELEGRAM_TIMEZONE`. Панель опрашивается раз в минуту; webhook панели не нужен и не принимается.

Токен Remnawave хранится только в Core. Пароль владельца — не короче 12 символов; переменная применяется при создании первой учётной записи.

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

Откройте `https://subgate.example.com` и войдите с данными из `.env`. Caddy использует [deploy/Caddyfile](../deploy/Caddyfile), обращается к `core:8080` по сети Docker и хранит сертификаты в отдельных volumes. Для HTTP/3 дополнительно разрешите UDP-порт 443.

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


`SUBGATE_TRUSTED_PROXIES` задаёт CIDR только тех прокси, которыми вы управляете. Пустое значение отключает доверие к пересланному IP клиента. Для корректного определения IP клиента укажите адрес непосредственного прокси, который Core видит в Docker-сети; это не обязательно `127.0.0.1` даже при Nginx на хосте. Для Caddy это адрес контейнера Caddy. Закрепите адрес или используйте отдельную сеть только для доверенных сервисов, прежде чем доверять её CIDR.

### Подключение VPN-нод: готовый образ GHCR

На ноде уже должен работать Remnanode/Xray. Включите Xray StatsService на локальном адресе и запись `access.log` с email пользователя; файл должен быть доступен агенту для чтения. Агент использует сеть хоста (`network_mode: host`) для доступа к локальному StatsService и не открывает входящих HTTP-портов.

1. В Core дождитесь синхронизации ноды из Remnawave.
2. Откройте **«Агенты → Подключить агент»**, выберите ноду и создайте одноразовый токен.
3. Выполните предложенную интерфейсом команду на выбранной Linux-ноде. Токен действует 15 минут.

Можно вызвать тот же установщик вручную. Подставьте HTTPS-адрес Core и UUID ноды из Remnawave; токен установщик запросит интерактивно:

```bash
installer=$(mktemp)
curl -fsSL https://subgate.example.com/install-agent.sh -o "$installer"
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


## Если установка остановилась

Установщики сохраняют конфигурацию и данные. Исправьте указанную причину и повторите команду core; для агента используйте `--resume`.

| Сообщение | Что проверить |
| --- | --- |
| Panel preflight / HTTP 401 или 403 | URL панели без `/api`, API-токен с чтением nodes/users, ограничения прокси панели. Проверка делает только GET. |
| Public HTTPS unavailable | DNS A/AAAA, порты 80/443, журнал Caddy; при `--proxy external` — готовый HTTPS-прокси на порт `SUBGATE_CORE_PORT` (8080 по умолчанию). |
| access.log not found / permission denied | Фактический bind mount Remnanode; `--log-dir` и `--log-gid`. Группе нужны чтение файла и проход по каталогу. |
| Xray StatsService | `auto` ищет abstract Unix socket Remnanode, иначе использует 127.0.0.1:10085. Для другого локального адреса задайте `--stats-addr`. Для файлового Unix socket нужен mount внутрь контейнера. |
| Enrollment HTTP 401 | Токен истёк или использован. Создайте новый в Core и передайте `--resume --enrollment-token ТОКЕН`. |
| agent-data already contains a token | Остался volume предыдущей установки. Восстановите её конфигурацию и используйте `--resume`; не удаляйте очередь. |
| Unhealthy после запуска | Смотрите `docker compose logs --tail=100 agent`: готовность требует принятого core heartbeat. При `--resume` сохранённый постоянный токен используется повторно. |

`--check` агента проверяет источники и HTTPS без отправки телеметрии. Готовность контейнера означает успешный heartbeat за последние 90 секунд; диагностика источников остаётся видна в Core. При отозванном постоянном токене нужна повторная регистрация. Установщик не меняет Xray, firewall и существующие сайты Nginx.

Новые команды `check` и `health` должны присутствовать в опубликованном образе. Перед выкладкой нового установщика выпустите образ из того же коммита. Для старой установки сохраните копии `/opt/subgate-agent/.env` и `compose.yaml`, скачайте новые установщик и `compose.agent.yaml` из выбранного выпуска, замените сохранённые файлы установщика и Compose, затем выполните `--update` с соответствующим `--image`. Файл `.env` и volume сохраняются; повторная регистрация не нужна.

## Проверки развёртывания

`make test-deploy` собирает оба образа и запускает изолированный Docker-сценарий: проверка панели, чтение лога и запись volume от UID 10001, одноразовая регистрация, готовность по heartbeat, пересоздание контейнеров с сохранением ключа и постоянного токена. Используются локальные заглушки панели и StatsService. CI запускает этот же сценарий и проверки сбоев установщиков. Публичные DNS и выпуск сертификата проверяются установщиком на целевом сервере.
