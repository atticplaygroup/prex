#!/bin/bash

set -eu

SUI_NETWORK=localnet

if [ ${SUI_NETWORK} = "mainnet" ]; then
  SUI_NODE_URL=https://fullnode.mainnet.sui.io
  SUI_GRAPHQL_URL=https://sui-mainnet.mystenlabs.com/graphql
elif [ ${SUI_NETWORK} = "testnet" ]; then
  SUI_NODE_URL=https://fullnode.testnet.sui.io
  SUI_GRAPHQL_URL=https://sui-testnet.mystenlabs.com/graphql
elif [ ${SUI_NETWORK} = "devnet" ]; then
  SUI_NODE_URL=https://fullnode.devnet.sui.io
  SUI_GRAPHQL_URL=https://sui-devnet.mystenlabs.com/graphql
elif [ ${SUI_NETWORK} = "localnet" ]; then
  SUI_NODE_URL=http://127.0.0.1:9000
  SUI_GRAPHQL_URL=http://127.0.0.1:9125/graphql
else
  {
    echo "unknown network ${SUI_NETWORK}"
    exit 1
  }
fi

export SUI_NODE_URL
export SUI_NETWORK
export SUI_GRAPHQL_URL
