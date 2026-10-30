# Текущее состояние

- 2026-10-14: R13/R14 завершённые проходы; R15 generic/WS/HTTPUpgrade/gRPC/XHTTP проверены.
- DONE3 / PARTIAL167 / STUB8 / MISSING72 / OUT_OF_SCOPE39; v1 не готов.
- Production только GET; test panel3.4.5/node3.4.2/Xray26.7.28; secrets не выводить.
- XHTTP Core/manual/Agent: sanitized path/Host/SNI, OPTIONS preflight xhttp-options, quorum/latency.
- Реальный Xray: auto/packet-up/stream-up/stream-one/splithttp + literal percent/hash; wrong path/Host reject PASS.
- Reviewer bugs RED→GREEN: literal path URL semantics; h3-only UDP config unverified/no failure votes.
- Final Linux make check и read-only security review PASS; coverage healthprobe81.6%/panel78.1%/server68.0%/agent52.0%.
- Далее R15 QUIC/HTTP3, затем REALITY; B01/B02 открыты.
- Regional/browser/full transport matrix не проверены; диск около1.1GiB, caches/образы не раздувать.
