package main

var cli struct {
	StartNode    StartNode    `cmd:"start-node" help:"start venn" default:"withargs"`
	StartGateway StartGateway `cmd:"start-gateway" help:"start gateway"`
}
