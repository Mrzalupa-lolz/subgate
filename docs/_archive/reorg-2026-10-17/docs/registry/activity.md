# Реестр: activity (F20 journal + storage)

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

Объединённый файл activity-journal + activity-storage: требования F20 (activity journal).

## Статус (сводка)

| Компонент | Статус | Пояснение |
|---|---|---|
| F-014, F-133–F-136 (audit, goal, source, scope) | PARTIAL | Runtime: AES-GCM JSON в SQLite, journal создан, методы не вызываются |
| F-137 (parse & discard raw) | PARTIAL | - |
| F-138 (agent coalescing) | MISSING | agent coalescing нет |
| F-139 (chatter control) | MISSING | chatter caps нет |
| F-140 (dictionary encoding) | STUB | - |
| F-141 (columnar segments) | STUB | - |
| F-142–F-147 (tiered retention) | STUB/MISSING | hot tier PARTIAL, warm/cold STUB, legal hold MISSING |
| F-148 (disk budget) | STUB | - |
| F-149 (WAL/crash-safe) | PARTIAL | - |
| F-150 (targets: ≤8B/event, ≥30×, ≥100k/s) | MISSING | не проверено |
| F-151–F-154 (query/UI) | PARTIAL | q/group не поддерживаются; JSON/CSV export есть, NDJSON нет |
| F-155–F-157 (security/audit/purge) | PARTIAL | R11 owner access, R12 read/export audit + key rotation PASS; TOTP/permissions отменены Q6, bulk alerts нет |
| F-159–F-164 (acceptance) | MISSING | - |
| F-185 (sensitive handling) | PARTIAL | - |
| F-266 (categories) | PARTIAL | - |

## Доказательства (Evidence)

**Файлы/функции:** core/internal/server/telemetry.go (telemetry/queryActivity/readActivity/exportActivity), activity_access.go (recordActivityAccess/parseActivityFilters/allowActivityRead/purgeActivity), core/internal/server/identity.go (activityIdentities), core/internal/activity/journal.go (Journal).

**Тесты (PASS):**
- R11: TestOwnerActivityAccess (owner full access, purge, anonymous401/CSRF403)
- R12: TestActivityReadExportAuditFilters/SharedRateLimit/InvalidFiltersAreNotSuccessfulReads/AuditIntegrityAndQueryFailure/ReadBudgetRecovery
- R12 key rotation: TestActivityKeyRotation (current encrypt + 8 previous decrypt, FIFO/symlink rejection)
- TestTelemetryIdempotencyEncryptionAndReadAudit
- TestActivityPanelLinksResolveXrayIdentitiesAndPurge
- TestActivityIdentitiesDoNotExpandAmbiguousAliases
- TestCSVExportEscapesSpreadsheetFormula

**Gaps (чего нет):**
- agent coalescing/chatter caps/dictionaries
- columnar segments/bloom/WAL journal ack
- tier compaction (hot→warm→cold), disk quota/legal hold
- capture switches/quality indicators/categories/session scrubber
- NDJSON export, q/group support
- автоматическое перешифрование/retirement
- bulk alerts

## Ссылки

См. [PROJECT_PLAN roadmap R28–R35](../PROJECT_PLAN.md) для этапов реализации.
