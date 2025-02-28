package election

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"gfx.cafe/util/go/fxplus"
	"github.com/go-redsync/redsync/v4"
	redsyncrueidis "github.com/go-redsync/redsync/v4/redis/rueidis"
	"github.com/google/uuid"
	"github.com/redis/rueidis"
	"github.com/redis/rueidis/rueidiscompat"
)

type spawnedCtx struct {
	ctx    context.Context
	ch     chan context.Context
	cancel context.CancelCauseFunc
}

// TODO: rewrite this project
type RedilockStrategy struct {
	log       *slog.Logger
	namespace string
	redis     rueidiscompat.Cmdable

	members map[string]Member
	id      uuid.UUID

	isLeader   atomic.Bool
	leaderLock *redsync.Mutex

	mu   sync.Mutex
	ctxs []spawnedCtx
}

// Join will join the election and error if it cannot
// it will run until ctx is cancelled, and will otherwise keep retrying to rejoin the election if it errors
func (m *RedilockStrategy) Join(ctx context.Context) error {
	m.log.Info("attempting to discover leader")
	// discovery checks if there is a leader key set.
	// if this fails, it means we can't connect to redis
	knownLeader, err := m.discoverLeader(ctx)
	if err != nil {
		return err
	}
	m.log.Info("i think the leader is", "knownLeader", knownLeader)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer func() {
			ticker.Stop()
			m.leaderLock.UnlockContext(ctx)
			m.log.Info("election loop shutdown", "uuid", m.id)
		}()
		for {
			err := m.loop(ctx)
			if err != nil {
				if fxplus.IsShutdownOrCancel(err) {
					return
				}
				m.log.Error("leader election errored", "err", err)
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				if errors.Is(err, context.Canceled) {
					return
				}
				if err != nil {
					m.log.Error("leader election loop ended with error", "err", err)
				}
				return
			}
		}
	}()
	return nil
}

func NewRedilockStrategy(
	namespacePrefix string,
	client rueidis.Client,
	logger *slog.Logger,
) *RedilockStrategy {
	compat := rueidiscompat.NewAdapter(client)
	m := &RedilockStrategy{
		log:   logger,
		redis: compat,
	}
	rs := redsync.New(redsyncrueidis.NewPool(compat))
	m.leaderLock = rs.NewMutex(fmt.Sprintf("venn:%s:leader:redsync", namespacePrefix))
	m.id = uuid.New()
	return m
}

func (p *RedilockStrategy) IsLeader() bool {
	return p.isLeader.Load()
}

func (p *RedilockStrategy) AcquireLease(ctx context.Context) <-chan context.Context {
	o := make(chan context.Context, 1)
	if ctx == nil {
		ctx = context.Background()
	}
	sc := spawnedCtx{
		ctx: ctx,
		ch:  o,
	}
	p.mu.Lock()
	p.ctxs = append(p.ctxs, sc)
	p.mu.Unlock()
	// update subs in the background
	defer func() {
		go p.updateSubs()
	}()
	return o
}

func (m *RedilockStrategy) updateSubs() {
	// this lock needs to be held the entire time, it protects every spawnedCtx
	m.mu.Lock()
	defer m.mu.Unlock()
	isLeader := m.isLeader.Load()
	ctxs := m.ctxs
	newCtxs := make([]spawnedCtx, 0, len(m.ctxs))
	for _, v := range ctxs {
		if isLeader {
			// ctx hasnt been sent to channel yet, if are are the leader, send it
			if v.ctx != nil {
				subctx, cn := context.WithCancelCause(v.ctx)
				v.ctx = nil
				v.cancel = cn
				v.ch <- subctx
				close(v.ch)
			}
			// regardless, these continue to exist. we don't cleanup closed ctxs until leadership is lost
			newCtxs = append(newCtxs, v)
		} else {
			// you are not leader, cancel any that you can,
			if v.cancel != nil {
				v.cancel(ErrLostLeadership)
			} else {
				// if there is no cancel set, that means that ctx is not nil, and we are waiting on a send to the channel, so we can defer to the next cycle
				newCtxs = append(newCtxs, v)
			}
		}
	}
	m.ctxs = newCtxs
	return
}

