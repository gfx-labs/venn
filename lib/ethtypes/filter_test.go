package ethtypes

/*
func TestFilterSplit(t *testing.T) {

	type testCase struct {
		orig [2]int
		amt  int
		exp  [][2]int
		res  [][2]int
	}

	runTest := func(c *testCase) {
		toSplit := &FilterQuery{
			FromBlock: BlockNumber(c.orig[0]),
			ToBlock:   BlockNumber(c.orig[1]),
		}
		for _, splt := range toSplit.SplitBlockRange(c.orig[1], c.amt) {
			c.res = append(c.res,
				[2]int{
					int(splt.FromBlock.Int64()),
					int(splt.ToBlock.Int64()),
				})
		}
	}

	cases := []*testCase{
		{
			[2]int{1, 123942},
			5,
			[][2]int{
				{1, 24789}, {24790, 49578}, {49579, 74367}, {74368, 99156}, {99157, 123942},
			},
			nil,
		},
		{
			[2]int{1, 16},
			5,
			[][2]int{
				{1, 16},
			},
			nil,
		},
		{
			[2]int{1, 17},
			5,
			[][2]int{
				{1, 4}, {5, 8}, {9, 12}, {13, 16}, {17, 17},
			},
			nil,
		},
	}

	for _, v := range cases {
		runTest(v)
	}
	for _, v := range cases {
		assert.Equal(t, v.exp, v.res)
	}

}
*/
