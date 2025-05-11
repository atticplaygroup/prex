#!/bin/bash

set -eu

CLAIM_QUANITY="${1}"
SERVICE_ID="${2}"
AUTH_TOKEN="${3}"
ACCOUNT_ID="${4}"
IS_FREE_TOKEN="${5}"
ACTIVATE="${6}"

if [ "${IS_FREE_TOKEN}" = 1 ]; then
  CLAIM_METHOD=claimFree
else
  CLAIM_METHOD=claim
fi

# TODO: support other service providers
PAYMENT_ADDRESS=$(curl -s http://localhost:3000/v1/payment-methods | \
  jq -r '.paymentMethods[] | select(.environment=="DEVNET") .address')
[ -z ${PAYMENT_ADDRESS} ] || [ ${PAYMENT_ADDRESS} = "null" ] && (
  echo "failed to get payment address"
  exit 1
)

LIST_FULFILLED_ORDERS_RESPONSE=$(curl -s -H "Authorization: Bearer ${AUTH_TOKEN}" \
  -H "Content-Type: application/json" \
  "http://localhost:3000/v1/services/${SERVICE_ID}/fulfilled-orders?account_id=${ACCOUNT_ID}&min_remaining_quantity=1")
ORDER_NAME=$(echo "${LIST_FULFILLED_ORDERS_RESPONSE}" | \
  jq -c .fulfilledOrders[] | head -n1 | jq -r .name)
[ -z ${ORDER_NAME} ] || [ ${ORDER_NAME} = "null" ] && (
  echo "${LIST_FULFILLED_ORDERS_RESPONSE}"
  echo "failed to get fulfilled order"
  exit 1
)

QUOTA_TOKEN_RESPONSE=$(curl -s -H "Authorization: Bearer ${AUTH_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "'"${ORDER_NAME}"'",
    "audience": "'"${PAYMENT_ADDRESS}"'",
    "arg_json": "'{\\\"count\\\":"${CLAIM_QUANITY}"}'",
    "account_id": '${ACCOUNT_ID}'
  }' \
  "http://localhost:3000/v1/${ORDER_NAME}:${CLAIM_METHOD}")
QUOTA_TOKEN=$(echo ${QUOTA_TOKEN_RESPONSE} | jq -r .token)
[ -z ${QUOTA_TOKEN} ] || [ ${QUOTA_TOKEN} = "null" ] && {
  echo ${QUOTA_TOKEN_RESPONSE}
  echo "failed to get quota token for service ${SERVICE_ID}"
  exit 1
}

if [ "${ACTIVATE}" = 1 ]; then
  PARSED_QUOTA_TOKEN=$(echo ${QUOTA_TOKEN} | awk -F. '{printf $2}' | base64 -d || true)
  EXPIRE_AT=$(date -u -d @$(echo ${PARSED_QUOTA_TOKEN} | jq -r .exp) +"%Y-%m-%dT%H:%M:%SZ")
  ORDER_CLAIM_ID=$(echo ${PARSED_QUOTA_TOKEN} | jq -r .jti)
  ORDER_QUANTITY=$(echo ${PARSED_QUOTA_TOKEN} | jq -r .quota_quantity)
  # SERVICE_ID=$(echo ${PARSED_QUOTA_TOKEN} | jq -r .service_id)

  curl -s -H "Authorization: Bearer ${AUTH_TOKEN}" \
    -H "grpc-metadata-x-prex-quota: Bearer ${QUOTA_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{
    "service_id": '${SERVICE_ID}',
    "order_claim_id": '${ORDER_CLAIM_ID}',
    "quota_quantity": '${ORDER_QUANTITY}',
    "expire_at": "'"${EXPIRE_AT}"'"
  }' \
    "http://localhost:3000/v1/quota-token:activate" | jq .success >/dev/null
fi

echo "${QUOTA_TOKEN}"
