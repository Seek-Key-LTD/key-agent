-- PicoOracle Agent DDL for Oracle ADB (lake5)
-- Pure SQL, no PL/SQL packages required.
-- Vector type uses VECTOR(384, FLOAT32) — Oracle 23.4+ ADB.

-- ── Memory: deep layer (encrypted) ──
CREATE TABLE PICO_MEMORY_DEEP (
    id           RAW(16) DEFAULT SYS_GUID() PRIMARY KEY,
    agent_name   VARCHAR2(256) NOT NULL,
    content_enc  BLOB NOT NULL,                      -- AES-256-GCM ciphertext
    iv           RAW(12) NOT NULL,
    salt         RAW(12),
    embedding    VECTOR(384, FLOAT32),               -- 384-dim embedding
    layer        VARCHAR2(32) NOT NULL,              -- "deep"
    confidence   NUMBER(3,2) DEFAULT 0.00,
    reason       VARCHAR2(512),
    ts           TIMESTAMP DEFAULT SYSTIMESTAMP NOT NULL
);

-- ── Memory: shared layer (plaintext, cross-agent) ──
CREATE TABLE PICO_MEMORY_SHARED (
    id           RAW(16) DEFAULT SYS_GUID() PRIMARY KEY,
    agent_name   VARCHAR2(256) NOT NULL,
    content      CLOB NOT NULL,
    embedding    VECTOR(384, FLOAT32),
    layer        VARCHAR2(32) NOT NULL,              -- "shared"
    confidence   NUMBER(3,2) DEFAULT 0.00,
    reason       VARCHAR2(512),
    ts           TIMESTAMP DEFAULT SYSTIMESTAMP NOT NULL
);

-- ── Sessions ──
CREATE TABLE PICO_SESSION (
    session_id   VARCHAR2(128) NOT NULL,
    app_name     VARCHAR2(128) NOT NULL,
    user_id      VARCHAR2(256) NOT NULL,
    state_json   CLOB,
    events_json  CLOB,
    created_ts   TIMESTAMP DEFAULT SYSTIMESTAMP NOT NULL,
    updated_ts   TIMESTAMP DEFAULT SYSTIMESTAMP NOT NULL,
    PRIMARY KEY (app_name, session_id)
);

-- ── Indexes ──
CREATE INDEX IDX_MEM_DEEP_AGENT  ON PICO_MEMORY_DEEP (agent_name);
CREATE INDEX IDX_MEM_SHARED_AGENT ON PICO_MEMORY_SHARED (agent_name);
CREATE INDEX IDX_SESSION_USER    ON PICO_SESSION (user_id);
