#!/usr/bin/env python3
# Copyright 2026 Google LLC
# 12-Seat Agent EVM Wallet Initializer & Equal Micropayment Distributor

import asyncio
import json
import os
import time
from eth_account import Account
from web3 import Web3
from cdp import CdpClient

API_KEY_ID = "f9fa69de-69f5-4d29-add5-e808d5e7a2fa"
API_KEY_SECRET = "GFE4E8sNH5EyoR7FRKYnM4sJzZlAehZdsOcczxdpbvYNTu/cHLXhmwjkzO6YG/uZOrFMwEvtFzPl5eUlDr7A0A=="
RPC_URL = "https://sepolia.base.org"
CHAIN_ID = 84532

SEATS = [
    "topaz", "ruby", "amber", "emerald", "sapphire", "diamond",
    "opal", "garnet", "onyx", "pearl", "jade", "quartz"
]

async def main():
    print("==================================================================")
    print("💎 INITIALIZING 12 AGENT SEAT EVM WALLETS & DISTRIBUTING FAUCET ETH")
    print("==================================================================")

    client = CdpClient(api_key_id=API_KEY_ID, api_key_secret=API_KEY_SECRET)
    w3 = Web3(Web3.HTTPProvider(RPC_URL))

    # 1. Generate/Derive 12 Agent Seat EVM Wallets
    wallets = {}
    print("\n--- 1. Generating 12 Agent Seat EVM Keypairs ---")
    for i, seat in enumerate(SEATS, 1):
        acc = Account.create(f"seed_agent_seat_{seat}_2026_adk")
        wallets[seat] = {
            "seat_id": i,
            "seat_name": seat.capitalize(),
            "address": acc.address,
            "private_key": acc.key.hex(),
            "status": "Active" if i <= 6 else "Vacant Seat Initialized"
        }
        print(f"  [Seat {i:02d}: {seat.capitalize()}] EVM Address: {acc.address} ({wallets[seat]['status']})")

    # 2. Claim Faucet ETH for Distributor Master Wallet
    distributor_acc = Account.create("master_distributor_seed_2026_v2")
    print(f"\n--- 2. Requesting Base Sepolia Testnet ETH Faucet for Distributor Wallet ---")
    print(f"Distributor Address: {distributor_acc.address}")

    tx_hash = await client.evm.request_faucet(
        address=distributor_acc.address,
        network="base-sepolia",
        token="eth"
    )
    print(f"🎉 Base Sepolia Faucet Claimed! TxHash: {tx_hash}")
    print("Waiting 10s for on-chain block confirmation...")
    await asyncio.sleep(10)

    dist_balance = w3.eth.get_balance(distributor_acc.address)
    eth_balance = w3.from_wei(dist_balance, 'ether')
    print(f"💰 Distributor Wallet Confirmed Balance: {eth_balance} ETH")

    # 3. Divide Faucet ETH into 12 Equal Shares and Transfer with incrementing nonce
    if dist_balance > 0:
        share_wei = int(dist_balance * 0.85 / 12)  # Leave 15% for gas fees
        share_eth = w3.from_wei(share_wei, 'ether')
        print(f"\n--- 3. Dividing Balance into 12 Equal Shares ({share_eth} ETH per Seat) ---")

        base_nonce = w3.eth.get_transaction_count(distributor_acc.address)
        gas_price = w3.eth.gas_price

        for idx, seat in enumerate(SEATS):
            target_addr = wallets[seat]["address"]
            tx = {
                'nonce': base_nonce + idx,
                'to': target_addr,
                'value': share_wei,
                'gas': 21000,
                'gasPrice': int(gas_price * 1.1),
                'chainId': CHAIN_ID
            }

            signed_tx = w3.eth.account.sign_transaction(tx, distributor_acc.key)
            sent_tx = w3.eth.send_raw_transaction(signed_tx.raw_transaction)
            wallets[seat]["dist_tx_hash"] = sent_tx.hex()
            wallets[seat]["balance_eth"] = str(share_eth)
            print(f"  ✅ [Seat {idx+1:02d}: {seat.capitalize()}] Sent {share_eth} ETH | TxHash: {sent_tx.hex()}")
            time.sleep(0.3)

    # 4. Save 12-Seat Wallet Manifest
    manifest_path = "/home/ben/key-agent/bin/12_agent_seats_evm_manifest.json"
    os.makedirs("/home/ben/key-agent/bin", exist_ok=True)
    with open(manifest_path, "w") as f:
        json.dump(wallets, f, indent=2)

    print(f"\n==================================================================")
    print(f"🎉 ALL 12 AGENT SEAT EVM WALLETS INITIALIZED & FUNDED!")
    print(f"📁 Manifest saved to: {manifest_path}")
    print("==================================================================")

if __name__ == "__main__":
    asyncio.run(main())
