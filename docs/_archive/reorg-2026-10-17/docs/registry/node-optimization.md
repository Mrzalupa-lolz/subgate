# Реестр: node-optimization

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### Part A. Per-project functions / 6. jestivald/node-accelerator (Bash; MIT) — the best node-side spec

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-241 | **Optimizer:** XanMod BBRv3, tier-aware sysctl, RPS/RFS/XPS, zram, MSS clamp, FD limits, NIC tuning, hourly log-rotation timer.<br>Источник: [inventory:52](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-272 | Node optimizer/advisor with dry-run + rollback (BBR/cake, RPS/RFS/XPS, NIC, limits, MSS, zram, journald, XanMod) — F24 **NEW**<br>Источник: [inventory:102](../../subgate-feature-inventory.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Не реализовано; runtime не проверено. |
