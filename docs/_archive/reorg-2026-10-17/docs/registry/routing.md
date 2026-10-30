# Реестр: routing

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 1. Product principles

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-013 | **Unknown = untouched / never crash on unknown input.** New protocols, new log formats, new panel fields must degrade gracefully, never panic.<br>Источник: [v4:26](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-routing](EVIDENCE.md#e-routing); TestPerRequestRoutingKeepsUnmappedProtocolsAndHealthyFallback; TestLeastPingRejectsStaleSamplesAndUsesFallback; TestJSONAndYAMLRepairSelectors; TestPolicyInvalidConditionsAndTierReferenceCannotBeDeleted | Явные node_ids; условия только country/ASN/UA. Нет squad/tag/plan/operator/network-type layouts, burstObservatory, полноценного balancer strategy rewrite и automatic quarantine. |

#### 3. Platform, language and stack

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-025 | JSON: `tidwall/gjson`+`sjson` for surgical edits of Xray JSON; `bytedance/sonic` or `goccy/go-json` for full decodes; benchmark and pick<br>Источник: [v4:53](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-routing](EVIDENCE.md#e-routing); TestPerRequestRoutingKeepsUnmappedProtocolsAndHealthyFallback; TestLeastPingRejectsStaleSamplesAndUsesFallback; TestJSONAndYAMLRepairSelectors; TestPolicyInvalidConditionsAndTierReferenceCannotBeDeleted | Явные node_ids; условия только country/ASN/UA. Нет squad/tag/plan/operator/network-type layouts, burstObservatory, полноценного balancer strategy rewrite и automatic quarantine. |

#### 6. Universal protocol and transport support (hard requirement)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-058 | **Design rule: opaque-by-default.** The transformer treats outbounds/entries as opaque objects identified by `tag`, `protocol`, address/port and remark; it never whitelists protocols for passthrough. Protocol-specific knowledge lives in a small **protocol descriptor registry** (probe type, per-user-stats capability, port semantics) with a **generic default** for unknown protocols (generic TCP/UDP reachability or "unprobed" — never an error).<br>Источник: [v4:110](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-routing](EVIDENCE.md#e-routing); TestPerRequestRoutingKeepsUnmappedProtocolsAndHealthyFallback; TestLeastPingRejectsStaleSamplesAndUsesFallback; TestJSONAndYAMLRepairSelectors; TestPolicyInvalidConditionsAndTierReferenceCannotBeDeleted | Явные node_ids; условия только country/ASN/UA. Нет squad/tag/plan/operator/network-type layouts, burstObservatory, полноценного balancer strategy rewrite и automatic quarantine. |
| F-062 | Acceptance: adding a never-seen protocol to a config profile requires **no SubGate code change** for passthrough, balancing and health (generic), and produces a visible "unknown protocol (generic handling)" badge.<br>Источник: [v4:114](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-routing](EVIDENCE.md#e-routing); TestPerRequestRoutingKeepsUnmappedProtocolsAndHealthyFallback; TestLeastPingRejectsStaleSamplesAndUsesFallback; TestJSONAndYAMLRepairSelectors; TestPolicyInvalidConditionsAndTierReferenceCannotBeDeleted | Явные node_ids; условия только country/ASN/UA. Нет squad/tag/plan/operator/network-type layouts, burstObservatory, полноценного balancer strategy rewrite и automatic quarantine. |

#### 8. Functional requirements / F3 — Tiers, policies, transport-aware selection

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-085 | Tiers 1..N per policy; strategies map to Xray balancers (`random`, `roundRobin`, `leastPing`, `leastLoad`) with editable `burstObservatory`; tier k+1 engages when tier k has `&lt; min_healthy` healthy nodes (or via `fallbackTag`). Rewrite `routing.balancers[].selector`, outbounds, observatory.<br>Источник: [v4:155](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-routing](EVIDENCE.md#e-routing); TestPerRequestRoutingKeepsUnmappedProtocolsAndHealthyFallback; TestLeastPingRejectsStaleSamplesAndUsesFallback; TestJSONAndYAMLRepairSelectors; TestPolicyInvalidConditionsAndTierReferenceCannotBeDeleted | Явные node_ids; условия только country/ASN/UA. Нет squad/tag/plan/operator/network-type layouts, burstObservatory, полноценного balancer strategy rewrite и automatic quarantine. |
| F-086 | Tier membership from **panel tags**, squads, or explicit mapping. Policies select layouts by squad/tag/country/**network type &amp; operator (IP-intel)**/User-Agent/app/plan; first match wins; default mandatory.<br>Источник: [v4:156](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-routing](EVIDENCE.md#e-routing); TestPerRequestRoutingKeepsUnmappedProtocolsAndHealthyFallback; TestLeastPingRejectsStaleSamplesAndUsesFallback; TestJSONAndYAMLRepairSelectors; TestPolicyInvalidConditionsAndTierReferenceCannotBeDeleted | Явные node_ids; условия только country/ASN/UA. Нет squad/tag/plan/operator/network-type layouts, burstObservatory, полноценного balancer strategy rewrite и automatic quarantine. |
| F-087 | Layouts may order/exclude entries by transport (any protocol per section 6).<br>Источник: [v4:157](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-routing](EVIDENCE.md#e-routing); целевого теста нет; не проверено | Требуемого полного поведения нет. Явные node_ids; условия только country/ASN/UA. Нет squad/tag/plan/operator/network-type layouts, burstObservatory, полноценного balancer strategy rewrite и automatic quarantine. |
| F-088 | Acceptance: a mobile-operator client and a home-ISP client of the same user receive different layouts; losing all tier-1 nodes yields tier-2, never an empty subscription.<br>Источник: [v4:158](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-routing](EVIDENCE.md#e-routing); TestPerRequestRoutingKeepsUnmappedProtocolsAndHealthyFallback; TestLeastPingRejectsStaleSamplesAndUsesFallback; TestJSONAndYAMLRepairSelectors; TestPolicyInvalidConditionsAndTierReferenceCannotBeDeleted | Явные node_ids; условия только country/ASN/UA. Нет squad/tag/plan/operator/network-type layouts, burstObservatory, полноценного balancer strategy rewrite и automatic quarantine. |

#### 8. Functional requirements / F4 — Pinning

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-089 | Persist `user → node` (or ordered list); initial choice by **rendezvous hashing** over healthy nodes of the top eligible tier, capacity-weighted; re-pin only on quarantine/removal/quota/operator action; configurable sticky-return hysteresis.<br>Источник: [v4:161](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-pin](EVIDENCE.md#e-pin); TestPerRequestRoutingKeepsUnmappedProtocolsAndHealthyFallback | Только ручной user-&gt;node pin с проверкой online. Нет weighted rendezvous, initial assignment, sticky return, списков/modes, pinned-user cap, rebalance/dry-run/change budget/exit metric. |
| F-090 | Output modes `single`, `ordered-fallback`, `balancer-preferred`; per-node max pinned users; rebalance with dry-run and change budget; metric "exit changes per user per day".<br>Источник: [v4:162](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-pin](EVIDENCE.md#e-pin); целевого теста нет; не проверено | Требуемого полного поведения нет. Только ручной user-&gt;node pin с проверкой online. Нет weighted rendezvous, initial assignment, sticky return, списков/modes, pinned-user cap, rebalance/dry-run/change budget/exit metric. |
