#!/bin/bash

set -eu

PREX_CONFIG_PATH="${1}"
SERVICE_ID="${2}"
BID_PRICE="${3}"
BID_QUANTITY="${4}"
USE_FREE_TOKEN="${5}"

export PREX_CONFIG_PATH

LOGIN_RESPONSE=$(bash scripts/login-account.sh "${PREX_CONFIG_PATH}")
ACCOUNT_ID=$(echo "${LOGIN_RESPONSE}" | jq -r .account.accountId)
AUTH_TOKEN=$(echo "${LOGIN_RESPONSE}" | jq -r .accessToken)

FREE_QUOTA_SERVICE_ID=1
SOLD_QUOTA_SERVICE_ID=2

if [ "${USE_FREE_TOKEN}" = 1 ]; then
  QUOTA_SERVICE_ID="${FREE_QUOTA_SERVICE_ID}"
else
  QUOTA_SERVICE_ID="${SOLD_QUOTA_SERVICE_ID}"
fi

QUOTA_TOKEN=$(claim-token.sh 1 ${QUOTA_SERVICE_ID} ${AUTH_TOKEN} ${ACCOUNT_ID} ${USE_FREE_TOKEN} 1)
if [ -z "${QUOTA_TOKEN}" ] || [ "${QUOTA_TOKEN}" = "null" ]; then
  echo "claim service ${QUOTA_SERVICE_ID} for account ${ACCOUNT_ID} failed"
  exit 1
fi

# Buy sold token
curl -s -H "Authorization: Bearer ${AUTH_TOKEN}" \
    -H "grpc-metadata-x-prex-quota: Bearer ${QUOTA_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{
      "account_id": '${ACCOUNT_ID}',
      "quantity": '"${BID_QUANTITY}"',
      "bid_price": '"${BID_PRICE}"',
      "min_expire_time": "'$(date -u +"%Y-%m-%dT%H:%M:%SZ")'"
    }' \
    -X POST "http://localhost:3000/v1/services/${SERVICE_ID}/sell-orders:match"
