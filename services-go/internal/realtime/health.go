package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// healthCheckTimeout is how long a published probe has to come back.
const healthCheckTimeout = time.Second

// HealthCheckManager verifies that Redis pub/sub is actually delivering.
//
// A connection to Redis that is up but not delivering is the failure this
// catches: every client stays connected and every edit is silently lost. So
// the check publishes a uniquely-keyed message on each channel and requires it
// back exactly once.
type HealthCheckManager struct {
	host string
	pid  int
	log  *slog.Logger

	counter atomic.Int64

	mu       sync.Mutex
	inFlight map[string]*probe
	failing  map[string]bool
}

type probe struct {
	id    string
	count int
}

// NewHealthCheckManager builds a health check manager.
func NewHealthCheckManager(log *slog.Logger) *HealthCheckManager {
	host, _ := os.Hostname()
	return &HealthCheckManager{
		host: host, pid: os.Getpid(), log: log,
		inFlight: map[string]*probe{},
		failing:  map[string]bool{},
	}
}

// Begin starts a probe on a channel and returns the key to publish.
func (h *HealthCheckManager) Begin(channel string) string {
	id := fmt.Sprintf("host=%s:pid=%d:count=%d", h.host, h.pid, h.counter.Add(1)-1)
	p := &probe{id: id}
	h.mu.Lock()
	h.inFlight[channel] = p
	h.mu.Unlock()

	time.AfterFunc(healthCheckTimeout, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.inFlight[channel] != p {
			return
		}
		// Anything other than exactly one delivery is a fault: zero means the
		// channel is dead, more than one means duplicate delivery.
		isFailing := p.count != 1
		if isFailing {
			h.log.Error("redis channel health check error", slog.String("channel", channel),
				slog.Int("count", p.count), slog.String("id", p.id))
		}
		h.failing[channel] = isFailing
	})
	return id
}

// Record notes that a probe message came back.
func (h *HealthCheckManager) Record(channel, key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if p := h.inFlight[channel]; p != nil && p.id == key {
		p.count++
	}
}

// IsFailing reports whether any channel is unhealthy.
func (h *HealthCheckManager) IsFailing() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, failing := range h.failing {
		if failing {
			return true
		}
	}
	return false
}

// Status returns the per-channel health, for logging.
func (h *HealthCheckManager) Status() map[string]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	status := make(map[string]bool, len(h.failing))
	for channel, failing := range h.failing {
		status[channel] = failing
	}
	return status
}

// runContinualPubsubTraffic publishes a probe on each channel every 20
// seconds.
//
// Besides checking delivery, this keeps the connection warm: an idle pub/sub
// connection is exactly what a load balancer or firewall drops silently.
func (s *Service) runContinualPubsubTraffic(ctx context.Context) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for channel, manager := range map[string]*ChannelManager{
				"applied-ops":   s.appliedOps,
				"editor-events": s.editorEvents,
			} {
				key := s.health.Begin(channel)
				payload, err := json.Marshal(map[string]any{
					"health_check": true,
					"key":          key,
					"date":         time.Now().Format(time.UnixDate),
				})
				if err != nil {
					continue
				}
				// Published on the base channel, which every instance is
				// subscribed to regardless of which rooms it holds.
				manager.Publish(ctx, "", string(payload), false)
			}
		}
	}
}
