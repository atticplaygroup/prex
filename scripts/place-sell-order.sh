#!/bin/bash

set -eu

PREX_CONFIG_PATH="${1}"
SERVICE_ID="${2}"
ASK_PRICE="${3}"
SELL_QUANTITY="${4}"

export PREX_CONFIG_PATH

ACCOUNT_JSON=$(prex client account)
MY_SUI_ADDRESS=$(echo ${ACCOUNT_JSON} | jq -r .address)
MY_USERNAME=$(echo ${ACCOUNT_JSON} | jq -r .username)
MY_PASSWORD=$(echo ${ACCOUNT_JSON} | jq -r .password)

LOGIN_RESPONSE=$(curl -s -X POST http://localhost:3000/v1/login \
  -d '{
    "username":"'"${MY_USERNAME}"'",
    "password":"'"${MY_PASSWORD}"'"
  }')
ACCOUNT_ID=$(echo "${LOGIN_RESPONSE}" | jq -r .account.accountId)
AUTH_TOKEN=$(echo "${LOGIN_RESPONSE}" | jq -r .accessToken)
echo "${AUTH_TOKEN}"

if [ "${AUTH_TOKEN}" = "null" ]; then
  echo "failed to get AUTH_TOKEN: ${AUTH_TOKEN}"
  exit 1
fi

FREE_QUOTA_SERVICE_ID=1
SOLD_QUOTA_SERVICE_ID=2

SOLD_QUOTA_TOKEN=$(bash scripts/claim-token.sh 1 ${SOLD_QUOTA_SERVICE_ID} ${AUTH_TOKEN} ${ACCOUNT_ID} 0 1)
if [ -z "${SOLD_QUOTA_TOKEN}" ] || [ "${SOLD_QUOTA_TOKEN}" = "null" ]; then
  echo "claim service ${SOLD_QUOTA_SERVICE_ID} for account ${ACCOUNT_ID} success"
  exit 1
fi

curl -s -H "Authorization: Bearer ${AUTH_TOKEN}" \
    -H "grpc-metadata-x-prex-quota: Bearer ${SOLD_QUOTA_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{
      "account_id": '${ACCOUNT_ID}',
      "sell_order": {
        "seller_id": '${ACCOUNT_ID}',
        "service_id": '${SERVICE_ID}',
        "ask_price": '${ASK_PRICE}',
        "quantity": '${SELL_QUANTITY}',
        "order_expire_time": "'$(date -d "1 day" +"%Y-%m-%dT%H:%M:%SZ")'",
        "service_expire_time": "'$(date -d "1 day" +"%Y-%m-%dT%H:%M:%SZ")'"
      }
    }' \
    -X POST "http://localhost:3000/v1/services/${SERVICE_ID}/sell-orders:create"

