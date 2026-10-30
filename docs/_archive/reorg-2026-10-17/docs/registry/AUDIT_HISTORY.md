> История исходного аудита; перечисленные удаления и проверки относятся к нему, а не к разделению плана.

### Где прежние ИИ-документы не подтверждались

Оценка относится к **рабочим версиям файлов на начало аудита**, не к восстановленной истории Git. Более раннее содержание не использовалось как доказательство.

| Документ / заявление | Что показал код и этот аудит |
|---|---|
| README: «0.2.0 release is production shaped»; PROJECT_COMPLETE: «release has repeatable Go and Docker builds» | Regular Go build воспроизведён; Docker/production rollout/подписанный опубликованный релиз не проверены. Сам текущий PROJECT_COMPLETE оговаривает неполный scope — он не заявляет, что вся спецификация завершена. Название файла не основание приписывать ему такое заявление. |
| docs/implementation-status: F6 «Implemented…limits» | Учёт реализован, но TestInjectionAndQuotaVerticalSlice **требует** исходную подписку после quota exhaustion; исключения ноды по F6 нет. |
| docs/implementation-status: F3 «Implemented selection» | Выбор ноды работает, но node_ids/online/UsersOnline — не panel-tag/squad/operator/transport-aware balancer. Нельзя считать весь F3 DONE. |
| docs/implementation-status: F5 «Implemented … CRUD» | Утверждение верно для ограниченного CRUD. Нет scope в PUT, dry-run/version/rollback; без selected node injections вообще не выполняются. |
| docs/ipintel: «SubGate builds a merged MMDB» и CLI `subgate ipintel lookup` | Core читает предоставленные локальные MMDB/JSON. Builder/feeds/CLI/bulk endpoint отсутствуют. Ссылка check-ip-sources.sh не имеет соответствующего файла. Даты/лицензии источников — неподтверждённые предположения, не результаты проверки. |
| docs/design: схема с remnawave-admin; P0_STATUS/P0_SUMMARY: «Design Complete» | Интеграция отсутствует и запрещена владельцем; P0 discovery/golden corpus/MMDB prototype/benchmarks не доказаны. Design description может быть проектным намерением; это не «P0 delivered» и не готовая архитектура. |
| P0_STATUS: make docs/p0-check-xray/p0-capture-logs/p0-report/init/dev-setup | В текущем Makefile таких targets нет. Команды не переносились. |
| CLAUDE: «Current Phase P0» и «Preact frontend» | В рабочем дереве runnable Core/Agent, API/tests/vanilla UI; P0-ярлык и Preact не описывают фактическое состояние. |
| docs/validation и docs/deployment-validation: прежние PASS Linux/browser/live node и ссылки /tmp на удалённом host | Это текст ИИ. Удалённые результаты/артефакты не проверялись, никаких SSH/production credentials не использовано. Они не перенесены как наблюдённые проверки; Linux/race/live Xray/Docker/browser здесь остаются не проверено. |
| BUILD/START: «statically built», operator workflows | Команды статической Linux сборки сохранены как инструкции; в этом аудите проверены regular Windows build, tests и isolated Core HTTP. Installer/services/Compose runtime не проверено. |
| docs/protocol-matrix/xray-logs/xray-stats: универсальные возможности и форматы | В документах есть пометки discovery-required; их нельзя повышать до доказанного протокольного coverage. Реальный log corpus и real-Xray tests всех сочетаний отсутствуют. |

Дополнительные фиктивные/заготовочные места в коде: `seedDemo`/demo subscription — явно synthetic; `ipintel.LoadTestData` — hardcoded данные, production New его не вызывает; `settings.activity_hot_days=14` — hardcode, фактический TTL из config может отличаться; `AuthConfig.TOTPRequired` и многие storage/source fields не подключены к поведению; `activity.Journal.Compact` лишь маркирует memory records tiers, не выполняет требуемую compaction; `TSDB` комментарий «rollup data would be in SQLite» и schema `speed_rollup_*` не являются rollup pipeline. Существование этих мест отмечено как STUB/неподключённая utility, код не удалялся.

