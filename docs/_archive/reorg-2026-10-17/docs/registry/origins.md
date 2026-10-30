# Реестр: origins

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 8. Functional requirements / F8 — "Who is where": maps and per-user origins

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-094 | Two views: (a) **subscription-request origins** and (b) **connection origins from node access logs (F19)** — the latter is the accurate one. Aggregations by country/city/ASN/operator/network type over time, CGNAT hints, per-user history (last N distinct ASN/country/city with timestamps), filters by operator.<br>Источник: [v4:174](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-origins](EVIDENCE.md#e-origins); TestTelemetryIdempotencyEncryptionAndReadAudit; TestActivityPanelLinksResolveXrayIdentitiesAndPurge (только events/aliases) | Есть src_ip в activity. Нет sub-request origin pipeline, map/bundled geo, origin enrichment/aggregations/history/CGNAT, per-tier origin retention/no-IP/hash switches. |
| F-095 | Bundled map data only (no external tile servers; never leak user IPs). Retention is configurable per tier (defaults: raw 30 d, aggregates 365 d); IP truncation (/24, /48), salted hashing and a global **no-IP mode** are optional switches (off by default).<br>Источник: [v4:175](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-origins](EVIDENCE.md#e-origins); целевого теста нет; не проверено | Требуемого полного поведения нет. Есть src_ip в activity. Нет sub-request origin pipeline, map/bundled geo, origin enrichment/aggregations/history/CGNAT, per-tier origin retention/no-IP/hash switches. |

#### 8. Functional requirements / F19 — Xray log pipeline (all levels, including `debug`)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-129 | **Connection origins:** client IPs seen in node access logs are enriched with IP intelligence (section 7) to power the accurate "who is where" views (F8): per user, per node, per ASN/operator/country/city, CGNAT hints.<br>Источник: [v4:234](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-origins](EVIDENCE.md#e-origins); целевого теста нет; не проверено | Требуемого полного поведения нет. Есть src_ip в activity. Нет sub-request origin pipeline, map/bundled geo, origin enrichment/aggregations/history/CGNAT, per-tier origin retention/no-IP/hash switches. |
