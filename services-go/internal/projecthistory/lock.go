package projecthistory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// One project is processed at a time, and the lock is what enforces it. Two
// workers taking the same queue would each read the same updates, each send
// them to the history, and the project would come back with everything twice.
//
// The lock carries a value unique to whoever took it, so that releasing it can
// check it is still theirs. A lock that timed out and was taken by somebody
// else must not be released by the first holder: that would hand the project to
// a third worker while the second is still working on it.

const (
	// lockTTL is how long a lock lives without being extended.
	lockTTL = 360 * time.Second
	// lockTestInterval is how often a waiter tries again.
	lockTestInterval = 50 * time.Millisecond
	// maxLockWaitTime is how long it waits before giving up.
	maxLockWaitTime = 10 * time.Second
	// minLockExtensionInterval is how often an extension is worth making. A
	// long task extends the lock as it goes, and doing so on every step would
	// be a round trip per step.
	minLockExtensionInterval = time.Second
)

// The scripts are run as one step so that the check and the action cannot be
// separated by somebody else taking the lock in between.
const (
	unlockScript = `if redis.call("get", KEYS[1]) == ARGV[1] then ` +
		`return redis.call("del", KEYS[1]) else return 0 end`
	extendScript = `if redis.call("get", KEYS[1]) == ARGV[1] then ` +
		`return redis.call("expire", KEYS[1], ARGV[2]) else return 0 end`
)

// ErrLockTimeout is returned when the lock could not be taken in time.
var ErrLockTimeout = errors.New("timed out waiting for the project lock")

// ErrLockLost is returned when a lock turns out to be somebody else's, which
// means this worker overran and the project is being processed elsewhere.
var ErrLockLost = errors.New("the lock was taken by somebody else")

// LockManager takes and holds the per-project lock.
type LockManager struct {
	redis *redis.Client
	keys  Keys

	// hostname, pid and random make a lock value that no other process will
	// produce, and counter makes each one from this process distinct.
	hostname string
	pid      int
	random   string
	counter  atomic.Int64
}

// NewLockManager builds the manager.
func NewLockManager(client *redis.Client) *LockManager {
	hostname, _ := os.Hostname()
	random := make([]byte, 4)
	_, _ = rand.Read(random)
	return &LockManager{
		redis: client, hostname: hostname, pid: os.Getpid(),
		random: hex.EncodeToString(random),
	}
}

// HealthCheck takes and releases a lock of its own, which is what says Redis
// is reachable and that the scripts the locking relies on still work.
//
// It uses a key of its own rather than a project's, so that a health check
// never waits behind a flush or holds one up.
func (m *LockManager) HealthCheck(ctx context.Context) error {
	key := fmt.Sprintf("HistoryLock:HealthCheck:host=%s:pid=%d:random=%s",
		m.hostname, m.pid, m.random)
	return m.RunWithLock(ctx, key, func(ctx context.Context, lock *Lock) error {
		return nil
	})
}

// randomLock builds a value nobody else will write.
func (m *LockManager) randomLock() string {
	return fmt.Sprintf("locked:host=%s:pid=%d:random=%s:time=%d:count=%d",
		m.hostname, m.pid, m.random, time.Now().UnixMilli(), m.counter.Add(1)-1)
}

// TryLock takes the lock if it is free, and reports whether it got it.
func (m *LockManager) TryLock(ctx context.Context, key string) (string, bool, error) {
	value := m.randomLock()
	ok, err := m.redis.SetNX(ctx, key, value, lockTTL).Result()
	if err != nil {
		return "", false, fmt.Errorf("redis error trying to get lock %s: %w", key, err)
	}
	if !ok {
		return "", false, nil
	}
	return value, true, nil
}

// GetLock waits for the lock, up to the maximum wait.
func (m *LockManager) GetLock(ctx context.Context, key string) (string, error) {
	deadline := time.Now().Add(maxLockWaitTime)
	for {
		value, got, err := m.TryLock(ctx, key)
		if err != nil {
			return "", err
		}
		if got {
			return value, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("%w: %s", ErrLockTimeout, key)
		}
		select {
		case <-time.After(lockTestInterval):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

// CheckLock reports whether the lock is free.
func (m *LockManager) CheckLock(ctx context.Context, key string) (bool, error) {
	exists, err := m.redis.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists != 1, nil
}

// ReleaseLock gives the lock back, provided it is still this holder's.
func (m *LockManager) ReleaseLock(ctx context.Context, key, value string) error {
	result, err := m.redis.Eval(ctx, unlockScript, []string{key}, value).Result()
	if err != nil {
		return err
	}
	if released, ok := result.(int64); !ok || released != 1 {
		// The lock expired and somebody else has it. Saying so is the point:
		// this worker has been running without a lock, and whatever it did may
		// have been done alongside somebody else doing the same.
		return fmt.Errorf("%w: %s", ErrLockLost, key)
	}
	return nil
}

// Lock is a lock that is being held.
type Lock struct {
	manager  *LockManager
	key      string
	value    string
	takenAt  time.Time
	released bool
}

// Extend pushes the expiry out, if it has been long enough to be worth it.
func (l *Lock) Extend(ctx context.Context) error {
	if time.Since(l.takenAt) < minLockExtensionInterval {
		return nil
	}
	result, err := l.manager.redis.Eval(ctx, extendScript,
		[]string{l.key}, l.value, int(lockTTL.Seconds())).Result()
	if err != nil {
		return fmt.Errorf("redis error trying to extend lock %s: %w", l.key, err)
	}
	if extended, ok := result.(int64); !ok || extended != 1 {
		return fmt.Errorf("%w: %s", ErrLockLost, l.key)
	}
	l.takenAt = time.Now()
	return nil
}

// Release gives the lock back.
func (l *Lock) Release(ctx context.Context) error {
	if l.released {
		return nil
	}
	l.released = true
	return l.manager.ReleaseLock(ctx, l.key, l.value)
}

// RunWithLock takes the lock, runs the work, and gives the lock back whatever
// happens.
//
// The work is handed the lock so that it can extend it: a long task that does
// not would have the lock expire underneath it, and another worker would start
// on the same project.
func (m *LockManager) RunWithLock(ctx context.Context, key string,
	work func(ctx context.Context, lock *Lock) error) error {

	value, err := m.GetLock(ctx, key)
	if err != nil {
		return err
	}
	lock := &Lock{manager: m, key: key, value: value, takenAt: time.Now()}

	workErr := work(ctx, lock)
	// The release is not cancelled with the request: a lock left behind stops
	// the project being processed until it expires.
	releaseErr := lock.Release(context.WithoutCancel(ctx))
	if workErr != nil {
		return workErr
	}
	return releaseErr
}
