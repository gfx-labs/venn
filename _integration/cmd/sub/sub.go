package main

import (
	"context"
	"log"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func _main() error {
	chain := "hyperevm"
	client, err := ethclient.Dial("ws://localhost:8545/" + chain)
	if err != nil {
		return err
	}
	ctx, cn := context.WithTimeout(context.Background(), 5*time.Second)
	defer cn()

	resp := make(chan *types.Header)
	sub, err := client.SubscribeNewHead(ctx, resp)
	if err != nil {
		return err
	}
	for {
		select {
		case err := <-sub.Err():
			return err
		case header := <-resp:
			log.Println("new header:", header.Number)
		}
	}

	return nil
}

func main() {
	if err := _main(); err != nil {
		panic(err)
	}
}
