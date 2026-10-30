# Реестр: backups

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 8. Functional requirements / F18 — Backups

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-121 | Scheduled encrypted backups of SubGate state (SQLite snapshot + settings) to local disk, Telegram (auto-split large files) and optional S3; rotation by count/age; **safe restore** (single transaction, abort-and-rollback on any error, path-traversal-safe, superadmin only). The activity journal (F20) is excluded from backups by default (size, sensitivity); optional encrypted per-user/per-range export.<br>Источник: [v4:221](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-backup](EVIDENCE.md#e-backup); TestBackupRestoresCommittedWALData; TestEncryptedBackupRestoresRealDatabase | Локальный encrypted WAL snapshot доказан. Snapshot включает activity, вопреки default exclusion. CLI restore не проверен этим тестом: только signature/header, не integrity/transaction/version-safe restore. Нет schedules/rotation/panel backup/TG split/S3 SigV4/runtime remote destinations. |

#### 8. Functional requirements / F20 — User activity journal: "where did this user go, at any moment" — with compact storage

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-158 | Backups (F18) exclude the journal by default (size and sensitivity); optional encrypted per-user/per-range export.<br>Источник: [v4:274](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-backup](EVIDENCE.md#e-backup); целевого теста нет; не проверено | Требуемого полного поведения нет. Локальный encrypted WAL snapshot доказан. Snapshot включает activity, вопреки default exclusion. CLI restore не проверен этим тестом: только signature/header, не integrity/transaction/version-safe restore. Нет schedules/rotation/panel backup/TG split/S3 SigV4/runtime remote destinations. |

#### Part A. Per-project functions / 1. Case211/remnawave-admin (web + Telegram bot + agent; Python/FastAPI, React, PostgreSQL; AGPL-3.0 since 2.16)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-221 | **Backups:** create/restore/upload/send to Telegram/rotation/safe restore.<br>Источник: [inventory:22](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Локальный encrypted WAL snapshot доказан. Snapshot включает activity, вопреки default exclusion. CLI restore не проверен этим тестом: только signature/header, не integrity/transaction/version-safe restore. Нет schedules/rotation/panel backup/TG split/S3 SigV4/runtime remote destinations. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-278 | Backups of SubGate **and the panel**, scheduled, Telegram, safe restore with version check, pre-update snapshots — F18 extended **NEW**<br>Источник: [inventory:108](../../subgate-feature-inventory.md) | **PARTIAL** | [E-backup](EVIDENCE.md#e-backup); TestBackupRestoresCommittedWALData; TestEncryptedBackupRestoresRealDatabase | Локальный encrypted WAL snapshot доказан. Snapshot включает activity, вопреки default exclusion. CLI restore не проверен этим тестом: только signature/header, не integrity/transaction/version-safe restore. Нет schedules/rotation/panel backup/TG split/S3 SigV4/runtime remote destinations. |
