# Реестр: anti-scan

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 0. Role and mission

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-007 | **protection** (anti-scan, geo-block, drop detection) and an **operations console** with notifications.<br>Источник: [v4:16](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-antiscan](EVIDENCE.md#e-antiscan); целевого теста нет; не проверено | UA deny подключён, не проверено. AnalyzeRequest не вызывает ApplyTarpit, поэтому escalation не включает delay. Нет node handshake pipeline, subnet/ASN correlation, config/UI allowlist, firewall export/Telegram topic; hardcoded UA deny включает curl/wget. |

#### 8. Functional requirements / F12 — Anti-scan

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-099 | Sources: gateway request log (masked identifiers) **and node log signals (F19)**. Detectors: invalid-`shortUuid` enumeration, rate anomalies, scanner/headless UAs, odd paths, cross-IP patterns, **invalid/failed handshake bursts on inbounds** (collect real log samples in P0; do not hard-code strings from memory), grouped by IP/subnet/**ASN/network type**.<br>Источник: [v4:187](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-antiscan](EVIDENCE.md#e-antiscan); целевого теста нет; не проверено | UA deny подключён, не проверено. AnalyzeRequest не вызывает ApplyTarpit, поэтому escalation не включает delay. Нет node handshake pipeline, subnet/ASN correlation, config/UI allowlist, firewall export/Telegram topic; hardcoded UA deny включает curl/wget. |
| F-100 | Escalating actions: tarpit/decoy → in-app deny → export to reverse proxy / node firewall via agent → alert in a **separate Telegram topic** with correlation (earlier requests from same IP, /24, ASN+UA fingerprint, first/last seen, top paths). Allowlists; optional public-blocklist import.<br>Источник: [v4:188](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-antiscan](EVIDENCE.md#e-antiscan); целевого теста нет; не проверено | UA deny подключён, не проверено. AnalyzeRequest не вызывает ApplyTarpit, поэтому escalation не включает delay. Нет node handshake pipeline, subnet/ASN correlation, config/UI allowlist, firewall export/Telegram topic; hardcoded UA deny включает curl/wget. |
