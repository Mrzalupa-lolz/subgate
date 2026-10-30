# SubGate: план и сводка реестра

Исходный аудит: 2026-10-13, Windows/amd64, Go 1.26.2. Кодовая версия **0.2.0** не доказывает готовность production-релиза. При разделении документа реализация и статусы не пересматривались.

## Цель и границы

SubGate — самостоятельный продукт владельца, заменяющий remnawave-admin. Предметные интеграции: **Remnawave по `remnawave-openapi.json` и VPN-ноды**; коннектор remnawave-admin запрещён. Panel — источник пользователей, статуса/срока подписок и собственного traffic accounting.

Требования: [prompt v4](../subgate-ai-prompt-v4.md) и [feature inventory](../subgate-feature-inventory.md). Их не редактировать. Более новые прямые указания владельца имеют приоритет; конфликты остаются открытыми вопросами. Inventory Part A — референс чужих продуктов, Part B — объединённый каталог. Reference/optional/non-goal строки не разрешают реализацию автоматически. Billing/sales/CRM, payload/URL inspection и multi-tenant SaaS исключены v4; scope редактора/abuse engine требует решения владельца.

Владелец разрешил автономную разработку по PROMPT.md, шаг 0 и roadmap R01–R60; ответы Q1–Q10 имеют приоритет. `F-001…F-289` — ID строк аудита, не исходные F1…F36 и не число независимых функций.

## Инварианты

- Fail-open, сохранение unknown formats/entries и неоднозначных mappings; не выдавать пустую подписку. Сейчас не доказаны transformer recovery/300 ms deadline и reverse-proxy fallback при убитом Core.
- Panel token только Core; отдельные revocable agent tokens; Agent outbound-only. Xray counters всегда `reset=false`, внешние reset без отрицательных дельт; accounting панели не менять.
- Нет Docker socket/чтения чужих PostgreSQL-схем; log files read-only, без FIFO sink. Только destination host:port/connection metadata, без payload/URL/pages.
- Client IP только direct peer и доверенная proxy chain; normalise v4mapped. CDN ranges, IPv6 /64 и CGNAT ещё отсутствуют.
- Secrets не раскрывать. AES-GCM сейчас защищает activity payloads, gateway snapshots, local backup и token-bound spool, а не всю SQLite/config. Key loss/rotation влияют на восстановление.
- По Q6 одна password-only учётка с полным доступом; R10 extra accounts блокируют startup/login/session. Activity range/filter/outcome audit реализован в R12; полный retention/bulk alerts ещё отсутствуют.
- Injection/quota/enrollment/manual-sync success audit mutations атомарны; panel action audit не гарантирует final outcome.
- Remote Agent требует TLS. Глобальная SSRF-защита всех URL не доказана. Linux — production-цель; native Windows проверки не доказывают Linux readiness.
- Queue/storage должны быть ограничены, gaps явными. Spool 16 MiB и bounded batches есть; hard RSS/disk budgets и полный cleanup отсутствуют.
- DONE только при behavioral assertions + наблюдённом PASS либо ручной проверке. CRUD/schema/utility, зелёная сборка и ИИ-отчёт не равны функции. SKIP не PASS.
- Сохранять чужие изменения; без reset/clean/checkout/restore, staging/commit без поручения. Не читать secrets/.env/keys/существующие DB/backup; проверки только с synthetic temporary state. Перед переписью docs — копия вне репозитория.

## Архитектура сейчас

Clients → внешний nginx/Caddy → Core (`chi/net/http`, embedded plain HTML/CSS/JS) → Remnawave REST/subscription upstream. Webhook HMAC/replay invalidates gateway cache; periodic sync default 1 min, без jitter. Core хранит SQLite WAL/state/snapshots/activity/speed/usage/audit. Отдельный Agent → HTTP JSON telemetry + encrypted spool → Core; Agent читает local gRPC StatsService и access-log. LiveStore → SSE full snapshots 1 Hz.

