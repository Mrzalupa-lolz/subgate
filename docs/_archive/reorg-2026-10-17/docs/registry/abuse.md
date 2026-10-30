# Реестр: abuse

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### Part A. Per-project functions / 1. Case211/remnawave-admin (web + Telegram bot + agent; Python/FastAPI, React, PostgreSQL; AGPL-3.0 since 2.16)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-217 | **Anti-abuse:** 7 analyzers (temporal, geographic with impossible travel, ASN, behaviour profile, devices, HWID, User-Agent), scoring → actions (warn / soft-block / temp-block / hard-block / speed cut), dedupe window + escalation, delayed measures with re-check, CGNAT buffer, 60+ Russian agglomerations, speed-consumption monitoring, P2P/torrent detection, customer warnings with templates, node-side eBPF speed shaper.<br>Источник: [inventory:18](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-268 | Abuse engine: 7 analyzers, scoring, escalation ladder, delayed measures, CGNAT buffer, P2P/torrent — F21 **NEW**<br>Источник: [inventory:98](../../subgate-feature-inventory.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Не реализовано; runtime не проверено. |
