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

PREX_GRPC_PORT=50051 ./bin/prex server connect &
PREX_PID=$!

./bin/prex server gateway &
GATEWAY_PID=$!

curl --fail -s "http://${PREX_URL}/v1/ping" | jq .pong || exit 1

bash scripts/register-account.sh ${TEMP_DIR}/seller.yml > /dev/null
bash scripts/register-account.sh ${TEMP_DIR}/buyer.yml > /dev/null

echo "register accounts success"

SELLER_DID=$(prex client account -c=${TEMP_DIR}/seller.yml | jq -r .username)
QUANTITY=100
SESSION_CREATION_TOKEN=$(bash scripts/buy-token.sh ${TEMP_DIR}/seller.yml ${SELLER_DID} ${QUANTITY} | jq -r .token)

[ -n "${SESSION_CREATION_TOKEN}" ] || (
    echo "failed to get session createion token"
    exit 1
)

echo "${SESSION_CREATION_TOKEN}"

echo "API test success"

kill ${PREX_PID}
kill ${GATEWAY_PID}

rm -r ${TEMP_DIR}
