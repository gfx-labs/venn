package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"gfx.cafe/gfx/venn/lib/config"
	"gfx.cafe/gfx/venn/svc/gateway/services/gnat"
	"gfx.cafe/open/jrpc/pkg/jjson"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/fx"
)

type Telemetry struct {
	log     *slog.Logger
	stream  jetstream.JetStream
	subject string

	pending chan jetstream.PubAckFuture

	enabled bool
}

type Entry struct {
	RequestID string        `json:"request_id"`
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration"`
	UsageKey  string        `json:"usage_key"`

	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`

	Metadata map[string]any `json:"metadata"`
	Extra    any            `json:"extra"`
}

type Params struct {
	fx.In

	Config *config.Telemetry

	Gnat *gnat.Gnat

	Ctx context.Context
	Lc  fx.Lifecycle
	Log *slog.Logger
}

type Result struct {
	fx.Out

	Output *Telemetry
}

func New(p Params) (r Result, err error) {
	o := &Telemetry{}
	o.log = p.Log
	r.Output = o

	o.enabled = p.Config.Enabled
	if !o.enabled {
		return
	}
	if p.Config.JetstreamStreamConfig == nil {
		return r, errors.New("telemetry jetstream stream config is nil")
	}

	js, err := jetstream.New(p.Gnat.Conn())
	if err != nil {
		return r, err
	}
	// buffered channel to store async publishes
	o.pending = make(chan jetstream.PubAckFuture, 128)
	o.subject = p.Config.JetstreamStreamConfig.Name
	o.stream = js
	_, err = js.CreateOrUpdateStream(p.Ctx, *p.Config.JetstreamStreamConfig)
	if err != nil {
		return r, err
	}

	go func() {
		select {
		case pending := <-o.pending:
			select {
			case err := <-pending.Err():
				o.log.Error("telemetry publish error", "err", err)
			case <-pending.Ok():
			}
		case <-p.Ctx.Done():
			return
		}
	}()
	p.Lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			select {
			case <-js.PublishAsyncComplete():
			case <-ctx.Done():
			}
			return nil
		},
	})

	return
}

func (o *Telemetry) Publish(ctx context.Context, e *Entry) error {
	if !o.enabled {
		return nil
	}
	buf, err := jjson.Marshal(e)
	if err != nil {
		return err
	}
	fut, err := o.stream.PublishAsync(o.subject, buf)
	if err != nil {
		return err
	}
	o.pending <- fut

	return nil
}
