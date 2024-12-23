package ethtypes

import (
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
)

type Uint256 uint256.Int

func (T *Uint256) MarshalJSON() ([]byte, error) {
	return []byte(`"` + (*uint256.Int)(T).Hex() + `"`), nil
}

func (T *Uint256) UnmarshalJSON(b []byte) error {
	return (*uint256.Int)(T).UnmarshalJSON(b)
}

var _ json.Marshaler = (*Uint256)(nil)
var _ json.Unmarshaler = (*Uint256)(nil)

type Head struct {
	Hash   common.Hash    `json:"hash"`
	Number hexutil.Uint64 `json:"number"`
}

type blockHeader struct {
	Head
	ParentHash       common.Hash    `json:"parentHash"`
	ExtraData        hexutil.Bytes  `json:"extraData"`
	Nonce            hexutil.Bytes  `json:"nonce"`
	LogsBloom        hexutil.Bytes  `json:"logsBloom"`
	GasLimit         Uint256        `json:"gasLimit"`
	WithdrawalsRoot  *common.Hash   `json:"withdrawalsRoot"`
	Timestamp        hexutil.Uint64 `json:"timestamp"`
	ReceiptsRoot     common.Hash    `json:"receiptsRoot"`
	Difficulty       Uint256        `json:"difficulty"`
	GasUsed          Uint256        `json:"gasUsed"`
	StateRoot        common.Hash    `json:"stateRoot"`
	Miner            common.Address `json:"miner"`
	MixHash          common.Hash    `json:"mixHash"`
	Sha3Uncles       common.Hash    `json:"sha3Uncles"`
	TransactionsRoot common.Hash    `json:"transactionsRoot"`
	BaseFeePerGas    Uint256        `json:"baseFeePerGas"`

	Size            Uint256           `json:"size"`
	Uncles          []any             `json:"uncles"`
	Withdrawals     []BlockWithdrawal `json:"withdrawals"`
	TotalDifficulty Uint256           `json:"totalDifficulty"`
}

type BlockHeader struct {
	blockHeader

	Transactions      []*Transaction `json:"transactions"`
	TransactionHashes []common.Hash  `json:"-"`
}

func (T *BlockHeader) UnmarshalJSON(xs []byte) error {
	x := (*struct {
		blockHeader

		Transactions      []*Transaction `json:"transactions"`
		TransactionHashes []common.Hash  `json:"-"`
	})(T)

	if err := json.Unmarshal(xs, x); err != nil {
		return err
	}

	if T.Transactions != nil {
		T.TransactionHashes = make([]common.Hash, 0, len(T.Transactions))
	}
	for _, txn := range T.Transactions {
		T.TransactionHashes = append(T.TransactionHashes, txn.Hash)
	}

	return nil
}

type TruncatedBlockHeader struct {
	blockHeader

	Transactions      []*Transaction `json:"-"`
	TransactionHashes []common.Hash  `json:"transactions"`
}

type TransactionlessBlockHeader struct {
	blockHeader
}

type BlockWithdrawal struct {
	Amount         Uint256        `json:"amount"`
	Address        common.Address `json:"address"`
	Index          hexutil.Uint64 `json:"index"`
	ValidatorIndex hexutil.Uint64 `json:"validatorIndex"`
}

type TransactionReceipt struct {
	BlockHash         common.Hash     `json:"blockHash"`
	BlockNumber       hexutil.Uint64  `json:"blockNumber"`
	ContractAddress   *common.Address `json:"contractAddress"`
	CumulativeGasUsed Uint256         `json:"cumulativeGasUsed"`
	EffectiveGasPrice Uint256         `json:"effectiveGasPrice"`
	From              common.Address  `json:"from"`
	GasUsed           Uint256         `json:"gasUsed"`
	Logs              Logs            `json:"logs"`
	LogsBloom         hexutil.Bytes   `json:"logsBloom,omitempty"`
	Status            hexutil.Uint64  `json:"status"`
	To                *common.Address `json:"to"`
	TransactionHash   common.Hash     `json:"transactionHash"`
	TransactionIndex  hexutil.Uint64  `json:"transactionIndex"`
	Type              hexutil.Uint64  `json:"type"`
}

type Transaction struct {
	GasPrice         Uint256         `json:"gasPrice"`
	ChainID          hexutil.Uint64  `json:"chainId"`
	BlockHash        common.Hash     `json:"blockHash"`
	Type             hexutil.Uint64  `json:"type"`
	Gas              Uint256         `json:"gas"`
	S                Uint256         `json:"s"`
	From             common.Address  `json:"from"`
	Hash             common.Hash     `json:"hash"`
	TransactionIndex hexutil.Uint64  `json:"transactionIndex"`
	Nonce            hexutil.Uint64  `json:"nonce"`
	Input            hexutil.Bytes   `json:"input"`
	BlockNumber      hexutil.Uint64  `json:"blockNumber"`
	To               *common.Address `json:"to"`
	V                Uint256         `json:"v"`
	R                Uint256         `json:"r"`
	Value            Uint256         `json:"value"`

	AccessList           []TransactionAccess `json:"accessList,omitempty"`
	MaxFeePerGas         Uint256             `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas Uint256             `json:"maxPriorityFeePerGas,omitempty"`
}

type TransactionAccess struct {
	Address     common.Address `json:"address"`
	StorageKeys []common.Hash  `json:"storageKeys"`
}

// type Transaction struct {
//	BlockHash            common.Hash    `json:"blockHash"`
//	BlockNumber          hexutil.Uint   `json:"blockNumber"`
//	ChainId              string         `json:"chainId"`
//	From                 common.Addresses `json:"from"`
//	Gas                  Uint256    `json:"gas"`
//	GasPrice             Uint256    `json:"gasPrice"`
//	MaxPriorityFeePerGas string         `json:"maxPriorityFeePerGas"`
//	MaxFeePerGas         string         `json:"maxFeePerGas"`
//	AccessList           []any          `json:"accessList"`
//	YParity              hexutil.Uint   `json:"yParity"`
//	Hash                 common.Hash    `json:"hash"`
//	Input                string         `json:"input"`
//	Nonce                hexutil.Uint   `json:"nonce"`
//	R                    string         `json:"r"`
//	S                    string         `json:"s"`
//	To                   common.Addresses `json:"to"`
//	TransactionIndex     hexutil.Uint   `json:"transactionIndex"`
//	Type                 hexutil.Uint   `json:"type"`
//	V                    hexutil.Uint   `json:"v"`
//	Value                Uint256    `json:"value"`
// }

/*
type Transaction map[string]any

func (t Transaction) Hash() common.Hash {
	if val, ok := t["hash"]; ok {
		if sal, ok := val.(string); ok {
			return common.HexToHash(sal)
		}
	}
	return common.Hash{}
}
*/
