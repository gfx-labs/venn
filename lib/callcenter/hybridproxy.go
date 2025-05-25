package callcenter

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"gfx.cafe/open/jrpc"
	"gfx.cafe/open/jrpc/contrib/codecs/websocket"
	"gfx.cafe/open/jrpc/contrib/extension/subscription"
	"github.com/valyala/bytebufferpool"
)

func init() {
	subscription.SetServiceMethodSeparator("_")
}

type HybridProxy struct {
	baseUrl          string
	websocketBaseUrl string
	logger           *slog.Logger
}

func NewHybridProxy(logger *slog.Logger, baseUrl string) *HybridProxy {
	return &HybridProxy{
		baseUrl:          baseUrl,
		websocketBaseUrl: strings.Replace(baseUrl, "http", "ws", 1),
		logger:           logger,
	}
}

func (p *HybridProxy) EndpointHandler(to string) (jrpc.Handler, error) {
	// initialize the http client
	joinedHttpUrl, err := url.JoinPath(p.baseUrl, to)
	if err != nil {
		return nil, err
	}
	httpClient, err := jrpc.Dial(joinedHttpUrl)
	if err != nil {
		return nil, err
	}

	// initialize the websocket pool
	joinedWsUrl, err := url.JoinPath(p.websocketBaseUrl, to)
	if err != nil {
		return nil, err
	}
	pool := newSocketPool(func(ctx context.Context) (*websocket.Client, error) {
		return websocket.DialWebsocket(ctx, joinedWsUrl, "")
	}, 8)
	p.logger.Debug("created endpoint handler", "to", to)
	return jrpc.HandlerFunc(func(w jrpc.ResponseWriter, r *jrpc.Request) {
		switch r.Method {
		case "eth_subscribe":
			p.handleEthSubscription(pool, w, r)
		default:
			handleHttp(httpClient, w, r)
		}
	}), nil
}

func (p *HybridProxy) handleEthSubscription(pool *socketPool, w jrpc.ResponseWriter, r *jrpc.Request) {
	first := true
	notifier, ok := subscription.NotifierFromContext(r.Context())
	if !ok {
		_ = w.Send(nil, subscription.ErrNotificationsUnsupported)
		return
	}
	for {
		// exit the handler if the context is done
		select {
		case <-r.Context().Done():
			// this will call the deferr handler which will call unsubscribe
			return
		case <-notifier.Err():
			return
		default:
		}
		if !first {
			// sleep for a second between retries
			time.Sleep(1 * time.Second)
		}
		// get a conn. if we fail to do so, we immediately error to the consumer
		socketConn, err := pool.get(r.Context())
		if err != nil {
			if first {
				w.Send(nil, err)
				return
			} else {
				p.logger.Error("failed to get socket conn", "err", err)
				continue
			}
		}
		// we cannot fail to create the subscriber, ever
		subscriber, err := subscription.UpgradeConn(socketConn.conn, nil)
		if err != nil {
			w.Send(nil, err)
			return
		}
		ch := make(chan json.RawMessage)
		sub, err := subscriber.Subscribe(r.Context(), "eth", ch, r.Params)
		if err != nil {
			if first {
				w.Send(nil, err)
				return
			} else {
				p.logger.Error("failed to subscribe", "err", err)
				continue
			}
		}
		first = false
		// we will keep listening to the subscription until something dies
		func() {
			defer func() {
				_ = sub.Unsubscribe()
			}()
			for {
				select {
				case data := <-ch:
					err = notifier.Notify(data)
					if err != nil {
						return
					}
					// TODO: we need to emit a ratelimit event here.
				case <-r.Context().Done():
					return
				case <-sub.Err():
					return
				case <-notifier.Err():
					return
				}
			}
		}()

	}
}

func handleHttp(conn jrpc.Conn, w jrpc.ResponseWriter, r *jrpc.Request) {
	params := r.Params
	if len(params) == 0 {
		params = nil
	}
	buf := bytebufferpool.Get()
	defer bytebufferpool.Put(buf)
	resultRaw := json.RawMessage(buf.B)
	resultPtr := &resultRaw
	err := conn.Do(r.Context(), resultPtr, r.Method, r.Params)
	_ = w.Send(*resultPtr, err)
	buf.B = []byte(*resultPtr)
}

func newSocketPool(dialer func(ctx context.Context) (*websocket.Client, error), size int) *socketPool {
	return &socketPool{
		pool:   make(map[int]*socketPoolConn, size),
		dialer: dialer,
		size:   size,
	}
}

type socketPool struct {
	pool   map[int]*socketPoolConn
	dialer func(ctx context.Context) (*websocket.Client, error)

	size   int
	nextId int

	mu sync.Mutex
}

type socketPoolConn struct {
	id   int
	conn *websocket.Client
}

func (p *socketPool) get(ctx context.Context) (*socketPoolConn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		if p.nextId >= p.size {
			p.nextId = 0
		}
		if conn, ok := p.pool[p.nextId]; ok {
			select {
			case <-conn.conn.Closed():
				// if the connection is closed, we need to redial it.
			default:
				p.nextId++
				return conn, nil
			}
		}
		conn, err := p.dialer(ctx)
		if err != nil {
			return nil, err
		}
		poolConn := &socketPoolConn{id: p.nextId, conn: conn}
		p.pool[p.nextId] = poolConn
		p.nextId++
		return poolConn, nil
	}
}
