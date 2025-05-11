-- name: StartWithdrawal :one
INSERT INTO withdrawals (
  account_id,
  withdraw_address,
  amount,
  priority_fee
) VALUES (
  $1, $2, $3, $4
)
RETURNING *
;

-- name: CancelWithdrawalById :one
DELETE FROM withdrawals
WHERE withdrawal_id = $1
AND processing_withdrawal_id IS NULL
RETURNING withdrawal_id, account_id, amount, priority_fee
;

-- name: SelectCandidateWithdrawals :many
SELECT
  *
FROM withdrawals 
WHERE processing_withdrawal_id IS NULL
ORDER BY priority_fee DESC, create_time
LIMIT @retrieve_count
FOR UPDATE SKIP LOCKED
;

-- name: ProcessWithdrawals :many
UPDATE withdrawals
  SET
    processing_withdrawal_id = @processing_withdrawal_id
  WHERE withdrawal_id = ANY(@withdrawal_ids::bigint[])
  RETURNING *
;

-- name: SetWithdrawalBatch :one
INSERT INTO processing_withdrawals (
  transaction_digest,
  transaction_bytes_base64,
  total_priority_fee,
  withdrawal_status
) VALUES (
  $1, $2, $3, 'processing'
)
RETURNING *
;

-- name: SetWithdrawalSuccess :one
UPDATE processing_withdrawals
  SET
    withdrawal_status = 'succeeded'
  WHERE withdrawal_status = 'processing'
  AND transaction_digest = @transaction_digest
  RETURNING *
;

-- name: ListWithdrawals :many
SELECT
  *
FROM withdrawals
WHERE account_id = $1
ORDER BY create_time
LIMIT $2
OFFSET $3
;

-- name: CleanOldWithdrawals :many
DELETE FROM processing_withdrawals
WHERE create_time < @clean_time
-- 'processing' withdrawals must wait being marked to avoid losing money
AND withdrawal_status = 'succeeded'
RETURNING *
;

-- name: ListProcessingWithdrawals :many
SELECT
  *
FROM processing_withdrawals
WHERE withdrawal_status = 'processing'
ORDER BY total_priority_fee DESC, create_time
LIMIT $1
OFFSET $2
;
