#!/bin/bash

set -eu

# bash create-deposit.sh 10000000
AMOUNT=${1}

source scripts/connect-sui.sh

function get_coin_address () {
  QUERY_TEMPLATE='
  query GetCoin($address: String!) {
    address(address: $address) {
      address
      coins {
        nodes {
          address
          coinBalance
        }
      }
    }
  }
  '
  COIN_ADDRESS=$(curl -s -X POST "${SUI_GRAPHQL_URL}" \
    --header 'Content-Type: application/json'       \
    --data '{
      "query": "'"$(echo ${QUERY_TEMPLATE} | tr -d '\n')"'",
      "variables": {"address": "'${MY_SUI_ADDRESS}'"}
    }
  ' | jq -c '.data.address.coins.nodes[]' | head -n1 | jq -r '.address')
  echo "${COIN_ADDRESS}"
}

MY_SUI_ADDRESS=$(prex client account | jq -r .address)

COIN_ADDRESS=$(get_coin_address)

echo "Using ${SUI_NETWORK}"

[ -n "${COIN_ADDRESS}" ] || prex client sui-faucet \
  --network=${SUI_NETWORK}

sleep 5 # Wait for the transaction to finalize

COIN_ADDRESS=$(get_coin_address)

[ -z "${COIN_ADDRESS}" ] && (
  echo "Failed to get coin address"
  exit 1
)

SUI_NETWORK_ENV=PAYMENT_ENVIRONMENT_$(echo ${SUI_NETWORK} | tr '[:lower:]' '[:upper:]')

PAYMENT_ADDRESS=$(curl -s http://localhost:3000/v1/payment-methods | \
  jq -r '.paymentMethods[] | select(.environment=="'"${SUI_NETWORK_ENV}"'") .address')
[ -z ${PAYMENT_ADDRESS} ] || [ ${PAYMENT_ADDRESS} = "null" ] && (
  echo "failed to get payment address"
  exit 1
)

prex client sui-transfer \
  --coin=${COIN_ADDRESS} \
  --amount=${AMOUNT} \
  --recipient=${PAYMENT_ADDRESS} \
  --network=${SUI_NETWORK}
