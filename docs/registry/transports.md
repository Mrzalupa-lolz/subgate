# Xray: transports, stats capability и редактор
О чём: opaque protocols, транспортные пробы и текстовый Xray toolkit F27.
Когда читать: R15 continuation, protocol matrix, REALITY/QUIC или реализация editor.

## Opaque contract
Код: pkg/healthprobe/{descriptor,probe,http,grpc,xhttp,quic}.go, core/internal/panel/inbound.go.
Registry names/aliases привязаны к Xray26.7.28; неизвестный protocol сохраняется и показывает generic.
Unknown protocol требует explicit supported transport; unknown/missing transport/security — unverified.
Known raw/TCP может доказать только connect; security=tls требует normal cert+hostname validation.
REALITY на generic TCP даёт только TCP reachability, не REALITY acceptance.
WireGuard/mKCP/Hysteria/generic UDP/legacy QUIC остаются unverified без корректной/credentialed пробы.
Протоколы/transport/security не whitelist для passthrough; future fields/tuic/anytls не удалять.
Требуется stats matrix per protocol: per-user либо inbound fallback с маркировкой not per-user; полная матрица не доказана.

## Пробы: что реализовано
| Конфигурация | Проверка | Предел |
|---|---|---|
| TCP/raw | connect | только endpoint |
| TLS | SNI+certificate+hostname | не proxy authentication |
| WS/websocket | fresh RFC6455 challenge, matching101/headers/accept | transport acceptance |
| HTTPUpgrade | pinned Xray GET101/Upgrade/Connection без WS challenge | отдельно от RFC6455 |
| gRPC | HTTP2, empty Hunk frame, content-type, valid envelopes, final grpc-status0 | не user auth |
| XHTTP/splithttp | OPTIONS configured literal path,200+Xray CORS | route preflight, не session |
| XHTTP TLS alpn exactly h3 | QUIC TLS1.3+HTTP3 OPTIONS | xhttp-h3-options, не arbitrary QUIC |
| REALITY HTTP/gRPC/XHTTP | unverified | acceptance ещё нет |
| generic UDP/WG/mKCP/Hysteria | unverified | credential/protocol check нужен |

Raw inbounds проецируются только в sanitized path/Host/SNI/HTTP3; clients/keys/certs/arbitrary headers не сохраняются.
method имеет приоритет над network; WS/HTTPUpgrade ed удаляется; invalid/mismatch config даёт unverified.
Для gRPC поддержаны serviceName/multiMode/authority, normal/custom/default //Tun paths.
XHTTP modes/extra нормализованы по pinned version; literal percent/hash экранируется на wire без изменения server path.
Host/SNI не выбирают destination; redirects/proxy environment запрещены, response headers16KiB, gRPC body4KiB.
Нормальная TLS validation обязательна; deadlines/context cancellation закрывают sockets.

## HTTP/3: исправление старого статуса
quic.go уже есть в исходном baseline этого поручения; старое «h3-only всегда unverified» устарело.
DNS resolve context-bound до UDP socket, до8 IP candidates с поделённым3s budget; incomplete DNS coverage unverified.
HTTP3 имеет bounded flow-control windows, normal TLS1.3, фиксированный panel endpoint, response header bound16KiB.
Legacy TCP votes для H3-only не превращаются в failure quorum; mixed endpoints требуют реального coverage.
TestQUICHandshakeProbe, TestHTTP3UnsupportedSettingsStayUnverified, TestHTTP3UntrustedCertificateAndUDPDeadline,
TestHTTP3ResolutionHonorsDeadlineAndClosesSockets, TestHTTP3UncoveredDNSCandidatesRemainUnverified,
TestAgentReportsHTTP3Probe, TestHTTP3LegacyVotesCannotQuarantine PASS в этом аудите.
Live Xray HTTP3 opt-in SKIP; WS/HTTPUpgrade тоже SKIP, gRPC/XHTTP parent PASS имеют только SKIP children.
Исторические live PASS из старого R15 — архивная запись, не свежая проверка.

## Текстовый редактор v1
Q1 явно включает F27 в v1: editor в web UI с syntax highlighting, autocomplete, config validation перед apply,
hover documentation (description/args/allowed values) для node Xray version26.7.28.
Встроенная документация — offline fallback; разрешены обновления данных, не замена SubGate binary.
В коде Xray editor отсутствует: configEditor в app.js редактирует только SubGate policy objects.
Нужны profile read/write по точному OpenAPI, explicit confirmation, test-panel-only writes, audit final outcome, version conflict/rollback.
Toolkit Part B также требует linter, XHTTP-nginx/Selfsteal/REALITY/WARP generators, balancer/observatory, snippets и link import/export.
Это clean-room реализация по public descriptions; копирование AGPL sibling code и connector запрещены.
Сначала versioned data+read editor, затем по одному completion/hover/linter/apply workflow; не заменять текстовый editor графическим.
