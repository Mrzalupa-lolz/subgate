#!/bin/bash
# check-xray-stats.sh – Verify Xray gRPC StatsService
# Usage: ./check-xray-stats.sh <node-ip> [port]

set -e

NODE_IP="${1:?Node IP required}"
PORT="${2:-10085}"  # Default Xray API port

echo "=== Checking Xray gRPC StatsService ==="
echo "Target: $NODE_IP:$PORT"
echo

# Test basic connectivity
echo "1. Testing TCP connectivity..."
if timeout 5 bash -c "echo >/dev/tcp/$NODE_IP/$PORT" 2>/dev/null; then
    echo "   ✓ Port $PORT is open"
else
    echo "   ✗ Port $PORT is not reachable"
    exit 1
fi
echo

# Try to detect Xray process and version
echo "2. Checking Xray version..."
if ssh "root@$NODE_IP" 'xray version 2>/dev/null || echo "Xray not in PATH"' | head -1; then
    echo "   ✓ Xray detected"
else
    echo "   ⚠ Could not determine version"
fi
echo

# Check for Xray config
echo "3. Checking Xray config..."
ssh "root@$NODE_IP" '
  if [ -f /etc/xray/config.json ]; then
    echo "   ✓ Config at /etc/xray/config.json"
    # Check for stats settings
    if grep -q "StatsService" /etc/xray/config.json 2>/dev/null; then
      echo "   ✓ StatsService is configured"
    else
      echo "   ⚠ StatsService not found in config"
    fi
  elif [ -f /opt/xray/config.json ]; then
    echo "   ✓ Config at /opt/xray/config.json"
  else
    echo "   ⚠ Could not find Xray config"
  fi
'
echo

# Check if gRPC tools are available
echo "4. Attempting gRPC call (requires grpcurl or similar)..."
if command -v grpcurl &>/dev/null; then
    echo "   ✓ grpcurl is available"
    # Attempt a simple service enumeration (may fail without proto descriptor)
    grpcurl -plaintext "$NODE_IP:$PORT" list 2>/dev/null || echo "   ⚠ gRPC service enumeration failed (may need proto descriptor)"
else
    echo "   ⚠ grpcurl not found; install: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest"
fi
echo

# Summary
echo "=== Summary ==="
echo "• Xray API port: $NODE_IP:$PORT"
echo "• Next: capture access logs with 'capture-xray-logs.sh'"
echo "• See: docs/xray-stats.md for expected counter format"
