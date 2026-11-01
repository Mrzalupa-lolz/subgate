#!/usr/bin/env bash
# shellcheck disable=SC2016 # Test secrets intentionally contain literal dollar signs.
# Exercise failure ordering and idempotency without touching host services.
set -euo pipefail
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
mkdir -p "$work/bin" "$work/repo/deploy"
cp -- "$repo/deploy/install-core.sh" "$repo/deploy/compose.yaml" "$work/repo/deploy/"
touch "$work/repo/go.mod"
export CORE_TEST_LOG=$work/commands
cat > "$work/bin/uname" <<'EOF'
#!/usr/bin/env bash
echo Linux
EOF
cat > "$work/bin/id" <<'EOF'
#!/usr/bin/env bash
echo 0
EOF
cat > "$work/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$CORE_TEST_LOG"
if [[ "$*" == 'compose up --help' ]]; then echo '--wait-timeout'; fi
if [[ "$*" == *'core -check'* && ${FAIL_PANEL:-0} == 1 ]]; then exit 17; fi
if [[ "$*" == *'exec -T core'* ]]; then printf 'https://monitor.example.org'; fi
EOF
cat > "$work/bin/curl" <<'EOF'
#!/usr/bin/env bash
[[ ${FAIL_PUBLIC:-0} == 0 ]] || exit 18
if [[ ${WRONG_PUBLIC:-0} == 1 ]]; then echo '<html>another application</html>'
else echo '{"database":true,"status":"ok"}'
fi
EOF
chmod +x "$work/bin/"*
export PATH=$work/bin:$PATH
export SUBGATE_DOMAIN=monitor.example.org SUBGATE_PANEL_URL=https://panel.example.org
export SUBGATE_PANEL_TOKEN='test-$token# with spaces' SUBGATE_ADMIN_PASSWORD='safe-$password-123'
installer=$work/repo/deploy/install-core.sh
# Failed API validation must never start services.
if FAIL_PANEL=1 bash "$installer" > "$work/output" 2>&1; then echo 'panel failure ignored'; exit 1; fi
if grep -q 'up -d' "$CORE_TEST_LOG"; then echo 'started despite failed panel preflight'; exit 1; fi
grep -Fq 'test-$$token# with spaces' "$work/repo/deploy/.env"
cp -- "$work/repo/deploy/.env" "$work/original.env"
: > "$CORE_TEST_LOG"
bash "$installer" --proxy external --check > "$work/output"
if grep -q 'up -d' "$CORE_TEST_LOG"; then echo '--check started services'; exit 1; fi
bash "$installer" --proxy external > "$work/output"
grep -q 'up -d --wait --wait-timeout 180 core$' "$CORE_TEST_LOG"
[[ $(cat "$work/repo/deploy/.proxy-mode") == external ]]
: > "$CORE_TEST_LOG"
bash "$installer" --update > "$work/output"
if grep -q -- '--profile tls' "$CORE_TEST_LOG"; then echo 'update forgot external proxy mode'; exit 1; fi
cmp -- "$work/original.env" "$work/repo/deploy/.env"
if FAIL_PUBLIC=1 bash "$installer" --update > "$work/output" 2>&1; then echo 'public HTTPS failure ignored'; exit 1; fi
if WRONG_PUBLIC=1 bash "$installer" --update > "$work/output" 2>&1; then echo 'wrong proxy upstream accepted'; exit 1; fi
bash "$installer" --proxy caddy > "$work/output"
grep -q 'up -d --wait --wait-timeout 180 core caddy' "$CORE_TEST_LOG"
if grep -Fq "$SUBGATE_PANEL_TOKEN" "$work/output"; then echo 'token leaked to output'; exit 1; fi
echo 'Core installer lifecycle checks passed'
