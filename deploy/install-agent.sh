#!/usr/bin/env bash
# shellcheck disable=SC2016 # Variables in container commands expand inside the container.
# Install the published agent on a Linux Docker node.
set -Eeuo pipefail
umask 077
core='' node='' token=${ENROLLMENT_TOKEN:-} log_dir='' log_gid='' stats=auto
install_dir=/opt/subgate-agent
image=ghcr.io/mrzalupa-lolz/subgate-agent:latest
update=0 resume=0 check=0 image_set=0
write_compose() {
  cat <<'SUBGATE_AGENT_COMPOSE'
# Linux only: shares the node's local StatsService network namespace.
name: subgate-agent
services:
  agent:
    image: "${SUBGATE_AGENT_IMAGE:-ghcr.io/mrzalupa-lolz/subgate-agent:latest}"
    network_mode: host
    environment:
      SUBGATE_CORE_URL: "${SUBGATE_CORE_URL:?Set a Core HTTPS URL}"
      NODE_ID: "${NODE_ID:?Set the panel node UUID}"
      XRAY_STATS_ADDR: "${XRAY_STATS_ADDR:-auto}"
    command:
      - -token-file
      - /data/token
      - -spool-file
      - /data/pending.sgb
      - -stats-addr
      - "${XRAY_STATS_ADDR:-auto}"
      - -access-log
      - /xray-log/access.log
      - -health-file
      - /tmp/subgate-heartbeat
    healthcheck:
      test: ["CMD", "/usr/local/bin/subgate-agent", "health", "-health-file", "/tmp/subgate-heartbeat"]
      interval: 10s
      timeout: 5s
      start_period: 10s
      retries: 9
    volumes:
      - agent-data:/data
      - "${XRAY_LOG_DIR:-/var/log/remnanode}:/xray-log:ro"
    # Grant only the existing log-reader group's GID (not the docker group).
    group_add:
      - "${XRAY_LOG_GID:?Set the numeric log-reader group GID}"
    read_only: true
    tmpfs:
      - /tmp:rw,noexec,nosuid,size=16m
    cap_drop: [ALL]
    security_opt: [no-new-privileges:true]
    pids_limit: 128
    restart: unless-stopped
    stop_grace_period: 20s
volumes:
  agent-data:
SUBGATE_AGENT_COMPOSE
}
usage() {
  cat <<'EOF'
Usage: install-agent.sh --core-url URL --node-id UUID [options]
       install-agent.sh --update [--image IMAGE] [--install-dir DIR]
       install-agent.sh --resume [--install-dir DIR]
       install-agent.sh --check [--install-dir DIR]
Options:
  --enrollment-token TOKEN  One-time token (otherwise prompt, or ENROLLMENT_TOKEN)
  --log-dir DIR            Host directory containing access.log (auto-detected)
  --log-gid GID            Log-reader group (defaults to access.log's group)
  --stats-addr ADDRESS     Local Xray StatsService (default: auto)
  --image IMAGE            Published image (default: GHCR latest)
  --install-dir DIR        Installation directory (default: /opt/subgate-agent)
  --update                 Pull and restart an installed agent; no enrollment
  --resume                 Continue a saved installation; reuse the durable token
  --check                  Check saved sources and Core; no enrollment or restart
Requires Linux, root, Docker Engine and Docker Compose v2 with --wait-timeout.
EOF
}
fail() { printf 'subgate-agent: %s\n' "$*" >&2; exit 1; }
while (($#)); do
  case "$1" in
    --update) update=1; shift ;;
    --resume) resume=1; shift ;;
    --check) check=1; shift ;;
    -h|--help) usage; exit 0 ;;
    --core-url|--node-id|--enrollment-token|--log-dir|--log-gid|--stats-addr|--image|--install-dir)
      (($# >= 2)) || fail "$1 requires a value"
      case "$1" in
        --core-url) core=$2 ;; --node-id) node=$2 ;; --enrollment-token) token=$2 ;;
        --log-dir) log_dir=$2 ;; --log-gid) log_gid=$2 ;; --stats-addr) stats=$2 ;;
        --image) image=$2; image_set=1 ;; --install-dir) install_dir=$2 ;;
      esac
      shift 2 ;;
    *) fail "unknown option: $1" ;;
  esac
