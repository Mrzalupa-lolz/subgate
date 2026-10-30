# Реестр: performance

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 1. Product principles

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-010 | **Fast and exact.** Performance budgets (section 11) are requirements verified by benchmarks in CI.<br>Источник: [v4:23](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-perf](EVIDENCE.md#e-perf); целевого теста нет; не проверено | Benchmark/profile/report/regression gates отсутствуют (rg Benchmark не находит функций). Бюджеты из требований не измерены; make bench без benchmark functions не доказательство. |

#### 3. Platform, language and stack

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-040 | Runtime: `automaxprocs`, `automemlimit`, `sync.Pool` on hot paths, pprof behind auth; <br>Источник: [v4:68](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-perf](EVIDENCE.md#e-perf); целевого теста нет; не проверено | Benchmark/profile/report/regression gates отсутствуют (rg Benchmark не находит функций). Бюджеты из требований не измерены; make bench без benchmark functions не доказательство. |

#### 11. Performance budgets (verified by CI benchmarks; change only via ADR)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-174 | Gateway: cache-hit p99 ≤ 5 ms in-process; fresh transform p99 ≤ 25 ms excluding upstream; ≥ 10 000 req/s cache hits on 2 vCPU; panel change visible ≤ 1 s after webhook.<br>Источник: [v4:302](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-perf](EVIDENCE.md#e-perf); целевого теста нет; не проверено | Benchmark/profile/report/regression gates отсутствуют (rg Benchmark не находит функций). Бюджеты из требований не измерены; make bench без benchmark functions не доказательство. |
| F-175 | IP-intel lookup ≤ 5 µs p99; hot-swap with zero failed lookups.<br>Источник: [v4:303](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-perf](EVIDENCE.md#e-perf); целевого теста нет; не проверено | Benchmark/profile/report/regression gates отсутствуют (rg Benchmark не находит функций). Бюджеты из требований не измерены; make bench без benchmark functions не доказательство. |
| F-176 | Agent @ 1 s sampling with 5 000 online users: ≤ 1.5 % of one core, ≤ 40 MB RSS; log pipeline ≥ 100 000 lines/s per core at `debug` within a fixed memory cap.<br>Источник: [v4:304](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-perf](EVIDENCE.md#e-perf); целевого теста нет; не проверено | Benchmark/profile/report/regression gates отсутствуют (rg Benchmark не находит функций). Бюджеты из требований не измерены; make bench без benchmark functions не доказательство. |
| F-177 | Core: ≥ 200 000 user-samples/s ingestion; UI diff stream ≤ 100 KB/s per viewer; ≤ 16 ms/frame rendering 10 000-row virtualised table; ≤ 512 MB RSS at 20 nodes × 5 000 online users (raw rings capped).<br>Источник: [v4:305](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-perf](EVIDENCE.md#e-perf); целевого теста нет; не проверено | Benchmark/profile/report/regression gates отсутствуют (rg Benchmark не находит функций). Бюджеты из требований не измерены; make bench без benchmark functions не доказательство. |
| F-178 | Activity journal (F20): ≥ 100 000 events/s ingest on 4 vCPU, ≤ 8 B/event in the hot tier, ≥ 30× reduction vs raw text, user-day query p95 ≤ 300 ms, compaction ≤ 25 % CPU, bounded disk (quota enforced).<br>Источник: [v4:306](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-perf](EVIDENCE.md#e-perf); целевого теста нет; не проверено | Benchmark/profile/report/regression gates отсутствуют (rg Benchmark не находит функций). Бюджеты из требований не измерены; make bench без benchmark functions не доказательство. |
| F-179 | `make bench` with reproducible synthetic generators, checked-in pprof profiles, regression thresholds, published results table.<br>Источник: [v4:307](../../subgate-ai-prompt-v4.md) | **MISSING** | [E-perf](EVIDENCE.md#e-perf); целевого теста нет; не проверено | Benchmark/profile/report/regression gates отсутствуют (rg Benchmark не находит функций). Бюджеты из требований не измерены; make bench без benchmark functions не доказательство. |
