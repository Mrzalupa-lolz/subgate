# Реестр: fleet-operations

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### Part A. Per-project functions / 8. DonMatteoVPN/Reshala-Remnawave-Bedolaga (Bash TUI; ideas)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-247 | Skynet fleet manager (mass commands, auto-capture), per-user traffic shaper, dashboard with Speedtest and "how many users can this node carry", UFW/Fail2Ban/sysctl security wizard, Docker deep clean, Remnawave lifecycle. Roadmap: Telegram control, alerting, web panel, auto-response.<br>Источник: [inventory:62](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-276 | Fleet ops: remote terminal, script catalogue, mass commands, auto-capture, Docker hygiene, node/Xray-core lifecycle, log rotation, auto-restart, WARP/Tor outbound — F26 **NEW** (optional)<br>Источник: [inventory:106](../../subgate-feature-inventory.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Не реализовано; runtime не проверено. |
