package ethtypes

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-faster/jx"
)

// The FilterQueryTopics list restricts matches to particular event topics. Each event has a list
// of topics. Topics matches a prefix of that list. An empty element slice matches any
// topic. Non-empty elements represent an alternative that matches any of the
// contained topics.
//
// Examples:
// {} or nil          matches any topic list
// {{A}}              matches topic A in first position
// {{}, {B}}          matches any topic in first position AND B in second position
// {{A}, {B}}         matches topic A in first position AND B in second position
// {{A, B}, {C, D}}   matches topic (A OR B) in first position AND (C OR D) in second position
type FilterQueryTopics [][]common.Hash

func (topics *FilterQueryTopics) UnmarshalJSON(v []byte) error {
	var raw []any
	if err := json.Unmarshal(v, &raw); err != nil {
		return err
	}

	// topics is an array consisting of strings and/or arrays of strings.
	// JSON null values are converted to common.Hash{} and ignored by the filter manager.
	if len(raw) > 0 {
		*topics = make([][]common.Hash, len(raw))
		for i, t := range raw {
			switch topic := t.(type) {
			case nil:
				// ignore topic when matching logs

			case string:
				// match specific topic
				top, err := decodeTopic(topic)
				if err != nil {
					return err
				}
				(*topics)[i] = []common.Hash{top}

			case []any:
				// or case e.g. [null, "topic0", "topic1"]
				for _, rawTopic := range topic {
					if rawTopic == nil {
						// null component, match all
						(*topics)[i] = nil
						break
					}
					if topic, ok := rawTopic.(string); ok {
						parsed, err := decodeTopic(topic)
						if err != nil {
							return err
						}
						(*topics)[i] = append((*topics)[i], parsed)
					} else {
						return fmt.Errorf("invalid topic(s)")
					}
				}
			default:
				return fmt.Errorf("invalid topic(s)")
			}
		}
	}

	return nil
}

type OneOrArray[T any] []T

func (o *OneOrArray[T]) UnmarshalJSON(b []byte) error {
	d := jx.DecodeBytes(b)
	switch d.Next() {
	case jx.Null:
		*o = OneOrArray[T]{}
		return nil
	case jx.Array:
		iter, err := d.ArrIter()
		if err != nil {
			return err
		}
		*o = (*o)[:0]
		for iter.Next() {
			raw, err := d.Raw()
			if err != nil {
				return err
			}

			var v T
			if err = json.Unmarshal(raw, &v); err != nil {
				return err
			}

			*o = append(*o, v)
		}
		return nil
	default:
		var v T
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}

		*o = OneOrArray[T]{v}
		return nil
	}
}

func (o OneOrArray[T]) MarshalJSON() ([]byte, error) {
	if len(o) == 1 {
		return json.Marshal(o[0])
	}

	return json.Marshal([]T(o))
}

var _ json.Unmarshaler = (*OneOrArray[any])(nil)
var _ json.Marshaler = (OneOrArray[any])(nil)

// FilterQuery contains options for contract log filtering.
type FilterQuery struct {
	BlockHash *common.Hash               `json:"blockHash,omitempty"` // used by eth_getLogs, return logs only from block with this hash
	FromBlock *BlockNumber               `json:"fromBlock,omitempty"` // beginning of the queried range, nil means genesis block
	ToBlock   *BlockNumber               `json:"toBlock,omitempty"`   // end of the range, nil means latest block
	Addresses OneOrArray[common.Address] `json:"address,omitempty"`   // restricts matches to events created by specific contracts
	Topics    FilterQueryTopics          `json:"topics,omitempty"`
}

/*
func (filter *FilterQuery) SplitBlockRange(bestBlock int, size int) []*FilterQuery {
	if filter.BlockHash != nil {
		// blockhash provided, return self
		return []*FilterQuery{filter}
	}
	// otherwise, it's a range query. see if from and to block are populated
	if filter.FromBlock == nil {
		filter.FromBlock = big.NewInt(int64(bestBlock))
	}
	if filter.ToBlock == nil {
		filter.ToBlock = big.NewInt(int64(bestBlock))
	}
	from := filter.FromBlock.Int64()
	to := filter.ToBlock.Int64()
	if from <= 0 {
		from = int64(bestBlock)
	}
	if to <= 0 {
		to = int64(bestBlock)
	}
	// bad query,
	if to-from < 0 {
		from, to = to, from
	}
	if to-from < 16 {
		// too small, so forward.
		thisFilter := &FilterQuery{}
		thisFilter.Addresses = filter.Addresses
		thisFilter.Topics = filter.Topics
		thisFilter.FromBlock = big.NewInt(from)
		thisFilter.ToBlock = big.NewInt(to)
		return []*FilterQuery{thisFilter}
	}
	// otherwise, iterate through in chunks of up to size (to - from) / size
	chunkSize := (to - from) / int64(size)

	filters := make([]*FilterQuery, 0, size)
	curFrom := from
	curTo := curFrom + chunkSize
	done := false
	for !done {
		if curTo >= to {
			done = true
			curTo = to
		}
		thisFilter := &FilterQuery{}
		thisFilter.Addresses = filter.Addresses
		thisFilter.Topics = filter.Topics
		thisFilter.FromBlock = big.NewInt(curFrom)
		thisFilter.ToBlock = big.NewInt(curTo)
		filters = append(filters, thisFilter)
		curFrom = curTo + 1
		curTo = curTo + 1 + chunkSize
	}
	return filters
}
*/

