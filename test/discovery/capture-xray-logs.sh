#!/bin/bash
# capture-xray-logs.sh – Capture golden Xray log corpus
# Usage: ./capture-xray-logs.sh <node-ip> <duration_seconds> <output_dir>

set -e

NODE_IP="${1:?Node IP required}"
DURATION="${2:?Duration (seconds) required}"
OUTPUT_DIR="${3:?Output directory required}"

mkdir -p "$OUTPUT_DIR"

echo "=== Capturing Xray Logs ==="
echo "Node: $NODE_IP"
echo "Duration: $DURATION seconds"
echo "Output: $OUTPUT_DIR"
echo

# Capture Xray version
echo "1. Capturing Xray version..."
ssh "root@$NODE_IP" 'xray version' > "$OUTPUT_DIR/xray-version.txt" 2>&1 || echo "Version capture failed" >> "$OUTPUT_DIR/xray-version.txt"

# Capture access log (last N lines)
echo "2. Capturing access log..."
ssh "root@$NODE_IP" '
  if [ -f /var/log/xray/access.log ]; then
    tail -c 1000000 /var/log/xray/access.log  # Last 1MB
  elif [ -f /root/.xray/access.log ]; then
    tail -c 1000000 /root/.xray/access.log
  else
    echo "Access log not found" >&2
  fi
' > "$OUTPUT_DIR/access.log" 2>&1

# Capture error log at different levels
echo "3. Capturing error log..."
ssh "root@$NODE_IP" '
  if [ -f /var/log/xray/error.log ]; then
    tail -c 1000000 /var/log/xray/error.log
  elif [ -f /root/.xray/error.log ]; then
    tail -c 1000000 /root/.xray/error.log
  else
    echo "Error log not found" >&2
  fi
' > "$OUTPUT_DIR/error.log" 2>&1

# Try journalctl if available
echo "4. Attempting journalctl capture..."
ssh "root@$NODE_IP" '
  journalctl -u xray -n 5000 --no-pager 2>/dev/null || echo "journalctl not available or xray not in systemd"
' > "$OUTPUT_DIR/journalctl.log" 2>&1 || true

# Analyze logs
echo "5. Analyzing captured logs..."

cat > "$OUTPUT_DIR/corpus-summary.json" <<EOF
{
  "capture_date": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "node": "$NODE_IP",
  "duration_seconds": $DURATION,
  "files": {
    "access_log": {
      "lines": $(wc -l < "$OUTPUT_DIR/access.log" || echo 0),
      "size_bytes": $(stat -f%z "$OUTPUT_DIR/access.log" 2>/dev/null || stat -c%s "$OUTPUT_DIR/access.log" 2>/dev/null || echo 0)
    },
    "error_log": {
      "lines": $(wc -l < "$OUTPUT_DIR/error.log" || echo 0),
      "size_bytes": $(stat -f%z "$OUTPUT_DIR/error.log" 2>/dev/null || stat -c%s "$OUTPUT_DIR/error.log" 2>/dev/null || echo 0)
    }
  },
  "notes": "This is a sample of real Xray logs. See docs/xray-logs.md for format analysis."
}
EOF

# Sample unique lines to detect protocols and patterns
echo "6. Detecting protocols and patterns..."
echo "   Access log sample (first 10 lines):"
head -10 "$OUTPUT_DIR/access.log" | sed 's/^/     /' || true

echo
echo "=== Capture Complete ==="
echo "Output files:"
ls -lh "$OUTPUT_DIR"/ | tail -n +2 | sed 's/^/  /'
echo
echo "Next: analyze logs and populate docs/xray-logs.md"
