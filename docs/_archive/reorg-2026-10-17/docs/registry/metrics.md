# Реестр: metrics

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 3. Platform, language and stack

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-039 | Logging/metrics: `log/slog` JSON; optional `/metrics` via `prometheus/client_golang`; <br>Источник: [v4:67](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-metrics](EVIDENCE.md#e-metrics); целевого теста нет; не проверено | Public text exporter есть, не проверено запросом. Нет 30+ fleet gauges, Grafana dashboards/sync watchdog; сторонние observability tools не обязательная зависимость. |

#### Part A. Per-project functions / 1. Case211/remnawave-admin (web + Telegram bot + agent; Python/FastAPI, React, PostgreSQL; AGPL-3.0 since 2.16)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-219 | **Monitoring:** Prometheus `/metrics` (30+ gauges), 5 Grafana dashboards, sync watchdog, fleet monitoring including non-Remnawave servers.<br>Источник: [inventory:20](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Public text exporter есть, не проверено запросом. Нет 30+ fleet gauges, Grafana dashboards/sync watchdog; сторонние observability tools не обязательная зависимость. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-279 | Prometheus `/metrics` + Grafana dashboards + sync watchdog (optional exports) — F35 **NEW**<br>Источник: [inventory:109](../../subgate-feature-inventory.md) | **PARTIAL** | [E-metrics](EVIDENCE.md#e-metrics); целевого теста нет; не проверено | Public text exporter есть, не проверено запросом. Нет 30+ fleet gauges, Grafana dashboards/sync watchdog; сторонние observability tools не обязательная зависимость. |
