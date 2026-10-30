-- SubGate Database Schema (SQLite WAL)
-- Version: 1

-- Settings and configuration (versioned, hot-reloadable)
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    version INTEGER DEFAULT 1,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Admin users
CREATE TABLE IF NOT EXISTS admin_users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    email TEXT,
    password_hash TEXT NOT NULL,
    role TEXT DEFAULT 'admin', -- 'superadmin', 'admin', 'viewer'
    totp_secret TEXT,
    webauthn_key TEXT,
    enabled BOOLEAN DEFAULT 1,
    last_login DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_admin_users_username ON admin_users(username);
CREATE INDEX idx_admin_users_enabled ON admin_users(enabled);

-- API keys
CREATE TABLE IF NOT EXISTS api_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key_hash TEXT UNIQUE NOT NULL,
    admin_id INTEGER,
    agent_id INTEGER,
    name TEXT NOT NULL,
    scopes TEXT, -- JSON array of scopes
    last_used DATETIME,
    enabled BOOLEAN DEFAULT 1,
    expires_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(admin_id) REFERENCES admin_users(id)
);

CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);

-- Audit log (all admin actions)
CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    admin_id INTEGER,
    action TEXT NOT NULL,
    resource TEXT,
    resource_id TEXT,
    details TEXT, -- JSON
    status TEXT DEFAULT 'success', -- 'success', 'failure'
    error TEXT,
    ip_address TEXT,
    user_agent TEXT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(admin_id) REFERENCES admin_users(id)
);

CREATE INDEX idx_audit_log_admin_id ON audit_log(admin_id);
CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp DESC);

-- Nodes (from panel)
CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    host_id TEXT,
    ip_address TEXT,
    status TEXT DEFAULT 'online', -- 'online', 'offline', 'maintenance'
    tags TEXT, -- JSON array
    config_profile TEXT,
    capacity_mbps INTEGER,
    health_status TEXT DEFAULT 'unknown',
    last_health_check DATETIME,
    last_sync DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_nodes_host_id ON nodes(host_id);
CREATE INDEX idx_nodes_status ON nodes(status);
CREATE INDEX idx_nodes_health_status ON nodes(health_status);

-- Hosts (servers)
CREATE TABLE IF NOT EXISTS hosts (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    ip_address TEXT,
    tags TEXT, -- JSON array
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Mapping: hosts to nodes
CREATE TABLE IF NOT EXISTS host_node_mappings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    host_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    mapping_type TEXT DEFAULT 'primary',
    UNIQUE(host_id, node_id),
    FOREIGN KEY(host_id) REFERENCES hosts(id),
    FOREIGN KEY(node_id) REFERENCES nodes(id)
);

-- Health state (current status)
CREATE TABLE IF NOT EXISTS health_state (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT NOT NULL UNIQUE,
    status TEXT DEFAULT 'healthy', -- 'healthy', 'suspected', 'quarantined', 'recovering', 'maintenance'
    check_count INTEGER DEFAULT 0,
    last_success DATETIME,
    last_failure DATETIME,
    failure_count INTEGER DEFAULT 0,
    consecutive_failures INTEGER DEFAULT 0,
    recovered_at DATETIME,
    manual_override BOOLEAN DEFAULT 0,
    maintenance_until DATETIME,
    FOREIGN KEY(node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_health_state_node_id ON health_state(node_id);
CREATE INDEX idx_health_state_status ON health_state(status) WHERE status != 'healthy';

-- Health check history
CREATE TABLE IF NOT EXISTS health_checks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT NOT NULL,
    check_type TEXT, -- 'tcp', 'tls', 'quic', 'http', 'grpc'
    result TEXT DEFAULT 'pass', -- 'pass', 'fail', 'timeout'
    duration_ms INTEGER,
    error_message TEXT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

CREATE INDEX idx_health_checks_node_id_timestamp ON health_checks(node_id, timestamp DESC);

-- Quarantine events (history)
CREATE TABLE IF NOT EXISTS quarantine_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT NOT NULL,
    user_id TEXT,
    reason TEXT, -- 'health_failure', 'manual', 'maintenance'
    consecutive_failures INTEGER,
    quarantine_until DATETIME,
    manual_override BOOLEAN DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    resolved_at DATETIME,
    FOREIGN KEY(node_id) REFERENCES nodes(id)
);

CREATE INDEX idx_quarantine_events_node_id ON quarantine_events(node_id);
CREATE INDEX idx_quarantine_events_user_id ON quarantine_events(user_id);

-- Tiers
CREATE TABLE IF NOT EXISTS tiers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    display_order INTEGER,
    strategy TEXT DEFAULT 'random', -- 'random', 'roundRobin', 'leastPing', 'leastLoad'
    min_healthy INTEGER DEFAULT 1,
    fallback_tier_id INTEGER,
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Policies
CREATE TABLE IF NOT EXISTS policies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    tier_id INTEGER NOT NULL,
    conditions TEXT, -- JSON with country, asn, ua, plan
    layout TEXT, -- JSON with entry ordering/exclusion
    display_order INTEGER,
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(tier_id) REFERENCES tiers(id)
);

