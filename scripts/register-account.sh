#! /bin/bash

set -eu

PREX_CONFIG_PATH="${1}"

export PREX_CONFIG_PATH

ACCOUNT_JSON=$(prex client account)

MY_SUI_ADDRESS=$(echo ${ACCOUNT_JSON} | jq -r .address)
MY_USERNAME=$(echo ${ACCOUNT_JSON} | jq -r .username)
MY_PASSWORD=$(echo ${ACCOUNT_JSON} | jq -r .password)

source scripts/connect-sui.sh

bash scripts/create-deposit.sh 100000000

[ -z "${MY_SUI_ADDRESS}" ] && (
  echo "failed to get sui address"
  exit 1
)
echo "Sui address: ${MY_SUI_ADDRESS}"
QUERY_TEMPLATE='
query GetTransactionDetails($sentAddress: String!) {
  transactionBlocks(filter: {sentAddress: $sentAddress}) {
    nodes {
      digest
    }
  }
}'
sleep 5 # Wait for the transaction to finalize
CHAIN_DIGEST=$(curl --fail -s -X POST "${SUI_GRAPHQL_URL}" \
  --header 'x-sui-rpc-show-usage: true' \
  --header 'Content-Type: application/json' \
  --data '{
    "query": "'"$(echo ${QUERY_TEMPLATE} | tr -d '\n')"'",
    "variables": {"sentAddress": "'${MY_SUI_ADDRESS}'"}
  }
' | jq -r '.data.transactionBlocks.nodes[].digest' | head -n1)
echo "${CHAIN_DIGEST}"
[ -z "${CHAIN_DIGEST}" ] && (
  echo "failed to get transaction digest"
  exit 1
)

CHALLENGE_JSON=$(curl -s "http://localhost:3000/v1/challenge?address=${MY_SUI_ADDRESS}")
echo ${CHALLENGE_JSON}
CHALLENGE=$(echo ${CHALLENGE_JSON} | jq -r .challenge)
START_TIME=$(echo ${CHALLENGE_JSON} | jq -r .startTime)
[ -z "${CHALLENGE}" ] && (
  echo "failed to get challenge"
  exit 1
)

SIGNATURE=$(prex client sign -m "${CHALLENGE}" | jq -r .signature)
echo ${SIGNATURE}
[ -z "${SIGNATURE}" ] && (
  echo "failed to get signature"
  exit 1
)

TTL="$((24*3600))s"
BODY='{
  "username": "'"${MY_USERNAME}"'",
  "password": "'"${MY_PASSWORD}"'",
  "ttl": "'"${TTL}"'", 
  "proof": {
    "chain_digest": "'"${CHAIN_DIGEST}"'",
    "challenge": "'"${CHALLENGE}"'",
    "start_time": "'"${START_TIME}"'",
    "signature": "'"${SIGNATURE}"'"
  }
}'
echo "${BODY}"
curl --fail -s -X POST http://localhost:3000/v1/deposit -d "${BODY}"
