package ethtypes

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Logs []*Log

type Receipts []*TransactionReceipt

type LogAddressTopics struct {
	Address common.Address `json:"address"`
	Topics  []common.Hash  `json:"topics"`
}

type Log struct {
	LogAddressTopics

	Data             hexutil.Bytes  `json:"data"`
	TransactionHash  common.Hash    `json:"transactionHash"`
	TransactionIndex hexutil.Uint64 `json:"transactionIndex"`
	BlockHash        common.Hash    `json:"blockHash"`
	BlockNumber      hexutil.Uint64 `json:"blockNumber"`
	LogIndex         hexutil.Uint64 `json:"logIndex"`
	Removed          bool           `json:"removed"`

	Timestamp hexutil.Uint64 `json:"timestamp,omitempty"`
}

type CompiledFilter struct {
	Addresses []common.Address
	Topics    [][]common.Hash
	AddrMap   map[common.Address]struct{}
	TopicMap  []map[common.Hash]struct{}
}

func CompileFilter(addresses []common.Address, topics [][]common.Hash) *CompiledFilter {
	if len(addresses) == 0 && len(topics) == 0 {
		return nil
	}

	addrMap := make(map[common.Address]struct{}, len(addresses))
	topicMap := make([]map[common.Hash]struct{}, len(topics))
	// populate addr map
	for _, v := range addresses {
		addrMap[v] = struct{}{}
	}
	// populate topic map
	for idx, v := range topics {
		for _, vv := range v {
			if topicMap[idx] == nil {
				topicMap[idx] = make(map[common.Hash]struct{})
			}
			topicMap[idx][vv] = struct{}{}
		}
	}

	return &CompiledFilter{
		Addresses: addresses,
		Topics:    topics,
		AddrMap:   addrMap,
		TopicMap:  topicMap,
	}
}

func (c *CompiledFilter) Check(l LogAddressTopics) bool {
	if c == nil {
		return true
	}

	addrMap := c.AddrMap
	topicMap := c.TopicMap

	// check address if addrMap is not empty
	if len(addrMap) != 0 {
		if _, ok := addrMap[l.Address]; !ok {
			// not there? skip this log
			return false
		}
	}
	// if there are no topics provided, then match all, so found is true and we include
	for idx, topicSet := range topicMap {
		// if the topicSet is empty, match all as wildcard, so move on to next
		if len(topicSet) == 0 {
			continue
		}
		if idx >= len(l.Topics) {
			return false
		}
		// the topicSet isnt empty, so the topic must be included.
		if _, ok := topicSet[l.Topics[idx]]; !ok {
			// the topic wasn't found, so we should skip this log
			return false
		}
	}
	return true
}

func (c *CompiledFilter) Filter(logs Logs) Logs {
	if c == nil {
		return logs
	}

	o := make(Logs, 0, len(logs))
	for _, v := range logs {
		if c.Check(v.LogAddressTopics) {
			o = append(o, v)
		}
	}
	return o
}

func (logs Logs) Filter(addresses []common.Address, topics [][]common.Hash) Logs {
	cf := CompileFilter(addresses, topics)
	return cf.Filter(logs)
}
