#!/usr/bin/env python3
"""Isolated executable E2E. Uses loopback and never changes Xray configuration."""

import argparse
import http.cookiejar
import json
import os
from pathlib import Path
import shutil
import signal
import socket
import sqlite3
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def wait_for(check, label, timeout=15):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            value = check()
            if value:
                return value
        except (OSError, urllib.error.URLError):
            pass
        time.sleep(0.1)
    raise AssertionError("Timed out: " + label)


class Validation:
    def __init__(self, args):
        self.args = args
        self.bins = Path(args.bin_dir).resolve()
        parent = Path(args.work_dir).resolve()
        parent.mkdir(parents=True, exist_ok=True)
        self.work = Path(tempfile.mkdtemp(prefix="e2e-", dir=parent))
        os.chmod(self.work, 0o700)
        (self.work / "data").mkdir(mode=0o700)
        self.base = "http://127.0.0.1:" + str(args.port)
        self.jar = http.cookiejar.CookieJar()
        self.http = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar))
        self.processes = []
        self.logs = []
        self.results = []
        self.core = None
        self.agent = None
        self.token_file = self.work / "agent-token"
        self.access = self.work / "access.log"
        self.spool = self.work / "pending.sgb"
        self.access.touch(mode=0o600)
        self.token = ""

    def passed(self, label, **details):
        result = {"check": label, "result": "passed", **details}
        self.results.append(result)
        print(json.dumps(result), flush=True)

    def request(self, method, path, payload=None, expected=200, bearer=None, csrf=True):
        raw = None if payload is None else json.dumps(payload).encode()
        headers = {"Content-Type": "application/json"}
        if csrf:
            headers["X-SubGate-CSRF"] = "1"
        if bearer:
            headers["Authorization"] = "Bearer " + bearer
        request = urllib.request.Request(self.base + path, raw, headers, method=method)
        try:
            response = self.http.open(request, timeout=5)
        except urllib.error.HTTPError as e:
            response = e
        with response:
            data = response.read()
            assert response.code == expected, f"{method} {path}: HTTP {response.code}, expected {expected}"
            if response.headers.get("Content-Type", "").startswith("application/json"):
                return json.loads(data)
            return data

    def launch(self, name, command, cwd=None):
        log = open(self.work / (name + ".log"), "ab", buffering=0)
        self.logs.append(log)
        process = subprocess.Popen(command, cwd=cwd or self.work, stdout=log, stderr=log)
        self.processes.append(process)
        return process

    def stop(self, process):
        if process is not None and process.poll() is None:
            process.send_signal(signal.SIGTERM)
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
                raise AssertionError("Process did not shut down gracefully")

    def start_core(self, restored=False):
        cwd = self.work / "restored" if restored else self.work
        database = cwd / "data" / "demo.db"
        self.core = self.launch("restored-core" if restored else "core", [
            str(self.bins / "subgate-core"), "-demo", "-listen", "127.0.0.1:" + str(self.args.port),
            "-database", "file:" + str(database),
        ], cwd=cwd)
        wait_for(lambda: self.request("GET", "/health")["database"], "Core health")

    def login(self):
        self.jar.clear()
        result = self.request("POST", "/api/v1/auth/login", {"username": "admin", "password": "subgate-demo"})
        assert result["role"] == "superadmin"

    def start_agent(self, stats_address=None):
        self.agent = self.launch("agent", [
            str(self.bins / "subgate-agent"), "-core-addr", self.base, "-node-id", "demo-node-1",
            "-token-file", str(self.token_file), "-stats-addr", stats_address or self.args.stats_address,
            "-access-log", str(self.access), "-spool-file", str(self.spool),
        ])

    def append_event(self, host, user="alex"):
        event = {"user_id": user, "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                 "src_ip": "198.51.100.23", "dst_host": host, "dst_port": 443,
                 "network": "tcp", "result": "accepted", "inbound_tag": "validation", "outbound_tag": "direct"}
        with self.access.open("a") as log:
            log.write(json.dumps(event) + "\n")

    def journal(self):
        return self.request("GET", "/api/v1/activity?user=1&limit=1000")["data"]

    def run(self):
        with socket.socket() as probe:
            probe.bind(("127.0.0.1", self.args.port))
        self.start_core()
        self.request("GET", "/api/v1/users", expected=401)
        self.login()
        self.request("POST", "/api/v1/agents/enrollment-tokens", {"node_id": "demo-node-1"}, expected=403, csrf=False)
        for asset in ("/", "/app.js", "/styles.css"):
            assert self.request("GET", asset)
        self.passed("authentication, CSRF, embedded assets")

        enrollment = self.request("POST", "/api/v1/agents/enrollment-tokens", {"node_id": "demo-node-1"}, expected=201)
        command = [str(self.bins / "subgate-agent"), "enroll", "-core-addr", self.base,
                   "-node-id", "demo-node-1", "-enrollment-token", enrollment["token"], "-token-file", str(self.token_file)]
        result = subprocess.run(command, cwd=self.work, capture_output=True, timeout=15)
        assert result.returncode == 0, "Agent enrollment command failed"
        self.token = self.token_file.read_text().strip()
        assert self.token and self.token_file.stat().st_mode & 0o777 == 0o600
        self.request("POST", "/api/v1/agents/enroll", {"node_id": "demo-node-1", "token": enrollment["token"]}, expected=401)
        self.passed("CLI enrollment, one-time token, credential permissions")

        self.start_agent()
        wait_for(lambda: any(a["online"] for a in self.request("GET", "/api/v1/agents")["data"]), "Agent heartbeat")
        if self.args.require_stats:
            samples = wait_for(lambda: self.request("GET", "/api/v1/live")["data"], "real Xray samples")
            self.passed("real Xray StatsService to Agent to Core live telemetry", users=len(samples))
        self.append_event("online-validation.example.invalid")
        wait_for(lambda: any(e["dst_host"] == "online-validation.example.invalid" for e in self.journal()), "access log event by panel user ID")
        self.passed("Agent access log, encrypted journal, panel ID alias resolution")

        now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        batch = {"batch_id": "validation-duplicate", "node_id": "demo-node-1",
                 "samples": [{"user_id": "alex@example.invalid", "timestamp": now, "uplink_bps": 123, "downlink_bps": 456}],
                 "events": [{"user_id": "demo-alex", "timestamp": now, "dst_host": "duplicate-validation.example.invalid", "dst_port": 443}]}
        self.request("POST", "/api/v1/agents/telemetry", batch, bearer=self.token)
        assert self.request("POST", "/api/v1/agents/telemetry", batch, bearer=self.token)["duplicate"]
        assert sum(e["dst_host"] == "duplicate-validation.example.invalid" for e in self.journal()) == 1
        assert any(e["dst_host"] == "duplicate-validation.example.invalid" for e in self.request("GET", "/api/v1/activity?user=alex")["data"])
        self.passed("duplicate batch replay and cross-alias journal queries")

        self.stop(self.agent)
        self.stop(self.core)
        self.start_agent("127.0.0.1:1")
        time.sleep(0.3)
        self.append_event("offline-validation.example.invalid")
        wait_for(lambda: self.spool.exists() and self.spool.stat().st_size > 32, "encrypted offline spool")
        raw = self.spool.read_bytes()
        assert raw.startswith(b"SUBGATE-SPOOL-1\x00") and b"offline-validation" not in raw
        assert self.spool.stat().st_mode & 0o777 == 0o600
        self.stop(self.agent)
        self.start_core()
        self.login()
        self.start_agent("127.0.0.1:1")
        wait_for(lambda: any(e["dst_host"] == "offline-validation.example.invalid" for e in self.journal()), "durable spool replay after restart")
        wait_for(lambda: not self.spool.exists(), "acknowledged spool removal")
        assert sum(e["dst_host"] == "offline-validation.example.invalid" for e in self.journal()) == 1
        self.passed("Core outage, encrypted spool, Agent restart and exactly-once ingestion")

        self.request("DELETE", "/api/v1/agents/demo-node-1")
        self.request("POST", "/api/v1/agents/telemetry", batch, expected=401, bearer=self.token)
        self.append_event("revoked-validation.example.invalid")
        wait_for(lambda: self.agent.poll() is not None, "revoked Agent exits")
        assert self.agent.returncode != 0 and self.spool.exists()
        assert not any(e["dst_host"] == "revoked-validation.example.invalid" for e in self.journal())
        self.passed("revocation rejects ingress and stops Agent while retaining pending telemetry")

        backup = self.request("POST", "/api/v1/backups", expected=201)
        encrypted = self.request("GET", "/api/v1/backups/" + backup["name"])
        assert encrypted.startswith(b"SUBGATE1") and b"online-validation" not in encrypted
        with sqlite3.connect(self.work / "data/demo.db") as db:
            payloads = db.execute("SELECT payload FROM sg_activity").fetchall()
            assert payloads and all(b"validation.example.invalid" not in p[0] for p in payloads)
        self.stop(self.core)
        restored = self.work / "restored/data"
        restored.mkdir(parents=True, mode=0o700)
        key = self.work / "data/subgate.key"
        restore_command = [str(self.bins / "subgate-core"), "-restore", str(self.work / "data/demo-backups" / backup["name"]),
                           "-key-file", str(key), "-output", str(restored / "demo.db")]
        assert subprocess.run(restore_command, cwd=self.work, capture_output=True, timeout=15).returncode == 0
        assert subprocess.run(restore_command, cwd=self.work, capture_output=True, timeout=15).returncode != 0
        bad_key = self.work / "wrong.key"
        bad_key.write_bytes(os.urandom(32))
        invalid = restore_command.copy()
        invalid[invalid.index("-key-file") + 1] = str(bad_key)
        invalid[invalid.index("-output") + 1] = str(restored / "wrong.db")
        assert subprocess.run(invalid, cwd=self.work, capture_output=True, timeout=15).returncode != 0
        assert not (restored / "wrong.db").exists()
        shutil.copy2(key, restored / "subgate.key")
        self.start_core(restored=True)
        self.login()
        assert len(self.journal()) == 3
        assert self.request("GET", "/api/v1/agents")["data"][0]["status"] == "revoked"
        audit = self.request("GET", "/api/v1/audit?limit=1000")["data"]
        assert any(row["action"] == "backup.create" for row in audit)
        self.passed("encrypted backup download, offline restore, wrong-key/overwrite rejection and restart persistence")

    def finish(self):
        for process in reversed(self.processes):
            self.stop(process)
        for log in self.logs:
            log.close()
        report = {"checks": self.results, "work_dir": str(self.work), "processes_stopped": all(p.poll() is not None for p in self.processes)}
        (self.work / "report.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps({"report": str(self.work / "report.json"), "processes_stopped": report["processes_stopped"]}), flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bin-dir", required=True)
    parser.add_argument("--work-dir", default="/tmp/subgate-validation")
    parser.add_argument("--port", type=int, default=18088)
    parser.add_argument("--stats-address", default="auto")
    parser.add_argument("--require-stats", action="store_true", help="Require live samples from an existing local Xray; counters are never reset")
    args = parser.parse_args()
    if not 1024 <= args.port <= 65535:
        parser.error("port must be between 1024 and 65535")
    validation = Validation(args)
    try:
        validation.run()
    finally:
        validation.finish()


if __name__ == "__main__":
    main()
