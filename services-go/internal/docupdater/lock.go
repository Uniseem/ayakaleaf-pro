package docupdater

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rediskeys"
	"github.com/redis/go-redis/v9"
)

const (
	// lockTestInterval is the first wait between attempts, backing off to
	// maxTestInterval so a contended document does not have every waiter
	// hammering Redis.
	lockTestInterval = 50 * time.Millisecond
	maxTestInterval  = time.Second
	// maxLockWaitTime bounds how long a request will queue behind others.
	maxLockWaitTime = 10 * time.Second
	// defaultLockTTL is how long a lock survives if its holder dies.
	defaultLockTTL = 30 * time.Second
)

// ErrLockTimeout means the document stayed locked for longer than a request is
// willing to wait.
var ErrLockTimeout = errors.New("timeout acquiring document lock")

// ErrLockLost is returned when releasing a lock that is no longer held: it
// expired and somebody else has it now, so whatever was done under it was done
// unprotected.
var ErrLockLost = errors.New("tried to release timed out lock")

// unlockScript releases a lock only if it is still the one we took.
//
// Deleting the key unconditionally would let a process whose lock had expired
// delete the lock of whoever holds it now, and two writers would then apply
// operations to the same document at once.
const unlockScript = `if redis.call("get", KEYS[1]) == ARGV[1] then ` +
	`return redis.call("del", KEYS[1]) else return 0 end`

// Locker takes per-document locks in Redis.
type Locker struct {
	redis *redis.Client
	keys  rediskeys.Schema
	ttl   time.Duration

	host    string
	pid     int
	random  string
	counter atomic.Uint64
}

// NewLocker builds a locker. A ttl of zero uses the default of thirty seconds.
func NewLocker(client *redis.Client, keys rediskeys.Schema, ttl time.Duration) *Locker {
	if ttl <= 0 {
		ttl = defaultLockTTL
	}
	host, _ := os.Hostname()
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return &Locker{
		redis: client, keys: keys, ttl: ttl,
		host: host, pid: os.Getpid(), random: hex.EncodeToString(buf),
	}
}

// value builds a lock value unique to this process and this attempt, so a lock
// can only be released by the attempt that took it.
func (l *Locker) value() string {
	return fmt.Sprintf("locked:host=%s:pid=%d:random=%s:time=%d:count=%d",
		l.host, l.pid, l.random, time.Now().UnixMilli(), l.counter.Add(1)-1)
}

// TryLock takes the lock if it is free, returning an empty value if it is not.
func (l *Locker) TryLock(ctx context.Context, docID string) (string, error) {
	value := l.value()
	key := l.keys.BlockingKey(docID)
	started := time.Now()

	ok, err := l.redis.SetNX(ctx, key, value, l.ttl).Result()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}

	// Taking the lock is only useful if it was quick. If Redis took longer
	// than a request may hold one, the lock is given straight back rather than
	// used: the work under it would outlive the lock.
	if elapsed := time.Since(started); elapsed > maxRedisRequestLength {
		if err := l.Unlock(ctx, docID, value); err != nil {
			return "", err
		}
		return "", nil
	}
	return value, nil
}

// Lock waits for the lock, backing off between attempts.
func (l *Locker) Lock(ctx context.Context, docID string) (string, error) {
	started := time.Now()
	interval := lockTestInterval

	for {
		if time.Since(started) > maxLockWaitTime {
			return "", fmt.Errorf("%w for doc %s", ErrLockTimeout, docID)
		}
		value, err := l.TryLock(ctx, docID)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}

		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return "", ctx.Err()
		}
		interval = min(interval*2, maxTestInterval)
	}
}

// Unlock releases a lock, and reports if it had already expired.
func (l *Locker) Unlock(ctx context.Context, docID, value string) error {
	key := l.keys.BlockingKey(docID)
	released, err := l.redis.Eval(ctx, unlockScript, []string{key}, value).Int64()
	if err != nil {
		return err
	}
	if released != 1 {
		// The lock we took is gone, which means it expired and the work done
		// under it was not in fact protected.
		return fmt.Errorf("%w for doc %s", ErrLockLost, docID)
	}
	return nil
}

// CheckLock reports whether the document is free.
func (l *Locker) CheckLock(ctx context.Context, docID string) (bool, error) {
	exists, err := l.redis.Exists(ctx, l.keys.BlockingKey(docID)).Result()
	if err != nil {
		return false, err
	}
	return exists == 0, nil
}

// WithLock runs fn while holding the document lock.
func (l *Locker) WithLock(ctx context.Context, docID string, fn func(context.Context) error) error {
	value, err := l.Lock(ctx, docID)
	if err != nil {
		return err
	}
	fnErr := fn(ctx)

	// The lock is released even when the work failed, and a release error is
	// only reported when the work itself succeeded: an expired lock is worth
	// knowing about, but not at the cost of hiding the real failure.
	if err := l.Unlock(ctx, docID, value); err != nil && fnErr == nil {
		return err
	}
	return fnErr
}