/*
type filterQueryMarshaling struct {
	Addresses   []common.Addresses `json:"address,omitempty"`
	Topics    [][]common.Hash  `json:"topics,omitempty"`
	BlockHash *common.Hash     `json:"blockHash,omitempty"`
	FromBlock *string          `json:"fromBlock,omitempty"`
	ToBlock   *string          `json:"toBlock,omitempty"`
}

func (f *FilterQuery) MarshalJSON() ([]byte, error) {
	if f.BlockHash != nil {
		if f.FromBlock != nil || f.ToBlock != nil {
			return nil, fmt.Errorf("cannot specify both BlockHash and FromBlock/ToBlock")
		}
	}
	return json.Marshal(toFilterArg(f))
}

func toFilterArg(q *FilterQuery) any {
	o := &filterQueryMarshaling{}
	o.Addresses = q.Addresses
	o.Topics = q.Topics
	if q.BlockHash != nil {
		o.BlockHash = q.BlockHash
	} else {
		if q.FromBlock == nil {
			o.FromBlock = ptr.String("0x0")
		} else {
			o.FromBlock = ptr.String(toBlockNumArg(q.FromBlock))
		}
		o.ToBlock = ptr.String(toBlockNumArg(q.ToBlock))
	}
	return o
}

func toBlockNumArg(number *big.Int) string {
	if number == nil {
		return "latest"
	}
	pending := big.NewInt(-1)
	if number.Cmp(pending) == 0 {
		return "pending"
	}
	latest := big.NewInt(-2)
	if number.Cmp(latest) == 0 {
		return "latest"
	}
	return hexutil.EncodeBig(number)
}

// UnmarshalJSON sets *args fields with given data.
func (args *FilterQuery) UnmarshalJSON(data []byte) error {
	type input struct {
		BlockHash *common.Hash `json:"blockHash"`
		FromBlock *BlockNumber `json:"fromBlock"`
		ToBlock   *BlockNumber `json:"toBlock"`
		Addresses any          `json:"address"`
		Topics    []any        `json:"topics"`
	}

	var raw input
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if raw.BlockHash != nil {
		if raw.FromBlock != nil || raw.ToBlock != nil {
			// BlockHash is mutually exclusive with FromBlock/ToBlock criteria
			return fmt.Errorf("cannot specify both BlockHash and FromBlock/ToBlock, choose one or the other")
		}
		args.BlockHash = raw.BlockHash
	} else {
		if raw.FromBlock != nil {
			args.FromBlock = big.NewInt(raw.FromBlock.Int64())
		}

		if raw.ToBlock != nil {
			args.ToBlock = big.NewInt(raw.ToBlock.Int64())
		}
	}

	args.Addresses = []common.Addresses{}

	if raw.Addresses != nil {
		// raw.Addresses can contain a single address or an array of addresses
		switch rawAddr := raw.Addresses.(type) {
		case []any:
			for i, addr := range rawAddr {
				if strAddr, ok := addr.(string); ok {
					addr, err := decodeAddress(strAddr)
					if err != nil {
						return fmt.Errorf("invalid address at index %d: %w", i, err)
					}
					args.Addresses = append(args.Addresses, addr)
				} else {
					return fmt.Errorf("non-string address at index %d", i)
				}
			}
		case string:
			addr, err := decodeAddress(rawAddr)
			if err != nil {
				return fmt.Errorf("invalid address: %w", err)
			}
			args.Addresses = []common.Addresses{addr}
		default:
			return errors.New("invalid addresses in query")
		}
	}

	// topics is an array consisting of strings and/or arrays of strings.
	// JSON null values are converted to common.Hash{} and ignored by the filter manager.
	if len(raw.Topics) > 0 {
		args.Topics = make([][]common.Hash, len(raw.Topics))
		for i, t := range raw.Topics {
			switch topic := t.(type) {
			case nil:
				// ignore topic when matching logs

			case string:
				// match specific topic
				top, err := decodeTopic(topic)
				if err != nil {
					return err
				}
				args.Topics[i] = []common.Hash{top}

			case []any:
				// or case e.g. [null, "topic0", "topic1"]
				for _, rawTopic := range topic {
					if rawTopic == nil {
						// null component, match all
						args.Topics[i] = nil
						break
					}
					if topic, ok := rawTopic.(string); ok {
						parsed, err := decodeTopic(topic)
						if err != nil {
							return err
						}
						args.Topics[i] = append(args.Topics[i], parsed)
					} else {
						return fmt.Errorf("invalid topic(s)")
					}
				}
			default:
				return fmt.Errorf("invalid topic(s)")
			}
		}
	}

	return nil
}

func decodeAddress(s string) (common.Addresses, error) {
	b, err := hexutil.Decode(s)
	if err == nil && len(b) != common.AddressLength {
		err = fmt.Errorf("hex has invalid length %d after decoding; expected %d for address", len(b), common.AddressLength)
	}
	return common.BytesToAddress(b), err
}
*/

func decodeTopic(s string) (common.Hash, error) {
	b, err := hexutil.Decode(s)
	if err == nil && len(b) != common.HashLength {
		err = fmt.Errorf("hex has invalid length %d after decoding; expected %d for topic", len(b), common.HashLength)
	}
	return common.BytesToHash(b), err
}

type SubscriptionFilterQuery struct {
	Addresses OneOrArray[common.Address] `json:"address,omitempty"`
	Topics    FilterQueryTopics          `json:"topics,omitempty"`
}
