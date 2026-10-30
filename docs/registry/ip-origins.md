# IP intelligence и origins
О чём: local lookup, слои данных L1–L5, trusted source enrichment и карты.
Когда читать: feeds/MMDB, operator/geo classification, subscriber origins.

## Реализация
Код: core/internal/ipintel/ipintel.go, server/ipintel.go и clientip.go.
Core читает local JSON prefixes, MMDB или directory*.mmdb; reload off-path сохраняет last-good при invalid input.
TestPrefixLongestMatchAndLastGoodReload и TestMMDBLookupReloadAndSourceSwitch PASS.
UI lookup/source attribution есть, precision browser presentation не проверена.
Нет downloader/merge builder/checksum/license enforcement/schedule, bulk/CLI, curated editor/CSV и coverage report.
Stale warning45d hardcoded, не configurable N; <=5us p99 и hot-swap under load не измерены.

## Требуемые data layers
L1 ASN authoritative: IPtoASN PDDL/RIPE RIS/RouteViews, BGP.tools optional с проверенными terms/User-Agent.
L2 RIR delegated-extended RIPE/ARIN/APNIC/LACNIC/AFRINIC: новая сеть получает registration country при отсутствии geo.
L3 DB-IP Lite City/ASN CC BY4.0 с обязательным visible attribution; IPinfo Lite token/terms/share-alike проверить;
MaxMind GeoLite2 optional account/license; operator MMDBv2 допустим.
L4 network type: mobile/home-isp/hosting/vpn-proxy/cdn/edu/gov/unknown; ASdb/ipverse/BGP.tools/PeeringDB только с проверенной лицензией.
Mobile seed RU first по organisation с needs-review; name/CGNAT/rDNS heuristics дают suggestions, не auto-override.
L5 threat tags: scanners/government/skipa/Spamhaus DROP/FireHOL L1/Tor, confidence/provenance/override и update history.
Даты/URLs/лицензии старых docs не результат проверки; каждый source проверяется отдельно перед download/redistribution.

Builder должен merge off-path в internal MMDB и atomic hot-swap; checksum, last-good, stale alert configurable.
ASN precedence L1; geo country с учётом L2/AS-country contradictions и multinational explanation; medium confidence L1+L2 fallback.
Result: ASN/name/org, network_type, country/region/city/coordinates/timezone, confidence/sources/build dates.
City приблизительна, mobile egress не местоположение человека; no street pins и понятная precision label.
Lite country+ASN mode допустим для памяти, bounded cache/reload без lookup failures.

## Origins F8/F19
Требуются раздельные subscription-request и actual connection origins. Сейчас activity имеет src_ip, aggregates/maps отсутствуют.
Connection logs — более точный источник VPN origin; panel/agent identity и client IP должны быть согласованы.
Grouping по country/city/ASN/operator/network type за период, per-user last distinct history+timestamp, CGNAT hints и filters.
Trusted direct peer/proxy/v4mapped уже есть; CDN real-IP только от verified ranges, IPv6 /64 и many-users-per-IP ещё нужны.
Retention origins: raw30d/aggregates365d configurable, optional /24+/48 truncation, salted hashing/no-IP off by default.
Это отдельная origins retention, не разрешение продлить activity journal сверх Q6.
Bundled map data only, никакого external tile server/запроса IP third party.
IP lookup/mapping/security не должны блокировать subscription если feed/reload недоступен.
Acceptance: golden IP corpus, prefix есть L1+L2 но не geo → ASN+country/medium,
coverage report high-confidence fraction, source attribution, reload при нагрузке без failed lookups.
Интерфейсы CLI/bulk/curated overrides требуются, но сейчас есть только single lookup/reload endpoints.
