package blockstore

/*
TODO(garet) reenable cass eventually

type BlockFlags uint8

type RawBlock struct {
	Hash       []byte `json:"hash"`        // primary key
	ParentHash []byte `json:"parent_hash"` // hash of parent
	Number     int64  `json:"number"`
	Chain      string `json:"chain"`

	Flags BlockFlags `json:"flags"`

	Header   ethtypes.BlockHeader `json:"header"`
	Logs     ethtypes.Logs        `json:"logs"`
	Receipts ethtypes.Receipts    `json:"receipts"`
}

var (
	RawBlocksTable = table.New(cqlmigrations.RawBlockMetadata)
)

func Up(slog *slog.Logger, sessionP *gocqlx.Session) error {
	session := *sessionP
	log := func(ctx context.Context, session gocqlx.Session, ev migrate.CallbackEvent, name string) error {
		slog.Info("CQL Migration Event", "id", ev, "name", name)
		return nil
	}
	reg := migrate.CallbackRegister{}
	for i := 1; i <= 2; i++ {
		reg.Add(migrate.BeforeMigration, fmt.Sprintf("m%d,cql", 1), log)
		reg.Add(migrate.AfterMigration, fmt.Sprintf("m%d,cql", 1), log)
	}
	migrate.Callback = reg.Callback

	// First run prints data
	if err := migrate.FromFS(context.Background(), session, cqlmigrations.Files); err != nil {
		return err
	}

	// Second run skips the processed files
	if err := migrate.FromFS(context.Background(), session, cqlmigrations.Files); err != nil {
		return err
	}

	return nil
}

*/