done
[[ $(uname -s) == Linux ]] || fail 'run on the Linux VPN node'
[[ $(id -u) == 0 ]] || fail 'run with sudo or as root'
command -v docker >/dev/null || fail 'install Docker Engine on this node first: https://docs.docker.com/engine/install/'
docker info >/dev/null || fail 'Docker Engine is unavailable'
docker compose version >/dev/null || fail 'Docker Compose v2 is required'
[[ $(docker compose up --help) == *--wait-timeout* ]] || fail 'update Docker Compose: up --wait-timeout is required'
[[ $install_dir == /* && $install_dir != / ]] || fail '--install-dir must be an absolute directory other than /'
[[ $image =~ ^[a-zA-Z0-9][a-zA-Z0-9./:@_-]*$ ]] || fail 'invalid image reference'
((update + resume + check <= 1)) || fail 'choose one of --update, --resume or --check'
compose() { docker compose --project-name subgate-agent --env-file "$PWD/.env" -f "$PWD/compose.yaml" "$@"; }
preflight() {
  compose run --rm --no-deps agent check -token-file /data/token \
    -stats-addr "$1" -access-log /xray-log/access.log
}
# Resolve saved StatsService from Compose inside the container, avoiding dotenv
# evaluation and keeping exactly the same arguments as the running service.
saved_preflight() {
  compose run --rm --no-deps --entrypoint /bin/sh agent -c \
    'exec /usr/local/bin/subgate-agent check -token-file /data/token -access-log /xray-log/access.log -stats-addr "$XRAY_STATS_ADDR"'
}
request_token() {
  if [[ -z $token ]]; then
    [[ -r /dev/tty ]] || fail 'set ENROLLMENT_TOKEN or pass --enrollment-token'
    read -r -s -p 'Enrollment token: ' token </dev/tty || fail 'set ENROLLMENT_TOKEN for noninteractive enrollment'
    printf '\n'
  fi
  [[ -n $token ]] || fail 'enrollment token is required'
}
enroll_agent() {
  request_token
  ENROLLMENT_TOKEN=$token compose run --rm --no-deps -e ENROLLMENT_TOKEN agent enroll -token-file /data/token
  unset token ENROLLMENT_TOKEN
}
trap 'printf "\nAgent deployment failed. Saved configuration and volume are preserved.\nIf settings were saved, continue with: sudo bash %q --resume --install-dir %q\nFor an expired/consumed token, create a new one in Core and pass --enrollment-token.\n" "$install_dir/install-agent.sh" "$install_dir" >&2' ERR
if ((update || resume || check)); then
  [[ -f $install_dir/compose.yaml && -f $install_dir/.env ]] || fail 'no existing installation in --install-dir'
  cd -- "$install_dir"
  compose config --quiet
  if ((check)); then saved_preflight; printf 'Agent preflight passed.\n'; exit 0; fi
  # Use the saved image unless --image was explicitly supplied.
  if ((image_set)); then
    export SUBGATE_AGENT_IMAGE=$image
  fi
  if ((update || image_set)); then compose pull; fi
  saved_preflight
  if ((image_set)); then
    sed -i '/^SUBGATE_AGENT_IMAGE=/d' .env
    printf 'SUBGATE_AGENT_IMAGE=%s\n' "$image" >> .env
  fi
  if ((resume)); then
    if ! compose run --rm --no-deps --entrypoint /bin/sh agent -c 'test -s /data/token'; then
      enroll_agent
    fi
  elif ! compose run --rm --no-deps --entrypoint /bin/sh agent -c 'test -s /data/token'; then
    fail 'durable token is missing; use --resume to complete enrollment'
  fi
  compose up -d --wait --wait-timeout 120
  compose ps
  exit 0
fi
[[ -n $core && -n $node ]] || fail '--core-url and --node-id are required'
[[ $core == https://* || $core == http://127.0.0.1:* || $core == http://localhost:* ]] || fail 'Core must use HTTPS (HTTP allowed on loopback)'
[[ ! -e $install_dir/.env && ! -e $install_dir/compose.yaml ]] || fail 'installation already exists; use --update (configuration and token preserved)'
if [[ -z $log_dir ]]; then
  for candidate in /var/log/remnanode /var/log/xray /opt/remnanode/logs /opt/remnanode; do
    if [[ -f $candidate/access.log ]]; then log_dir=$candidate; break; fi
  done
fi
if [[ -z $log_dir ]]; then
  # Remnanode may keep logs in a custom bind mount; discover its host path.
  while IFS= read -r container; do
    [[ -n $container ]] || continue
    while IFS='|' read -r destination source; do
      case "$destination" in
        /var/log/remnanode|/var/log/xray|/usr/local/xray/logs)
          if [[ -f $source/access.log ]]; then log_dir=$source; break; fi ;;
        */access.log)
          if [[ -f $source && ${source##*/} == access.log ]]; then log_dir=$(dirname -- "$source"); break; fi ;;
      esac
    done < <(docker inspect --format '{{range .Mounts}}{{println .Destination "|" .Source}}{{end}}' "$container" | sed 's/ *| */|/')
    [[ -z $log_dir ]] || break
  done < <(docker ps --format '{{.Names}}' --filter name=remnanode)
fi
[[ $log_dir == /* && -f $log_dir/access.log ]] || fail 'access.log not found; pass --log-dir /actual/log/directory'
log_gid=${log_gid:-$(stat -c '%g' "$log_dir/access.log")}
[[ $log_gid =~ ^[0-9]+$ ]] || fail '--log-gid must be numeric'
for value in "$core" "$node" "$image" "$log_dir" "$stats"; do
  [[ $value != *\'* && $value != *$'\n'* && $value != *$'\r'* ]] || fail 'configuration values cannot contain quotes or newlines'
done
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
script_path=$script_dir/$(basename -- "${BASH_SOURCE[0]}")
# The Compose template is embedded below: downloading one installer is enough,
# and a later change on main cannot mix two different deployment definitions.
write_compose > "$work/compose.yaml"
{
  printf "SUBGATE_CORE_URL='%s'\nNODE_ID='%s'\n" "$core" "$node"
  printf "SUBGATE_AGENT_IMAGE='%s'\nXRAY_LOG_DIR='%s'\n" "$image" "$log_dir"
  printf "XRAY_LOG_GID='%s'\nXRAY_STATS_ADDR='%s'\n" "$log_gid" "$stats"
} > "$work/.env"
cd -- "$work"
compose config --quiet
compose pull
preflight "$stats"
# Refuse to overwrite a credential left by an earlier installation.
if compose run --rm --no-deps --entrypoint /bin/sh agent -c 'test -s /data/token'; then
  fail 'agent-data already contains a token; restore the original installation config and use --resume (do not delete the volume)'
fi
mkdir -p -- "$install_dir"
chmod 700 "$install_dir"
cp -- compose.yaml .env "$install_dir/"
cp -- "$script_path" "$install_dir/install-agent.sh"
cd -- "$install_dir"
enroll_agent
compose up -d --wait --wait-timeout 120
compose ps
printf '\nInstalled in %s\nLogs: cd %s && docker compose logs --tail=100 -f agent\n' "$install_dir" "$install_dir"
