# Реестр: injections

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 8. Functional requirements / F5 — Custom injections

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-091 | Rules scoped to all/group/user/policy/format: append/prepend raw entries (Xray outbound JSON, URI lines, Mihomo block, sing-box outbound), JSON Merge Patch / RFC 6902 on `xray-json`, remove by tag. Sandboxed templating (**no code execution**). Validation before activation, **dry-run per user with diff**, versioning, one-click rollback.<br>Источник: [v4:165](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-injection](EVIDENCE.md#e-injection); TestApplyInjectionsURIAppendPrependRemove; TestApplyInjectionsJSONPointerAndEndpointPatch; TestApplyInjectionsPreservesUnknownAndEnforcesLimits; TestInjectionAuditFailureRollsBackMutation; TestInjectionAndQuotaVerticalSlice | Нет версии/rollback/dry-run/diff/templates/RFC6902; PUT не принимает scope/group/policy/user. Не применяются без выбранной ноды. Нет status/client-core gating, WG/AWG pool и merge subscriptions. |

#### Part A. Per-project functions / 2. Mrvibecodic/remnawave-subscription-middleware (PHP; beta; closest relative of SubGate)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-232 | **Extra configs by squad:** WireGuard / AmneziaWG / VLESS (raw, ws, httpupgrade, grpc, h2, xhttp, mKCP, QUIC, Reality, Vision) with a **per-client-core compatibility matrix**; nothing injected for blocked/expired/over-limit users; built on the panel's subscription template.<br>Источник: [inventory:35](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Нет версии/rollback/dry-run/diff/templates/RFC6902; PUT не принимает scope/group/policy/user. Не применяются без выбранной ноды. Нет status/client-core gating, WG/AWG pool и merge subscriptions. |
| F-233 | WG/AWG config pool (one key per subscriber/device). Merge of two subscriptions into one link.<br>Источник: [inventory:36](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-254 | Injections incl. WG/AWG/VLESS extra configs with core-compat matrix and status gating — F5 extended **NEW**<br>Источник: [inventory:84](../../subgate-feature-inventory.md) | **PARTIAL** | [E-injection](EVIDENCE.md#e-injection); TestApplyInjectionsURIAppendPrependRemove; TestApplyInjectionsJSONPointerAndEndpointPatch; TestApplyInjectionsPreservesUnknownAndEnforcesLimits; TestInjectionAuditFailureRollsBackMutation; TestInjectionAndQuotaVerticalSlice | Нет версии/rollback/dry-run/diff/templates/RFC6902; PUT не принимает scope/group/policy/user. Не применяются без выбранной ноды. Нет status/client-core gating, WG/AWG pool и merge subscriptions. |
| F-255 | WG/AWG config pool per subscriber/device — F5b **NEW**<br>Источник: [inventory:85](../../subgate-feature-inventory.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Не реализовано; runtime не проверено. |
| F-256 | Merge of two subscriptions — F5c **NEW**<br>Источник: [inventory:86](../../subgate-feature-inventory.md) | **MISSING** | [E-missing](EVIDENCE.md#e-missing); целевого теста нет; не проверено | Не реализовано; runtime не проверено. |
