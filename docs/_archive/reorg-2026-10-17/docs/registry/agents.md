# Реестр: agents

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 3. Platform, language and stack

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-033 | Agent↔Core: bidirectional gRPC stream or WebSocket + protobuf + zstd, outbound-only; choose by benchmark (ADR)<br>Источник: [v4:61](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-agent](EVIDENCE.md#e-agent); TestEnrollmentAndSyncAuditFailures (R09 atomic token/agent + audit, rollback/retry PASS); TestEnrollmentOneTimeIdentityRevocation; TestEncryptedSpoolReplaysSameBatchAfterFailedSendAndRestart; TestTelemetryBatchBoundsAndExplicitOverflowCount; TestRevokedCredentialPreservesPendingForReenrollment; TestTelemetryRejectsExpiredOrFutureSendTime | Outbound HTTP JSON, не stream/protobuf+zstd. Нет downloadable signed one-liner, probe subcommand, backoff/auto-update/mTLS/nft plugins; heartbeat heap Alloc не RSS, нет CPU/disk/NIC/conntrack/retransmits/process metrics. |

#### 8. Functional requirements / F15 — Agent and Probe

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-107 | Enrollment via one-time token from the UI; install one-liner served by Core, version-pinned, checksum **and signature** verified; systemd unit; outbound-only; batching, back-off, replay protection, clock-skew tolerance; revocable per-agent tokens; optional auto-update.<br>Источник: [v4:201](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-agent](EVIDENCE.md#e-agent); TestEnrollmentOneTimeIdentityRevocation; TestEncryptedSpoolReplaysSameBatchAfterFailedSendAndRestart; TestTelemetryBatchBoundsAndExplicitOverflowCount; TestRevokedCredentialPreservesPendingForReenrollment; TestTelemetryRejectsExpiredOrFutureSendTime | Outbound HTTP JSON, не stream/protobuf+zstd. Нет downloadable signed one-liner, probe subcommand, backoff/auto-update/mTLS/nft plugins; heartbeat heap Alloc не RSS, нет CPU/disk/NIC/conntrack/retransmits/process metrics. |
| F-108 | Telemetry: heartbeat, CPU/RAM/disk, NIC throughput/drops, conntrack, TCP retransmits, Xray/remnanode state (read-only, least privilege, **no Docker socket**), per-user stats (F16), logs (F19).<br>Источник: [v4:202](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-agent](EVIDENCE.md#e-agent); TestEnrollmentOneTimeIdentityRevocation; TestEncryptedSpoolReplaysSameBatchAfterFailedSendAndRestart; TestTelemetryBatchBoundsAndExplicitOverflowCount; TestRevokedCredentialPreservesPendingForReenrollment; TestTelemetryRejectsExpiredOrFutureSendTime | Outbound HTTP JSON, не stream/protobuf+zstd. Нет downloadable signed one-liner, probe subcommand, backoff/auto-update/mTLS/nft plugins; heartbeat heap Alloc не RSS, нет CPU/disk/NIC/conntrack/retransmits/process metrics. |
| F-109 | Optional plugins (off by default): ingest reports from installed diagnostic tools; blocklist loader using **own nft set names** (never flush the ruleset) with dry-run, **anti-lockout safety timer**, automatic whitelist of the current SSH client and Core/panel IPs.<br>Источник: [v4:203](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-agent](EVIDENCE.md#e-agent); целевого теста нет; не проверено | Требуемого полного поведения нет. Outbound HTTP JSON, не stream/protobuf+zstd. Нет downloadable signed one-liner, probe subcommand, backoff/auto-update/mTLS/nft plugins; heartbeat heap Alloc не RSS, нет CPU/disk/NIC/conntrack/retransmits/process metrics. |

#### Part A. Per-project functions / 1. Case211/remnawave-admin (web + Telegram bot + agent; Python/FastAPI, React, PostgreSQL; AGPL-3.0 since 2.16)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-218 | **Node agent:** collects Xray log data, WebSocket command channel (remote terminal, script catalogue), install one-liner.<br>Источник: [inventory:19](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Outbound HTTP JSON, не stream/protobuf+zstd. Нет downloadable signed one-liner, probe subcommand, backoff/auto-update/mTLS/nft plugins; heartbeat heap Alloc не RSS, нет CPU/disk/NIC/conntrack/retransmits/process metrics. |
