package ethtypes

import (
	"encoding/json"

	"github.com/gocql/gocql"
)

func (b *BlockHeader) UnmarshalCQL(info gocql.TypeInfo, data []byte) error {
	err := json.Unmarshal(data, b)
	if err != nil {
		return err
	}
	return nil
}

func (b *BlockHeader) MarshalCQL(info gocql.TypeInfo) ([]byte, error) {
	return json.Marshal(b)
}

func (b *Logs) UnmarshalCQL(info gocql.TypeInfo, data []byte) error {
	err := json.Unmarshal(data, b)
	if err != nil {
		return err
	}
	return nil
}

func (b *Logs) MarshalCQL(info gocql.TypeInfo) ([]byte, error) {
	return json.Marshal(b)
}

func (b *Receipts) UnmarshalCQL(info gocql.TypeInfo, data []byte) error {
	err := json.Unmarshal(data, b)
	if err != nil {
		return err
	}
	return nil
}

func (b *Receipts) MarshalCQL(info gocql.TypeInfo) ([]byte, error) {
	return json.Marshal(b)
}
