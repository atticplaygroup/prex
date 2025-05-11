-- +migrate Up
CREATE TABLE accounts (
  account_id BIGSERIAL PRIMARY KEY,
  username VARCHAR(64) NOT NULL UNIQUE,
  password TEXT NOT NULL,
  balance BIGINT NOT NULL CHECK (balance >= 0),
  create_time TIMESTAMPTZ NOT NULL,
  expire_time TIMESTAMPTZ NOT NULL,
  privilege VARCHAR(10) NOT NULL,
  CHECK (privilege IN ('user', 'admin')),
  -- to prevent timestamp overflow
  CHECK (expire_time < '10000-12-31')
);

CREATE TABLE deposits (
  deposit_id BIGSERIAL PRIMARY KEY,
  transaction_digest VARCHAR(48) NOT NULL UNIQUE,
  epoch BIGINT NOT NULL,
  account_id BIGINT NOT NULL
);

CREATE TABLE services (
  service_id BIGSERIAL PRIMARY KEY,
  service_global_id UUID UNIQUE NOT NULL,
  display_name TEXT NOT NULL,
  token_policy_config TEXT NOT NULL,
  token_policy_type TEXT NOT NULL
);

CREATE TABLE active_orders (
  order_id BIGSERIAL PRIMARY KEY,
  service_id BIGINT NOT NULL,
  seller_id BIGINT NOT NULL,
  ask_price BIGINT NOT NULL CHECK (ask_price >= 0),
  quantity BIGINT NOT NULL CHECK (quantity >= 0),
  create_time TIMESTAMPTZ NOT NULL,
  order_expire_time TIMESTAMPTZ NOT NULL,
  service_expire_time TIMESTAMPTZ NOT NULL,
  FOREIGN KEY (seller_id) REFERENCES accounts (account_id) ON DELETE CASCADE,
  FOREIGN KEY (service_id) REFERENCES services (service_id) ON DELETE CASCADE,
  CHECK (order_expire_time > create_time),
  CHECK (service_expire_time >= order_expire_time)
);

CREATE TABLE fulfilled_orders (
  order_fulfillment_id BIGSERIAL PRIMARY KEY,
  service_id BIGINT NOT NULL,
  order_id BIGINT NOT NULL,
  buyer_id BIGINT NOT NULL,
  seller_id BIGINT NOT NULL,
  deal_price BIGINT NOT NULL CHECK (deal_price >= 0),
  deal_quantity BIGINT NOT NULL CHECK (deal_quantity >= 0),
  deal_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  remaining_quantity BIGINT NOT NULL CHECK (remaining_quantity >= 0),
  service_expire_time TIMESTAMPTZ NOT NULL,
  CHECK (buyer_id != seller_id),
  FOREIGN KEY (buyer_id) REFERENCES accounts (account_id) ON DELETE CASCADE
);

CREATE TABLE claimed_orders (
  order_claim_id BIGSERIAL PRIMARY KEY,
  order_fulfillment_id BIGINT NOT NULL,
  audience_address BYTEA NOT NULL,
  claim_quantity BIGINT NOT NULL CHECK (claim_quantity >= 0),
  claim_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (order_fulfillment_id) REFERENCES fulfilled_orders (order_fulfillment_id) ON DELETE CASCADE
);

CREATE TABLE processing_withdrawals (
  processing_withdrawal_id BIGSERIAL PRIMARY KEY,
  transaction_digest TEXT UNIQUE NOT NULL,
  transaction_bytes_base64 TEXT NOT NULL,
  total_priority_fee BIGINT NOT NULL CHECK(total_priority_fee >= 0),
  withdrawal_status VARCHAR(10) NOT NULL CHECK(withdrawal_status IN ('processing', 'succeeded')),
  create_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE withdrawals (
  withdrawal_id BIGSERIAL PRIMARY KEY,
  account_id BIGINT NOT NULL,
  withdraw_address BYTEA NOT NULL UNIQUE,
  amount BIGINT NOT NULL CHECK (amount >= 0),
  priority_fee BIGINT NOT NULL CHECK (priority_fee >= 0),
  processing_withdrawal_id BIGINT,
  create_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  -- Cannot delete account if have pending withdrawals
  FOREIGN KEY (account_id) REFERENCES accounts (account_id),
  FOREIGN KEY (processing_withdrawal_id) REFERENCES processing_withdrawals(processing_withdrawal_id) ON DELETE CASCADE
);

CREATE INDEX ON accounts (username);
CREATE INDEX ON accounts (create_time);
CREATE INDEX ON accounts (expire_time);

CREATE INDEX ON active_orders (seller_id);
CREATE INDEX ON active_orders (ask_price);
CREATE INDEX ON active_orders (create_time);
CREATE INDEX ON active_orders (order_expire_time);
CREATE INDEX ON active_orders (service_expire_time);

CREATE INDEX ON fulfilled_orders (order_id);
CREATE INDEX ON fulfilled_orders (buyer_id);
CREATE INDEX ON fulfilled_orders (seller_id);
CREATE INDEX ON fulfilled_orders (buyer_id, seller_id);
CREATE INDEX ON fulfilled_orders (deal_time);

CREATE INDEX ON claimed_orders (order_fulfillment_id);
CREATE INDEX ON claimed_orders (audience_address);
CREATE INDEX ON claimed_orders (claim_time);

CREATE INDEX ON withdrawals (account_id);
CREATE INDEX ON withdrawals (withdraw_address);
CREATE INDEX ON withdrawals (processing_withdrawal_id);
CREATE INDEX ON withdrawals (priority_fee);
CREATE INDEX ON withdrawals (create_time);

CREATE INDEX ON processing_withdrawals (transaction_digest);
CREATE INDEX ON processing_withdrawals (total_priority_fee);

CREATE INDEX ON services (service_global_id);

-- +migrate Down
DROP TABLE withdrawals;
DROP TABLE processing_withdrawals;
DROP TABLE claimed_orders;
DROP TABLE fulfilled_orders;
DROP TABLE active_orders;
DROP TABLE services;
DROP TABLE deposits;
DROP TABLE accounts;
