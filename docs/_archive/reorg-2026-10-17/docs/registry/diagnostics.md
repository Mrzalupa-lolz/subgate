# Реестр: diagnostics

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 8. Functional requirements / F9 — Block check "16–20"

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-096 | Detect DPI that stalls/kills connections after ~16–20 KiB, per inbound, from inside the censorship region. **Default raw-TLS mode:** connect with the inbound's SNI and read ≥ 64 KiB, record bytes at first stall/reset. **Advanced mode:** real proxied transfer via a spawned xray-core client with a probe user. Verdict `ok&#124;suspect&#124;blocked` (default signature: stall/reset within 14–24 KiB). Matrix per (vantage × node × inbound); always alerts; quarantine/demote only if enabled. Run via `subgate probe` in-region or an operator-supplied SOCKS5/HTTP proxy.<br>Источник: [v4:178](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-block](EVIDENCE.md#e-block); TestBlockCheckRestrictsTargetToSyncedNodeAndHonorsTLS | Проверяется target restriction/TLS, не DPI detection. Core читает сокет без запроса, EOF до 64KiB может дать ok. Нет regional probe binary/proxy/vantage matrix, xray client transfer, alert/quarantine actions; 16–20 signature не проверена. |

#### 8. Functional requirements / F11 — YouTube region scan

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-098 | Every N hours per node the agent determines the region YouTube sees for the node egress (locale/consent handled; no API keys). If it becomes a forbidden region (default `RU`): dedicated alert, stop re-checking that node until reset/cooldown, optional policy action.<br>Источник: [v4:184](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-youtube](EVIDENCE.md#e-youtube); TestYouTubeAPIRejectsUntrustedEndpoints; TestYouTubeAPIDoesNotFollowRedirectToInternalService (только URL restrictions) | Исполняется с Core; node_id только метка. Нет per-node egress scheduling, forbidden-region transitions, stop/cooldown, alerts/policy actions; parsing/region в живом YouTube не проверено. |

#### Part A. Per-project functions / 6. jestivald/node-accelerator (Bash; MIT) — the best node-side spec

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-243 | **Diagnose:** kernel/BBR/conntrack/NIC/PSI/UDP errors, remnanode container state, node-port vs firewall desync, TLS cert expiry, whitelist consistency, retransmit-cause analysis, disk/log sensors, JSON output.<br>Источник: [inventory:54](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |
| F-244 | **Forensics (`na-report`):** drops by reason, timeline, top IPs with verdict, top ASN, per-IP deep verdict.<br>Источник: [inventory:55](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part A. Per-project functions / 7. Case211/node-diagnostic (Bash + eBPF; MIT)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-246 | 27 diagnostic checks (system, network, throughput, **service reachability for 19 services**, **IP reputation**, Xray/remnanode, open-ports audit), `optimize` with namespaced drop-ins and rollback, generate-only `protect` with auto-revert timer, `bbr3`, **eBPF per-client-IP shaper** (EDT) with whitelist and dynamic penalty, installers (node, Selfsteal, NetBird, monitoring).<br>Источник: [inventory:59](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part A. Per-project functions / 9. saveksme/multitest (+ censorcheck)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-248 | Launcher for ipregion, censorcheck (DNS/IP/HTTP(S)/DPI/regional layers; ~90 % accuracy; regional caveat; needs TLS listener on 443), iperf3 to Russian servers, YABS, IP.Check.Place, IPQuality, bench.sh, sysbench; BBR+Cake and IPv6-off utilities.<br>Источник: [inventory:65](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-273 | Diagnostic catalogue: 27 checks, 19-service reachability, IP reputation, open-port audit, cert expiry, retransmit analysis — F24 **NEW**<br>Источник: [inventory:103](../../subgate-feature-inventory.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Не реализовано; runtime не проверено. |
| F-274 | Blocking radar: DNS/IP/HTTP(S)/DPI/regional, iperf3 to RU servers, benchmarks — F9 extended + F24 **NEW**<br>Источник: [inventory:104](../../subgate-feature-inventory.md) | **PARTIAL** | [E-block](EVIDENCE.md#e-block); TestBlockCheckRestrictsTargetToSyncedNodeAndHonorsTLS | Проверяется target restriction/TLS, не DPI detection. Core читает сокет без запроса, EOF до 64KiB может дать ok. Нет regional probe binary/proxy/vantage matrix, xray client transfer, alert/quarantine actions; 16–20 signature не проверена. |
