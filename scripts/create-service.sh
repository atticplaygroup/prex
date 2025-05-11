#!/bin/bash

set -eu

PREX_CONFIG_PATH="${1}"

export PREX_CONFIG_PATH

FREE_QUOTA_SERVICE_ID=1
SOLD_QUOTA_SERVICE_ID=2

LOGIN_RESPONSE=$(bash scripts/login-account.sh "${PREX_CONFIG_PATH}")
ACCOUNT_ID=$(echo "${LOGIN_RESPONSE}" | jq -r .account.accountId)
AUTH_TOKEN=$(echo "${LOGIN_RESPONSE}" | jq -r .accessToken)
echo "${AUTH_TOKEN}"

USER_SOLD_QUOTA_TOKEN=$(claim-token.sh 1 ${SOLD_QUOTA_SERVICE_ID} ${AUTH_TOKEN} ${ACCOUNT_ID} 0 1)
echo "claim service ${SOLD_QUOTA_SERVICE_ID} for account ${ACCOUNT_ID} success"

# use sold token to create service
SERVICE_GLOBAL_ID=$(cat /proc/sys/kernel/random/uuid)
echo "${SERVICE_GLOBAL_ID}"

curl -H "Authorization: Bearer ${AUTH_TOKEN}" \
  -H "grpc-metadata-x-prex-quota: Bearer ${USER_SOLD_QUOTA_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "product_token_policy": {
      "unit_price":1
    },
    "globalId": "'"${SERVICE_GLOBAL_ID}"'",
    "display_name": "i am an example service"
}' -X POST http://localhost:3000/v1/services:create
