# Реестр: geo-protection

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 8. Functional requirements / F7 — Geo-block

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-093 | Country/ASN/**network-type**/CIDR rules, allow/deny, per-policy scope, per-user exemptions, curated overrides. **Unknown-IP behaviour configurable** (allow/deny/grace); thanks to L1+L2 (section 7) newly allocated ranges are still classified. Block response configurable (403, 404-mimic, empty subscription, decoy). IPv6 supported. Blocked-request logging rate-limited.<br>Источник: [v4:171](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-geo](EVIDENCE.md#e-geo); TestTrustedProxyChainCannotBeSpoofed (только IP trust, не geo enforcement) | CIDR/country/ASN/network_type ветви есть, geo enforcement не проверено отдельно. Нет user exemptions/policy scope/unknown grace/decoy/response switch и source L1+L2; порядок по object ID вместо priority; utility Engine не подключён. |

#### 12. Security and privacy

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-181 | **Trusted proxies only** for client-IP headers; SSRF protection on every operator-supplied URL (deny private ranges unless allowed) including data-source downloaders and webhooks.<br>Источник: [v4:312](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-geo](EVIDENCE.md#e-geo); TestTrustedProxyChainCannotBeSpoofed (только IP trust, не geo enforcement) | CIDR/country/ASN/network_type ветви есть, geo enforcement не проверено отдельно. Нет user exemptions/policy scope/unknown grace/decoy/response switch и source L1+L2; порядок по object ID вместо priority; utility Engine не подключён. |
