package test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/util/go/generic"
	"github.com/ethereum/go-ethereum/common"

	"github.com/gfx-labs/venn/lib/ethtypes"
)

const chain = "zksync"
const blockNumber = 31080299

var filters = []ethtypes.SubscriptionFilterQuery{
	{
		Topics: [][]common.Hash{
			{
				common.HexToHash("0x783cca1c0412dd0d695e784568c96da2e9c22ff989357a2e8b1d9b2b4e6b7118"),
				common.HexToHash("0x98636036cb66a9c19a37435efc1e90142190214e8abeb821bdba3f2990dd4c95"),
				common.HexToHash("0x7a53080ba414158be7ec69b987b5fb7d07dee101fe85488f0853ae16239d0bde"),
				common.HexToHash("0x0c396cd989a39f4459b5fa1aed6a9a8dcdbc45908acfd67e028cd568da98982c"),
				common.HexToHash("0x70935338e69775456a85ddef226c395fb668b63fa0115f5f20610b388e6ca9c0"),
				common.HexToHash("0xc42079f94a6350d7e6235f29174924f928cc2ac818eb64fed8004e115fbcca67"),
				common.HexToHash("0xbdbdb71d7860376ba52b25a5028beea23581364a40522f6bcfb86bb1f2dca633"),
				common.HexToHash("0x559322b66708f28c8fe1386f8cd96634d0c2fdb250d925b71a6c15e11e26069c"),
				common.HexToHash("0x61e6e7599d7fb0402072db6fa40efca86866f1c441dc2515a79575743ab59cf7"),
				common.HexToHash("0x7694d48c5e21d47a725e1e62a0159727e60d57f1e42d9f83e30199fffaefc8ac"),
				common.HexToHash("0x74ebbab24ee8774729982ac100f931306b160f431f7f1483cc380186d56754bc"),
				common.HexToHash("0x250d91a5317f78e9f385385f5e6073f9a759b17cfa6f5d2374163436fbb44ae4"),
			},
		},
	},
	{
		Addresses: []common.Address{
			common.HexToAddress("0x0616e5762c1E7Dc3723c50663dF10a162D690a86"),
		},
		Topics: [][]common.Hash{
			{
				common.HexToHash("0x3067048beee31b25b2f1681f88dac838c8bba36af25bfb2b7cf7473a5847e35f"),
				common.HexToHash("0x26f6a048ee9138f2c0ce266f322cb99228e8d619ae2bff30c67f8dcf9d2377b4"),
				common.HexToHash("0x40d0efd1a53d60ecbf40971b9daf7dc90178c3aadc7aab1765632738fa8b8f01"),
				common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"),
			},
		},
	},
}

func Test(t *testing.T) {
	expected, err := jrpc.Dial(fmt.Sprintf("https://venn.apiary.software/%s", chain))
	if err != nil {
		t.Error(err)
		return
	}
	actual, err := jrpc.Dial(fmt.Sprintf("https://venn.staging.gfx.town/%s", chain))
	if err != nil {
		t.Error(err)
		return
	}

	var bn = ethtypes.BlockNumber(blockNumber)

	var expectedBlock any
	if err = expected.Do(context.Background(), &expectedBlock, "eth_getBlockByNumber", []any{bn, true}); err != nil {
		t.Error(err)
		return
	}

	var actualBlock any
	if err = actual.Do(context.Background(), &actualBlock, "eth_getBlockByNumber", []any{bn, true}); err != nil {
		t.Error(err)
		return
	}

	if !reflect.DeepEqual(expectedBlock, actualBlock) {
		t.Error("expected block != actual block")
		return
	}

	for _, filter := range filters {
		filterQuery := ethtypes.FilterQuery{
			FromBlock: &bn,
			ToBlock:   &bn,
			Addresses: filter.Addresses,
			Topics:    filter.Topics,
		}

		var expectedLogs any
		if err = expected.Do(context.Background(), &expectedLogs, "eth_getLogs", []any{filterQuery}); err != nil {
			t.Error(err)
			return
		}

		var actualLogs any
		if err = actual.Do(context.Background(), &actualLogs, "eth_getLogs", []any{filterQuery}); err != nil {
			t.Error(err)
			return
		}

		if !reflect.DeepEqual(expectedLogs, actualLogs) {
			t.Error("expected logs != actual logs")
			t.Errorf("for query: %s", generic.Must(json.Marshal(filterQuery)))
			t.Errorf("expected: %v", expectedLogs)
			t.Errorf("actual: %v", actualLogs)
			return
		}
	}
}
