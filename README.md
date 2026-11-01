# SubGate

SubGate автоматизирует мониторинг VPN-инфраструктуры Remnawave: принимает телеметрию нод и внутренние журналы Xray, показывает нагрузку и формирует объяснимые наблюдения для разбора владельцем.

Подписки и управление пользователями обслуживает `remnawave-subscription-middleware/`. SubGate не участвует в выдаче подписок. Внешняя DDoS/firewall-защита остаётся отдельным скриптам на нодах.

## Что работает

- Синхронизация нод и идентификаторов аккаунтов с Remnawave, webhooks и опрос панели; доступ панели по умолчанию только GET.
- Agent: Xray StatsService/access.log, live-скорости, история расхода, источники и назначения соединений, диагностика сбора и очередей.
- Наблюдения Xray: частота записей, разные IP назначения и порты одного назначения за минуту. Настраиваемые пороги, зашифрованные агрегаты и атомарный ingest без повторного подсчёта идентичных записей.
- «Разбор аномалий»: измерение/порог/время/аккаунт/нода, переход в журнал, заметки и решения владельца с аудитом. Наблюдения сохраняются после перезапуска, хранятся 14 дней; UI показывает последние 200.
- Проверки доступности нод, IP/ASN/страны, поиск, экспорт, резервные копии и аудит.
- Обновлённая консоль: цветные состояния, крупные кнопки, читаемые подписи; вкладка «Пользователи» удалена.

## Анализ и решения

Метка P2P, большой расход и множество соединений сами по себе не доказывают нарушение. Текущие три правила создают наблюдения для проверки, не называют активность атакой и не применяют санкций. Начальные пороги: 3000 записей, 128 портов одного назначения, 512 IP назначения за фиксированную UTC-минуту. Их нужно настроить на своей нагрузке. Поздние события остаются в журнале; текущие наблюдения формируются по событиям не старше 2 минут. При насыщении ограниченных агрегатов показывается неполнота данных.

F25 shaper реализован в дополнительном Linux worker: TC/TBF на выделенных интерфейсах аккаунта, абсолютный TTL, независимый systemd watchdog, исключения, откат частичного применения, аудит и подтверждение исполнения. Автоматика по умолчанию выключена; лимит, срок и ограничиваемые протоколы задаёт владелец в UI. Повторные подтверждения nDPI сопоставляются с длительной перегрузкой ноды и независимыми счётчиками Xray.

Дополнительный процесс libnDPI/libpcap передаёт классификацию и счётчики без содержимого трафика. Анализ перегрузки учитывает ёмкость интерфейса, CPU, потери, полноту минут, baseline и hysteresis. Для автоматического ограничения нужна настроенная эксклюзивная привязка выделенного сетевого пути к UUID аккаунта; общий IP/Xray access.log этого не доказывают. [Сборка, настройка и границы](docs/NODE_MONITORING.md), [план](docs/PROJECT_PLAN.md), [проверки](docs/MONITORING_VALIDATION.md), [API](docs/api.md).

## Архитектура и стек

```text
Remnawave API ← Core ← Agent: счётчики, журналы Xray, health
                 ↕
           SQLite + веб-консоль

VPN-клиент → middleware → сервис подписок Remnawave
```

Go (Linux), HTTP JSON + SSE, SQLite/WAL без CGO, встроенный HTML/CSS/vanilla JavaScript. В CI/Docker Go 1.26, директива go.mod — 1.25.0. Агент не открывает входящий HTTP-сервер и не получает токен панели. Данные журнала и наблюдений шифруются; cookie mutations требуют CSRF/Origin.

После запуска новой версии `/sub/...`, короткие subscription aliases, `/request-signals`, `/tiers`, `/policies`, `/pins` возвращают 404. Старые данные не удаляются. [Распределение функций с middleware](docs/middleware-integration.md), [архив кода](_archive/monitoring-scope-2026-11-01/README.md).

## Установка через Docker

Команды ниже выполняются в Bash на Linux. На центральном сервере нужны Git, Docker Engine и плагин Docker Compose v2; на VPN-нодах — Docker Engine, Compose v2 и curl. Инструкция установки Docker: https://docs.docker.com/engine/install/.

Core запускается на центральном сервере за HTTPS-прокси Caddy или Nginx. На каждой VPN-ноде запускается отдельный Agent из готового образа `ghcr.io/mrzalupa-lolz/subgate-agent:latest`. Go на серверах устанавливать не нужно: Core собирается внутри Docker, агент скачивается из GitHub Packages (GHCR).

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
```

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
| `SUBGATE_SYNTHETIC_PANEL_URL`, `SUBGATE_SYNTHETIC_PANEL_TOKEN` | Отдельная синтетическая панель для проверки контракта API |
| `SUBGATE_SYNTHETIC_XRAY_BINARY`                                  | Изолированный Xray для транспортных проверок                                                |
| `SUBGATE_TEST_XRAY_SERVER`                                       | Интеграция с Xray StatsService                                                                                       |
| `SUBGATE_RUN_NGINX_FALLBACK=1`                                   | Docker/Nginx-сценарий отказа Core                                                                                 |

Без нужного окружения эти сценарии пропускаются. Успешный `go test ./...` с пропусками не подтверждает проверку живой панели, Xray и Nginx fallback. Для внешних проверок используйте отдельную тестовую инфраструктуру.
