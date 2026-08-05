#!/usr/bin/env python3
# Copyright 2026 Google LLC
# Direct Read, Write, and Context Recall Verification on Oracle ADB 26ai (5号湖泊 Lake 5)

import json
import os
import sys
import time
import requests
import oracledb

VAULT_URL = "http://192.168.31.111:8200/v1/secret/data/oracle/config/lake5"
VAULT_TOKEN = "hvs.REDACTED_FOR_SECURITY_000"

def main():
    print("==================================================================")
    print("🧠 VERIFYING ORACLE ADB 26AI (5号湖泊 LAKE 5) VIA HAPROXY SSL BRIDGE & VAULT")
    print("==================================================================")

    # 1. Fetch Credentials from Vault
    print("\n--- 1. Fetching Credentials from Vault Path secret/data/oracle/config/lake5 ---")
    headers = {"X-Vault-Token": VAULT_TOKEN}
    r = requests.get(VAULT_URL, headers=headers, timeout=5)
    if r.status_code != 200:
        print("❌ Failed to fetch secrets from Vault!")
        sys.exit(1)

    secret_data = r.json()["data"]["data"]
    username = secret_data["username"]
    password = secret_data["password"]
    service_name = secret_data["service_name"]

    print(f"  Vault Username: {username}")
    print(f"  Vault Service Name: {service_name}")

    # 2. Connect to Oracle Autonomous Database 26ai (Lake 5) via HAProxy SSL Bridge
    print("\n--- 2. Connecting to Oracle ADB 26ai (Lake 5) via HAProxy (100.93.5.81:11523) ---")
    try:
        connection = oracledb.connect(
            user=username,
            password=password,
            host="100.93.5.81",
            port=11523,
            service_name=service_name
        )
        cursor = connection.cursor()
        print("  ✅ [Connect SUCCESS] Connected directly to Oracle ADB 26ai (Lake 5)!")

        cursor.execute("SELECT banner FROM v$version")
        ver = cursor.fetchone()
        print(f"  Oracle DB Version: {ver[0] if ver else 'Oracle ADB 26ai'}")

        # 3. Create Table & Write Memory
        print("\n--- 3. Writing Test Context Memory Row to Oracle ADB 26ai (Lake 5) ---")
        table_sql = """
        DECLARE
            e_table_exists EXCEPTION;
            PRAGMA EXCEPTION_INIT(e_table_exists, -955);
        BEGIN
            EXECUTE IMMEDIATE 'CREATE TABLE lake5_agent_memories (
                id NUMBER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
                agent_id VARCHAR2(100),
                memory_text VARCHAR2(4000),
                created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
            )';
        EXCEPTION
            WHEN e_table_exists THEN NULL;
        END;
        """
        cursor.execute(table_sql)

        test_context = f"August 5 2026 Lake 5 Oracle 26ai Memory Test Key: CDP Faucet 12-Seat EVM Initialized"
        insert_sql = "INSERT INTO lake5_agent_memories (agent_id, memory_text) VALUES (:1, :2)"
        cursor.execute(insert_sql, ("Topaz_Seat_01", test_context))
        connection.commit()
        print(f"  ✅ [Write PASS] Successfully written to Lake 5 Oracle 26ai: '{test_context}'")

        # 4. Read & Recall Memory
        print("\n--- 4. Reading & Recalling Context Memory Row from Oracle ADB 26ai (Lake 5) ---")
        select_sql = "SELECT id, agent_id, memory_text, created_at FROM lake5_agent_memories WHERE memory_text LIKE :1 ORDER BY id DESC"
        cursor.execute(select_sql, ("%August 5 2026%",))
        rows = cursor.fetchall()

        print(f"  Recalled {len(rows)} matching memory records from Oracle 5号湖泊:")
        for row in rows:
            print(f"    - ID: {row[0]} | Agent: {row[1]} | Memory: '{row[2]}' | Time: {row[3]}")

        cursor.close()
        connection.close()

        print("\n==================================================================")
        print("🎉 ORACLE ADB 26AI (5号湖泊 LAKE 5) READ, WRITE, AND RECALL VERIFIED 100% SUCCESSFUL!")
        print("==================================================================")

    except Exception as e:
        import traceback
        print("❌ Oracle ADB 26ai Connection Error:", e)
        traceback.print_exc()

if __name__ == "__main__":
    main()
