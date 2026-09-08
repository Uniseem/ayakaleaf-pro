package realtime

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rediskeys"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/socketio"
	"github.com/redis/go-redis/v9"
)

// Config carries the settings real-time reads from its environment.
type Config struct {
	CookieName     string
	SessionSecrets []string
	SessionPrefix  string

	MaxUpdateSize               int
	PendingUpdateListShardCount int
	PublishOnIndividualChannels bool
	ExposeHostname              bool
	ContinualPubsubTraffic      bool

	// ShutdownDrainTimeWindow is how many minutes a drain is spread over, and
	// GracefulReconnectTimeout how long a client that ignores the request to
	// reconnect is left alone before it is cut off.
	ShutdownDrainTimeWindow  int
	GracefulReconnectTimeout time.Duration

	BehindProxy     bool
	TrustedProxyIPs string

	// Keys names the Redis keys shared with document-updater. The zero value
	// is the upstream schema.
	Keys rediskeys.Schema
}

// clientContext is the per-connection state the Node service keeps on
// client.ol_context.
//
// It is written once when the client joins a project and read from every
// pub/sub goroutine afterwards, so every field goes through the lock.
type clientContext struct {
	// PublicID is the id shared with other clients. The socket.io session id
	// is security sensitive -- knowing it is enough to be mistaken for that
	// client -- so it never leaves this process.
	PublicID string

	mu               sync.RWMutex
	user             *User
	projectID        string
	privilegeLevel   string
	ownerID          string
	isRestrictedUser bool
	isTokenMember    bool
	isInvitedMember  bool
	connectedTime    time.Time
	docs             map[string]bool

	remoteIP    string
	userAgent   string
	isDebugging bool
	connectedAt time.Time

	// pingID and pongID track the server-side liveness probe; they differ when
	// the client has not answered the last one.
	pingID int64
	pongID int64

	// joinLeaveEpoch lets a joinDoc notice that another join or leave
	// overtook it, so a slow join cannot resurrect a document the client has
	// already left.
	joinLeaveEpoch atomic.Int64
}

func (c *clientContext) UserID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.user == nil {
		return ""
	}
	return c.user.ID
}

func (c *clientContext) ProjectID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.projectID
}

func (c *clientContext) PrivilegeLevel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.privilegeLevel
}

func (c *clientContext) IsRestrictedUser() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isRestrictedUser
}

func (c *clientContext) IsInvitedMember() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isInvitedMember
}

func (c *clientContext) canAccessDoc(docID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.docs[docID]
}

func (c *clientContext) addAccessToDoc(docID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.docs[docID] = true
}

// Service is the real-time service: it owns the socket.io server, the Redis
// connections and the state that ties them together.
type Service struct {
	log    *slog.Logger
	cfg    Config
	io     *socketio.Server
	rooms  *RoomManager
	users  *ConnectedUsersManager
	web    *WebClient
	du     *DocumentUpdaterClient
	store  *SessionStore
	health *HealthCheckManager
	drain  *DrainManager
	addr   *AddressResolver

	keys rediskeys.Schema

	editorEvents *ChannelManager
	appliedOps   *ChannelManager

	realtimeRedis   *redis.Client
	docUpdaterRedis *redis.Client

	mu       sync.RWMutex
	contexts map[string]*clientContext
	queues   map[string]chan queuedEvent

	// shutDownInProgress rejects new connections during a drain, and
	// shutDownComplete marks the instance unhealthy so the load balancer
	// stops sending it traffic.
	shutDownInProgress atomic.Bool
	shutDownComplete   atomic.Bool
}

// Deps are the collaborators a Service is built from.
type Deps struct {
	Log             *slog.Logger
	Config          Config
	Web             *WebClient
	DocumentUpdater *DocumentUpdaterClient
	Sessions        *SessionStore
	PubSubRedis     *redis.Client
	RealtimeRedis   *redis.Client
	DocUpdaterRedis *redis.Client
}

// New builds the service and starts its Redis subscriptions.
func New(ctx context.Context, deps Deps) *Service {
	s := &Service{
		log:             deps.Log,
		cfg:             deps.Config,
		web:             deps.Web,
		du:              deps.DocumentUpdater,
		store:           deps.Sessions,
		keys:            deps.Config.Keys,
		users:           NewConnectedUsersManager(deps.RealtimeRedis, deps.Config.Keys, deps.Log),
		health:          NewHealthCheckManager(deps.Log),
		addr:            NewAddressResolver(deps.Config.BehindProxy, deps.Config.TrustedProxyIPs),
		realtimeRedis:   deps.RealtimeRedis,
		docUpdaterRedis: deps.DocUpdaterRedis,
		contexts:        map[string]*clientContext{},
		queues:          map[string]chan queuedEvent{},
	}
	s.io = socketio.NewServer(s, deps.Log)
	s.drain = NewDrainManager(s.io, deps.Log)

	// Both base channels are subscribed before any client can connect, so no
	// event can arrive at a room that is not yet listening.
	s.editorEvents = NewChannelManager(ctx, deps.PubSubRedis, "editor-events", deps.Log, s.processEditorEvent)
	s.appliedOps = NewChannelManager(ctx, deps.PubSubRedis, "applied-ops", deps.Log, s.processAppliedOp)
	s.rooms = NewRoomManager(s.io, s.editorEvents, s.appliedOps, deps.Log)

	if deps.Config.ContinualPubsubTraffic {
		deps.Log.Debug("continualPubsubTraffic enabled")
		go s.runContinualPubsubTraffic(ctx)
	}
	return s
}

// IO exposes the socket.io server so the HTTP mux can route to it.
func (s *Service) IO() *socketio.Server { return s.io }

// context returns the per-connection state, or nil for a connection that never
// finished connecting.
func (s *Service) context(c *socketio.Conn) *clientContext {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.contexts[c.ID]
}

func (s *Service) setContext(c *socketio.Conn, cc *clientContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contexts[c.ID] = cc
}

func (s *Service) dropContext(c *socketio.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.contexts, c.ID)
}

func (s *Service) setQueue(c *socketio.Conn, queue chan queuedEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queues[c.ID] = queue
}

func (s *Service) queue(c *socketio.Conn) chan queuedEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.queues[c.ID]
}

func (s *Service) dropQueue(c *socketio.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.queues, c.ID)
}

// newPublicID generates the id other clients see, in the same shape as the
// base64id package socket.io uses: 15 random bytes, URL-safe base64.
func newPublicID() string {
	buf := make([]byte, 15)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand does not fail in practice, and a connection without an
		// id cannot be tracked at all, so this is the one place a fallback
		// would be worse than the panic it avoids.
		return "P." + base64.RawURLEncoding.EncodeToString([]byte(time.Now().String()))[:20]
	}
	return "P." + base64.RawURLEncoding.EncodeToString(buf)
}
