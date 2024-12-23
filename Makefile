


gensql:
	go run github.com/jschaf/pggen/cmd/pggen@latest gen go \
	  --query-glob 'query/*.sql' \
		--go-type 'numeric=github.com/holiman/uint256.Int' \
		--go-type 'uint256=github.com/holiman/uint256.Int' \
		--go-type 'address=gfx.cafe/open/eth-pg/src/types.Address' \
		--go-type 'ethword=gfx.cafe/open/eth-pg/src/types.Hash' \
	  --go-type '_address=gfx.cafe/open/eth-pg/src/types.Address[]' \
	  --go-type '_ethword=gfx.cafe/open/eth-pg/src/types.Hash[]' \
		--go-type 'uint64=int64'\
		--go-type 'bytea=[]byte' \
		--go-type '_bytea=gfx.cafe/open/eth-pg/src/types.ByteaArray' \
		--go-type 'access_list=[]byte' \
		--go-type 'txn_type=int' \
    --postgres-connection "user=chaindata password=chaindata host=gvm port=5432 dbname=ethpg-goerli"
