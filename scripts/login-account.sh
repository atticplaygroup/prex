#!/bin/bash

set -eu

PREX_CONFIG_PATH="${1}"

export PREX_CONFIG_PATH

ARG_JSON='{\"count\":10}'

ACCOUNT_JSON=$(prex client account)
MY_SUI_ADDRESS=$(echo ${ACCOUNT_JSON} | jq -r .address)
MY_USERNAME=$(echo ${ACCOUNT_JSON} | jq -r .username)
MY_PASSWORD=$(echo ${ACCOUNT_JSON} | jq -r .password)

LOGIN_RESPONSE=$(curl -s -X POST http://localhost:3000/v1/login \
  -d '{
    "username":"'"${MY_USERNAME}"'",
    "password":"'"${MY_PASSWORD}"'"
  }')
echo "${LOGIN_RESPONSE}"
