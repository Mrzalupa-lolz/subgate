# Реестр: scope

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 4. Integration contracts / 4.2 `remnawave-admin` (optional connector)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-049 | Known from its public release notes (current line v2.18.x): Python/FastAPI + React + PostgreSQL (+ optional Redis), node-agent/collector, Prometheus `/metrics` with Bearer token, REST API v2 and v3 `violations:read`, outbound webhooks, violation/HWID/torrent analyzers, Xray editor, backups, plugin API; **AGPL-3.0 since 2.16.0** (≤ 2.15.x MIT).<br>Источник: [v4:83](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Коннектор remnawave-admin запрещён текущим поручением; строка сохранена как исключённое исходное требование, не roadmap к реализации. Не реализовано; runtime не проверено. |
| F-050 | **Rules:** communicate **only over documented HTTP interfaces**; never read its PostgreSQL schema; never copy its code; treat its data as untrusted.<br>Источник: [v4:84](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Коннектор remnawave-admin запрещён текущим поручением; строка сохранена как исключённое исходное требование, не roadmap к реализации. Не реализовано; runtime не проверено. |
| F-051 | **Connector features (toggleable):** show violation/HWID data in SubGate user detail; forward SubGate alerts to its webhook; optionally expose per-user live speed to it; deep links both ways; shared-secret + IP allowlist.<br>Источник: [v4:85](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Коннектор remnawave-admin запрещён текущим поручением; строка сохранена как исключённое исходное требование, не roadmap к реализации. Не реализовано; runtime не проверено. |
| F-052 | Coexistence: all SubGate ports configurable (its collector/agents use 8081 by default), no Docker-socket sharing.<br>Источник: [v4:86](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Коннектор remnawave-admin запрещён текущим поручением; строка сохранена как исключённое исходное требование, не roadmap к реализации. Не реализовано; runtime не проверено. |
| F-053 | Not in v1: re-implementing its violation detectors or Xray editor.<br>Источник: [v4:87](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Коннектор remnawave-admin запрещён текущим поручением; строка сохранена как исключённое исходное требование, не roadmap к реализации. Не реализовано; runtime не проверено. |

#### 17. Non-goals

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-199 | Billing, sales bot, replacing the Remnawave panel UI, re-implementing `remnawave-admin` detectors or Xray editor, Windows/macOS support, inspecting user traffic content (payloads, URLs, page bodies — only destination host:port metadata is journaled), multi-tenant SaaS in v1.<br>Источник: [v4:345](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Границы v4, а не обещание реализации; F21/F27 inventory противоречат запрету detectors/editor. Текущее поручение исключает remnawave-admin integration. Не реализовано; runtime не проверено. |

#### Part A. Per-project functions / 1. Case211/remnawave-admin (web + Telegram bot + agent; Python/FastAPI, React, PostgreSQL; AGPL-3.0 since 2.16)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-224 | **Bedolaga integration:** CRM dashboard, clients, subscriptions, referrals, promo codes, marketing analytics, client cabinet.<br>Источник: [inventory:25](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-282 | Billing/CRM (Bedolaga), referrals, promo, payments — integration only (non-goal to rebuild)<br>Источник: [inventory:112](../../subgate-feature-inventory.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Integration-only/non-goal в inventory; текущая граница разрешает только Remnawave и ноды. Не выполнять без ответа владельца. Не реализовано; runtime не проверено. |

#### Part B. The merged catalogue (union) → SubGate / Overlap decisions

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-288 | Where `remnawave-admin` already provides a capability, SubGate re-implements it **from public descriptions only (clean-room; AGPL-3.0)**, so the operator can run SubGate alone. The optional connector remains for coexistence.<br>Источник: [inventory:120](../../subgate-feature-inventory.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Решение источника требований; clean-room detectors/editor конфликтует с v4 §17; optional connector запрещён текущим поручением. Не реализовано; runtime не проверено. |
| F-289 | Two modules are deliberately risky and flagged optional: the **full mail server** and the **eBPF shaper**.<br>Источник: [inventory:121](../../subgate-feature-inventory.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Решение источника требований; clean-room detectors/editor конфликтует с v4 §17; optional connector запрещён текущим поручением. Не реализовано; runtime не проверено. |
