#!/bin/bash

set -eu

PREX_CONFIG_PATH="${1}"
AUDIENCE="${2}"
QUANTITY="${3}"

export PREX_CONFIG_PATH

LOGIN_RESPONSE=$(bash scripts/login-account.sh "${PREX_CONFIG_PATH}")
ACCOUNT_ID=$(echo "${LOGIN_RESPONSE}" | jq -r .account.accountId)
AUTH_TOKEN=$(echo "${LOGIN_RESPONSE}" | jq -r .accessToken)

curl --fail -s -H "Authorization: Bearer ${AUTH_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{
      "amount": '"${QUANTITY}"',
      "audience":  "'"${AUDIENCE}"'"
    }' \
    -X POST "http://localhost:3000/v1/buy-token"