CREATE INDEX idx_policies_enabled ON policies(enabled);

-- Users cache (from panel)
CREATE TABLE IF NOT EXISTS users_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    short_uuid TEXT UNIQUE NOT NULL,
    email TEXT,
    squad_id TEXT,
    status TEXT DEFAULT 'active', -- 'active', 'disabled', 'expired'
    plan_name TEXT,
    hwid_limit INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_cache_short_uuid ON users_cache(short_uuid);
CREATE INDEX idx_users_cache_email ON users_cache(email);

-- User subscriptions (format cache)
CREATE TABLE IF NOT EXISTS user_subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    format TEXT, -- 'xray-json', 'uri-list', 'mihomo-yaml', 'singbox-json'
    sub_hash TEXT, -- Hash of last config served
    policy_version INTEGER,
    last_request DATETIME,
    UNIQUE(user_id, format),
    FOREIGN KEY(user_id) REFERENCES users_cache(id) ON DELETE CASCADE
);

-- Pins (user → node mapping for sticky routing)
CREATE TABLE IF NOT EXISTS pins (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    node_id TEXT NOT NULL,
    pin_mode TEXT DEFAULT 'single', -- 'single', 'ordered_fallback', 'balancer_preferred'
    ordered_fallback TEXT, -- JSON: [node1, node2, ...]
    last_rebalance DATETIME,
    last_update DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, node_id),
    FOREIGN KEY(user_id) REFERENCES users_cache(id) ON DELETE CASCADE,
    FOREIGN KEY(node_id) REFERENCES nodes(id)
);

CREATE INDEX idx_pins_user_id ON pins(user_id);

-- Quotas
CREATE TABLE IF NOT EXISTS quotas (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT NOT NULL,
    scope TEXT, -- 'per_user', 'per_node'
    bytes_limit INTEGER64,
    period TEXT DEFAULT 'month', -- 'day', 'week', 'month'
    period_type TEXT DEFAULT 'calendar', -- 'calendar', 'rolling'
    timezone TEXT DEFAULT 'UTC',
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(node_id) REFERENCES nodes(id)
);

-- Usage snapshots (from panel or agent)
CREATE TABLE IF NOT EXISTS usage_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT NOT NULL,
    user_id INTEGER,
    bytes_used INTEGER64,
    source TEXT DEFAULT 'panel', -- 'panel', 'agent'
    snapshot_date DATE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(node_id) REFERENCES nodes(id),
    FOREIGN KEY(user_id) REFERENCES users_cache(id)
);

CREATE INDEX idx_usage_snapshots_date ON usage_snapshots(snapshot_date DESC);

-- Injections (custom rules)
CREATE TABLE IF NOT EXISTS injections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    scope TEXT, -- 'all', 'group', 'user', 'policy'
    scope_id TEXT,
    rule_type TEXT, -- 'append', 'prepend', 'patch', 'remove'
    format TEXT DEFAULT 'xray-json',
    content TEXT NOT NULL,
    version INTEGER DEFAULT 1,
    approved_by INTEGER,
    created_by INTEGER NOT NULL,
    approved_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(created_by) REFERENCES admin_users(id),
    FOREIGN KEY(approved_by) REFERENCES admin_users(id)
);

CREATE INDEX idx_injections_version ON injections(version DESC);

-- Geo-blocking rules
CREATE TABLE IF NOT EXISTS geo_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    rule_type TEXT, -- 'country', 'asn', 'cidr', 'network_type'
    action TEXT DEFAULT 'deny', -- 'allow', 'deny'
    "values" TEXT, -- JSON array of countries/ASNs/CIDRs
    policy_id INTEGER,
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(policy_id) REFERENCES policies(id)
);

