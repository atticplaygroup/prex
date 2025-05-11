-- name: CreateService :one
INSERT INTO services (
  service_global_id,
  display_name,
  token_policy_config,
  token_policy_type
) VALUES (
  $1, $2, $3, $4
)
RETURNING *
;

-- name: RemoveService :one
DELETE FROM services
WHERE service_id = $1
RETURNING *
;

-- name: FindServiceByGlobalId :one
SELECT * FROM services
WHERE service_global_id = $1
;

-- name: QueryService :one
SELECT * FROM services
WHERE service_id = $1
;

-- name: ListServices :many
SELECT * FROM services
WHERE service_id >= $1
ORDER BY service_id
LIMIT $2
OFFSET $3
;
