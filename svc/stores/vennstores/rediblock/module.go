package rediblock

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"gfx.cafe/util/go/generic"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/lib/stores/blockstore"
	"gfx.cafe/gfx/venn/svc/services/redi"
)

type Params struct {
	fx.In

	Chains map[string]*config.Chain
	Log    *slog.Logger
	Redi   *redi.Redis
}

type Result struct {
	fx.Out

	Rediblock *Rediblock `optional:"true"`
}

type Rediblock struct {
	log  *slog.Logger
	redi *redi.Redis
}

func New(params Params) (r Result, err error) {
	if params.Redi == nil {
		params.Log.Info("rediblock disabled", "reason", "no redis")
		return
	}
	r.Rediblock = &Rediblock{
		log:  params.Log,
		redi: params.Redi,
	}
	return r, nil
}

var checkReorgScript = redis.NewScript(`
	for i, key in ipairs(KEYS) do
		local actual = redis.call('GET', key)
		local expected = ARGV[i]

		if actual and actual ~= expected then
			return actual
		end
	end

	return ""
`)

var addEntriesScript = redis.NewScript(`
	redis.replicate_commands()

	local head = tonumber(redis.call('GET', KEYS[1]))
	if head == nil then
		head = 0
	end

	local secondsPerBlock = ARGV[1]

	local i = 2
	local j = 2
	while true do
		local byHashValue = KEYS[i]
		local byHashNumber = KEYS[i+1]
		local byNumberValue = KEYS[i+2]
		local byNumberHash = KEYS[i+3]
		i = i + 4

		if not byHashValue then
			break
		end

		local value = ARGV[j]
		local number = tonumber(ARGV[j+1])
		local hash = ARGV[j+2]
		j = j + 3

		if number > head then
			head = number
			redis.call('SET', KEYS[1], number)
		end

		local behind = (head - number) + 1
		if behind < 1 then
			behind = 1
		end

		local exp = math.floor(behind * secondsPerBlock)
		if exp < 1 then
			exp = 1
		end

		if exp > 3600 then
		  exp = 3600
		end

		redis.call('SETEX', byHashValue, exp, value)
		redis.call('SETEX', byHashNumber, exp, number)
		redis.call('SETEX', byNumberValue, exp, value)
		redis.call('SETEX', byNumberHash, exp, hash)
	end

	return head
`)

func (s *Rediblock) namespace(chain *config.Chain) string {
	return "venn" + ":" + s.redi.Namespace() + ":{" + chain.Name + "}"
}

func (s *Rediblock) Get(ctx context.Context, chain *config.Chain, typ blockstore.EntryType, query blockstore.Query) ([]*blockstore.Entry, error) {
	switch q := query.(type) {
	case blockstore.QueryHash:
		pipeline := s.redi.C().Pipeline()

		valueCmd := pipeline.Get(ctx,
			fmt.Sprintf("%s:entries:by_hash:%d:%s:value", s.namespace(chain), typ, common.Hash(q).Hex()),
		)
		numberCmd := pipeline.Get(ctx,
			fmt.Sprintf("%s:entries:by_hash:%d:%s:number", s.namespace(chain), typ, common.Hash(q).Hex()),
		)

		if _, err := pipeline.Exec(ctx); err != nil {
			return nil, err
		}

		value, err := valueCmd.Bytes()
		if err != nil {
			return nil, err
		}

		return []*blockstore.Entry{
			{
				BlockHash:   common.Hash(q),
				BlockNumber: hexutil.Uint64(generic.Must(numberCmd.Uint64())),
				Value:       value,
			},
		}, nil
	case blockstore.QueryRange:
		if q.End-q.Start < 0 {
			return nil, nil
		}

		pipeline := s.redi.C().Pipeline()

		commands := make([]*redis.StringCmd, 0, (q.End-q.Start+1)*2)
		for i := q.Start; i <= q.End; i++ {
			commands = append(commands,
				pipeline.Get(ctx,
					fmt.Sprintf("%s:entries:by_number:%d:%d:value", s.namespace(chain), typ, uint64(i)),
				),
				pipeline.Get(ctx,
					fmt.Sprintf("%s:entries:by_number:%d:%d:hash", s.namespace(chain), typ, uint64(i)),
				),
			)
		}

		if _, err := pipeline.Exec(ctx); err != nil {
			return nil, err
		}

		results := make([]*blockstore.Entry, 0, q.End-q.Start+1)
		for i := 0; i < len(commands)/2; i++ {
			value, err := commands[i*2].Bytes()
			if err != nil {
				return nil, err
			}
			results = append(results,
				&blockstore.Entry{
					BlockHash:   common.HexToHash(commands[i*2+1].Val()),
					BlockNumber: q.Start + hexutil.Uint64(i),
					Value:       value,
				},
			)
		}

		return results, nil
	default:
		return nil, errors.New("unknown query")
	}
}

func (s *Rediblock) reorg(ctx context.Context, chain *config.Chain) error {
	s.log.Info("reorg detected", "chain", chain.Name)

	keys, err := s.redi.C().Keys(ctx, fmt.Sprintf("%s:entries:by_number:*", s.namespace(chain))).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			err = nil
		}
		return err
	}

	return s.redi.C().Del(ctx, keys...).Err()
}

func (s *Rediblock) Put(ctx context.Context, chain *config.Chain, typ blockstore.EntryType, entries ...*blockstore.Entry) error {
	keys := make([]string, 0, len(entries)*4+1)
	values := make([]any, 0, len(entries)*3+1)

	keys = append(keys,
		fmt.Sprintf("%s:head", s.namespace(chain)),
	)

	values = append(values,
		int(math.Max(1, chain.BlockTimeSeconds)),
	)

	var reorgKeys []string
	var reorgValues []any

	for _, entry := range entries {
		if entry.ParentHash != nil {
			if reorgKeys == nil {
				reorgKeys = make([]string, 0, len(entries))
				reorgValues = make([]any, 0, len(entries))
			}

			reorgKeys = append(reorgKeys,
				fmt.Sprintf("%s:entries:by_number:%d:%d:hash", s.namespace(chain), typ, uint64(entry.BlockNumber-1)),
			)
			reorgValues = append(reorgValues,
				entry.ParentHash.Hex(),
			)
		}

		keys = append(keys,
			fmt.Sprintf("%s:entries:by_hash:%d:%s:value", s.namespace(chain), typ, entry.BlockHash.Hex()),
			fmt.Sprintf("%s:entries:by_hash:%d:%s:number", s.namespace(chain), typ, entry.BlockHash.Hex()),
			fmt.Sprintf("%s:entries:by_number:%d:%d:value", s.namespace(chain), typ, uint64(entry.BlockNumber)),
			fmt.Sprintf("%s:entries:by_number:%d:%d:hash", s.namespace(chain), typ, uint64(entry.BlockNumber)),
		)
		values = append(values,
			[]byte(entry.Value),
			uint64(entry.BlockNumber),
			entry.BlockHash.Hex(),
		)
	}

	// check for reorgs
	if len(reorgKeys) != 0 {
		reorg, err := checkReorgScript.Run(ctx, s.redi.C(), reorgKeys, reorgValues...).Text()
		if err != nil {
			return err
		}
		if reorg != "" {
			if err = s.reorg(ctx, chain); err != nil {
				return err
			}
		}
	}

	err := addEntriesScript.Run(ctx, s.redi.C(), keys, values...).Err()
	if errors.Is(err, redis.Nil) {
		err = nil
	}

	return err
}

var _ blockstore.Store = (*Rediblock)(nil)
