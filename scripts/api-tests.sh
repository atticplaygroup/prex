#!/bin/bash

set -eu

TEMP_DIR=$(mktemp -d)

sql-migrate down
sql-migrate up

PREX_URL=localhost:3000

ADMIN_ACCOUNT=$(./bin/prex client account --config=${TEMP_DIR}/admin.yml)
ADMIN_USERNAME=$(echo "${ADMIN_ACCOUNT}" | jq -r .username)
ADMIN_PASSWORD=$(echo "${ADMIN_ACCOUNT}" | jq -r .password)

export ADMIN_USERNAME
export ADMIN_PASSWORD

ENABLE_PREX_QUOTA_LIMITER=1 ENABLE_SERVICE_REGISTRATION_WHITELIST=1 ./bin/prex server start &
PREX_PID=$!

./bin/prex server gateway &
GATEWAY_PID=$!

curl -s "http://${PREX_URL}/v1/ping" | jq .pong || exit 1

# TODO: get from service
FREE_QUOTA_SERVICE_ID=1
SOLD_QUOTA_SERVICE_ID=2

bash scripts/register-account.sh ${TEMP_DIR}/seller.yml > /dev/null
bash scripts/register-account.sh ${TEMP_DIR}/buyer.yml > /dev/null

echo "register accounts success"

ASK_PRICE=10
QUANTITY=100000
bash scripts/place-sell-order.sh ${TEMP_DIR}/admin.yml ${SOLD_QUOTA_SERVICE_ID} ${ASK_PRICE} ${QUANTITY}

echo "Admin place sold quota order success"

bash scripts/create-service.sh ${TEMP_DIR}/admin.yml

echo "Admin create service success"

# Seller
BID_PRICE=20
BID_QUANTITY=100
USE_FREE_TOKEN=1
bash scripts/buy-token.sh ${TEMP_DIR}/seller.yml ${SOLD_QUOTA_SERVICE_ID} ${BID_PRICE} ${BID_QUANTITY} ${USE_FREE_TOKEN}

NEW_SERVICE_ID=3

ASK_PRICE=12
QUANTITY=120000
bash scripts/place-sell-order.sh ${TEMP_DIR}/seller.yml ${NEW_SERVICE_ID} ${ASK_PRICE} ${QUANTITY}

echo "Seller place sold quota order success"

# Buyer
BID_PRICE=20
BID_QUANTITY=100
USE_FREE_TOKEN=1
bash scripts/buy-token.sh ${TEMP_DIR}/buyer.yml ${SOLD_QUOTA_SERVICE_ID} ${BID_PRICE} ${BID_QUANTITY} ${USE_FREE_TOKEN}

echo "Buyer buy quota token success"

BID_PRICE=14
BID_QUANTITY=50
USE_FREE_TOKEN=0
bash scripts/buy-token.sh ${TEMP_DIR}/buyer.yml ${NEW_SERVICE_ID} ${BID_PRICE} ${BID_QUANTITY} ${USE_FREE_TOKEN}

echo "Buyer buy service token success"

LOGIN_RESPONSE=$(bash scripts/login-account.sh ${TEMP_DIR}/buyer.yml)
ACCOUNT_ID=$(echo "${LOGIN_RESPONSE}" | jq -r .account.accountId)
AUTH_TOKEN=$(echo "${LOGIN_RESPONSE}" | jq -r .accessToken)
CLAIM_QUANTITY=30
IS_FREE_TOKEN=0
ACTIVATE=0
bash scripts/claim-token.sh ${CLAIM_QUANTITY} ${NEW_SERVICE_ID} ${AUTH_TOKEN} ${ACCOUNT_ID} ${IS_FREE_TOKEN} ${ACTIVATE}

echo "Buyer claim service token success"

echo "API test success"

kill ${PREX_PID}
kill ${GATEWAY_PID}

rm -r ${TEMP_DIR}
