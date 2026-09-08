package realtime

import (
	"log/slog"
	"sync"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/socketio"
)

// DrainManager empties an instance of its clients gradually.
//
// A deploy cannot just close every socket: the editor would reconnect all of
// them at once, and the thundering herd would land on whichever instance is
// left. Instead clients are asked to reconnect a few per second, and the
// editor picks a moment when the user is not mid-keystroke.
type DrainManager struct {
	io  *socketio.Server
	log *slog.Logger

	mu        sync.Mutex
	stop      chan struct{}
	reconnect map[string]bool
}

// NewDrainManager builds a drain manager.
func NewDrainManager(io *socketio.Server, log *slog.Logger) *DrainManager {
	return &DrainManager{io: io, log: log, reconnect: map[string]bool{}}
}

// StartDrainTimeWindow drains the current clients over the given number of
// minutes, never slower than four per second.
func (d *DrainManager) StartDrainTimeWindow(minsToDrain int, onDrained func()) {
	if minsToDrain <= 0 {
		return
	}
	perMin := float64(d.io.CountClients()) / float64(minsToDrain)
	rate := perMin / 60
	if rate < 4 {
		rate = 4
	}
	d.StartDrain(rate, onDrained)
}

// StartDrain asks clients to reconnect at the given rate, in clients per
// second. A rate of zero stops any drain in progress.
func (d *DrainManager) StartDrain(rate float64, onDrained func()) {
	d.mu.Lock()
	if d.stop != nil {
		close(d.stop)
		d.stop = nil
	}
	if rate == 0 {
		d.mu.Unlock()
		return
	}
	d.log.Info("starting drain", slog.Float64("rate", rate))

	// A rate below one client per second becomes one client per longer
	// interval, which is how a slow drain is expressed.
	interval := time.Second
	perTick := int(rate)
	if rate < 1 {
		interval = time.Duration(float64(time.Second) / rate)
		perTick = 1
	}
	stop := make(chan struct{})
	d.stop = stop
	d.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if d.reconnectNClients(perTick) {
					if onDrained != nil {
						onDrained()
						onDrained = nil
					}
					return
				}
			}
		}
	}()
}

// reconnectNClients asks up to n clients that have not been asked yet, and
// reports whether every client has now been told.
func (d *DrainManager) reconnectNClients(n int) bool {
	drained := 0
	for _, c := range d.io.Clients() {
		d.mu.Lock()
		asked := d.reconnect[c.ID]
		if !asked {
			d.reconnect[c.ID] = true
		}
		d.mu.Unlock()
		if asked {
			continue
		}
		d.log.Debug("asking client to reconnect gracefully", slog.String("client", c.ID))
		_ = c.Emit("reconnectGracefully")
		drained++
		if drained == n {
			break
		}
	}
	if drained < n {
		d.log.Info("all clients have been told to reconnectGracefully")
		return true
	}
	return false
}