Два executables, spec **3.4.5**, один generated users reader + ручные endpoints, vanilla UI. Startup version diagnostic и чтение synthetic panel3.4.5 проверены; production запись запрещена. Gateway cache TTL30s/stale5min; выбирает одну node + injections. Health engine подключён к Core/Agent scheduled TCP/TLS quorum и maintenance windows; quarantine/recovering исключает однозначно mapped ноды из gateway, с fallback и original при исчерпании tiers; exhausted quota возвращает original. Activity — encrypted SQLite rows; IP-intel local/manual, без feeds/builder. Telegram только manual test. Installer local-only; signatures/updater/Core ACME/proxy fallback отсутствуют. Детали: [api.md](api.md), [Roadmap](PROJECT_PLAN.md#roadmap-по-этапам).

## Статусы по областям и оглавление реестра

DONE — весь объём строки доказан; PARTIAL — неполная реализация/проверка; STUB — схема/неподключённая заготовка; MISSING — требуемой реализации/gate нет. Повторы и reference строки сохраняются; сводка не является процентом готовности. Evidence относится к исходному аудиту: `FuzzTransform` только seeds, `TestLiveStatsService` SKIP.

| Область / файл | DONE | PARTIAL | STUB | MISSING | OUT_OF_SCOPE | Всего |
|---|---:|---:|---:|---:|---:|---:|
| [abuse](registry/abuse.md) | 0 | 0 | 0 | 1 | 1 | 2 |
| [activity](registry/activity.md) | 0 | 18 | 6 | 11 | 0 | 35 |
| [agents](registry/agents.md) | 0 | 3 | 0 | 1 | 1 | 5 |
| [anti-scan](registry/anti-scan.md) | 0 | 3 | 0 | 0 | 0 | 3 |
| [automation](registry/automation.md) | 0 | 0 | 0 | 1 | 1 | 2 |
| [backups](registry/backups.md) | 0 | 2 | 0 | 1 | 1 | 4 |
| [deployment](registry/deployment.md) | 0 | 7 | 0 | 0 | 2 | 9 |
| [devices](registry/devices.md) | 0 | 0 | 0 | 2 | 2 | 4 |
| [diagnostics](registry/diagnostics.md) | 0 | 3 | 0 | 1 | 4 | 8 |
| [drop-detection](registry/drop-detection.md) | 0 | 0 | 1 | 0 | 0 | 1 |
| [fleet-operations](registry/fleet-operations.md) | 0 | 0 | 0 | 1 | 1 | 2 |
| [gateway](registry/gateway.md) | 0 | 13 | 0 | 3 | 3 | 19 |
| [geo-protection](registry/geo-protection.md) | 0 | 2 | 0 | 0 | 0 | 2 |
| [health](registry/health.md) | 0 | 10 | 0 | 0 | 0 | 10 |
| [injections](registry/injections.md) | 0 | 2 | 0 | 2 | 2 | 6 |
| [ip-intelligence](registry/ip-intelligence.md) | 0 | 10 | 0 | 5 | 0 | 15 |
| [metrics](registry/metrics.md) | 0 | 2 | 0 | 0 | 1 | 3 |
| [node-optimization](registry/node-optimization.md) | 0 | 0 | 0 | 1 | 1 | 2 |
| [node-protection](registry/node-protection.md) | 0 | 0 | 0 | 2 | 2 | 4 |
| [notifications](registry/notifications.md) | 0 | 2 | 0 | 0 | 1 | 3 |
| [origins](registry/origins.md) | 0 | 1 | 0 | 2 | 0 | 3 |
| [panel](registry/panel.md) | 2 | 9 | 0 | 0 | 2 | 13 |
| [performance](registry/performance.md) | 0 | 0 | 0 | 8 | 0 | 8 |
| [plugins](registry/plugins.md) | 0 | 0 | 0 | 1 | 0 | 1 |
| [quality](registry/quality.md) | 0 | 18 | 0 | 5 | 0 | 23 |
| [quotas](registry/quotas.md) | 0 | 2 | 0 | 0 | 0 | 2 |
| [relay-topology](registry/relay-topology.md) | 0 | 0 | 0 | 1 | 0 | 1 |
| [routing](registry/routing.md) | 0 | 8 | 0 | 2 | 0 | 10 |
| [scope](registry/scope.md) | 0 | 0 | 0 | 9 | 1 | 10 |
| [security](registry/security.md) | 0 | 4 | 0 | 2 | 1 | 7 |
| [storage](registry/storage.md) | 0 | 14 | 0 | 0 | 0 | 14 |
| [subscription-lifecycle](registry/subscription-lifecycle.md) | 0 | 0 | 0 | 1 | 1 | 2 |
| [support](registry/support.md) | 0 | 0 | 0 | 2 | 4 | 6 |
| [telemetry](registry/telemetry.md) | 1 | 14 | 0 | 1 | 1 | 17 |
| [ui](registry/ui.md) | 0 | 10 | 1 | 2 | 3 | 16 |
| [webhooks](registry/webhooks.md) | 0 | 1 | 0 | 1 | 1 | 3 |
| [xray-logs](registry/xray-logs.md) | 0 | 9 | 0 | 2 | 0 | 11 |
| [xray-toolkit](registry/xray-toolkit.md) | 0 | 0 | 0 | 1 | 2 | 3 |
| **Всего** | 3 | 167 | 8 | 72 | 39 | **289** |

## Roadmap по этапам

Будущие R01–R60: каждый проход 1–2 часа, один проверяемый результат; большая задача делится, незавершённое остаётся PARTIAL. Новые Test* — планируемые, не evidence. Scope-зависимые ветви ждут ответа владельца. Полные исходные формулировки, отдельные substeps и команды: [ROADMAP](registry/ROADMAP.md). Для тестовых критериев `require_test` из [OPERATIONS](registry/OPERATIONS.md) требует наличие и PASS конкретного теста; затем Linux `make check`. Только synthetic/test fixtures.

| Этапы | Результат | Критерий готовности |
|---|---|---|
| R01–R04 | Panel contract/scopes/version | Removal grep, exact fixture tests и generation/spec drift |
| R05–R08 | Gateway/cache/fail-open | HEAD metadata; panic/300ms original bytes; killed Core fallback; webhook <1s |
| R09–R12 | Audit/single-owner/activity access по Q6 | Atomic audit failure; password-only owner; range/filter/export audit |
| R13–R18 | Health/tiers/probes/pin | Scheduling/N-M/quorum fixtures; actual exclusion; generic unknown; weighted rendezvous |
| R19–R23 | Quotas/injections/geo | Eligible fallback без reset; 80/90% transitions; scoped gating/pool; geo priority |
| R24–R27 | IP-intel/origins | Проверенные licences; last-good/atomic MMDB; trusted CDN и origin aggregates |
| R28–R35 | Logs/activity storage/query | Golden corpus/rotation; coalescing/caps; benchmark/AEAD/WAL/compaction/eviction/purge/session tests |
| R36–R40 | Telemetry/diagnostics/drops | Sampling budget/rollup; regional >=64KiB stall; node egress cooldown; baseline learning |
| R41–R45 | Protection/alerts/ops/backups | Tarpit/transition recovery/confirmed action tests; activity exclusion; safe restore version/integrity |
| R46–R48 | Wizard/UI/install/update | Screen/rollback/virtualization tests; unsigned release rejection; update rollback/ACME |
| R49–R50 | Grace/devices/presets/relay | Restore-on-payment, HWID/catalogue/app preset, subtractive accounting tests |
| R51–R58 | Inventory/optional расширения | Один workflow/check на проход; scope ответы до F21/F25/F26/F27; mail/CRM outside-v1 |
| R59–R60 | Quality/load/chaos | Linux make check; real-Xray без SKIP; fuzz/perf; iperf ±5%, no-reset, DB/Core/skew/log flood |

После прохода пересматривать только затронутые ID, assertions и gaps. `make check` требует Linux/make/POSIX shell/CGO/C compiler; не включает browser/live-Xray/fuzz/performance. Старый аудит: build/vet/tests/UI syntax PASS на Windows; race был блокирован CGO, Linux/Docker/browser/production не проверены. Эти проверки не запускались заново при разделении docs.

## Открытые вопросы

До решений соответствующие расширения не реализовывать. Полные исходные вопросы: [QUESTIONS](registry/QUESTIONS.md).

1. Какой scope v1 у F21 analysers/F27 editor при конфликте v4 и inventory?
2. Допустимы ли Telegram/email/S3/feeds/CRM/release downloads в границе внешних интеграций?
3. Part A только референс или весь каталог? Нужен v1/optional список; Android vs PWA, billing/editing конфликтуют.
4. При exhausted **всех** eligible nodes: serve all/panic+alert или другая политика? Одна exhausted с fallback должна исключаться.
5. Версия живой панели, apps/formats/users/nodes/scopes и безопасный способ test access (без секретов в чат)?
6. Activity defaults14/90/365, группы/исключения/legal hold/disk budget, permissions/backup exclusion?
7. Буквальный Go/Linux/ogen/Preact/stream/segments контракт или отклонения только после ADR/benchmarks?
8. Sync5min+jitter vs1min: SLA/нагрузка и immutable aliases для rename/revocation?
9. Какие v1/Linux/real-Xray/load/fail-open gates нужны релизу0.2.0?
10. После объединения старых P0/design/ADR документов решения/assumptions вести в плане до изменения правил?

## Расширения (F21–F36 из inventory Part B)

Эти функции добавлены в [inventory](../subgate-feature-inventory.md) Part B как NEW; их scope требует решений владельца Q1–Q3. Не реализовывать без явного подтверждения.

| Функция | Статус | Roadmap | Замечания |
|---|---|---|---|
| F21 — Abuse engine (7 analysers, scoring, escalation) | Q1 scope | R51 | Q1: только сигнализация, без автосанкций |
| F22 — Automation (rules/schedules/templates) | MISSING | R52 | - |
| F23 — Node protection (nftables, ban-once, limits) | MISSING | R53 | - |
| F24 — Diagnostics/optimizer (27 checks, BBR, tuning) | MISSING | R54 | - |
| F25 — eBPF per-IP shaper | Q1 optional | R55 | Опционально, риски |
| F26 — Fleet ops (remote terminal, scripts, lifecycle) | Q1 optional | R55 | Опционально |
| F27 — Xray toolkit (linter, generators, snippets) | Q1 scope | R56 | Q1: текстовый редактор с подсветкой |
| F29 — Grace squad (expired → limited, restore-on-payment) | MISSING | R49 | - |
| F30 — HWID/request log/client catalogue | MISSING | R49 | - |
| F31 — Per-app response rules & presets | MISSING | R49 | - |
| F32 — Multi-hop relay topology & subtractive accounting | MISSING | R50 | - |
| F33 — Support desk (tickets, SLA, templates) | Q2 optional | R57 | Q2: email excluded |
| F33b — Built-in mail server | OUT_OF_SCOPE | - | Q2: исключён, не рекомендуется |
| F35 — Prometheus /metrics & Grafana | MISSING | R58 | - |
| F36 — Plugin system (sandboxed) | MISSING | R58 | - |

См. также [BLOCKED](BLOCKED.md) для зависимостей.

## Навигация, evidence и архив

## Навигация, evidence и архив

- [INDEX: ID | область | статус](registry/INDEX.md). Поиск: `rg -n 'F-111' docs/registry/INDEX.md`, затем открыть только указанную область текущего этапа. Весь реестр целиком не читать.
- [REGISTRY_GUIDE](registry/REGISTRY_GUIDE.md) — исходное пояснение формулировок.
- [ROADMAP R01–R60](registry/ROADMAP.md); [OPERATIONS](registry/OPERATIONS.md) — команды, require_test, результаты.
- [AUDIT_HISTORY](registry/AUDIT_HISTORY.md) — происхождение изменений, connector removal inventory.
- [HTTP справочник](api.md); [DECISIONS](DECISIONS.md) — ключевые решения Q1–Q10.
- Архив документации: `~/SubGate-docs-archive-2026-10-14/` (OVERVIEW, EVIDENCE, activity-journal/storage, исходные subgate-*.md).

Старые копии перед разделением: `C:\Users\maxim\AppData\Local\Temp\subgate-plan-split-67b41147e9ab40c1a597dae40530cf82`, Windows audit `/tmp/subgate-step0-docs-ld050eci` и backup R01–R15 (`/tmp/subgate-r*-doc-backup`).

## Автономный цикл, 2026-10-14

Baseline `01bc690`; до: DONE 1, PARTIAL 180, STUB 8, MISSING 100. Шаг 0: 39 целых ID OUT_OF_SCOPE (Part A 37, самообновление 1, email 1). В смешанных ID исключены только email/S3/автодействия/самообновление; остальной объём остаётся в scope. Подробности: [DECISIONS](DECISIONS.md), [BLOCKED](BLOCKED.md), [STATUS](../STATUS.md). Копии перед правкой: `/tmp/subgate-step0-docs-ld050eci`.

R15 generic pass2026-10-14: pinned descriptor, opaque default and Generic visibility Core/Agent/manual; incomplete/manual limited endpoints remain unverified. TestUnknownProtocolGenericHandling plus negative regressions, final Linux make check/read-only review PASS. Shared healthprobe94.3% targeted coverage; broad IDs remain PARTIAL, counts3/167/8/72/39 unchanged. HTTP/QUIC/REALITY follow; liveXray/browser not verified. Backup `/tmp/subgate-r15-generic-doc-backup`.

R15 WS pass2026-10-14: configured path/Host/SNI sanitized from panel rawInbound, actual bounded WS handshake Core/manual/Agent+quorum. TestHTTPTransportProbe and negatives PASS; isolated actual Xray26.7.28 TestLiveWebSocketTransportProbe PASS. Final Linux make check/security re-review PASS; browser/regional unverified, other transports remain. Counts unchanged3/167/8/72/39. Backup `/tmp/subgate-r15-ws-doc-backup`.

R15 HTTPUpgrade2026-10-14: sanitized parameters and separate101 handshake Core/manual/Agent/report/quorum/latency. Runtime regressions GREEN, final make check/security review and actual isolated Xray26.7.28 live acceptance/rejection PASS. Counts unchanged3/167/8/72/39, gRPC/XHTTP/QUIC/REALITY next; browser/regional unverified. Backup `/tmp/subgate-r15-httpupgrade-doc-backup`.

R15 gRPC2026-10-14: sanitized configured methods and HTTP2 emptyHunk/status0/frame validation; actual isolated Xray26.7.28 normal/multi/custom/default service positive/negative PASS. Final Linux make check/security review PASS; health remains PARTIAL, counts3/167/8/72/39 unchanged. XHTTP/QUIC/REALITY next. Backup `/tmp/subgate-r15-grpc-doc-backup`.

R15 XHTTP2026-10-14: R15 XHTTP: sanitized path/Host/SNI, aliases/modes/extra pinned26.7.28; Core/manual/Agent OPTIONS preflight reports xhttp-options, quorum/latency accept. TestXHTTPConfiguredPathProbe/AgentReportsXHTTPConfiguredPathProbe/XHTTPOptionsRequiresConfiguredRoute PASS; plain200/redirect/wrong route/Host/oversize/deadline/TLS hostname reject. Literal percent/hash paths preserved (RED→GREEN, actual Xray PASS); TLS exactly ALPN[h3] unverified without TCP/failure votes (RED→GREEN). TestLiveXHTTPConfiguredPathProbe actual Xray26.7.28 auto/packet-up/stream-up/stream-one/splithttp and percent25/percent/hash PASS. This proves configured HTTP route, not XHTTP session/proxy-user authentication. Final Linux make check and read-only security re-review PASS; QUIC/REALITY and full transport matrix remain. Counts unchanged3/167/8/72/39; health PARTIAL. Backup /tmp/subgate-r15-xhttp-doc-backup.