-- ASN overrides (operator curated)
CREATE TABLE IF NOT EXISTS asn_overrides (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    asn INTEGER UNIQUE NOT NULL,
    as_org TEXT,
    operator_type TEXT, -- 'mobile', 'home_isp', 'hosting', 'vpn', 'cdn', 'edu', 'gov'
    tags TEXT, -- JSON array
    reviewed BOOLEAN DEFAULT 0,
    confidence TEXT DEFAULT 'low', -- 'high', 'medium', 'low'
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- IP intelligence data sources metadata
CREATE TABLE IF NOT EXISTS ipintel_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    url TEXT,
    format TEXT, -- 'mmdb', 'csv', 'json'
    license TEXT,
    enabled BOOLEAN DEFAULT 1,
    last_refresh DATETIME,
    next_refresh DATETIME,
    file_size INTEGER64,
    file_hash TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Subscription requests (2-day retention)
CREATE TABLE IF NOT EXISTS sub_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    short_uuid TEXT,
    client_ip TEXT,
    user_agent TEXT,
    decision TEXT DEFAULT 'allow', -- 'allow', 'deny', 'error'
    response_code INTEGER,
    response_size INTEGER,
    format_requested TEXT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_sub_requests_timestamp ON sub_requests(timestamp DESC);
CREATE INDEX idx_sub_requests_short_uuid ON sub_requests(short_uuid);

-- Subscription request aggregates (365-day retention)
CREATE TABLE IF NOT EXISTS sub_request_aggregates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date DATE,
    client_ip_hash TEXT,
    asn INTEGER,
    country TEXT,
    decision TEXT,
    count INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_sub_request_aggregates_date ON sub_request_aggregates(date DESC);

-- Connection origin aggregates
CREATE TABLE IF NOT EXISTS conn_origin_aggregates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date DATE,
    node_id TEXT,
    asn INTEGER,
    country TEXT,
    city TEXT,
    operator_type TEXT,
    protocol TEXT,
    count INTEGER DEFAULT 1,
    FOREIGN KEY(node_id) REFERENCES nodes(id)
);

CREATE INDEX idx_conn_origin_aggregates_date ON conn_origin_aggregates(date DESC);

-- Scanner events
CREATE TABLE IF NOT EXISTS scanner_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    detector_type TEXT, -- 'uuid_enum', 'rate_anomaly', 'ua_scanner', 'handshake_burst'
    source_ip TEXT,
    source_asn INTEGER,
    incident_count INTEGER,
    first_seen DATETIME,
    last_seen DATETIME,
    escalation_level INTEGER DEFAULT 0, -- 0:none, 1:tarpit, 2:deny, 3:export
    alerted BOOLEAN DEFAULT 0,
    resolved BOOLEAN DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_scanner_events_escalation ON scanner_events(escalation_level);

-- Bans (users/IPs)
CREATE TABLE IF NOT EXISTS bans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ban_type TEXT, -- 'ip', 'asn', 'user'
    target TEXT,
    reason TEXT,
    permanent BOOLEAN DEFAULT 0,
    until DATETIME,
    created_by INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(created_by) REFERENCES admin_users(id)
);

-- Block detection results (16–20 KiB)
CREATE TABLE IF NOT EXISTS block_check_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT NOT NULL,
    inbound_tag TEXT,
    vantage_point TEXT, -- 'agent', 'probe'
    verdict TEXT DEFAULT 'ok', -- 'ok', 'suspect', 'blocked'
    stall_bytes INTEGER,
    duration_ms INTEGER,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(node_id) REFERENCES nodes(id)
);

-- Baselines (for drop detection)
CREATE TABLE IF NOT EXISTS baselines (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    metric_type TEXT, -- 'online_users', 'traffic_mbps', 'requests_per_min'
    node_id TEXT,
    hour_of_week INTEGER, -- 0-167 (week*24+hour)
    median_value REAL,
    mad REAL, -- median absolute deviation
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(node_id) REFERENCES nodes(id)
);

-- Alerts
CREATE TABLE IF NOT EXISTS alerts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    category TEXT, -- 'health', 'drops', 'youtube', 'block16_20', 'antiscan', 'bandwidth', 'logs', 'system'
    severity TEXT DEFAULT 'info', -- 'info', 'warning', 'critical'
    node_id TEXT,
    title TEXT NOT NULL,
    message TEXT,
    dedupe_key TEXT,
    cooldown_until DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    fired_at DATETIME,
    cleared_at DATETIME,
    FOREIGN KEY(node_id) REFERENCES nodes(id)
);

