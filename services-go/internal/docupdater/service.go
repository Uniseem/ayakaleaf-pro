package docupdater

import (
	"context"
	"log/slog"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rediskeys"
	"github.com/redis/go-redis/v9"
)

// Config carries the settings document-updater reads from its environment.
type Config struct {
	// MaxDocLength is the size a document may not exceed, in UTF-16 code
	// units.
	MaxDocLength int
	// DispatcherCount is how many workers wait on the queue real-time pushes
	// edits onto. It also decides how many shards that queue has, so it has to
	// match what real-time was told.
	DispatcherCount int
	// SmoothingOffset spreads the expiry of stored operations, so a burst of
	// documents loaded together does not expire together.
	SmoothingOffset time.Duration
	// ContinuousBackgroundFlush turns on the worker that drops projects nobody
	// has been editing.
	ContinuousBackgroundFlush bool
	// PublishOnIndividualChannels sends each applied operation on a per-document
	// channel rather than one shared one. It has to match what real-time
	// subscribes to.
	PublishOnIndividualChannels bool
	// GracefulShutdownDelay is how long to keep answering after being asked to
	// stop, so requests already in flight finish.
	GracefulShutdownDelay time.Duration

	// Keys names the Redis keys shared with real-time. The zero value is the
	// upstream schema.
	Keys rediskeys.Schema
}

// Deps are the things the service is built out of.
type Deps struct {
	Log    *slog.Logger
	Config Config

	// DocUpdaterRedis holds the documents, and PubSubRedis carries the applied
	// operations back to the editors. They are separate connections because
	// the Node service keeps them separate: a deployment may point one at a
	// different instance.
	DocUpdaterRedis *redis.Client
	PubSubRedis     *redis.Client

	Web         *WebClient
	Persistence *PersistenceClient
	History     *HistoryClient

	// Snapshots keeps a copy of a document from before an edit destroyed a
	// marker in it. It is the only thing here that touches the database
	// directly, and it may be nil, in which case no copy is kept.
	Snapshots *SnapshotStore
}

// Service is a running document-updater.
type Service struct {
	Docs    *DocumentManager
	Project *ProjectManager
	Updates *UpdateManager
	Redis   *RedisStore

	dispatcher *Dispatcher
	config     Config
	log        *slog.Logger
}

// New builds the service and starts the workers that wait on the queue.
func New(ctx context.Context, deps Deps) *Service {
	log := deps.Log
	cfg := deps.Config

	store := NewRedisStore(deps.DocUpdaterRedis, cfg.Keys, cfg.MaxDocLength,
		cfg.SmoothingOffset, log)
	locker := NewLocker(deps.DocUpdaterRedis, cfg.Keys, 0)
	history := NewHistoryQueue(deps.DocUpdaterRedis, cfg.Keys)
	bridge := NewRealTimeBridge(deps.DocUpdaterRedis, deps.PubSubRedis, cfg.Keys,
		cfg.PublishOnIndividualChannels, log)

	docs := NewDocumentManager(store, deps.Persistence, locker, history,
		deps.History, cfg.MaxDocLength, log)
	updates := NewUpdateManager(store, docs, locker, bridge, history, deps.Web,
		deps.Snapshots, cfg.MaxDocLength, log)
	// The two need each other: a write through the API becomes an update, and
	// an update reads the document through the manager.
	docs.UseUpdateManager(updates)
	project := NewProjectManager(store, docs, history, deps.History, log)

	service := &Service{
		Docs: docs, Project: project, Updates: updates, Redis: store,
		dispatcher: NewDispatcher(deps.DocUpdaterRedis, updates,
			cfg.DispatcherCount, log),
		config: cfg, log: log,
	}

	go service.dispatcher.Run(ctx)
	if cfg.ContinuousBackgroundFlush {
		log.Info("starting continuous background flush")
		go service.backgroundFlush(ctx)
	}
	return service
}

// Server builds the HTTP surface.
func (s *Service) Server(history *HistoryClient) *Server {
	return NewServer(s.Docs, s.Project, s.Redis, history, s.config.MaxDocLength, s.log)
}

// backgroundFlush drops the projects nobody has been editing, for as long as
// the service runs.
//
// The pace depends on what it finds: a backlog is worked through as fast as it
// can be, and an empty queue is checked once a second.
func (s *Service) backgroundFlush(ctx context.Context) {
	const (
		shortDelay = 10 * time.Millisecond
		longDelay  = time.Second
		// A high limit, so a pass always gets through the backlog rather than
		// stopping halfway and starting again.
		limit = 1000
	)

	for {
		flushed, err := s.Project.FlushAndDeleteOldProjects(ctx, DeleteQueueOptions{
			Limit:        limit,
			Timeout:      time.Second,
			MinDeleteAge: 3 * time.Minute,
		})
		if err != nil {
			s.log.Error("error in the background flush", slog.String("err", err.Error()))
		}

		delay := longDelay
		if flushed > 10 {
			delay = shortDelay
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			s.log.Info("discontinuing the background flush")
			return
		}
	}
}

// GracefulShutdownDelay is how long to keep answering after being asked to
// stop.
func (s *Service) GracefulShutdownDelay() time.Duration {
	return s.config.GracefulShutdownDelay
}
