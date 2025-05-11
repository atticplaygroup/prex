-- name: CreateOrder :one
INSERT INTO active_orders (
  seller_id,
  service_id,
  ask_price,
  quantity,
  create_time,
  order_expire_time,
  service_expire_time
) VALUES (
  $1, $2, $3, $4, CURRENT_TIMESTAMP, @order_expire_time::timestamptz, @service_expire_time::timestamptz
)
RETURNING *
;

-- name: CancelOrder :one 
DELETE FROM active_orders
WHERE order_id = $1 and seller_id = $2
RETURNING *
;

-- name: MatchOneOrder :one
WITH matched_order AS (
  SELECT 
    order_id
    , quantity
    , service_expire_time
  FROM active_orders a
  WHERE
    a.ask_price <= @bid_price AND
    quantity > 0 AND
    a.seller_id != @buyer_id AND
    a.service_id = @service_id AND
    a.service_expire_time >= @min_expire_time AND
    a.order_expire_time > CURRENT_TIMESTAMP
  ORDER BY a.ask_price ASC
  LIMIT 1
  FOR UPDATE SKIP LOCKED
), fulfilled_order AS (
  UPDATE active_orders b
  SET quantity = CASE
    WHEN b.quantity > @bid_quantity THEN
      b.quantity - @bid_quantity
    ELSE
      0
    END
  FROM matched_order
  WHERE b.order_id = matched_order.order_id
  RETURNING
    b.order_id,
    @service_id AS service_id,
    @buyer_id AS buyer_id,
    seller_id,
    b.ask_price AS deal_price,
    CASE WHEN matched_order.quantity > @bid_quantity THEN @bid_quantity ELSE matched_order.quantity END AS deal_quantity,
    matched_order.service_expire_time
)
INSERT INTO fulfilled_orders (
  order_id,
  service_id,
  buyer_id,
  seller_id,
  deal_price,
  deal_quantity,
  remaining_quantity,
  service_expire_time
)
SELECT
  order_id,
  service_id,
  buyer_id,
  seller_id,
  deal_price,
  deal_quantity,
  deal_quantity,
  service_expire_time
FROM fulfilled_order
RETURNING *
;

-- name: CleanInactiveOrders :many
DELETE FROM active_orders
WHERE quantity = 0
OR order_expire_time < CURRENT_TIMESTAMP
RETURNING *
;

-- name: CleanExpiredFulfilledOrders :many
DELETE FROM fulfilled_orders
WHERE remaining_quantity = 0
OR service_expire_time < CURRENT_TIMESTAMP
RETURNING *
;

-- name: QueryFulfilledOrder :one
SELECT * FROM fulfilled_orders
WHERE buyer_id = $1 AND order_fulfillment_id = $2
FOR UPDATE
;

-- name: ListFulfilledOrders :many
SELECT * FROM fulfilled_orders
WHERE buyer_id = $1
AND order_fulfillment_id >= $2
AND remaining_quantity >= $5
LIMIT $3
OFFSET $4
;

-- name: ListFulfilledOrdersByService :many
SELECT * FROM fulfilled_orders
WHERE buyer_id = $1
AND service_id = $2
AND order_fulfillment_id >= $3
AND remaining_quantity >= $6
LIMIT $4
OFFSET $5
;

-- name: ClaimFulfilledOrderOfQuantity :one
UPDATE fulfilled_orders
SET remaining_quantity = remaining_quantity - @claim_quantity::bigint
WHERE order_fulfillment_id = @order_fulfillment_id::bigint
RETURNING *
;

-- name: NewClaimOrder :one
INSERT INTO claimed_orders (
  order_fulfillment_id,
  audience_address,
  claim_quantity
) VALUES (
  $1, $2, $3
)
RETURNING *
;