### Remnawave-admin: результаты поиска и список удаления

Поиск `remnawave[-_]admin|Case211|violations:read` выполнен по source/config/deploy/CI/doc text с явными расширениями; `.env`, secrets/keys/DB/backup, `.agents`/`.codex` не читались. В Go/JS/HTML/CSS, проверенных YAML, shell/Python сценариях и workflow совпадений не обнаружено. Варианты custom naming и runtime secret config **не проверено**.

| Место | Решение |
|---|---|
| `docs/design.md:81` optional connector в архитектуре | Удалён вместе с устаревшим design после архивирования; новая архитектура выше без коннектора. |
| `ASSUMPTIONS.md:81` hostile remnawave-admin data trust | Удалён вместе с ASSUMPTIONS после архивирования. |
| `CLAUDE.md:48` integration/reference guidance | Удалён при замене CLAUDE на указатель AGENTS, исходная версия архивирована. |
| `subgate-ai-prompt-v4.md:18,32,44,82–87,97,213,317,345,360,373` | **Не удалять/не править**. Включает connector, architecture, optional forwarding, threat model, references, phase P4; текущая инструкция имеет приоритет. |
| `subgate-feature-inventory.md:3,8,14–27,120` | **Не удалять/не править**. Public feature reference допустим, optional coexistence connector отменён текущим scope. |
| Коннектор/HTTP endpoints/DB config/зависимости/Agent integration в коде | Не найдены, список кода для удаления пуст по проверенному поиску. Отдельный roadmap R01 оставлен для контрольного поиска и удаления только если появится подтверждённый inventory; сейчас код не удалять. |

Упоминания remnawave-admin в этом плане и AGENTS — описание границы/результат аудита, не рекомендация интеграции. Чужие feature descriptions — clean-room reference, не лицензия копировать код.

### Решения по документации и архив

Архив создан **до переписи/удаления**: `C:\Users\maxim\Desktop\subgate-docs-backup\20261013-audit`, 26 `.md` файлов с исходными относительными путями. Скопированы 8 удаляемых root-docs, прежние README/CLAUDE и все 16 прежних docs/*.md. Copy-Item завершился успешно, размеры оригинала/копии проверены; содержимое backup-файлов не читалось. Архив не коммитился.

Удалены после переноса полезного:

- Root: `ASSUMPTIONS.md`, `BUILD.md`, `IMPLEMENTATION_PLAN.md`, `P0_PLAN.md`, `P0_STATUS.md`, `P0_SUMMARY.md`, `PROJECT_COMPLETE.md`, `START.md` — конкурирующие планы/статусы, устаревшие commands или неподтверждённые conclusions.
- docs: `api-mapping.md` — contract info перенесён в architecture/API; `deployment.md` — actual setup/ownership/namespace/settings сюда и README; `implementation-status.md` и `release-readiness.md` — заменены реестром/roadmap.
- docs: `validation.md`, `deployment-validation.md`, `xray-runtime.md` — historical AI claims не доказательство; их результаты не перенесены как PASS.
- docs: `design.md`, `ipintel.md`, `protocol-matrix.md`, `xray-stats.md`, `xray-logs.md` — проектные обещания/непроверенные discovery matrices; требования сохранены в source files и реестре, реальные ограничения описаны выше.
- docs/adr: `0001-language.md`, `0002-activity-journal-storage.md` — мотивы выбора Go и нерешённое сравнение segment formats учтены в architecture/R31, performance claims без benchmark не сохранялись как факт.
- docs/runbooks: `restore.md` — commands и ограничения offline recovery перенесены в раздел 6.

В `docs/` остаются только **PROJECT_PLAN.md** и **api.md** (исправленный справочник существующих routes/semantics, без roadmap). Empty directories не удалялись. `test/discovery/README.md` и `core/internal/ipintel/testdata/README.md` оставлены рядом с scripts/fixtures, вне docs: они не планы и не evidence; их старые discovery claims не считались выполненными. `.agents`/skill docs не изменялись. Защищённые requirements/OpenAPI/skills-lock/mod/sum/Makefile/.dockerignore/code/tests не удалены и не переписаны.

