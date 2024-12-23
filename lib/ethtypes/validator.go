package ethtypes

import (
	"encoding/json"
	"math/big"

	jsoniter "github.com/json-iterator/go"
)

var jpool = jsoniter.NewIterator(jsoniter.ConfigCompatibleWithStandardLibrary).Pool()

func decodeNumber(v any) int {
	switch c := v.(type) {
	case string:
		if c == "latest" || c == "pending" {
			return -1
		}
		a := new(big.Int)
		a.SetString(c, 0)
		return int(a.Int64())
	case int:
		return c
	case int64:
		return int(c)
	case float32:
		return int(c)
	case float64:
		return int(c)
	}
	// can't parse? just assume its like a latest or something
	return -1
}

func DecodeNumber(v any) int {
	return decodeNumber(v)
}

func double(v int) [2]int {
	return [2]int{v, v}
}

// Will return -2 if it is a hash, so we need an archive node
// Will return -1 if should try to get info for latest block
// Will return 0 if block does not matter for the request
// otherwise, will return the block that is required.
func ParseCallBlock(method string, params json.RawMessage) [2]int {
	iter := jpool.BorrowIterator(params)
	defer iter.Pool().ReturnIterator(iter)
	var v any
	switch method {
	// is a hash request
	case "eth_getBlockByHash",
		"eth_getBlockTransactionCountByHash",
		"eth_getUncleCountByBlockHash",
		"eth_getTransactionReceipt",
		"eth_getTransactionByBlockHashAndIndex",
		"eth_getTransactionByHash":
		return double(-2)
	case "eth_blockNumber":
		return [2]int{-1, -1}
	case // block is first arg
		"eth_getBlockTransactionCountByBlockNumberAndIndex",
		"eth_getTransactionByBlockNumberAndIndex",
		"eth_getBlockByNumber":
		if !iter.ReadArray() {
			return [2]int{-1, -1}
		}
		iter.ReadVal(&v)
		return double(decodeNumber(v))
	case // block is second arg
		"eth_estimateGas",
		"eth_createAccessList",
		"eth_getBalance",
		"eth_getTransactionCount",
		"eth_getCode",
		"eth_call":
		if !iter.ReadArray() {
			return double(-1)
		}
		iter.Skip()
		if !iter.ReadArray() {
			return double(-1)
		}
		iter.ReadVal(&v)
		return double(decodeNumber(v))
	case "eth_getProof", "eth_getStorageAt": // block is the third arg
		for i := 0; i < 2; i++ {
			if !iter.ReadArray() {
				return double(-1)
			}
			iter.Skip()
		}
		iter.ReadVal(&v)
		return double(decodeNumber(v))
	case "eth_getLogs", "erigon_getLogs": // block is in a filter object
		if !iter.ReadArray() {
			return double(-1)
		}
		// read the from and to block, if they exist
		o := [2]int{-1, -1}
		have := 0
		for s := iter.ReadObject(); s != ""; s = iter.ReadObject() {
			switch s {
			case "fromBlock":
				iter.ReadVal(&v)
				o[0] = decodeNumber(v)
				have = have + 1
			case "toBlock":
				iter.ReadVal(&v)
				o[1] = decodeNumber(v)
				have = have + 1
			default:
				iter.Read()
			}
			if have >= 2 {
				break
			}
		}
		return o
	}
	return double(0)
}
