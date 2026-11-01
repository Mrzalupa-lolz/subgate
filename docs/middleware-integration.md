# SubGate и Remnawave Subscription Middleware

Решение владельца 2026-11-01: продукты работают независимо, SubGate не участвует в выдаче подписок.

| Область | Владелец |
| --- | --- |
| Подписки/зеркала, устройства/HWID/UA запросов, grace, configs, branding, чат, управление пользователями | remnawave-subscription-middleware |
| Связь с Remnawave, сбор телеметрии нод, внутренние логи Xray, анализ аномалий и очередь разбора | SubGate |
| nDPI, точная атрибуция и временный shaper подтверждённых нарушителей | План SubGate; исполнители пока не реализованы |
| Внешний DDoS и firewall | Отдельные скрипты на нодах |

VPN-клиент обращается к middleware и сервису подписок Remnawave напрямую. Core обращается к API Remnawave, Agent отправляет Core статистику и логи Xray. Цепочка middleware → SubGate → подписки больше не используется.

Из активного Core удалены gateway, transforms, requestmonitor, aliases `/sub/...` и `/{shortUuid}`, API request-signals, tiers/policies/pins и вкладка пользователей. Снимок пользователей нужен для атрибуции журналов, а не для управления подписчиками. Квоты остаются учётом телеметрии и не изменяют подписку или data plane.

Старые таблицы/правила не удаляются. Код: [архив контура подписок](../_archive/monitoring-scope-2026-11-01/README.md). Более ранние injection/user-action изменения: [архив 2026-10-31](../_archive/middleware-overlap-2026-10-31/README.md).

После запуска новой версии прежние URL подписок SubGate вернут 404. Домен подписок должен вести к middleware. Эта локальная правка не меняет DNS, не публикует и не развёртывает сервисы. `SUBGATE_SUBSCRIPTION_URL` больше не читается. Вебхуки Remnawave по-прежнему могут поступать напрямую в SubGate или через fan-out middleware с собственным HMAC-секретом.
