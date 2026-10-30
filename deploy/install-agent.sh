#!/usr/bin/env bash
# Install the published agent on a Linux Docker node.
set -euo pipefail
umask 077
core= node= token=${ENROLLMENT_TOKEN:-} log_dir= log_gid= stats=auto
install_dir=/opt/subgate-agent
image=ghcr.io/mrzalupa-lolz/subgate-agent:latest
update=0 image_set=0
usage() {
  cat <<'EOF'
Usage: install-agent.sh --core-url URL --node-id UUID [options]
       install-agent.sh --update [--image IMAGE] [--install-dir DIR]
Options:
  --enrollment-token TOKEN  One-time token (otherwise prompt, or ENROLLMENT_TOKEN)
  --log-dir DIR            Host directory containing access.log (auto-detected)
  --log-gid GID            Log-reader group (defaults to access.log's group)
  --stats-addr ADDRESS     Local Xray StatsService (default: auto)
  --image IMAGE            Published image (default: GHCR latest)
  --install-dir DIR        Installation directory (default: /opt/subgate-agent)
  --update                 Pull and restart an installed agent; no enrollment
Requires Linux, root, curl, Docker Engine and Docker Compose v2.
EOF
}
fail() { printf 'subgate-agent: %s\n' "$*" >&2; exit 1; }
while (($#)); do
  case "$1" in
    --update) update=1; shift ;;
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
[[ $install_dir == /* && $install_dir != / ]] || fail '--install-dir must be an absolute directory other than /'
[[ $image =~ ^[a-zA-Z0-9][a-zA-Z0-9./:@_-]*$ ]] || fail 'invalid image reference'
if ((update)); then
  [[ -f $install_dir/compose.yaml && -f $install_dir/.env ]] || fail 'no existing installation in --install-dir'
  cd -- "$install_dir"
  # Use the saved image unless --image was explicitly supplied.
  if ((image_set)); then
    export SUBGATE_AGENT_IMAGE=$image
    docker compose pull
    # Persist the selected version for subsequent restarts and updates.
    sed -i '/^SUBGATE_AGENT_IMAGE=/d' .env
    printf 'SUBGATE_AGENT_IMAGE=%s\n' "$image" >> .env
  else
    docker compose pull
  fi
  docker compose up -d --wait
  docker compose ps
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
if [[ -z $token ]]; then
  read -r -s -p 'Enrollment token: ' token </dev/tty
  printf '\n'
fi
[[ -n $token ]] || fail 'enrollment token is required'
command -v curl >/dev/null || fail 'curl is required'
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
script_path=$script_dir/$(basename -- "${BASH_SOURCE[0]}")
if [[ -f $script_dir/compose.agent.yaml ]]; then
  cp -- "$script_dir/compose.agent.yaml" "$work/compose.yaml"
else
  curl -fsSL --retry 3 https://raw.githubusercontent.com/Mrzalupa-lolz/subgate/main/deploy/compose.agent.yaml -o "$work/compose.yaml"
fi
{
  printf "SUBGATE_CORE_URL='%s'\nNODE_ID='%s'\n" "$core" "$node"
  printf "SUBGATE_AGENT_IMAGE='%s'\nXRAY_LOG_DIR='%s'\n" "$image" "$log_dir"
  printf "XRAY_LOG_GID='%s'\nXRAY_STATS_ADDR='%s'\n" "$log_gid" "$stats"
} > "$work/.env"
cd -- "$work"
docker compose config --quiet
docker compose pull
# Check access as the actual container user before consuming the one-time token.
docker compose run --rm --no-deps --entrypoint /bin/sh agent -c 'test -r /xray-log/access.log' ||
  fail 'container cannot read access.log; grant read/traverse permission to its log-reader group or pass --log-gid'
export ENROLLMENT_TOKEN=$token
docker compose run --rm --no-deps -e ENROLLMENT_TOKEN agent enroll -token-file /data/token
unset ENROLLMENT_TOKEN token
mkdir -p -- "$install_dir"
chmod 700 "$install_dir"
cp -- compose.yaml .env "$install_dir/"
cp -- "$script_path" "$install_dir/install-agent.sh"
cd -- "$install_dir"
docker compose up -d --wait
docker compose ps
printf '\nInstalled in %s\nLogs: cd %s && docker compose logs --tail=100 -f agent\n' "$install_dir" "$install_dir"
