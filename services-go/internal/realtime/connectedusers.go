package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/rediskeys"
	"github.com/redis/go-redis/v9"
)

const (
	oneHour  = time.Hour
	oneDay   = 24 * time.Hour
	fourDays = 4 * oneDay

	// userTimeout is how long a connected_user hash survives without a
	// refresh.
	userTimeout = oneHour / 4
	// refreshTimeout is the age past which a client is not reported as
	// connected: only clients that answered the last refresh broadcast count.
	refreshTimeout = 10 * time.Second
)

// ConnectedUser is one entry in the collaborator list the editor shows.
type ConnectedUser struct {
	ClientID   string          `json:"client_id"`
	Connected  bool            `json:"connected"`
	ClientAge  float64         `json:"client_age"`
	UserID     string          `json:"user_id,omitempty"`
	FirstName  string          `json:"first_name,omitempty"`
	LastName   string          `json:"last_name,omitempty"`
	Email      string          `json:"email,omitempty"`
	CursorData json.RawMessage `json:"cursorData,omitempty"`
}

// ConnectedUsersManager tracks who is in a project and where their cursor is.
//
// The state lives in Redis rather than in this process because a project's
// collaborators may be connected to different real-time instances.
type ConnectedUsersManager struct {
	redis *redis.Client
	keys  rediskeys.Schema
	log   *slog.Logger
}

// NewConnectedUsersManager builds a manager over the realtime Redis.
func NewConnectedUsersManager(client *redis.Client, keys rediskeys.Schema, log *slog.Logger) *ConnectedUsersManager {
	return &ConnectedUsersManager{redis: client, keys: keys, log: log}
}

// CountConnectedClients reports how many clients the whole cluster has in a
// project.
func (m *ConnectedUsersManager) CountConnectedClients(ctx context.Context, projectID string) (int64, error) {
	return m.redis.SCard(ctx, m.keys.ClientsInProject(projectID)).Result()
}

// UpdateUserPosition marks a user as present, and records their cursor when
// there is one.
//
// The same call serves a fresh connection and a cursor move, so a cursor
// update revives an entry that expired while the user was idle.
func (m *ConnectedUsersManager) UpdateUserPosition(
	ctx context.Context, projectID, clientID string, user *User, cursor any,
) error {
	m.log.Debug("marking user as joined or connected",
		slog.String("project", projectID), slog.String("client", clientID))

	key := m.keys.ConnectedUser(projectID, clientID)
	fields := []any{
		"last_updated_at", strconv.FormatInt(time.Now().UnixMilli(), 10),
		"user_id", user.ID,
		"first_name", user.FirstName,
		"last_name", user.LastName,
		"email", user.Email,
	}
	if cursor != nil {
		encoded, err := json.Marshal(cursor)
		if err != nil {
			return err
		}
		fields = append(fields, "cursorData", string(encoded))
	}

	pipe := m.redis.TxPipeline()
	pipe.SAdd(ctx, m.keys.ClientsInProject(projectID), clientID)
	pipe.SCard(ctx, m.keys.ClientsInProject(projectID))
	pipe.Expire(ctx, m.keys.ClientsInProject(projectID), fourDays)
	pipe.HSet(ctx, key, fields...)
	pipe.Expire(ctx, key, userTimeout)
	_, err := pipe.Exec(ctx)
	return err
}

// RefreshClient extends a client's entry after it answered a refresh
// broadcast, which is what keeps it in the collaborator list.
func (m *ConnectedUsersManager) RefreshClient(ctx context.Context, projectID, clientID string) {
	key := m.keys.ConnectedUser(projectID, clientID)
	pipe := m.redis.TxPipeline()
	pipe.HSet(ctx, key, "last_updated_at", strconv.FormatInt(time.Now().UnixMilli(), 10))
	pipe.Expire(ctx, key, userTimeout)
	if _, err := pipe.Exec(ctx); err != nil {
		m.log.Warn("problem refreshing connected client", slog.String("project", projectID),
			slog.String("client", clientID), slog.String("err", err.Error()))
	}
}

// MarkUserAsDisconnected removes a client from a project.
func (m *ConnectedUsersManager) MarkUserAsDisconnected(ctx context.Context, projectID, clientID string) error {
	m.log.Debug("marking user as disconnected",
		slog.String("project", projectID), slog.String("client", clientID))

	pipe := m.redis.TxPipeline()
	pipe.SRem(ctx, m.keys.ClientsInProject(projectID), clientID)
	remaining := pipe.SCard(ctx, m.keys.ClientsInProject(projectID))
	pipe.Expire(ctx, m.keys.ClientsInProject(projectID), fourDays)
	pipe.Del(ctx, m.keys.ConnectedUser(projectID, clientID))
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	if remaining.Val() == 0 {
		// The project is empty again; the marker is only meaningful while
		// somebody is in it.
		if err := m.redis.GetDel(ctx, m.keys.ProjectNotEmptySince(projectID)).Err(); err != nil &&
			err != redis.Nil {
			m.log.Warn("could not collect projectNotEmptySince",
				slog.String("project", projectID), slog.String("err", err.Error()))
		}
		return nil
	}

	// Clients remain, so record when the project stopped being empty -- but
	// only the first time, which is what the NX does.
	now := strconv.FormatInt(time.Now().Unix(), 10)
	pipe = m.redis.TxPipeline()
	pipe.Get(ctx, m.keys.ProjectNotEmptySince(projectID))
	pipe.SetNX(ctx, m.keys.ProjectNotEmptySince(projectID), now, 31*oneDay)
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		m.log.Warn("could not get/set projectNotEmptySince",
			slog.String("project", projectID), slog.String("err", err.Error()))
	}
	return nil
}

// GetConnectedUsers lists the collaborators the editor should show.
func (m *ConnectedUsersManager) GetConnectedUsers(ctx context.Context, projectID string) ([]ConnectedUser, error) {
	clientIDs, err := m.redis.SMembers(ctx, m.keys.ClientsInProject(projectID)).Result()
	if err != nil {
		return nil, err
	}

	users := make([]ConnectedUser, 0, len(clientIDs))
	for _, clientID := range clientIDs {
		fields, err := m.redis.HGetAll(ctx, m.keys.ConnectedUser(projectID, clientID)).Result()
		if err != nil {
			return nil, err
		}
		if fields["user_id"] == "" {
			// The entry expired: the client is gone, or has not answered a
			// refresh in a while.
			continue
		}
		lastUpdated, _ := strconv.ParseInt(fields["last_updated_at"], 10, 64)
		age := time.Duration(time.Now().UnixMilli()-lastUpdated) * time.Millisecond
		if age >= refreshTimeout {
			continue
		}
		user := ConnectedUser{
			ClientID:  clientID,
			Connected: true,
			ClientAge: age.Seconds(),
			UserID:    fields["user_id"],
			FirstName: fields["first_name"],
			LastName:  fields["last_name"],
			Email:     fields["email"],
		}
		if raw := fields["cursorData"]; raw != "" {
			if json.Valid([]byte(raw)) {
				user.CursorData = json.RawMessage(raw)
			}
		}
		users = append(users, user)
	}
	return users, nil
}
