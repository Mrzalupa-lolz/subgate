# Реестр: panel

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 1. Product principles

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-015 | **Tolerant reader, strict writer** toward the panel API.<br>Источник: [v4:28](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-panel](EVIDENCE.md#e-panel); TestRemnawaveV3Contract; TestPanelRejectsInvalidContractAndStatus; TestPanelSyncCommitsCompleteSnapshotAndPreservesLastGood | Ручной клиент вместо ogen; нет проверки версии при старте, scopes matrix, полного panel-native каталога; тестовый HTTP fixture не доказывает совместимость живой панели. |

#### 2. Operator environment (kept deliberately generic)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-016 | Remnawave 3.x panel (verify exact version) managing Xray nodes on Linux servers; users mostly in a censoring region (RU) including mobile/"LTE whitelist" conditions; the operator may also run `Case211/remnawave-admin` (optional integration, section 4.2). **Do not hard-code anything about the current server list or protocols**: SubGate must handle *any* protocol/transport Xray supports (section 6), including ones not deployed today. Fill during P0:<br>Источник: [v4:32](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-panel](EVIDENCE.md#e-panel); TestRemnawaveV3Contract; TestPanelRejectsInvalidContractAndStatus; TestPanelSyncCommitsCompleteSnapshotAndPreservesLastGood | Ручной клиент вместо ogen; нет проверки версии при старте, scopes matrix, полного panel-native каталога; тестовый HTTP fixture не доказывает совместимость живой панели. |

#### 3. Platform, language and stack

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-023 | Remnawave API client: **generated with `ogen`** from the OpenAPI spec of the exact panel version; zero-reflection decoding; do not depend on the stale community Go SDK (reference only). Regenerate on panel upgrade (CI drift job); cross-check nullability/enums with `@remnawave/backend-contract`<br>Источник: [v4:51](../../subgate-ai-prompt-v4.md) | **PARTIAL** | `core/internal/panel/generated`, `scripts/generate-panel.py --check` PASS; TestGeneratedPanelUsersContract/Integrity PASS, live synthetic read PASS; make check PASS, security re-review без findings (Linux2026-10-14) | Один GET users generated DTO decoder + scopes matrix `docs/panel-contract.md`; остальные endpoints ручные, exact-byte normalization использует encoding/json, full zero-reflection и backend-contract cross-check ещё не выполнены. |

#### 4. Integration contracts / 4.1 Remnawave panel (mandatory)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-042 | **Auth:** panel **API token**, minimum scopes (document the scope list per feature). Never store admin passwords. Some endpoints are unreachable with API tokens — design only against reachable ones and degrade gracefully with a deep link to the panel.<br>Источник: [v4:74](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-panel](EVIDENCE.md#e-panel); TestRemnawaveV3Contract; TestPanelRejectsInvalidContractAndStatus; TestPanelSyncCommitsCompleteSnapshotAndPreservesLastGood  R03: scopes/action matrix `docs/panel-contract.md` по spec и test /api/tokens/scopes. | Полный каталог/scoped graceful degradation/deep links и confirmed actions ещё не проверены; один generated read endpoint. |
| F-043 | **Spec first:** obtain the OpenAPI spec for the exact panel version (panel's docs endpoint or the matching `remnawave/backend` release tag). If unobtainable, **stop and ask**. Never invent endpoints/fields.<br>Источник: [v4:75](../../subgate-ai-prompt-v4.md) | **DONE** | `remnawave-openapi.json` info.version3.4.5; `core/internal/panel/spec_test.go:TestPanelSpecReadContract` PASS; synthetic users/nodes/hosts fixtures; `make check` PASS (Linux 2026-10-14) | Контракт чтения сверён со схемой, nullable activeInbounds и unknown enums проверены. Полное generated покрытие и startup version check относятся к другим ID.
| F-046 | **Panel-native concepts:** internal/external squads, **tags on hosts and nodes** (drive tiers/policies from tags such as `tier:1`, `lte`, `whitelist`, `udp`), config profiles, HWID limits, consumption multipliers, bandwidth-stats endpoints, subscription request history.<br>Источник: [v4:78](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-panel](EVIDENCE.md#e-panel); TestRemnawaveV3Contract; TestPanelRejectsInvalidContractAndStatus; TestPanelSyncCommitsCompleteSnapshotAndPreservesLastGood | Ручной клиент вместо ogen; нет проверки версии при старте, scopes matrix, полного panel-native каталога; тестовый HTTP fixture не доказывает совместимость живой панели. |
| F-047 | **Actions through the API** (RBAC + audit + confirmation): enable/disable user, reset traffic, revoke subscription, node restart/enable/disable, host enable/disable/bulk. Heavy config editing stays in the panel (deep link).<br>Источник: [v4:79](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-panel](EVIDENCE.md#e-panel); TestRemnawaveV3Contract; TestPanelRejectsInvalidContractAndStatus; TestPanelSyncCommitsCompleteSnapshotAndPreservesLastGood | Ручной клиент вместо ogen; нет проверки версии при старте, scopes matrix, полного panel-native каталога; тестовый HTTP fixture не доказывает совместимость живой панели. |
| F-048 | Compatibility matrix in docs; startup check compares panel version with the generated client version.<br>Источник: [v4:80](../../subgate-ai-prompt-v4.md) | **DONE** | `docs/panel-contract.md` compatibility matrix; `core/internal/server/panel_version.go` + `panel/version.go`; TestPanelVersionMismatch RED→GREEN (warning/status/sync continuation/subscription bytes); make check/drift/live synthetic3.4.5 PASS; read-only security review без findings (Linux2026-10-14) | Критерий строки проверен; совместимость остальных версий не заявляется. Полное generated покрытие относится к F-023. |

#### 8. Functional requirements / F17 — Operations console

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-120 | Quick actions via the panel API (user enable/disable/reset traffic/revoke, node restart/enable/disable, host enable/disable/bulk) with RBAC, confirmations for destructive actions, full audit; global search over users/nodes/hosts; deep links into the panel.<br>Источник: [v4:218](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-panel](EVIDENCE.md#e-panel); TestRemnawaveV3Contract; TestPanelRejectsInvalidContractAndStatus; TestPanelSyncCommitsCompleteSnapshotAndPreservesLastGood | Ручной клиент вместо ogen; нет проверки версии при старте, scopes matrix, полного panel-native каталога; тестовый HTTP fixture не доказывает совместимость живой панели. |

#### Part A. Per-project functions / 2. Mrvibecodic/remnawave-subscription-middleware (PHP; beta; closest relative of SubGate)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-237 | Minimal API-token scope tables (and a paste-JSON scope list); panel 2.x/3.x id handling; shortUuid-length scanner filter.<br>Источник: [inventory:40](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Ручной клиент вместо ogen; нет проверки версии при старте, scopes matrix, полного panel-native каталога; тестовый HTTP fixture не доказывает совместимость живой панели. |

#### Part A. Per-project functions / 11. remnawave/panel (NestJS; AGPL-3.0)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-250 | Users, nodes, hosts, internal/external squads, config profiles with tags, snippets, subscription templates/settings/response rules/page configs, request history, HWID limits, API tokens with scopes, passkeys/OAuth2, infra billing, bandwidth stats, **node plugins (torrent blocker) and shared ASN/IP lists (3.3+)**, node integrations (3.3+).<br>Источник: [inventory:71](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Ручной клиент вместо ogen; нет проверки версии при старте, scopes matrix, полного panel-native каталога; тестовый HTTP fixture не доказывает совместимость живой панели. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-286 | Panel-native features to consume, not duplicate: HWID limits, torrent-blocker reports, request history, bandwidth stats, shared ASN/IP lists, billing nodes — INT **NEW**<br>Источник: [inventory:116](../../subgate-feature-inventory.md) | **PARTIAL** | [E-panel](EVIDENCE.md#e-panel); TestRemnawaveV3Contract; TestPanelRejectsInvalidContractAndStatus; TestPanelSyncCommitsCompleteSnapshotAndPreservesLastGood | Ручной клиент вместо ogen; нет проверки версии при старте, scopes matrix, полного panel-native каталога; тестовый HTTP fixture не доказывает совместимость живой панели. |

#### Part B. The merged catalogue (union) → SubGate / Overlap decisions

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-287 | Where the panel already provides a capability (HWID limit, torrent blocker, request history), SubGate **reads and visualises** it, and **pushes** its own curated data (e.g. ASN lists) into the panel's shared lists where an API exists.<br>Источник: [inventory:119](../../subgate-feature-inventory.md) | **PARTIAL** | [E-panel](EVIDENCE.md#e-panel); TestRemnawaveV3Contract; TestPanelRejectsInvalidContractAndStatus; TestPanelSyncCommitsCompleteSnapshotAndPreservesLastGood | Решение источника требований; clean-room detectors/editor конфликтует с v4 §17; optional connector запрещён текущим поручением. Ручной клиент вместо ogen; нет проверки версии при старте, scopes matrix, полного panel-native каталога; тестовый HTTP fixture не доказывает совместимость живой панели. |
