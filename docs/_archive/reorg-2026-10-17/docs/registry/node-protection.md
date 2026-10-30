# Реестр: node-protection

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### Part A. Per-project functions / 5. dotX12/traffic-guard (Go; MIT) + TrafficGuard-auto

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-240 | iptables/ipset blocking of scanner and government-network lists (multiple URLs, `file://`), persistence, UFW integration, optional aggregated CSV log with whois ASN/NETNAME, per-IP log rate limit, clean `uninstall`. TrafficGuard-auto: one-command installer with a log/statistics viewer (lists: government_networks, antiscanner, skipa).<br>Источник: [inventory:49](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part A. Per-project functions / 6. jestivald/node-accelerator (Bash; MIT) — the best node-side spec

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-242 | **Protect:** own nftables table (never flushes ruleset), strict/open/skip modes, AntiScan with ban-once, flag-drop, anti-spoof, per-IP SYN/UDP/connlimit, UDP bulk ports for Hysteria2/TUIC, node-port auto-detect + panel auto-whitelist, conntrack phantom-eviction, blocklists (Spamhaus DROP, FireHOL L1, Tor), **fleet auto-sync of whitelists**, CrowdSec, SSH anti-lockout, safety timer, dry-run, rollback.<br>Источник: [inventory:53](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-270 | Node protection (nftables, ban-once, per-IP limits, UDP bulk, ctguard, blocklists, fleet whitelist sync, safety timer) — F23 **NEW**<br>Источник: [inventory:100](../../subgate-feature-inventory.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Не реализовано; runtime не проверено. |
| F-275 | Per-client-IP eBPF shaper with dynamic penalty — F25 **NEW** (optional)<br>Источник: [inventory:105](../../subgate-feature-inventory.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Не реализовано; runtime не проверено. |
