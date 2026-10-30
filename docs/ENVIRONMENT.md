# Окружение разработки
О чём: локальная test-машина, ограничения ресурсов и пределы этого аудита.
Когда читать: запуск Linux/CGO/Docker проверок; не источник credentials.

Codex работает локально в /home/subgate/work/SubGate, SSH не требуется. Исторически Docker доступен, sudo нет.
На старте reorg свободно3.3GiB (df); прежние11GiB устарели. Перед крупными сборками проверять df -h.
Не удалять caches/images/data автоматически; docker prune/go clean не часть docs-поручения.
Sandbox bwrap не стартует: loopback RTM_NEWADDR Operation not permitted; checks выполнялись после host approval.
Токены/.env/ключи/DB не читать. Производственная панель только GET; тестовые writes лишь в отдельной среде.
B03 описывает ранее поднятые synthetic panel/node; текущий аудит их состояние не проверял.
