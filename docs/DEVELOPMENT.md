# Разработка и проверки

Рабочая платформа — Linux. В CI и Docker используется Go 1.26, директива `go.mod` — 1.25.0. Для обычных Core/Agent CGO не нужен; для race detector нужны CGO и C-компилятор. Дополнительно используются Make, Node.js, Bash, Python 3; для контейнерных сценариев — Docker Engine и Compose v2.

## Локальный запуск

Из корня репозитория:

```bash
go run ./core/cmd -demo -database :memory: -listen 127.0.0.1:8080
```

Откройте `http://127.0.0.1:8080`: `admin` / `subgate-demo`. Это публичные данные синтетической demo-среды. Для обычного запуска используйте [инструкцию установки](DEPLOYMENT.md).

## Основные команды

```bash
go build ./...
go vet ./...
go test ./...
make check
python3 scripts/check-docs.py
```

Тесты запускаются на Linux (в CI — Ubuntu). Сборка `core/internal/server` на Windows не проходит намеренно (`syscall.O_NOFOLLOW`); `go test ./...` через WSL работает. Контракт панели проверяется синтетическими фикстурами 3.4.8 (`core/internal/panel/testdata/`), сверка панельной схемы и генерация клиента не используются. Шаг CI «Panel generation and schema drift» заменён на `python3 scripts/check-docs.py`.

`make check` выполняет test, race, vet, проверку gofmt и JavaScript/Bash, сценарии установщиков, затем сборку Core/Agent. Он рассчитан на Linux. `make build` создаёт `bin/subgate-core` и `bin/subgate-agent`; worker и native nDPI собираются отдельно через `make build-worker build-ndpi`.

| Команда | Назначение |
| --- | --- |
| `make smoke` | Сборка и изолированная проверка Linux-бинарников |
| `make test-deploy` | Сборка образов и изолированный Docker lifecycle с заглушками панели/StatsService |
| `make bench` | Go-бенчмарки с расходом памяти |
| `python3 test/ndpi_smoke.py` | Синтетический PCAP-корпус для собранного native-коллектора |
| `python3 test/monitoring_ui.py DEMO_URL` | Playwright-проверка настроек анализа и мобильного интерфейса на demo Core |

Большинство тестов использует временные БД и локальные серверы. Скрипт UI требует установленного Playwright/Chromium и отказывается работать с обычным Core.

## Проверки с явным включением

| Переменная | Что проверяется |
| --- | --- |
| `SUBGATE_SYNTHETIC_PANEL_URL`, `SUBGATE_SYNTHETIC_PANEL_TOKEN` | Контракт отдельной тестовой панели |
| `SUBGATE_SYNTHETIC_XRAY_BINARY` | Транспорты изолированного Xray |
| `SUBGATE_TEST_XRAY_SERVER` | Реальный Xray StatsService |
| `SUBGATE_TEST_TC_NETNS=1` | TC/TBF и снятие ограничения в отдельном Linux network namespace |

Отсутствие окружения приводит к SKIP. Успешный общий suite с пропусками не подтверждает live-интеграцию. Parent PASS со всеми пропущенными дочерними сценариями тоже не является её проверкой.

Используйте отдельную тестовую инфраструктуру. Панель production доступна только для чтения. Не используйте рабочие БД, ключи или журналы как fixtures; не отправляйте уведомления и не меняйте реальные интерфейсы при обычном прогоне.

## Последние сохранённые результаты

Это сведения из проверки реализации от **2026-11-01**, а не результат каждого последующего запуска. Свежие изменения требуют своих проверок.

- Ubuntu/WSL, Go 1.26: полный `go test ./... -count=1`, `go vet ./...`, полный race-прогон, `make build build-worker` и `make lint` прошли. Результаты относятся к прежнему набору проверок; health/quorum/maintenance с тех пор удалены из кода.
- Сценарии анализа проверяли дедупликацию, перезапуск, rollback вместе с аудитом, качество источников, отклонение неоднозначной атрибуции и полный путь ingest → команда → результат → отмена → снятие.
- TC/TBF проверен на настоящем ядре в изолированном namespace с veth: установка в обоих направлениях и снятие после TTL. Это не замер throughput рабочей ноды.
- Native-коллектор собран с `-Wall -Wextra -Werror` против nDPI 4.14.0 и 4.2.0. Синтетический корпус включает IPv4/IPv6 BitTorrent, HTTP, STUN/WebRTC, Source Engine UDP, неизвестный шифротекст, обрезанные пакеты и повторное использование TCP tuple. BitTorrent в 4.2 имеет guessed confidence и не допускается для автоматики. Проверен allowlist полей stdout без payload/IP/SNI/infohash/URL.
- Playwright на временном demo Core проверил сохранение политики, включение/выключение автоматики, пустые состояния, отсутствие JavaScript exceptions и горизонтального переполнения на 390×844.

Снимки интерфейса: [обзор](screenshots/monitoring-overview.png), [разбор](screenshots/monitoring-incidents.png), [мобильный обзор](screenshots/monitoring-mobile.png), [настройки](screenshots/monitoring-analysis.png), [мобильные настройки](screenshots/monitoring-analysis-mobile.png).

### Синтетический ingest

```bash
go test ./core/internal/server -run NoTests -bench BenchmarkTelemetry20Nodes1000Accounts -benchtime=3s -benchmem -count=1
```

Сохранённый замер: Ryzen 5 3500, Linux/WSL, 6 procs; 20 зарегистрированных нод, 1000 аккаунтов, по 300 начальных записей на окно аккаунта. Путь включает HTTP handler, auth, JSON, шифрование, агрегацию и commit SQLite.

| Событий в пакете | Время на пакет | Событий/с | B/op | allocs/op |
| ---: | ---: | ---: | ---: | ---: |
| 50 | 6,382 мс | 7835 | 404442 | 6562 |
| 1000 | 68,055 мс | 14694 | 4563578 | 50327 |

Это in-memory SQLite и httptest внутри процесса. Диск, сеть, параллельная доставка, SSE и длительная нагрузка в замер не входят. Производственная ёмкость 20 нод / 1000 аккаунтов и точность nDPI на реальном VPN-трафике остаются [задачами проверки](PROJECT_PLAN.md).

## Работа с документацией

[Оглавление](INDEX.md) перечисляет актуальные документы. Назначение и ограничения реализации описываются в справочниках; будущая работа — в одном плане, принятые требования — в решениях. Не создавайте отдельные STATUS/FEATURE_STATUS/отчёты на каждый цикл и копии исходных промптов. История изменений хранится в Git.

После изменения Markdown запускайте `python3 scripts/check-docs.py` и `git diff --check`. Проверка документации проверяет локальные ссылки, изображения, заголовки и наличие документов в оглавлении; она не зависит от старых архивов, числа разделов или размера файлов.
