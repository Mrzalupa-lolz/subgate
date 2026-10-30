# Реестр: webhooks

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 4. Integration contracts / 4.1 Remnawave panel (mandatory)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-045 | **Webhooks (primary sync):** verify the **HMAC signature** against the raw body, replay protection, idempotency; sub-second cache invalidation; reconciliation loop every 5 min (jittered) repairs missed events.<br>Источник: [v4:77](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-webhook](EVIDENCE.md#e-webhook); R08 TestWebhookRefreshesUserBeforeDeadline; TestWebhookDuringPanelFetchStaysFresh; TestWebhookPreservesCanonicalNumericIdentityAndEventOrder; TestWebhookOlderThanCompletedSyncCannotRegressUser; TestWebhookOlderThanCompletedSyncCannotResurrectDeletedUser; TestWebhookUserPersistenceRollsBackReplayMarker | user.updated native payload обновляет local policy fields и инвалидирует cache синхронно <1s в synthetic tests; HMAC/replay/atomic user+marker, event order и stale full-sync защита проверены. Другие events только invalidation; live webhook deployment/fan-out не проверены. Q8 задаёт минутный reconciliation; jitter не внедрён. |

#### Part A. Per-project functions / 2. Mrvibecodic/remnawave-subscription-middleware (PHP; beta; closest relative of SubGate)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-234 | Webhook "tee" (fan-out with re-signing). Masking login page. Support chat widget on the stub page (Telegram + webhook).<br>Источник: [inventory:37](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Требуемого полного поведения нет. HMAC/replay/cache invalidation работают; нет немедленного обновления users/nodes, jitter, fan-out/re-signing; default sync 1 min, требование 5 min; &lt;=1 s end-to-end не измерено. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-261 | Webhook fan-out with re-signing — F14 extended **NEW**<br>Источник: [inventory:91](../../subgate-feature-inventory.md) | **MISSING** | [E-webhook](EVIDENCE.md#e-webhook); целевого теста нет; не проверено | Требуемого полного поведения нет. HMAC/replay/cache invalidation работают; нет немедленного обновления users/nodes, jitter, fan-out/re-signing; default sync 1 min, требование 5 min; &lt;=1 s end-to-end не измерено. |
