# Gateway, routing и relay
О чём: fail-open subscriptions, cache, tier selection/pins и F32 relay accounting.
Когда читать: critical path, format transforms, policies, proxy fallback.

## Gateway runtime
Код: core/internal/gateway/, core/internal/server/routing.go, server.go, core/internal/routing/.
GET/HEAD четыре aliases /sub/{shortUuid}[/{clientType}] и /{shortUuid}[/{clientType}].
Method/raw query, Accept-Language/UA/HWID/X-Ver-Os/all X-Device-* forwarded; auth/cookie/hop headers исключены.
Response subscription metadata+announce/announce-url сохраняются. HEAD не запускает body transform.
Cache variant содержит method/exact metadata, TTL30s stale5min, coalescing/background refresh<=32.
Generations/CAS не дают late refresh воскресить invalidated snapshot; authoritative4xx удаляет snapshot.
Raw upstream cache не transformed output; health/policy применяются на каждом request.
Policy namespace setter есть, но versioned runtime policy store ещё не bound.
TestSubscriptionHEADAndMetadata/MetadataCacheIsolation/RawHeaderCacheIsolation/ConnectionMetadataNotForwarded PASS.

Transformer panic/deadline300ms/worker saturation<=32 возвращают exact upstream body/headers; late result изолирован.
TestTransformPanicAndDeadlineFailOpen, TestTransformWorkersBoundedAndContextCancelled,
TestStaleSnapshotServesWhileRefreshRunsInBackground и TestLateSWRRefreshCannotUndoAuthoritativeForeground PASS.
DB/panel/partial-response/late-work full chaos matrix и полный real-fixture corpus ещё нужны.
Unknown formats/entries, numbers/fields, ambiguous node mappings сохранять; никогда пустая подписка из-за ошибки transform.

## Deployment modes
Совместная работа с middleware и распределение ответственности: [integration](../middleware-integration.md).
Shipped nginx /sub fallback переводит502/503/504 Core в /api/sub origin с query/method, strip credentials, verified TLS.
TestKilledCoreSubscriptionFallback opt-in в этом аудите SKIP; исторический synthetic nginx PASS сохранён в архиве.
Caddy сейчас только reverse_proxy без fallback; встроенный Core ACME отсутствует.
F1a mirror RU → configured origin и standalone/panel-side workflows не выделены и не проверены;
один custom upstream URL ещё не доказательство полного mirror mode.

## Tier/policy/pin runtime
Address mapping node IP/host→single node; shared/ambiguous hosts не исключаются автоматически.
Manual enabled user→online eligible node pin имеет приоритет над policies.
Policies first ascending priority, conditions country/ASN/UA, explicit tier node_ids.
random/roundRobin/leastLoad(UsersOnline)/leastPing10min verified sample; min_healthy и fallback cycle checks.
Health quarantined/recovering исключается на cache hit, fallback/unsafe transform возвращает original.
Quota exhaustion сейчас также возвращает original; это нарушает scoped Q4 stub, см. [subscription](subscription.md).
Нет tag/squad/app/plan/operator/network_type membership/layout, обязательного default policy, полноценного burstObservatory/balancer rewrite.
Нужны format-preserving Xray/Mihomo/sing-box/URI fixtures, transport-aware entries и first-match acceptance mobile vs home ISP.

Automatic capacity-weighted rendezvous pin отсутствует; нет ordered lists/single/ordered-fallback/balancer-preferred,
sticky-return hysteresis, max pinned/node, rebalance dry-run/change budget и exit-changes/user/day metric.
Один strategy+format transform либо один pin lifecycle — отдельная задача плана; большой «routing done» не допускается.

## F32 relay
Topology/multi-hop route resolution и subtractive direct/tunneled traffic accounting не реализованы.
Плоские user/node speed series не доказывают relay attribution.
Начать с одного validated relay edge, cycle rejection и synthetic route fixture; затем один-hop subtractive counter invariant.
Обязательно исключить двойной учёт, отрицательные bytes и воздействие на panel accounting; multi-hop отдельные tests.
Квоты и переданные middleware функции подписки описаны в [subscription](subscription.md), протоколы в [transports](transports.md).
