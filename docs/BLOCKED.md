# Блокеры
Формат: ID / статус / причина / следующий проверяемый шаг. Закрытые записи сохраняются.
Независимую работу продолжать; внутренние regressions исправлять без ожидания владельца.

| ID | Статус | Причина | Следующий шаг |
|---|---|---|---|
| B01 | Закрыт в reorg | Legacy F-049…053/F-288 смешивали запрещённый connector и obsolete non-goals; запрет уже явный | D11; connector исключён, F21/F27 разрешены ответом Q1, historical rows сохранены |
| B02 | Открыт: test-panel evidence | Не проверена заглушка Remnawave при all-node quota exhaustion | Synthetic user/test panel3.4.5; записать non-skipped response fixture; если stub нет — решение владельца |
| B03 | Закрыт2026-10-14 | Отдельная panel3.4.5+node3.4.2/Xray26.7.28 созданы, test GET contract ранее PASS | Credentials не читать/не печатать; текущий audit live не повторял |
| B04 | Открыт: внутренняя регрессия | go test/coverage/make check FAIL,5 health tests получают sql:no rows | Проверить scheduledNodeProbe test fixtures/синхронизированные nodes, исправить с assertions; повторить suite+race |
| B05 | Открыт: scope optional | Inventory own-client encryption/F25/F26/support/plugins later и CRM граница не равны обязательному v1 | Owner scope перед risky optional execution; независимые v1 задачи не блокирует |
| B06 | Открыт: acceptance | Live Stats/panel/nginx/Xray transports opt-in SKIP; real accounting/regional/browser/perf не доказаны | Isolated synthetic environment, non-skipped child assertions; production writes не использовать |
| B07 | Открыт: data policy | Legal hold/дополнительные группы capture из v4 конфликтуют с90-day deletion и single-owner Q6 | Q6 применять; продление90d/новые группы не придумывать, уточнить только при реализации |

## B04: воспроизводимые тесты
TestMaintenanceSuppression; TestMaintenanceSuppressesWindowEntirelyInsideSlowProbe;
TestQuarantineMinimumAndBackoffStartAfterSlowProbe; TestScheduledHealthUnknownCancellationAndAtomicFailure;
TestScheduledHealthIncompleteEndpointsCannotQuarantine.
Команда: go test -count=1 ./core/internal/server.
Наблюдалось17.10.2026 в baseline dd91b66, до правки docs; это не ошибка новых markdown.
