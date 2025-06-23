package callcenter

import (
	"cmp"
	"errors"
	"slices"
	"sync"
	"sync/atomic"

	"gfx.cafe/open/jrpc/pkg/jsonrpc"

	"gfx.cafe/gfx/venn/lib/jrpcutil"
	"gfx.cafe/gfx/venn/lib/util"
)

// Cluster combines multiple remotes and attempts each by priority.
type Cluster struct {
	priorities     []*clustererPriority
	remotes        []*RemoteWithConfig
	remotePriority []int
	mu             sync.RWMutex
}

type clustererPriority struct {
	priority int
	remotes  []*RemoteWithConfig
	round    atomic.Int64
}

func NewCluster() *Cluster {
	return &Cluster{}
}

func (T *Cluster) Add(priority int, remote *RemoteWithConfig) {
	T.mu.Lock()
	defer T.mu.Unlock()

	idx, _ := slices.BinarySearch(T.remotePriority, priority)
	if idx >= len(T.remotePriority) {
		T.remotes = append(T.remotes, remote)
		T.remotePriority = append(T.remotePriority, priority)
	} else {
		T.remotes = append(T.remotes, nil)
		T.remotePriority = append(T.remotePriority, 0)
		copy(T.remotes[idx+1:], T.remotes[idx:])
		copy(T.remotePriority[idx+1:], T.remotePriority[idx:])
		T.remotes[idx] = remote
		T.remotePriority[idx] = priority
	}

	idx, ok := slices.BinarySearchFunc(T.priorities, priority, func(priority *clustererPriority, i int) int {
		return cmp.Compare(priority.priority, i)
	})
	if ok {
		T.priorities[idx].remotes = append(T.priorities[idx].remotes, remote)
		return
	}

	T.priorities = append(T.priorities, nil)
	copy(T.priorities[idx+1:], T.priorities[idx:])
	T.priorities[idx] = &clustererPriority{
		priority: priority,
		remotes: []*RemoteWithConfig{
			remote,
		},
	}
}

func (T *Cluster) Remotes() []*RemoteWithConfig {
	T.mu.RLock()
	defer T.mu.RUnlock()

	return T.remotes
}

func (T *Cluster) ServeRPC(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
	T.mu.RLock()
	defer T.mu.RUnlock()

	var icept jrpcutil.Interceptor
	for i, p := range T.priorities {
		j := 0
		if len(p.remotes) > 1 {
			j = int(p.round.Add(1))
		}

		for k := 0; k < len(p.remotes); k++ {
			j++
			rem := p.remotes[j%len(p.remotes)]

			rem.Handler.ServeRPC(&icept, r)
			if icept.Error != nil {

				// check if error is a user error
				if util.IsUserError(icept.Error) {
					_ = w.Send(nil, icept.Error)
					return
				}

				// check if last possible remote. if not, try the other ones
				if i != len(T.priorities)-1 || k != len(p.remotes)-1 {
					continue
				}

				// if it's a head old error, just send the data we got
				if errors.Is(icept.Error, ErrHeadOld) {
					_ = w.Send(icept.Result, nil)
					return
				}

				_ = w.Send(nil, icept.Error)
				return
			}

			if icept.Result != nil {
				_ = w.Send(icept.Result, nil)
				return
			}
		}
	}
}
