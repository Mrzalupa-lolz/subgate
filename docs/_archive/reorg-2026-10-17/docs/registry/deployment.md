# Реестр: deployment

[План](../PROJECT_PLAN.md) · [INDEX](INDEX.md) · [Правила](REGISTRY_GUIDE.md)

#### 3. Platform, language and stack

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-017 | **Platform: Linux only** (amd64 + arm64; kernel ≥ 5.10; systemd). No Windows/macOS code paths, no portability shims — use Linux-native facilities freely (inotify, netlink, nftables, journald, epoll, optional eBPF). Build tags `linux`. Developers may build on other OSes only via containers.<br>Источник: [v4:40](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-deploy](EVIDENCE.md#e-deploy); целевого теста нет; не проверено | Файлы существуют; installation/Docker/Linux multiarch/CI run не проверено. Installer local-only/checksum optional/version 0.2.0, без signature/download/wizard/bootstrap token; Caddy external ACME, Core TLS static cert; fallback absent. |
| F-036 | TLS/ACME: `caddyserver/certmagic` (standalone mode); <br>Источник: [v4:64](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-deploy](EVIDENCE.md#e-deploy); целевого теста нет; не проверено | Файлы существуют; installation/Docker/Linux multiarch/CI run не проверено. Installer local-only/checksum optional/version 0.2.0, без signature/download/wizard/bootstrap token; Caddy external ACME, Core TLS static cert; fallback absent. |

#### 12. Security and privacy

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-184 | Least-privilege containers (non-root, read-only FS, dropped caps); **no Docker socket for Core or Agents**; dependency pinning, vulnerability scan, SBOM, signed releases.<br>Источник: [v4:315](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-deploy](EVIDENCE.md#e-deploy); целевого теста нет; не проверено | Файлы существуют; installation/Docker/Linux multiarch/CI run не проверено. Installer local-only/checksum optional/version 0.2.0, без signature/download/wizard/bootstrap token; Caddy external ACME, Core TLS static cert; fallback absent. |

#### 15. Deployment (Linux)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-195 | `install.sh` (version-pinned, checksum + signature verified, idempotent): installs Docker or the static binary + systemd unit, generates secrets, prints the URL and a one-time bootstrap token; the **browser wizard** does the rest. Multi-arch images (amd64/arm64).<br>Источник: [v4:335](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-deploy](EVIDENCE.md#e-deploy); целевого теста нет; не проверено | Файлы существуют; installation/Docker/Linux multiarch/CI run не проверено. Installer local-only/checksum optional/version 0.2.0, без signature/download/wizard/bootstrap token; Caddy external ACME, Core TLS static cert; fallback absent. |
| F-196 | Reverse-proxy examples (nginx/Caddy) **with fail-open fallback**; standalone mode with built-in ACME; all ports configurable.<br>Источник: [v4:336](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-deploy](EVIDENCE.md#e-deploy); целевого теста нет; не проверено | Файлы существуют; installation/Docker/Linux multiarch/CI run не проверено. Installer local-only/checksum optional/version 0.2.0, без signature/download/wizard/bootstrap token; Caddy external ACME, Core TLS static cert; fallback absent. |
| F-197 | Docs: quick start, upgrade/rollback, backup/restore, runbooks per alert, troubleshooting, panel-version compatibility matrix, data-source licences.<br>Источник: [v4:337](../../subgate-ai-prompt-v4.md) | **PARTIAL** | [E-deploy](EVIDENCE.md#e-deploy); целевого теста нет; не проверено | Файлы существуют; installation/Docker/Linux multiarch/CI run не проверено. Installer local-only/checksum optional/version 0.2.0, без signature/download/wizard/bootstrap token; Caddy external ACME, Core TLS static cert; fallback absent. |

#### Part A. Per-project functions / 3. DigneZzZ/remnawave-scripts (Bash; MIT)

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-238 | Panel/node/Selfsteal/WARP-Tor/NetBird installers; SHORT_UUID method picker; Caddy with MFA portal; standalone subscription-page; safe update with snapshot and migrations; PG upgrade helper; Telegram backups with retention; node: Xray-core update incl. pre-release, live Xray logs, log rotation, auto-restart, mass provisioning, version pinning; Selfsteal (11 masking templates, anti-fingerprint); WTM (WARP as WireGuard outbound, Tor); jsDelivr mirror fallback; self-updating scripts. Warning found: node ≥ 2.8 ignores the legacy `XTLS_API_PORT`.<br>Источник: [inventory:43](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Файлы существуют; installation/Docker/Linux multiarch/CI run не проверено. Installer local-only/checksum optional/version 0.2.0, без signature/download/wizard/bootstrap token; Caddy external ACME, Core TLS static cert; fallback absent. |

#### Part A. Per-project functions / 6. jestivald/node-accelerator (Bash; MIT) — the best node-side spec

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-245 | Signed releases (minisign) and pinned tags.<br>Источник: [inventory:56](../../subgate-feature-inventory.md) | **OUT_OF_SCOPE** | Q3: Part A — справочник, не scope v1; [ответы владельца](QUESTIONS.md#ответы-владельца-2026-10-13). Исходная формулировка сохранена; исключение не является DONE. | Inventory Part A: референс чужого проекта, не автоматически обещанная функция SubGate. Сравнение с кодом сохранено для полноты; выбор v1 уточняется владельцем. Не реализовано; runtime не проверено. |

#### Part B. The merged catalogue (union) → SubGate

| ID | Функция / исходное требование | Статус | Доказательства | Чего не хватает |
|---|---|---|---|---|
| F-284 | Installers/lifecycle for panel &amp; node (one-liner, idempotent, mirror fallback, pinned versions, signed releases) — F15/installer + F26 **NEW**<br>Источник: [inventory:114](../../subgate-feature-inventory.md) | **PARTIAL** | [E-deploy](EVIDENCE.md#e-deploy); целевого теста нет; не проверено | Файлы существуют; installation/Docker/Linux multiarch/CI run не проверено. Installer local-only/checksum optional/version 0.2.0, без signature/download/wizard/bootstrap token; Caddy external ACME, Core TLS static cert; fallback absent. |
