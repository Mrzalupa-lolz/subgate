# Нужный SubGate Remnawave API
О чём: выбранные paths spec3.4.5, назначение/ключевые поля, runtime vs planned.
Когда читать: panel reader/action/editor; schema остаётся [remnawave-openapi.json](../remnawave-openapi.json) без изменений.
Извлечение17.10.2026: jq по одному .paths[...] и выбранным .components.schemas[...], полный JSON не читать.
Все методы ниже подтверждены наличием в schema; planned/generic action не означает завершённый workflow.
Scopes/доступность API-token: [panel-contract](panel-contract.md). Production GET-only, mutations только test panel с confirmation/audit.

| Метод | Путь | Назначение | Ключевые поля | Состояние подключения |
|---|---|---|---|---|
| GET | /api/users | sync users | start/size<=1000; response.users,total; uuid/id,shortUuid,status,trafficLimitBytes,userTraffic.usedTrafficBytes | runtime |
| GET | /api/users/{userId} | user detail | userId numeric по spec; uuid projection не заменяет numeric path contract | runtime |
| GET | /api/users/by-short-uuid/{shortUuid} | subscription identity | shortUuid→response user | runtime |
| GET | /api/nodes | sync nodes/health | response[]: uuid,address,isConnected,isDisabled,tags,configProfile.activeInbounds | runtime |
| GET | /api/hosts | host→node mapping | response[]: uuid,address,port,nodes,isDisabled,remark | runtime |
| GET | /api/system/stats/recap | startup version | response.version (3s startup budget Core) | runtime |
| GET | /api/sub/{shortUuid} | upstream subscription | raw subscription body+headers, shortUuid | runtime |
| GET | /api/sub/{shortUuid}/{clientType} | format subscription | clientType: stash,singbox,mihomo,json,v2ray-json,clash | runtime |
| POST | /api/users/{userId}/actions/enable | enable user | numeric userId, schema-defined action response | generic action |
| POST | /api/users/{userId}/actions/disable | disable user | numeric userId | generic action |
| POST | /api/users/{userId}/actions/reset-traffic | reset panel user traffic | numeric userId; не Xray StatsService reset | generic action |
| POST | /api/users/{userId}/actions/revoke | revoke subscription | numeric userId; body revokeOnlyPasswords?,shortUuid? | planned |
| POST | /api/nodes/{uuid}/actions/enable | enable node | uuid | generic action |
| POST | /api/nodes/{uuid}/actions/disable | disable node | uuid | generic action |
| POST | /api/nodes/{uuid}/actions/restart | restart node | uuid | generic action |
| POST | /api/nodes/{uuid}/actions/reset-traffic | panel node traffic action | uuid; not StatsService counter reset | generic action |
| PATCH | /api/hosts | host mutation | UpdateHostBodyDto: uuid required, optional metadata | planned |
| POST | /api/hosts/bulk/enable | bulk enable | body uuids[] | planned |
| POST | /api/hosts/bulk/disable | bulk disable | body uuids[] | planned |
| GET | /api/config-profiles | text editor profile list | response.total,configProfiles[]: uuid,name,tags,config,inbounds,nodes | planned |
| GET | /api/config-profiles/{uuid} | text editor profile | uuid; response DTO exact по spec | planned |
| GET | /api/config-profiles/{uuid}/computed-config | computed validation view | uuid; exact response DTO before implementation | planned |
| PATCH | /api/config-profiles | editor apply | body uuid required; name?,config? (object) | planned |
| GET | /api/hwid/devices/{userId} | native HWID card | numeric userId, response DTO по spec | planned |
| GET | /api/subscription-request-history | native request history | query filters/pagination перед reader сверить spec | planned |
| GET | /api/bandwidth-stats/nodes/{uuid}/users | native per-node/user quota usage | uuid; timestamps/range сверить selected path | planned |
| GET | /api/bandwidth-stats/users/{userId} | coarse per-user bandwidth fallback | numeric userId; query exact по spec | planned |
| GET | /api/internal-squads | tier/grace squad list | native response shape, not rebuilt CRM | planned |
| GET | /api/external-squads | external squad membership | native response shape | planned |
| GET | /api/infra-billing/nodes | panel-native node billing display | read existing panel data only; billing engine excluded | planned |
| GET | /api/tokens/scopes | operator scopes discovery | не создавать keys через Core; actual scopes matrix panel-contract | operator only |

## Важные ограничения
API responses обычно envelope response; subscriptions raw. Request cap16MiB, redirects запрещены.
User path userId в spec numeric, cache использует uuid как stable identity: actions не могут механически подставлять cache UUID.
Node reset-traffic есть в spec; допустимые Core actions проверяются отдельно.
Revoke body: revokeOnlyPasswords defaultfalse, shortUuid16..64 optional; предпочтительно panel generates shortUuid.
Config apply PATCH /api/config-profiles, не PATCH /api/config-profiles/{uuid}.
Computed-config не заменяет validation; editor version Xray26.7.28.
Subscription OpenAPI содержит GET; Core HEAD отдельно проверен fixture upstream, HEAD schema не заявляется.
Native torrent/shared ASN-IP list paths не добавлены: сначала выбрать точный endpoint из spec, не угадывать API.

    jq '.paths["/api/config-profiles"].patch' remnawave-openapi.json
    jq '.components.schemas.UpdateConfigProfileBodyDto' remnawave-openapi.json

Локальная запись не использовалась, credentials не читались. Прежний scopes discovery не означает свежую live проверку.
