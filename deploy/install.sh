#!/usr/bin/env bash
# Install trusted, locally built Linux binaries. Never downloads or enrolls agents.
set -euo pipefail
umask 077
bin_dir=./bin
want_core=0
want_agent=0
start_services=0
checksums=
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
usage() {
  cat <<'EOF'
Usage: bash deploy/install.sh --bin-dir DIR [--core] [--agent] [--checksums FILE] [--start]
Choose --core and/or --agent. Installs binaries, service identities, env files and units.
Existing env files are preserved and backed up. Services start only with --start.
An Agent must be manually enrolled before --start. No network downloads are performed.
EOF
}
fail() { printf 'subgate installer: %s\n' "$*" >&2; exit 1; }
while (($#)); do
  case "$1" in
    --bin-dir) (($# >= 2)) || fail '--bin-dir requires DIR'; bin_dir=$2; shift 2 ;;
    --core) want_core=1; shift ;;
    --agent) want_agent=1; shift ;;
    --checksums) (($# >= 2)) || fail '--checksums requires FILE'; checksums=$2; shift 2 ;;
    --start) start_services=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown option: $1" ;;
  esac
 done
((want_core || want_agent)) || fail 'choose --core and/or --agent'
[[ $(uname -s) == Linux ]] || fail 'native Linux with systemd is required'
[[ $(id -u) == 0 ]] || fail 'run as root'
for command_name in install getent useradd groupadd systemctl od tr awk sha256sum mktemp; do
  command -v "$command_name" >/dev/null || fail "required command is unavailable: $command_name"
done
[[ -d "$bin_dir" ]] || fail "binary directory not found: $bin_dir"
bin_dir=$(cd -- "$bin_dir" && pwd)
if [[ -n "$checksums" ]]; then
  [[ -f "$checksums" && -r "$checksums" ]] || fail "checksum manifest unreadable: $checksums"
  checksums=$(cd -- "$(dirname -- "$checksums")" && pwd)/$(basename -- "$checksums")
fi
verify_binary() {
  local component=$1 binary="$bin_dir/subgate-$1" expected actual
  [[ -f "$binary" && -x "$binary" ]] || fail "missing executable: $binary"
  if [[ -n "$checksums" ]]; then
    expected=$(awk -v name="subgate-$component" '{file=$2; sub(/^\*/,"",file); n=split(file,parts,"/"); if(parts[n]==name) print $1}' "$checksums")
    [[ "$expected" =~ ^[a-fA-F0-9]{64}$ ]] || fail "manifest must contain one SHA-256 for subgate-$component"
    actual=$(sha256sum -- "$binary"); actual=${actual%% *}
    [[ ${actual,,} == ${expected,,} ]] || fail "SHA-256 mismatch: subgate-$component"
  fi
  [[ $("$binary" -version) == 0.2.0 ]] || fail "expected a runnable SubGate 0.2.0 Linux binary: $binary"
}
# Verify every selected input before installing anything.
((want_core == 0)) || verify_binary core
((want_agent == 0)) || verify_binary agent
ensure_identity() {
  local service_user=$1
  getent group "$service_user" >/dev/null || groupadd --system "$service_user"
  getent passwd "$service_user" >/dev/null || useradd --system --gid "$service_user" --home-dir "/var/lib/$service_user" --shell /usr/sbin/nologin "$service_user"
  install -d -o "$service_user" -g "$service_user" -m 0700 "/var/lib/$service_user"
}
backup_config() {
  local file=$1 copy
  if [[ -e "$file" ]]; then
    [[ -f "$file" && ! -L "$file" ]] || fail "configuration must be a regular file: $file"
    copy=$(mktemp "$file.bak.$(date -u +%Y%m%dT%H%M%SZ).XXXXXX")
    cp -- "$file" "$copy"; chmod 0600 "$copy"
    printf 'Preserved config; private backup: %s\n' "$copy"
  fi
}
write_setting() {
  local name=$1 value=$2
  [[ "$value" != *$'\n'* && "$value" != *$'\r'* ]] || fail "multiline value rejected: $name"
  value=${value//\\/\\\\}; value=${value//\"/\\\"}
  printf '%s="%s"\n' "$name" "$value"
}
install -d -m 0755 /usr/local/bin /etc/systemd/system
install -d -m 0700 /etc/subgate
if ((want_core)); then
  ensure_identity subgate
  backup_config /etc/subgate/core.env
  if [[ ! -e /etc/subgate/core.env ]]; then
    password=$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')
    [[ "$password" =~ ^[a-f0-9]{48}$ ]] || fail 'secure password generation failed'
    temporary=$(mktemp /etc/subgate/core.env.new.XXXXXX)
    {
      write_setting SUBGATE_LISTEN 127.0.0.1:8080
      write_setting SUBGATE_DATABASE_URL file:/var/lib/subgate/subgate.db
      write_setting SUBGATE_BACKUP_PATH /var/lib/subgate/backups
      write_setting SUBGATE_PANEL_URL "${SUBGATE_PANEL_URL:-https://panel.example.com}"
      write_setting SUBGATE_PANEL_TOKEN "${SUBGATE_PANEL_TOKEN:-replace-with-a-scoped-remnawave-api-token}"
      write_setting SUBGATE_PUBLIC_URL "${SUBGATE_PUBLIC_URL:-}"
      write_setting SUBGATE_TRUSTED_PROXIES "${SUBGATE_TRUSTED_PROXIES:-}"
      write_setting SUBGATE_ADMIN_USERNAME "${SUBGATE_ADMIN_USERNAME:-admin}"
      write_setting SUBGATE_ADMIN_PASSWORD "$password"
    } > "$temporary"
    mv -- "$temporary" /etc/subgate/core.env
    printf 'Created /etc/subgate/core.env with a random bootstrap password; review credentials before starting.\n'
  fi
  install -m 0755 "$bin_dir/subgate-core" /usr/local/bin/subgate-core
  install -m 0644 "$script_dir/subgate-core.service" /etc/systemd/system/subgate-core.service
fi
if ((want_agent)); then
  ensure_identity subgate-agent
  backup_config /etc/subgate/agent.env
  if [[ ! -e /etc/subgate/agent.env ]]; then
    temporary=$(mktemp /etc/subgate/agent.env.new.XXXXXX)
    {
      write_setting SUBGATE_CORE_URL "${SUBGATE_CORE_URL:-https://subgate.example.com}"
      write_setting NODE_ID "${NODE_ID:-replace-with-remnawave-node-uuid}"
      write_setting XRAY_STATS_ADDR "${XRAY_STATS_ADDR:-auto}"
      write_setting XRAY_ACCESS_LOG "${XRAY_ACCESS_LOG:-/var/log/remnanode/access.log}"
    } > "$temporary"
    mv -- "$temporary" /etc/subgate/agent.env
    printf 'Created /etc/subgate/agent.env; review settings and manually enroll the Agent.\n'
  fi
  install -m 0755 "$bin_dir/subgate-agent" /usr/local/bin/subgate-agent
  install -m 0644 "$script_dir/subgate-agent.service" /etc/systemd/system/subgate-agent.service
fi
systemctl daemon-reload
if ((start_services)); then
  # Validate placeholders/token before starting either selected component.
  if ((want_core)); then
    if awk '/^(SUBGATE_PANEL_URL|SUBGATE_PANEL_TOKEN)=/ && /panel\.example\.com|replace-with/ {bad=1} END {exit !bad}' /etc/subgate/core.env; then
      fail 'review the placeholder panel credentials in /etc/subgate/core.env before --start'
    fi
  fi
  if ((want_agent)); then
    [[ -s /var/lib/subgate-agent/token ]] || fail 'manually enroll into /var/lib/subgate-agent/token before --start'
    if awk '/^(SUBGATE_CORE_URL|NODE_ID)=/ && /subgate\.example\.com|replace-with/ {bad=1} END {exit !bad}' /etc/subgate/agent.env; then
      fail 'review the placeholder Agent settings before --start'
    fi
  fi
  ((want_core == 0)) || systemctl enable --now subgate-core
  ((want_agent == 0)) || systemctl enable --now subgate-agent
  printf 'Selected services enabled and started.\n'
else
  printf 'Installed selected components. Review /etc/subgate/*.env; services start only with --start or explicit systemctl commands.\n'
fi
