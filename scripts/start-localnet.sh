which sui || echo "Sui binary not found in PATH. Install at https://docs.sui.io/guides/developer/getting-started/sui-install#install-binaries"

RUST_LOG='off,sui_node=info' sui start --force-regenesis \
  --with-faucet \
  --with-graphql \
  --pg-port=5432 --pg-host=db \
  --pg-db-name=sui_indexer_v2 --with-indexer \
  --pg-user=postgres \
  --pg-password=postgres
