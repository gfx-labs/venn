package cass

import (
	"context"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"gfx.cafe/gfx/venn/lib/config"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v2"
	"go.uber.org/fx"
)

type Scylla struct {
	s   *gocqlx.Session
	cfg *config.Scylla
}

type ScyllaParams struct {
	fx.In

	Config *config.Scylla `optional:"true"`
	Log    *slog.Logger
	Lc     fx.Lifecycle
}

type ScyllaResult struct {
	fx.Out

	Scylla *Scylla
}

func New(params ScyllaParams) (o ScyllaResult, err error) {
	r := &Scylla{
		cfg: params.Config,
	}
	if params.Config == nil {
		params.Log.Info("Scylla disabled", "reason", "no cassandra block")
		return ScyllaResult{Scylla: nil}, nil
	}
	o.Scylla = r

	cluster := gocql.NewCluster()
	pu, err := url.Parse(string(params.Config.URI))
	if err != nil {
		return o, err
	}
	cluster.Hosts = strings.Split(pu.Hostname(), "--")

	if verString := pu.Query().Get("version"); len(verString) > 0 {
		version, err := strconv.Atoi(verString)
		if err != nil {
			return o, err
		}
		cluster.ProtoVersion = version
	}
	if len(pu.Port()) > 0 {
		portInt, err := strconv.Atoi(pu.Port())
		if err != nil {
			return o, err
		}
		cluster.Port = portInt
	}

	if pu.User != nil && len(pu.User.Username()) > 0 {
		pa := gocql.PasswordAuthenticator{}
		pa.Username = pu.User.Username()
		if pw, ok := pu.User.Password(); ok {
			pa.Password = pw
		}
		cluster.Authenticator = pa
	}

	if len(r.cfg.CertFile) > 0 {
		cluster.SslOpts = &gocql.SslOptions{
			CaPath: r.cfg.CertFile,
		}
	}

	cluster.Keyspace = r.cfg.Keyspace
	cluster.Hosts = append(cluster.Hosts, r.cfg.Hosts...)

	session, err := gocqlx.WrapSession(cluster.CreateSession())
	if err != nil {
		return o, err
	}

	params.Lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			session.Close()
			return nil
		},
	})
	r.s = &session

	return o, nil
}

func (r *Scylla) C() *gocqlx.Session {
	return r.s
}

func (r *Scylla) Keyspace() string {
	return r.cfg.Keyspace
}