// loop is a single loop of the main event loop. it should exit as soon as it can. it is not safe to run in more than one thread
func (m *RedilockStrategy) loop(ctx context.Context) error {
	// broadcast health updates member health key to 10 seconds from now
	err := m.broadcast_health(ctx)
	if err != nil {
		return err
	}
	// discovery checks if there is a leader key set,
	knownLeader, err := m.discoverLeader(ctx)
	if err != nil {
		return err
	}
	// if the known leader is found to be nil, we wait a second and try again
	// this is because the known leader should not be uuid.Nil, as discovery should claim the lock if it is not held
	if knownLeader == uuid.Nil {
		m.log.Info("known leader is nil, trying again")
		time.Sleep(1 * time.Second)
		return nil
	}
	// set isLeader to whether or not the known leader is ones self
	isLeader := knownLeader == m.id

	wasLeader := m.isLeader.Load()
	m.isLeader.Store(isLeader)

	m.updateSubs()

	isNewLeader := !wasLeader && isLeader
	lostLeader := wasLeader && !isLeader

	if isNewLeader {
		m.log.Info("i am the new leader", "uuid", m.id, "namespace", m.namespace)
	} else if lostLeader {
		m.log.Debug("i lost leadership", "uuid", m.id, "namespace", m.namespace, "think_leader", knownLeader)
	}

	if isLeader {
		// attempt to extend the leader lock
		m.leaderLock.Extend()
		// broadcast your health again
		m.broadcast_health(ctx)
		// return
		return nil
	}

	// if not the leader, attempt a mutiny
	success, err := m.mutiny(ctx, knownLeader)
	if err != nil {
		return err
	}
	// on successful mutiny, immediately go to next cycle (try to obtain leadership)
	if success {
		return nil
	}
	return nil
}

func (m *RedilockStrategy) mutiny(ctx context.Context, knownLeader uuid.UUID) (bool, error) {
	// check on the health of the leader
	healthCheck, err := m.redis.Exists(ctx, fmt.Sprintf("venn:%s:election:health:%s", m.namespace, knownLeader.String())).Result()
	if err != nil {
		return false, err
	}

	// nothing to do if isMember/healthcheck pases
	if healthCheck == 1 {
		return false, nil
	}

	m.log.Info("attempting overthrow", "uuid", m.id, "namespace", m.namespace, "overthrowing", knownLeader)
	// if the leader is not a member, then attempt to overthrow them
	err = m.overthrow(ctx)
	if err != nil {
		return false, err
	}
	m.log.Info("mutiny successful", "uuid", m.id, "namespace", m.namespace, "overthrowing", knownLeader)
	return true, nil
}

func (m *RedilockStrategy) discoverLeader(ctx context.Context) (uuid.UUID, error) {
	leaderKey := fmt.Sprintf("venn:%s:election:leader", m.namespace)
	for {
		ea, err := m.redis.Exists(ctx, leaderKey).Result()
		if err != nil {
			return uuid.UUID{}, err
		}
		if ea == 0 {
			// there was no leader, attempt to obtain the leader lock
			isLeader, err := m.attempt_promotion(ctx)
			if err != nil {
				m.log.Warn("could not obtain a leader lock, retrying in four seconds")
				time.Sleep(2 * time.Second)
				continue
			}
			if isLeader {
				return m.id, nil
			}
		} else if ea == 1 {
			leaderString, err := m.redis.Get(ctx, leaderKey).Result()
			if err != nil {
				return uuid.UUID{}, err
			}
			result, err := uuid.Parse(leaderString)
			if err != nil {
				m.log.Warn("could not parse uuid in leaderKey, deleting", "err", err)
				m.redis.Del(ctx, leaderKey).Result()
				continue
			}
			return result, nil
		}
	}
}

func (m *RedilockStrategy) attempt_promotion(ctx context.Context) (bool, error) {
	// try to get the lock
	if err := m.leaderLock.TryLock(); err != nil {
		return false, nil
	}
	// set the leader
	_, err := m.redis.Set(ctx, fmt.Sprintf("venn:%s:election:leader", m.namespace), m.id.String(), 0).Result()
	if err != nil {
		m.leaderLock.Unlock()
		return false, nil
	}
	// return that you are the leader
	return true, nil
}

func (m *RedilockStrategy) broadcast_health(ctx context.Context) error {
	dur := 5 * time.Second
	_, err := m.redis.Set(ctx, fmt.Sprintf("venn:%s:election:health:%s", m.namespace, m.id.String()), "OK", dur).Result()
	if err != nil {
		return err
	}
	return nil
}

func (m *RedilockStrategy) isOutlaw(ctx context.Context, id uuid.UUID) (bool, error) {
	isOutlaw, err := m.redis.SIsMember(ctx, fmt.Sprintf("venn:%s:election:outlaws", m.namespace), id.String()).Result()
	if err != nil {
		return false, err
	}
	return isOutlaw, err
}

func (m *RedilockStrategy) overthrow(ctx context.Context) error {
	_, err := m.redis.SAdd(ctx, fmt.Sprintf("venn:%s:election:outlaws", m.namespace), m.id.String()).Result()
	if err != nil {
		return err
	}

	_, err = m.redis.Del(ctx, fmt.Sprintf("venn:%s:election:leader", m.namespace)).Result()
	if err != nil {
		return err
	}
	return nil
}
