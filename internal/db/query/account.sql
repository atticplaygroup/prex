-- name: UpsertAccount :one
INSERT INTO accounts (
  username,
  password,
  balance,
  privilege,
  create_time,
  expire_time
) VALUES (
  @username, @password, @balance, @privilege, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP + @ttl::interval
)
ON CONFLICT (username) DO UPDATE SET
  balance = accounts.balance + EXCLUDED.balance,
  expire_time = accounts.expire_time + @ttl::interval
RETURNING *
;

-- name: DeleteInvalidAccounts :many
DELETE FROM accounts
WHERE
  expire_time < CURRENT_TIMESTAMP
RETURNING account_id
;

-- name: ChangeBalance :one
UPDATE accounts
SET balance = balance + @balance_change
WHERE account_id = @account_id
RETURNING *
;

-- name: GetAccount :one
SELECT
  *
FROM accounts
WHERE username = @username
;

-- name: QueryBalanceForShare :one
SELECT *
FROM accounts
WHERE account_id = @account_id
AND expire_time > CURRENT_TIMESTAMP
FOR SHARE
;

-- name: QueryBalance :one
SELECT *
FROM accounts
WHERE account_id = @account_id
AND expire_time > CURRENT_TIMESTAMP
;

-- name: AddDepositRecord :one
INSERT INTO deposits (
  transaction_digest,
  epoch,
  account_id
) VALUES (
  $1, $2, $3
)
RETURNING *
;
