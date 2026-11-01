#!/usr/bin/env bash
# Exercise installer lifecycle without contacting Docker or a real Core.
set -euo pipefail
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
mkdir -p "$work/bin" "$work/logs"
touch "$work/logs/access.log"
export INSTALLER_TEST_LOG=$work/docker.log
export INSTALLER_TEST_STATE=$work/token
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
printf '%s\n' "$*" >> "$INSTALLER_TEST_LOG"
if [[ "$*" == 'compose up --help' ]]; then echo '--wait-timeout'; fi
if [[ "$*" == *'agent enroll'* ]]; then
  [[ ${ENROLLMENT_TOKEN:-} == test-one-time-secret ]] || exit 31
  [[ ${FAIL_ENROLLMENT:-0} == 0 ]] || exit 32
  touch "$INSTALLER_TEST_STATE"
fi
if [[ "$*" == *'test -s /data/token'* ]]; then test -f "$INSTALLER_TEST_STATE"; exit; fi
if [[ "$*" == *'agent check'* && ${FAIL_LOG_ACCESS:-0} == 1 ]]; then exit 33; fi
if [[ "$*" == *'up -d --wait'* && ${FAIL_START:-0} == 1 ]]; then exit 34; fi
EOF
cat > "$work/bin/curl" <<'EOF'
#!/usr/bin/env bash
echo 'installer unexpectedly fetched a deployment template' >&2
exit 39
EOF
chmod +x "$work/bin/"*
export PATH=$work/bin:$PATH
installer=$repo/deploy/install-agent.sh
run_install() {
  ENROLLMENT_TOKEN=test-one-time-secret bash "$installer" \
    --core-url https://core.example.com --node-id test-node \
    --log-dir "$work/logs" --install-dir "$1"
}
run_install "$work/installed" > "$work/output"
[[ -f $work/installed/.env && -f $work/installed/install-agent.sh ]]
grep -q 'ghcr.io/mrzalupa-lolz/subgate-agent:latest' "$work/installed/.env"
if grep -q test-one-time-secret "$work/installed/.env"; then echo 'token leaked to env file'; exit 1; fi
grep -q 'agent enroll -token-file /data/token' "$INSTALLER_TEST_LOG"
grep -q 'up -d --wait --wait-timeout 120' "$INSTALLER_TEST_LOG"
rm -- "$INSTALLER_TEST_STATE"
# The command copied from Core downloads only the script, without sibling files.
mkdir "$work/downloaded"
cp -- "$installer" "$work/downloaded/install-agent.sh"
ENROLLMENT_TOKEN=test-one-time-secret bash "$work/downloaded/install-agent.sh" \
  --core-url https://core.example.com --node-id another-node \
  --log-dir "$work/logs" --install-dir "$work/download-install" > "$work/output"
[[ -f $work/download-install/compose.yaml && -f $work/download-install/install-agent.sh ]]
cmp -- "$work/download-install/compose.yaml" "$repo/deploy/compose.agent.yaml"
if run_install "$work/installed" > "$work/output" 2>&1; then echo 'existing config overwritten'; exit 1; fi
: > "$INSTALLER_TEST_LOG"
bash "$work/installed/install-agent.sh" --update --install-dir "$work/installed" \
  --image ghcr.io/mrzalupa-lolz/subgate-agent:0.3.0 > "$work/output"
grep -q 'SUBGATE_AGENT_IMAGE=ghcr.io/mrzalupa-lolz/subgate-agent:0.3.0' "$work/installed/.env"
if grep -q enroll "$INSTALLER_TEST_LOG"; then echo 'update attempted enrollment'; exit 1; fi
# An update without an image override preserves the pinned version.
bash "$installer" --update --install-dir "$work/installed" > "$work/output"
grep -q 'SUBGATE_AGENT_IMAGE=ghcr.io/mrzalupa-lolz/subgate-agent:0.3.0' "$work/installed/.env"
# Explicitly switching back to latest must also work.
bash "$installer" --update --install-dir "$work/installed" \
  --image ghcr.io/mrzalupa-lolz/subgate-agent:latest > "$work/output"
grep -q 'SUBGATE_AGENT_IMAGE=ghcr.io/mrzalupa-lolz/subgate-agent:latest' "$work/installed/.env"
rm -- "$INSTALLER_TEST_STATE"
if FAIL_ENROLLMENT=1 run_install "$work/failed-enroll" > "$work/output" 2>&1; then echo 'failed enrollment ignored'; exit 1; fi
[[ -e $work/failed-enroll/.env ]]
ENROLLMENT_TOKEN=test-one-time-secret bash "$installer" --resume --install-dir "$work/failed-enroll" > "$work/output"
rm -- "$INSTALLER_TEST_STATE"
: > "$INSTALLER_TEST_LOG"
if FAIL_LOG_ACCESS=1 run_install "$work/failed-logs" > "$work/output" 2>&1; then echo 'unreadable logs ignored'; exit 1; fi
if grep -q 'agent enroll' "$INSTALLER_TEST_LOG"; then echo 'token consumed before log access check'; exit 1; fi
[[ ! -e $work/failed-logs/.env ]]
# A failed first start is resumable without spending another one-time token.
if FAIL_START=1 run_install "$work/failed-start" > "$work/output" 2>&1; then echo 'failed start ignored'; exit 1; fi
: > "$INSTALLER_TEST_LOG"
bash "$installer" --resume --install-dir "$work/failed-start" > "$work/output"
if grep -q 'agent enroll' "$INSTALLER_TEST_LOG"; then echo 'resume repeated successful enrollment'; exit 1; fi
if grep -q ' pull' "$INSTALLER_TEST_LOG"; then echo 'resume unnecessarily required registry access'; exit 1; fi
bash "$installer" --check --install-dir "$work/failed-start" > "$work/output"
echo 'Agent installer lifecycle checks passed'
