# Xray logs, activity и хранение
О чём: F19 ingestion и F20 searchable destination journal, storage/privacy.
Когда читать: parser/tailer, encrypted events, retention/compaction, query/export/purge.

## Runtime
Код: pkg/xray/logs.go, agent/internal/agent/agent.go:collectLogs, server/telemetry.go, activity_access.go, identity.go.
Agent читает access.log с EOF, ParseLine извлекает metadata и отправляет wire.Activity.
Core пишет AES-GCM JSON rows sg_activity, не columnar segments.
Activity Journal utility создан в Server, но AddEvent/Compact/Export не подключены к telemetry path:
coalescing/NDJSON/tiers в utility не являются готовыми runtime функциями.
Query/export поддерживают user/node/from/to/domain; aliases расширяются только при unambiguous mapping.
Domain filter применяется к bounded result set, поэтому это не полный reverse index.
CSV/JSON export есть; session timeline/scrubber, domain-first-seen, comparisons/categories и NDJSON endpoint отсутствуют.
Encrypted audit+rate limit и key rollover — [security](security.md); точные HTTP limits — [api](../api.md).

## Требуемая capture semantics
Activity ON по умолчанию с visible indicator; global/node/user selection и исключения, capture-quality user/domain/client-IP fractions.
Sniffing/log.email наличие проверяется для каждого protocol/transport/version, изменение panel config только proposal+confirmation/restart.
Только destination host:port, никогда URL/page/payload. Local DNS/ECH/DoH могут давать только IP.
IP-only enrich ASN/org, cached reverse DNS, DNS-log hints с resolved_by и provenance; нельзя выдавать предположение за посещённый URL.
Event: ts,user,node,inbound,outbound,src_ip,dst_host|dst_ip,dst_port,net,result.
Данные sensitive, никакого external favicon/tile lookup. Изменение retention/erasure должно быть аудируемым.

## F19 gaps
Tail поддерживает append/rename/copytruncate и partial lines в коде; torture/golden tests отсутствуют.
Строка>1MiB сейчас прерывает tail; send в заполненный channel блокируется, skip-ahead/gap bound не реализован.
Это не FIFO Xray sink, но требуемой устойчивости collector при flood нет.
Нужны access/error/debug/info/warning/DNS/journald/docker-json-file sources read-only без Docker socket.
Classifiers/log helpers не связаны в all-level pipeline; unparsed ratio/bounded sample/drop counters и per-class adaptive limits отсутствуют.
Raw log text не сохранять; ephemeral live tail с audit и сроком жизни, error samples только operator opt-in.
Golden corpus по supported Xray versions/log levels, unknown tolerant parser, fuzz, >=100k lines/s/Agent core,
rotation rename/copytruncate/recreate/vacuum с явной границей loss/duplicates, A/B throughput Xray unchanged.
Configured level показывать и предупреждать о стоимости debug; change только через panel API explicit confirmation.

## F20 storage
Требуются Agent10s coalesce по user/destination/port/net/inbound/src/node, count/first/last; chatter caps с overflow summaries.
Dictionary domains/IP/ASN, publicsuffix/eTLD+1 optional aggregation, sorted(user,ts) immutable hourly/day segments,
delta/RLE/bit packing/varints+zstd, user range+bloom destination indexes, atomic writes.
Pure-Go Parquet/custom выбор только benchmark; SQLite D07 не разрешает молча отменить compactness budgets.
Agent WAL/spool ACK+idempotency, crash recovery, AEAD segments/dictionaries, bounded memory и disks, retention eviction должны быть проверены.
Q6 overrides прежние14/90/365:7d full, compression после7d, удаление через90d; budget50% свободного места при deployment.
Runtime defaults/UI ещё говорят14d; warm archive и deployment budget нет — это gap, не новая политика.
Legal hold из v4 сохранить как unresolved B07; не выдумывать исключение из90-day deletion.
Backup exclusion по умолчанию пока нарушен: local snapshots включают activity.
Purge нужен на всех tiers/dictionaries/indexes, без восстановления при replay; сейчас только SQLite rows.
Targets: ingest100k events/s4vCPU, <=8B/event, >=30x raw-text reduction, compaction<=25%CPU,
hot user-day p95<=300ms, warm month<=1.5s, cross-user domain day<=3s.
Нужны sessions gap5min/configurable, hot second-T±window, timelines/top destinations, filters ASN/operator/client IP и scrubber.
