package realtime

import (
	"log/slog"
	"time"
)

// DrainAndShutdown takes the instance out of service gracefully and calls
// exit once every client has gone.
//
// A real-time instance holds live editing sessions, so it cannot simply stop.
// The sequence is: stop accepting new connections, ask the existing clients to
// reconnect over a window long enough that they land elsewhere a few at a
// time, then cut off whatever is left and let the process end.
func (s *Service) DrainAndShutdown(signal string, exit func()) {
	if s.shutDownInProgress.Swap(true) {
		s.log.Info("shutdown already in progress, ignoring signal", slog.String("signal", signal))
		return
	}
	window := s.cfg.ShutdownDrainTimeWindow
	if window <= 0 {
		exit()
		return
	}
	s.log.Info("received interrupt, starting drain", slog.String("signal", signal),
		slog.Int("minutes", window))

	s.drain.StartDrainTimeWindow(window, func() {
		go s.exitWhenAllClientsHaveDisconnected(exit)

		// Clients that ignore the request to reconnect -- third-party or buggy
		// ones -- would keep the process alive forever, so they are cut off
		// once the grace period is over.
		time.AfterFunc(s.cfg.GracefulReconnectTimeout, func() {
			stale := s.io.Clients()
			if len(stale) > 0 {
				ids := make([]string, 0, len(stale))
				for _, c := range stale {
					ids = append(ids, c.ID)
					c.Close()
				}
				s.log.Info("forcefully disconnecting stale clients", slog.Any("staleClients", ids))
			}
			// The instance is now unhealthy on purpose, so the load balancer
			// stops sending it anything at all.
			s.shutDownComplete.Store(true)
		})
	})
}

func (s *Service) exitWhenAllClientsHaveDisconnected(exit func()) {
	for {
		if n := s.io.CountClients(); n == 0 {
			s.log.Info("no clients connected, exiting")
			exit()
			return
		} else {
			s.log.Info("clients still connected, not shutting down yet",
				slog.Int("connectedClients", n))
		}
		time.Sleep(5 * time.Second)
	}
}
