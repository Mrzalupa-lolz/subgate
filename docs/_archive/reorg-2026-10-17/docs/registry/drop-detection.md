# Реестр: drop-detection

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 8. Functional requirements / F10 — Traffic-drop detection

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-097 | Metrics: online users and traffic per node/global, subscription requests/min. Seasonal baseline (hour-of-week median + MAD, EWMA fallback). Alert below baseline × (1 − threshold) for T minutes; maintenance suppression; context (quarantines, blocks); dedupe, cooldown, recovery message; visible "learning" state.<br>Источник: [v4:181](../../subgate-ai-prompt-v4.md) | **STUB** | [E-drop](EVIDENCE.md#e-drop); целевого теста нет; не проверено | Нет detector runtime: median/MAD/EWMA, learning, T-minute evaluation, maintenance/context/recovery; таблица baselines не реализация. |
