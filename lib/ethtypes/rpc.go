package ethtypes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type BlockNumber int64

const (
	LatestExecutedBlockNumber = BlockNumber(-5)
	FinalizedBlockNumber      = BlockNumber(-4)
	SafeBlockNumber           = BlockNumber(-3)
	PendingBlockNumber        = BlockNumber(-2)
	LatestBlockNumber         = BlockNumber(-1)
	EarliestBlockNumber       = BlockNumber(0)
)

func trimChar(data []byte, chars []byte) []byte {
	for _, char := range chars {
		if len(data) >= 2 && data[0] == char && data[len(data)-1] == char {
			data = data[1 : len(data)-1]
			return data
		}
	}
	return data
}

func (bn BlockNumber) MarshalJSON() ([]byte, error) {
	switch bn {
	case EarliestBlockNumber:
		return []byte(`"earliest"`), nil
	case LatestBlockNumber:
		return []byte(`"latest"`), nil
	case PendingBlockNumber:
		return []byte(`"pending"`), nil
	case SafeBlockNumber:
		return []byte(`"safe"`), nil
	case FinalizedBlockNumber:
		return []byte(`"finalized"`), nil
	case LatestExecutedBlockNumber:
		return []byte(`"latestExecuted"`), nil
	default:
		return []byte(`"` + hexutil.EncodeUint64(uint64(bn)) + `"`), nil
	}
}

// UnmarshalJSON parses the given JSON fragment into a BlockNumber. It supports:
// - "latest", "earliest", "pending", "safe", or "finalized" as string arguments
// - the block number
// Returned errors:
// - an invalid block number error when the given argument isn't a known strings
// - an out of range error when the given block number is either too little or too large
func (bn *BlockNumber) UnmarshalJSON(data []byte) error {
	input := string(trimChar(bytes.TrimSpace(data), []byte{'"', '\''}))
	switch input {
	case "earliest":
		*bn = EarliestBlockNumber
		return nil
	case "latest":
		*bn = LatestBlockNumber
		return nil
	case "pending":
		*bn = PendingBlockNumber
		return nil
	case "safe":
		*bn = SafeBlockNumber
		return nil
	case "finalized":
		*bn = FinalizedBlockNumber
		return nil
	case "latestExecuted":
		*bn = LatestExecutedBlockNumber
		return nil
	case "null":
		*bn = LatestBlockNumber
		return nil
	}
	// Try to parse it as a number
	blckNum, err := strconv.ParseUint(input, 10, 64)
	if err != nil {
		a := new(big.Int)
		a.SetString(input, 0)
		blckNum = a.Uint64()
	}
	if blckNum > math.MaxInt64 {
		return fmt.Errorf("block number larger than int64")
	}
	*bn = BlockNumber(blckNum)
	return nil
}

func (bn BlockNumber) Int64() int64 {
	return int64(bn)
}

type BlockNumberOrHash struct {
	BlockNumber      *BlockNumber `json:"blockNumber,omitempty"`
	BlockHash        *common.Hash `json:"blockHash,omitempty"`
	RequireCanonical bool         `json:"requireCanonical,omitempty"`
}

func (bnh *BlockNumberOrHash) UnmarshalJSON(data []byte) error {
	type erased BlockNumberOrHash
	e := erased{}
	err := json.Unmarshal(data, &e)
	if err == nil {
		if e.BlockNumber != nil && e.BlockHash != nil {
			return fmt.Errorf("cannot specify both BlockHash and BlockNumber, choose one or the other")
		}
		if e.BlockNumber == nil && e.BlockHash == nil {
			return fmt.Errorf("at least one of BlockNumber or BlockHash is needed if a dictionary is provided")
		}
		bnh.BlockNumber = e.BlockNumber
		bnh.BlockHash = e.BlockHash
		bnh.RequireCanonical = e.RequireCanonical
		return nil
	}
	// Try simple number first
	blckNum, err := strconv.ParseUint(string(data), 10, 64)
	if err == nil {
		if blckNum > math.MaxInt64 {
			return fmt.Errorf("blocknumber too high")
		}
		bn := BlockNumber(blckNum)
		bnh.BlockNumber = &bn
		return nil
	}
	var input string
	if err := json.Unmarshal(data, &input); err != nil {
		return err
	}
	switch input {
	case "earliest":
		bn := EarliestBlockNumber
		bnh.BlockNumber = &bn
		return nil
	case "latest":
		bn := LatestBlockNumber
		bnh.BlockNumber = &bn
		return nil
	case "pending":
		bn := PendingBlockNumber
		bnh.BlockNumber = &bn
		return nil
	case "safe":
		bn := SafeBlockNumber
		bnh.BlockNumber = &bn
		return nil
	case "finalized":
		bn := FinalizedBlockNumber
		bnh.BlockNumber = &bn
		return nil
	default:
		if len(input) == 66 {
			hash := common.Hash{}
			err := hash.UnmarshalText([]byte(input))
			if err != nil {
				return err
			}
			bnh.BlockHash = &hash
			return nil
		} else {
			a := new(big.Int)
			a.SetString(input, 0)
			blckNum = a.Uint64()
			if blckNum > math.MaxInt64 {
				return fmt.Errorf("blocknumber too high")
			}
			bn := BlockNumber(blckNum)
			bnh.BlockNumber = &bn
			return nil
		}
	}
}

func (bnh *BlockNumberOrHash) Number() (BlockNumber, bool) {
	if bnh.BlockNumber != nil {
		return *bnh.BlockNumber, true
	}
	return BlockNumber(0), false
}

func (bnh *BlockNumberOrHash) Hash() (common.Hash, bool) {
	if bnh.BlockHash != nil {
		return *bnh.BlockHash, true
	}
	return common.Hash{}, false
}
