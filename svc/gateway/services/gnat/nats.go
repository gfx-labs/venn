package gnat

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gfx-labs/venn/lib/config"
	"gfx.cafe/util/go/fxplus"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"go.uber.org/fx"
)

type Gnat struct {
	log *slog.Logger

	c *nats.Conn
}

type Params struct {
	fx.In

	Config *config.Nats
	Ctx    context.Context

	Lc  fx.Lifecycle
	Log *slog.Logger
}

type Result struct {
	fx.Out

	Output   *Gnat
	Healther fxplus.Healther
}

func New(p Params) (r Result, err error) {
	o := &Gnat{}
	o.log = p.Log

	if p.Config.URI != "" {
		// connect to the nats server
		o.c, err = nats.Connect(string(p.Config.URI))
		if err != nil {
			return r, err
		}
	} else {
		// no nats server configured? try to create/join the embedded on port 48222 :)
		url := "nats://127.0.0.1:48222"
		opts := &server.Options{
			Port:      48222,
			JetStream: true,
			StoreDir:  "/tmp/jetstream",
		}
		ns, err := server.NewServer(opts)
		if err != nil {
			return r, err
		}
		ns.ConfigureLogger()
		ns.Start()
		if !ns.ReadyForConnections(5 * time.Second) {
			// failed to start nats
			return r, fmt.Errorf("failed to start embedded nats server")
		}
		p.Lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				ns.WaitForShutdown()
				return nil
			},
		})
		url = ns.ClientURL()
		o.c, err = nats.Connect(url)
		if err != nil {
			return r, err
		}
	}

	r.Output = o
	r.Healther = o
	return
}

func (o *Gnat) Conn() *nats.Conn {
	return o.c
}

func (o *Gnat) Health(ctx context.Context) error {
	if !o.c.IsConnected() {
		return fmt.Errorf("not connected to nats server")
	}
	return nil
}
