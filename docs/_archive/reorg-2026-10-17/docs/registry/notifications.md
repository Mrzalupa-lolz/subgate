# Реестр: notifications

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 3. Platform, language and stack

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-037 | Telegram: `go-telegram/bot` or `telebot`; <br>Источник: [v4:65](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-notify](EVIDENCE.md#e-notify); целевого теста нет; не проверено | Есть HTTP sender + dedupe/cooldown/quiet hours/test endpoint; доставка не проверено. Нет polling/commands/buttons/topics, RU/EN, proxy/email/webhook fanout/recovery/automatic event producers. AllowedUserIDs используются как ChatIDs, не command authorization. |

#### 8. Functional requirements / F14 — Notifications and Telegram bot

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-106 | Built-in bot with per-category topics (health, drops, youtube, 16-20, anti-scan, bandwidth, logs, system), webhooks, optional email. Severity, dedupe, cooldown, quiet hours, recovery messages, test button, RU/EN templates, proxy support. Commands (allow-listed Telegram IDs only): `/status`, `/node`, `/user`, `/top`, `/ip &lt;addr&gt;` (IP-intel lookup), quarantine/ban buttons.<br>Источник: [v4:198](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-notify](EVIDENCE.md#e-notify); целевого теста нет; не проверено | Есть HTTP sender + dedupe/cooldown/quiet hours/test endpoint; доставка не проверено. Нет polling/commands/buttons/topics, RU/EN, proxy/email/webhook fanout/recovery/automatic event producers. AllowedUserIDs используются как ChatIDs, не command authorization. |

#### Part A. Per-project functions / 1. Case211/remnawave-admin (web + Telegram bot + agent; Python/FastAPI, React, PostgreSQL; AGPL-3.0 since 2.16)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-214 | **Bot:** users/nodes/hosts management, bulk ops, templates/snippets/tokens, billing nodes, health/stats/bandwidth commands, roles, topic-routed notifications.<br>Источник: [inventory:15](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Есть HTTP sender + dedupe/cooldown/quiet hours/test endpoint; доставка не проверено. Нет polling/commands/buttons/topics, RU/EN, proxy/email/webhook fanout/recovery/automatic event producers. AllowedUserIDs используются как ChatIDs, не command authorization. |
