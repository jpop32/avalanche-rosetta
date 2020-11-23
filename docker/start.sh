#!/bin/bash

cat <<EOF >> /app/avalanchego-config.json
{
  "network-id": "$AVALANCHE_NETWORK",
  "http-host": "0.0.0.0",
  "api-keystore-enabled": false,
  "api-admin-enabled": false,
  "api-ipcs-enabled": false,
  "coreth-config": {
    "snowman-api-enabled": true,
    "coreth-admin-api-enabled": true,
    "net-api-enabled": true,
    "rpc-gas-cap": 2500000000,
    "rpc-tx-fee-cap": 100,
    "eth-api-enabled": true,
    "personal-api-enabled": true,
    "tx-pool-api-enabled": true,
    "debug-api-enabled": true,
    "web3-api-enabled": true
  }
}
EOF

cat <<EOF >> /app/rosetta-config.json
{
  "mode": "online",
  "rpc_endpoint": "http://localhost:9650",
  "listen_addr": "0.0.0.0:8081"
}
EOF

/app/avalanchego --config-file=/app/avalanchego-config.json & \
/app/rosetta -config=/app/rosetta-config.json