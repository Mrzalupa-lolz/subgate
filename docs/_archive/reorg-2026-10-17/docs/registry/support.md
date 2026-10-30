# Реестр: support

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### Part A. Per-project functions / 1. Case211/remnawave-admin (web + Telegram bot + agent; Python/FastAPI, React, PostgreSQL; AGPL-3.0 since 2.16)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-222 | **Built-in mail server:** MX delivery, DKIM/SPF/DMARC, inbound SMTP, spam score, bounce stop-list, DMARC report parsing, queue.<br>Источник: [inventory:23](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |
| F-223 | **Support desk:** tickets with SLA, templates, attachments, customer card, quiet-hours autoreply, external support-events API.<br>Источник: [inventory:24](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part A. Per-project functions / 2. Mrvibecodic/remnawave-subscription-middleware (PHP; beta; closest relative of SubGate)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-235 | **Encrypted subscription channel** (own clients only): rotating path label, key derived from the sub URL, fixed-size ciphertext responses.<br>Источник: [inventory:38](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-262 | Masking/decoy page + support chat widget — F7/F12 + F33 **NEW**<br>Источник: [inventory:92](../../subgate-feature-inventory.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Не реализовано; runtime не проверено. |
| F-280 | Support desk (tickets, SLA, templates), customer warnings (TG/email), outbound mail — F33 **NEW** (optional)<br>Источник: [inventory:110](../../subgate-feature-inventory.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Не реализовано; runtime не проверено. |
| F-281 | Built-in full mail server (MX, DKIM/SPF/DMARC, inbound) — F33b optional, **not recommended for v1**<br>Источник: [inventory:111](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q2б: email/полный mail server исключены; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Не реализовано; runtime не проверено. |
