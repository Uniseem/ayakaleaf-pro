package realtime

import (
	"context"
	"log/slog"
	"sync"

	"github.com/redis/go-redis/v9"
)

// ChannelManager owns one Redis subscription connection and the set of
// channels it is subscribed to.
//
// It mirrors the Node ChannelManager: a channel is subscribed at most once, no
// matter how many clients are in the room. RoomManager only calls in on the
// 0->1 and 1->0 transitions, and the map here makes a double subscribe
// harmless if that invariant is ever broken.
type ChannelManager struct {
	base   string
	sub    *redis.PubSub
	pub    *redis.Client
	log    *slog.Logger
	onMsg  func(channel string, payload string)
	metric func(op string, failed bool)

	mu         sync.Mutex
	subscribed map[string]bool
}

// NewChannelManager subscribes to the base channel and starts delivering
// messages to onMsg.
//
// The base channel is subscribed unconditionally, because that is where
// publishers write unless publishOnIndividualChannels is set. The per-entity
// channels added later are the sharded path.
func NewChannelManager(
	ctx context.Context,
	client *redis.Client,
	base string,
	log *slog.Logger,
	onMsg func(channel, payload string),
) *ChannelManager {
	m := &ChannelManager{
		base:       base,
		sub:        client.Subscribe(ctx, base),
		pub:        client,
		log:        log,
		onMsg:      onMsg,
		subscribed: map[string]bool{base: true},
	}
	go m.listen(ctx)
	return m
}

func (m *ChannelManager) listen(ctx context.Context) {
	ch := m.sub.Channel()
	for {
		select {
		case <-ctx.Done():
			_ = m.sub.Close()
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			m.onMsg(msg.Channel, msg.Payload)
		}
	}
}

// Subscribe joins the channel for one entity, e.g. editor-events:<projectId>.
func (m *ChannelManager) Subscribe(ctx context.Context, id string) error {
	channel := m.base + ":" + id
	m.mu.Lock()
	if m.subscribed[channel] {
		m.mu.Unlock()
		return nil
	}
	m.subscribed[channel] = true
	m.mu.Unlock()

	if err := m.sub.Subscribe(ctx, channel); err != nil {
		// Roll the bookkeeping back, or the channel would be considered
		// subscribed forever and every later attempt would be skipped.
		m.mu.Lock()
		delete(m.subscribed, channel)
		m.mu.Unlock()
		m.log.Error("failed to subscribe to channel", slog.String("channel", channel),
			slog.String("err", err.Error()))
		return err
	}
	m.log.Debug("subscribed to channel", slog.String("channel", channel))
	return nil
}

// Unsubscribe leaves an entity channel. Failures are logged and swallowed:
// this runs in the background after the last client left, and a stale
// subscription costs nothing but a little traffic.
func (m *ChannelManager) Unsubscribe(ctx context.Context, id string) {
	channel := m.base + ":" + id
	m.mu.Lock()
	if !m.subscribed[channel] {
		m.mu.Unlock()
		return
	}
	delete(m.subscribed, channel)
	m.mu.Unlock()

	if err := m.sub.Unsubscribe(ctx, channel); err != nil {
		m.log.Error("failed to unsubscribe from channel", slog.String("channel", channel),
			slog.String("err", err.Error()))
		return
	}
	m.log.Debug("unsubscribed from channel", slog.String("channel", channel))
}

// Publish writes to the base channel, or to the entity channel when the
// deployment publishes on individual channels.
func (m *ChannelManager) Publish(ctx context.Context, id, data string, individual bool) {
	channel := m.base
	if individual && id != "all" {
		channel = m.base + ":" + id
	}
	if err := m.pub.Publish(ctx, channel, data).Err(); err != nil {
		m.log.Error("failed to publish", slog.String("channel", channel),
			slog.String("err", err.Error()))
	}
}

// Close ends the subscription.
func (m *ChannelManager) Close() error { return m.sub.Close() }
