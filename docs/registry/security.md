# Безопасность, geo и anti-abuse
О чём: owner auth, client IP, geo gate, anti-scan, protection и сигнализация F21.
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
Enrollment/manual-sync/quota/injection success audit и mutation атомарны; panel operations ещё требуют final-outcome audit.
Storage key rollover после restart: один active32-byte key + до8 previous read keys, regular private files без symlink/FIFO.
Новые ciphertext active, старые readable previous; автоматического re-encrypt/retirement нет. Key loss делает данные невосстановимыми.

## Client IP и geo gate
Код: server/clientip.go, server/api.go:geoDenied; geoblock utility не доказывает runtime функций.
Trusted direct peer/proxy chain и v4mapped normalization есть; spoofed forwarded chain тестирован.
CDN real-IP только от проверенных диапазонов, IPv6 /64 grouping и CGNAT hints ещё нет.
Runtime geo CRUD поддерживает country/ASN/CIDR/network_type, first match по stored order, low-confidence country не deny.
Unknown allow/deny/grace, user exemption, policy scope, curated priority и 403/404/decoy/empty modes ещё не подключены полностью.
При неизвестном IP нужны явные semantics, IPv6 tests и rate-limited masked denied-request logs.
Политика намеренного geo deny отличается от аварийного fail-open; не возвращать пустую подписку из-за ошибок.

## Anti-scan и node protection
Код: antiscan/antiscan.go, server/server.go. Enumeration/rate/UA signals и deny подключены.
AnalyzeRequest не активирует ApplyTarpit: наличие ShouldTarpit не доказывает работающую escalation.
Node handshake corpus/signals, /24+ASN+UA correlation, configurable allowlists, firewall export и отдельный TG topic отсутствуют.
Не кодировать handshake строки по памяти; нужен реальный очищенный golden corpus; не блокировать клиентов только за curl/wget без согласованной политики.
F23: собственные nft tables/sets, никогда flush ruleset; strict/open/skip, ban-once, flag-drop/anti-spoof,
SYN/UDP/connection limits, Hysteria2/TUIC bulk ports, auto-detect node ports, panel/Core/SSH whitelist, ctguard,
threat lists, fleet sync, CrowdSec/UFW integration где уместно, safety timer/dry-run/rollback/anti-lockout.
Текущих nft runtime/plans/tests нет; сначала generate-only план и отказ трогать чужие правила.

## F21 и опциональный F25
Семь анализаторов temporal, geo/impossible-travel, ASN, behaviour, devices, HWID, UA не реализованы.
Нужны explainable evidence/thresholds, scoring, dedupe, delayed re-check, CGNAT buffer/agglomerations и P2P reports.
По D01 только сигналы: auto-ban/disable/speed penalty по обвинению запрещены.
F25 eBPF EDT shaper optional; независимое включение требует B05 и не может обходить запрет автоматических санкций.
Глобальная SSRF защита всех operator URLs не доказана: block-check ограничен synced targets, YouTube allowlist проверяется на каждом redirect.
Нужны URL policy для feeds/webhooks/editor docs, secret redaction, TLS/path/backup review, signed artifacts/SBOM/vulnerability gates.
