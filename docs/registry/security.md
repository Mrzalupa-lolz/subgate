# Безопасность и выявление подозрительной активности
О чём: owner auth, client IP, мониторинг запросов и сигнализация F21.
Когда читать: auth, пользовательский ввод, сетевые ограничения или сигналы злоупотреблений.

## Инварианты
Не читать/выводить secrets, .env*, private keys, существующие DB/backups. Тестировать на synthetic state.
Production panel только GET; разрешённые test-panel writes требуют подтверждения конкретной операции и итогового audit.
Xray accounting counters reset=false; destination metadata только host:port, без URL/content/payload.
Никаких чужих PostgreSQL schemas, FIFO log sink или Docker socket.
Одна password-only owner учётка по D06; роли/группы/TOTP/WebAuthn не добавлять.

## Auth и audit
Код: core/internal/server/auth.go, activity_access.go, storage_keys.go; core/internal/config/.
Sessions opaque/hash SQLite, HttpOnly/SameSite Strict, Secure по HTTPS/public URL; CSRF+Origin на cookie mutations.
Пять login attempts/min по resolved IP. Несколько existing accounts блокируют startup/login/session; автоматически их не удалять.
PBKDF2 используется вместо желаемого argon2id; persistent lockout и отдельные API keys не готовы.
TestSingleOwnerPasswordLogin, TestOwnerPasswordOnlyConfiguration, TestAuthenticationCSRFAndOwnerEnforcement,
TestSecureCookieTracksPublicEndpoint и TestOwnerActivityAccess PASS; это узкие проверки, не полный threat model.

Activity read/export: final outcome, owner, normalized filters/range/count шифруются и audit fail закрывает выдачу данных.
Общий бюджет60/min по owner переживает смену session/IP; bulk alert пока нет. Детали limits в [api](../api.md).
Enrollment/manual-sync/quota success audit и mutation атомарны; panel operations ещё требуют final-outcome audit.
Storage key rollover после restart: один active32-byte key + до8 previous read keys, regular private files без symlink/FIFO.
Новые ciphertext active, старые readable previous; автоматического re-encrypt/retirement нет. Key loss делает данные невосстановимыми.

## Наблюдения Xray и решения

Код: `core/internal/analysis/window.go`, `core/internal/server/analysis.go`, `telemetry.go`; интерфейс — «Разбор аномалий» и «Правила анализа».

`GET /api/v1/incidents` выдаёт объяснимые наблюдения: частоту уникальных записей Xray, число IP назначения и число портов одного назначения. `PUT /incidents/{id}/review` сохраняет решение и заметку владельца. `GET/PUT /analysis/policy` управляют порогами и проектом shaper. Доступ — только owner, изменения — с CSRF; чтение наблюдений и решения журналируются. Payload наблюдений, окон и заметок зашифрован. Аналитика коммитится вместе с телеметрией; retries не увеличивают счётчики. Состояние разбора сохраняется при новых измерениях.

Это наблюдения, не автоматический вердикт. Большой расход, общий IP и P2P отдельно не доказывают нарушение. Сканирование/исходящие атаки требуют контекста, повторных подтверждений и уверенной атрибуции. Пропуски данных и насыщение агрегатов должны быть видны оператору.

Новый requestmonitor и API request-signals убраны вместе с gateway: запросы подписок обслуживает middleware. Geo deny и geo-rules также не исполняются. F23 внешний node protection не входит в SubGate.

## F25 — временное ограничение подтверждённых нарушителей

Вернул в план владелец 2026-11-01. Цель — автоматическое временное ограничение при высокой уверенности, иначе ручной разбор. Значения лимита и срока пока не выбраны, исполнитель eBPF/TC не реализован. Проект настройки не применяет ограничений. Нужны TTL на ноде, автоматическое снятие при потере Core, allowlist, audit, idempotency и подтверждение применения.

## Глубокий анализ

Реализован отдельный nDPI-коллектор: классификация/счётчики без содержимого трафика, сброс прав после открытия capture, поддержка nDPI 4.14/4.2. Для санкции нужны эксклюзивные выделенные интерфейсы канонического аккаунта, два разных достоверных потока одного выбранного протокола в двух минутах, длительная перегрузка и независимые счётчики Xray. Общий IP, cache/port guesses и потери запрещают автоматическое решение. Неизвестное/зашифрованное содержимое остаётся неизвестным; две производные одного лога не считаются независимыми. Условия, режим атрибуции и ограничения: [NODE_MONITORING.md](../NODE_MONITORING.md).

Текущие окна/границы и этапы: [PROJECT_PLAN](../PROJECT_PLAN.md). Глобальная SSRF-защита operator URLs не доказана: block-check ограничен synced targets, YouTube allowlist проверяется на каждом redirect.
