# Реестр: xray-toolkit

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### Part A. Per-project functions / 1. Case211/remnawave-admin (web + Telegram bot + agent; Python/FastAPI, React, PostgreSQL; AGPL-3.0 since 2.16)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-220 | **Xray editor:** visual inbound/outbound/routing/DNS/reverse editor, balancers + observatory UI, WARP generator, protocol presets, link import/generator, geo viewer, topology graph, diagnostics.<br>Источник: [inventory:21](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part A. Per-project functions / 4. legiz-ru/my-remnawave (configs)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-239 | Per-platform client catalogues (multiapp / hwid), custom subscription-page templates, subscription templates for mihomo/stash/sing-box/xray-json, Happ/INCY routing presets, XHTTP-behind-nginx recipe (unix socket, `xmux`, `downloadSettings`), Prizrak-Box response rules with `pxa/pxd-template` headers and placeholders.<br>Источник: [inventory:46](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-277 | Xray config toolkit: linter, generators (XHTTP via nginx, Selfsteal/Reality dest, WARP), balancer/observatory editor, snippets, import/export of links — F27 **NEW**<br>Источник: [inventory:107](../../subgate-feature-inventory.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Не реализовано; runtime не проверено. |