CREATE INDEX idx_alerts_category ON alerts(category);
CREATE INDEX idx_alerts_created_at ON alerts(created_at DESC);

-- Agents
CREATE TABLE IF NOT EXISTS agents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT NOT NULL UNIQUE,
    version TEXT,
    token_hash TEXT,
    status TEXT DEFAULT 'unknown', -- 'online', 'offline', 'error'
    last_heartbeat DATETIME,
    cpu_percent REAL,
    memory_mb INTEGER,
    disk_free_gb INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(node_id) REFERENCES nodes(id)
);

-- Probes
CREATE TABLE IF NOT EXISTS probes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    region TEXT, -- geographic region
    node_id TEXT,
    status TEXT DEFAULT 'online',
    block_16_20_verdict TEXT, -- 'ok', 'suspect', 'blocked'
    youtube_region TEXT,
    last_check DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(node_id) REFERENCES nodes(id)
);

-- Speed rollups (1s raw via TSDB in-memory, these are aggregates)
CREATE TABLE IF NOT EXISTS speed_rollup_10s (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    node_id TEXT,
    timestamp DATETIME,
    uplink_bps INTEGER64,
    downlink_bps INTEGER64,
    sample_count INTEGER DEFAULT 1,
    UNIQUE(user_id, node_id, timestamp),
    FOREIGN KEY(user_id) REFERENCES users_cache(id),
    FOREIGN KEY(node_id) REFERENCES nodes(id)
);

CREATE TABLE IF NOT EXISTS speed_rollup_1m (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    node_id TEXT,
    timestamp DATETIME,
    uplink_avg_bps INTEGER64,
    downlink_avg_bps INTEGER64,
    uplink_peak_bps INTEGER64,
    downlink_peak_bps INTEGER64,
    uplink_p95_bps INTEGER64,
    downlink_p95_bps INTEGER64,
    UNIQUE(user_id, node_id, timestamp),
    FOREIGN KEY(user_id) REFERENCES users_cache(id),
    FOREIGN KEY(node_id) REFERENCES nodes(id)
);

CREATE TABLE IF NOT EXISTS speed_rollup_1h (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    timestamp DATETIME,
    uplink_avg_bps INTEGER64,
    downlink_avg_bps INTEGER64,
    uplink_peak_bps INTEGER64,
    downlink_peak_bps INTEGER64,
    UNIQUE(user_id, timestamp),
    FOREIGN KEY(user_id) REFERENCES users_cache(id)
);

-- Log signal aggregates
CREATE TABLE IF NOT EXISTS log_signal_aggregates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT,
    timestamp DATETIME,
    log_level TEXT, -- 'debug', 'info', 'warning', 'error'
    error_class TEXT, -- 'dial', 'dns', 'timeout', 'handshake', 'auth', 'io'
    count INTEGER DEFAULT 1,
    FOREIGN KEY(node_id) REFERENCES nodes(id)
);

-- Log samples (bounded, optional)
CREATE TABLE IF NOT EXISTS log_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT,
    timestamp DATETIME,
    log_level TEXT,
    message TEXT,
    FOREIGN KEY(node_id) REFERENCES nodes(id)
);

-- Activity journal index (points to on-disk segment files)
CREATE TABLE IF NOT EXISTS activity_segments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT UNIQUE NOT NULL,
    ts_min DATETIME,
    ts_max DATETIME,
    user_id_min INTEGER,
    user_id_max INTEGER,
    tier TEXT DEFAULT 'hot', -- 'hot', 'warm', 'cold'
    size_bytes INTEGER64,
    compressed_size_bytes INTEGER64,
    compression_ratio REAL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Activity dictionaries
CREATE TABLE IF NOT EXISTS activity_dictionaries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    segment_id INTEGER,
    dict_type TEXT, -- 'host', 'user', 'tag', 'asn'
    int_id INTEGER,
    string_value TEXT,
    UNIQUE(segment_id, dict_type, int_id),
    FOREIGN KEY(segment_id) REFERENCES activity_segments(id)
);

-- Activity retention state
CREATE TABLE IF NOT EXISTS activity_retention_state (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tier TEXT UNIQUE, -- 'hot', 'warm', 'cold'
    target_resolution TEXT, -- 'full', 'minute', 'day'
    last_compaction_ts DATETIME,
    next_compaction_ts DATETIME,
    config TEXT -- JSON with retention settings
);

-- Ensure important pragmas are set
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA cache_size = -64000;
PRAGMA temp_store = MEMORY;
