#!/usr/bin/env bash
# shellcheck disable=SC2016 # Variables in container commands expand inside the container.
# Run from a checkout; no host packages, proxy sites or firewall rules are changed.
set -Eeuo pipefail
umask 077
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
mode='' check=0 update=0
usage() {
  cat <<'EOF'
Usage: sudo bash deploy/install-core.sh [--proxy caddy|external] [--check|--update]
  First run prompts for domain, panel URL and API token; generates an admin password.
  Noninteractive: set SUBGATE_DOMAIN, SUBGATE_PANEL_URL, SUBGATE_PANEL_TOKEN
  and optionally SUBGATE_ADMIN_USERNAME / SUBGATE_ADMIN_PASSWORD.
  Existing deploy/.env is preserved and never sourced as shell code.
  --proxy caddy     Automatic HTTPS on free ports 80/443 (default on first run).
  --proxy external  Existing host proxy to 127.0.0.1:8080; configure it first.
  --check          Build and check settings + panel access without starting Core.
  --update         Rebuild and restart using saved proxy mode and existing data.
Requires Linux, Docker Engine, Compose v2 with --wait, curl and a Git checkout.
EOF
}
fail() { printf 'subgate-core: %s\n' "$*" >&2; exit 1; }
while (($#)); do
  case "$1" in
    --proxy) (($# >= 2)) || fail '--proxy requires caddy or external'; mode=$2; shift 2 ;;
    --check) check=1; shift ;;
    --update) update=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown option: $1" ;;
  esac
done
((check + update <= 1)) || fail 'choose --check or --update'
[[ $(uname -s) == Linux ]] || fail 'run on the Linux central server'
[[ $(id -u) == 0 ]] || fail 'run with sudo or as root'
for cmd in docker curl od tr mktemp; do
  command -v "$cmd" >/dev/null || fail "required command missing: $cmd"
done
docker info >/dev/null || fail 'Docker Engine is unavailable'
docker compose version >/dev/null || fail 'Docker Compose v2 is required'
[[ $(docker compose up --help) == *--wait-timeout* ]] || fail 'update Docker Compose: up --wait-timeout is required'
[[ -f $script_dir/compose.yaml && -f $script_dir/../go.mod ]] || fail 'run deploy/install-core.sh from a full checkout'
if [[ -z $mode && -f $script_dir/.proxy-mode ]]; then
  mode=$(cat -- "$script_dir/.proxy-mode")
fi
mode=${mode:-caddy}
[[ $mode == caddy || $mode == external ]] || fail '--proxy must be caddy or external'
if ((update)); then
  [[ -f $script_dir/.env ]] || fail 'no deploy/.env; run the installer without --update first'
fi
prompt() {
  local name=$1 label=$2 secret=${3:-0} value=${!1:-}
  if [[ -z $value ]]; then
    [[ -t 0 ]] || fail "set $name for noninteractive installation"
    if ((secret)); then read -r -s -p "$label: " value; printf '\n' >&2
    else read -r -p "$label: " value
    fi
  fi
  [[ -n $value && $value != *$'\n'* && $value != *$'\r'* ]] || fail "invalid $name"
  printf -v "$name" '%s' "$value"
}
setting() {
  local value=$2
  [[ $value != *$'\n'* && $value != *$'\r'* ]] || fail "multiline value rejected: $1"
  # Compose double quotes with escaped dollars also handle a final backslash.
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  value=${value//\$/\$\$}
  printf '%s="%s"\n' "$1" "$value"
}
if [[ ! -e $script_dir/.env ]]; then
  prompt SUBGATE_DOMAIN 'SubGate domain (e.g. monitor.example.org)'
  [[ $SUBGATE_DOMAIN =~ ^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$ && $SUBGATE_DOMAIN == *.* ]] || fail 'enter a DNS name without scheme, port or path'
  prompt SUBGATE_PANEL_URL 'Remnawave URL (e.g. https://panel.example.org)'
  prompt SUBGATE_PANEL_TOKEN 'Remnawave API token' 1
  password=${SUBGATE_ADMIN_PASSWORD:-}
  if [[ -z $password ]]; then
    password=$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')
    [[ $password =~ ^[a-f0-9]{48}$ ]] || fail 'password generation failed'
  fi
  ((${#password} >= 12)) || fail 'admin password must contain at least 12 characters'
  temporary=$(mktemp "$script_dir/.env.new.XXXXXX")
  trap 'rm -f -- "${temporary:-}"' EXIT
  {
    setting SUBGATE_DOMAIN "$SUBGATE_DOMAIN"
    setting SUBGATE_PUBLIC_URL "https://$SUBGATE_DOMAIN"
    setting SUBGATE_PANEL_URL "$SUBGATE_PANEL_URL"
    setting SUBGATE_PANEL_TOKEN "$SUBGATE_PANEL_TOKEN"
    setting SUBGATE_ADMIN_USERNAME "${SUBGATE_ADMIN_USERNAME:-admin}"
    setting SUBGATE_ADMIN_PASSWORD "$password"
    setting SUBGATE_TRUSTED_PROXIES "${SUBGATE_TRUSTED_PROXIES:-}"
  } > "$temporary"
  mv -- "$temporary" "$script_dir/.env"
  printf 'Created private deploy/.env. Admin credentials are stored there; keep a secure copy.\n'
fi
[[ -f $script_dir/.env && ! -L $script_dir/.env ]] || fail 'deploy/.env must be a regular file'
chmod 600 "$script_dir/.env"
compose=(docker compose --env-file "$script_dir/.env" -f "$script_dir/compose.yaml")
if [[ $mode == caddy ]]; then compose+=(--profile tls); fi
trap 'printf "\nCore deployment failed. Configuration and volumes are preserved.\nFix the reported error and rerun the same command.\nDiagnostics: docker compose --env-file %q -f %q logs --tail=100\n" "$script_dir/.env" "$script_dir/compose.yaml" >&2' ERR
"${compose[@]}" config --quiet
printf 'Building Core and checking panel access (GET only)...\n'
"${compose[@]}" build core
if [[ $mode == caddy ]]; then
  "${compose[@]}" run --rm --no-deps --entrypoint /bin/sh core -ec \
    'test "https://$SUBGATE_DOMAIN" = "$SUBGATE_PUBLIC_URL" || { echo "SUBGATE_DOMAIN must match the HTTPS SUBGATE_PUBLIC_URL for Caddy" >&2; exit 1; }; exec /usr/local/bin/subgate-core -check'
else
  "${compose[@]}" run --rm --no-deps core -check
fi
if ((check)); then
  printf 'Preflight passed. No Core service started.\n'
  exit 0
fi
printf '%s\n' "$mode" > "$script_dir/.proxy-mode"
services=(core)
[[ $mode != caddy ]] || services+=(caddy)
"${compose[@]}" up -d --wait --wait-timeout 180 "${services[@]}"
# Read the effective URL inside the service; do not parse or execute dotenv.
public_url=$("${compose[@]}" exec -T core /bin/sh -c 'printf "%s" "$SUBGATE_PUBLIC_URL"')
printf 'Checking public HTTPS endpoint: %s\n' "$public_url"
response=$(curl --fail --silent --show-error --retry 12 --retry-delay 5 --retry-all-errors \
  --connect-timeout 5 --max-time 10 --proto '=https' "$public_url/health") ||
  fail "Core is running but public HTTPS is unavailable. Check DNS A/AAAA, ports 80/443 and the proxy. For an existing proxy see $script_dir/nginx.conf.example. Then rerun this command."
[[ $response == *'"database":true'* && $response == *'"status":"ok"'* ]] ||
  fail 'public /health did not return Core readiness JSON; check the proxy upstream'
"${compose[@]}" ps
printf '\nCore ready: %s\nCredentials: %s/.env\nNext: open Agents in Core and create an enrollment token for each node.\n' "$public_url" "$script_dir"
