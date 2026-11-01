#!/usr/bin/env python3
"""Exercise production container users, mounts, preflight, enrollment and readiness.

Requires a Linux Docker Engine and locally built subgate-{core,agent}:0.2.0.
Uses isolated projects and deletes only their disposable volumes on exit.
"""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import uuid

ROOT = Path(__file__).resolve().parents[1]
project = "subgate-smoke-" + uuid.uuid4().hex[:10]


def run(*args, expected=0, **kwargs):
    result = subprocess.run(args, text=True, capture_output=True, **kwargs)
    if result.returncode != expected:
        raise AssertionError(f"{' '.join(args[:5])}: exit {result.returncode}\n{result.stdout}\n{result.stderr}")
    return result.stdout.strip()


with tempfile.TemporaryDirectory(prefix="subgate-containers-") as temporary:
    work = Path(temporary)
    # The fixture serves no real users and never resets Xray counters.
    run("docker", "build", "-f", str(ROOT / "test/deploy/Dockerfile"),
        "-t", "subgate-deploy-fixture:smoke", str(ROOT))
    logs = work / "logs"
    logs.mkdir(mode=0o755)
    (logs / "access.log").touch(mode=0o644)
    env = work / ".env"
    env.write_text(
        "SUBGATE_CORE_URL=http://127.0.0.1:18080\nNODE_ID=demo-node-1\n"
        "SUBGATE_AGENT_IMAGE=subgate-agent:0.2.0\nXRAY_STATS_ADDR=127.0.0.1:18085\n"
        f"XRAY_LOG_DIR='{logs.as_posix()}'\nXRAY_LOG_GID=0\n", encoding="utf-8")
    override = work / "compose.yaml"
    override.write_text("""
services:
  core:
    network_mode: host
    ports: !reset []
    command: ["-demo", "-listen", "127.0.0.1:18080", "-database", "file:/data/demo.db"]
    environment:
      SUBGATE_PANEL_URL: http://127.0.0.1:18081
      SUBGATE_PANEL_TOKEN: fixture-only-token
      SUBGATE_PUBLIC_URL: https://monitor.invalid
      SUBGATE_ADMIN_PASSWORD: fixture-only-password
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:18080/health"]
      interval: 2s
      start_period: 2s
  fixture:
    image: subgate-deploy-fixture:smoke
    network_mode: host
""", encoding="utf-8")
    compose = ["docker", "compose", "-p", project, "--env-file", str(env),
               "-f", str(ROOT / "deploy/compose.demo.yaml"),
               "-f", str(ROOT / "deploy/compose.agent.yaml"), "-f", str(override)]

    def dc(*args, **kwargs):
        return run(*compose, *args, **kwargs)

    try:
        dc("config", "--quiet")
        dc("up", "-d", "--wait", "--wait-timeout", "30", "fixture")
        # A real read-only panel preflight succeeds without initializing SQLite.
        dc("run", "--rm", "--no-deps", "core", "-check")
        dc("run", "--rm", "--no-deps", "--entrypoint", "/bin/sh", "core",
           "-c", "test ! -e /data/subgate.db && test ! -e /data/subgate.key")
        dc("up", "-d", "--wait", "--wait-timeout", "90", "core")
        dc("run", "--rm", "--no-deps", "agent", "check", "-token-file", "/data/token",
           "-access-log", "/xray-log/access.log", "-stats-addr", "127.0.0.1:18085")
        # Check negative source diagnostics against the actual container image.
        dc("run", "--rm", "--no-deps", "agent", "check", "-token-file", "/data/token",
           "-access-log", "/xray-log/missing.log", expected=1)
        token = dc("exec", "-T", "fixture", "/fixture", "token")
        dc("run", "--rm", "--no-deps", "-e", "ENROLLMENT_TOKEN", "agent",
           "enroll", "-token-file", "/data/token", env={**os.environ, "ENROLLMENT_TOKEN": token})
        credential = dc("run", "--rm", "--no-deps", "--entrypoint", "/bin/sh", "agent",
                        "-c", "test $(stat -c %a /data/token) = 600 && cat /data/token")
        dc("up", "-d", "--wait", "--wait-timeout", "90", "agent")
        # Wait for a second heartbeat containing the first StatsService sample.
        dc("exec", "-T", "fixture", "/fixture", "verify")
        key = dc("exec", "-T", "core", "sha256sum", "/data/subgate.key")
        dc("up", "-d", "--force-recreate", "--wait", "--wait-timeout", "90", "core", "agent")
        assert dc("exec", "-T", "core", "sha256sum", "/data/subgate.key") == key
        assert dc("exec", "-T", "agent", "cat", "/data/token") == credential
        dc("exec", "-T", "fixture", "/fixture", "verify")
        print("PASS: real containers: panel preflight, log and volume access, enrollment, heartbeat readiness, recreation preserves key and credential")
    finally:
        diagnostics = subprocess.run([*compose, "logs", "--tail=40"], text=True, capture_output=True)
        if sys.exc_info()[0]:
            print(diagnostics.stdout)
        dc("down", "-v", "--remove-orphans")
