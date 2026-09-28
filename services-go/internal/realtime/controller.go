package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/socketio"
)

// ProtocolVersion is bumped when a change is not backwards compatible: the
// client compares it across a reconnect and reloads the page when it differs.
const ProtocolVersion = 2

// flushIfEmptyDelay is how long the service waits after the last client of a
// project leaves before telling document-updater to flush it. The disconnect
// that triggered this has not finished yet, so an immediate check would still
// count the leaving client.
const flushIfEmptyDelay = 500 * time.Millisecond

// clientRefreshDelay is how long the collaborator list waits for clients on
// other instances to answer the refresh broadcast.
const clientRefreshDelay = time.Second

// Privilege levels, in the groupings AuthorizationManager checks.
var (
	canView   = map[string]bool{"readOnly": true, "readAndWrite": true, "review": true, "owner": true}
	canEdit   = map[string]bool{"readAndWrite": true, "owner": true}
	canReview = map[string]bool{"readAndWrite": true, "owner": true, "review": true}
)

// JoinProjectOutcome is what the client is told after joining.
type JoinProjectOutcome struct {
	Project          json.RawMessage `json:"project"`
	PublicID         string          `json:"publicId"`
	PermissionsLevel string          `json:"permissionsLevel"`
	ProtocolVersion  int             `json:"protocolVersion"`
}

// JoinProject authorises a connection and puts it in the project room.
func (s *Service) JoinProject(ctx context.Context, c *socketio.Conn, cc *clientContext, projectID string) (*JoinProjectOutcome, error) {
	if c.Disconnected() {
		return nil, nil
	}
	user := cc.user
	s.log.Info("user joining project", slog.String("user", cc.UserID()),
		slog.String("project", projectID), slog.String("client", c.ID),
		slog.String("remoteIp", cc.remoteIP))

	result, err := s.web.JoinProject(ctx, projectID, user)
	if err != nil {
		return nil, err
	}
	if c.Disconnected() {
		s.log.Info("client disconnected before joining project",
			slog.String("project", projectID), slog.String("client", c.ID))
		return nil, nil
	}
	if result.PrivilegeLevel == "" {
		return nil, ErrNotAuthorized
	}

	cc.mu.Lock()
	cc.projectID = projectID
	cc.privilegeLevel = result.PrivilegeLevel
	cc.ownerID = result.OwnerID()
	cc.isRestrictedUser = result.IsRestrictedUser
	cc.isTokenMember = result.IsTokenMember
	cc.isInvitedMember = result.IsInvitedMember
	cc.connectedTime = time.Now()
	cc.mu.Unlock()

	if err := s.rooms.JoinProject(ctx, c, projectID); err != nil {
		return nil, err
	}

	// Marking the user as present is not worth blocking the join on: a failure
	// costs a missing entry in the collaborator list, not a broken editor.
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.users.UpdateUserPosition(bg, projectID, cc.PublicID, user, nil); err != nil {
			s.log.Warn("background cursor update failed", slog.String("project", projectID),
				slog.String("client", c.ID), slog.String("err", err.Error()))
		}
	}()

	s.log.Debug("user joined project", slog.String("user", cc.UserID()),
		slog.String("project", projectID), slog.String("client", c.ID),
		slog.String("privilegeLevel", result.PrivilegeLevel))

	return &JoinProjectOutcome{
		Project:          result.Project,
		PublicID:         cc.PublicID,
		PermissionsLevel: result.PrivilegeLevel,
		ProtocolVersion:  ProtocolVersion,
	}, nil
}

// LeaveProject cleans up after a disconnect and flushes the project when the
// last client has gone.
func (s *Service) LeaveProject(ctx context.Context, c *socketio.Conn, cc *clientContext) {
	projectID := cc.ProjectID()
	if projectID == "" {
		return // the client never joined a project
	}
	s.log.Info("client leaving project", slog.String("project", projectID),
		slog.String("user", cc.UserID()), slog.String("client", c.ID))

	s.EmitToRoom(ctx, projectID, "clientTracking.clientDisconnected", cc.PublicID)

	if err := s.users.MarkUserAsDisconnected(ctx, projectID, cc.PublicID); err != nil {
		s.log.Error("error marking client as disconnected", slog.String("project", projectID),
			slog.String("client", c.ID), slog.String("err", err.Error()))
	}
	s.rooms.LeaveProjectAndDocs(ctx, c)

	// The flush happens after a delay because this client is still counted as
	// being in the room until its disconnect finishes unwinding.
	time.AfterFunc(flushIfEmptyDelay, func() {
		if s.io.CountClientsIn(projectID) != 0 {
			return
		}
		bg, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.du.FlushProject(bg, projectID, s.shutDownInProgress.Load()); err != nil {
			s.log.Error("error flushing to doc updater after leaving project",
				slog.String("project", projectID), slog.String("err", err.Error()))
		}
	})
}

// JoinDocOptions are the flags the editor passes with joinDoc.
type JoinDocOptions struct {
	EncodeRanges      bool     `json:"encodeRanges"`
	SupportsHistoryOT bool     `json:"supportsHistoryOT"`
	Age               *float64 `json:"age"`
}

// JoinDocResult is the payload of the joinDoc acknowledgement, in the argument
// order the editor's callback expects.
type JoinDocResult struct {
	Lines   []json.RawMessage
	Version int64
	Ops     json.RawMessage
	Ranges  json.RawMessage
	Type    string
}

// JoinDoc subscribes a client to a document and returns its current content.
func (s *Service) JoinDoc(
	ctx context.Context, c *socketio.Conn, cc *clientContext,
	docID string, fromVersion int64, options JoinDocOptions,
) (*JoinDocResult, error) {
	if c.Disconnected() {
		return nil, nil
	}
	// Claim an epoch up front. A leaveDoc or another joinDoc arriving while
	// this one is in flight bumps it, and this call then knows its result is
	// stale and must not be delivered.
	epoch := cc.joinLeaveEpoch.Add(1)

	projectID := cc.ProjectID()
	if projectID == "" {
		return nil, ErrNotJoined
	}
	s.log.Debug("client joining doc", slog.String("user", cc.UserID()),
		slog.String("project", projectID), slog.String("doc", docID),
		slog.Int64("fromVersion", fromVersion), slog.String("client", c.ID))

	if err := s.assertClientAuthorization(ctx, cc, docID); err != nil {
		return nil, err
	}
	if c.Disconnected() {
		return nil, nil
	}
	if epoch != cc.joinLeaveEpoch.Load() {
		return nil, ErrJoinLeaveEpochMismatch
	}

	// The per-doc applied-ops channel must be subscribed before the document
	// is sent, or an update applied in between would be lost and the client
	// would edit from a stale version.
	if err := s.rooms.JoinDoc(ctx, c, docID); err != nil {
		return nil, err
	}
	if c.Disconnected() {
		return nil, nil
	}

	doc, err := s.du.GetDocument(ctx, projectID, docID, fromVersion)
	if err != nil {
		return nil, err
	}
	if c.Disconnected() {
		return nil, nil
	}

	ranges := doc.Ranges
	if cc.IsRestrictedUser() {
		// Comments carry the names of collaborators a restricted user is not
		// allowed to see.
		ranges = stripComments(ranges)
	}

	lines := doc.Lines
	if doc.Type == "history-ot" {
		if !options.SupportsHistoryOT {
			// An old editor cannot read this document at all, so it is not
			// left holding a doc it will corrupt.
			s.rooms.LeaveDoc(ctx, c, docID)
			return nil, CodedError("client does not support history-ot", "")
		}
	} else {
		lines, err = encodeLinesForWebsockets(lines)
		if err != nil {
			return nil, err
		}
		if options.EncodeRanges {
			ranges = encodeRangesForWebsockets(ranges)
		}
	}

	cc.addAccessToDoc(docID)
	s.log.Debug("client joined doc", slog.String("user", cc.UserID()),
		slog.String("project", projectID), slog.String("doc", docID),
		slog.String("client", c.ID))

	return &JoinDocResult{
		Lines: lines, Version: doc.Version, Ops: doc.Ops, Ranges: ranges, Type: doc.Type,
	}, nil
}

// assertClientAuthorization checks project access, then document access,
// asking document-updater only when the answer is not already cached.
func (s *Service) assertClientAuthorization(ctx context.Context, cc *clientContext, docID string) error {
	if !canView[cc.PrivilegeLevel()] {
		return ErrNotAuthorized
	}
	if cc.canAccessDoc(docID) {
		return nil
	}
	// Not cached: confirm with document-updater that the document really
	// belongs to this project, so a client cannot open somebody else's
	// document through a project it can read.
	if err := s.du.CheckDocument(ctx, cc.ProjectID(), docID); err != nil {
		return err
	}
	cc.addAccessToDoc(docID)
	return nil
}

// LeaveDoc unsubscribes a client from a document.
func (s *Service) LeaveDoc(ctx context.Context, c *socketio.Conn, cc *clientContext, docID string) {
	cc.joinLeaveEpoch.Add(1)
	s.log.Debug("client leaving doc", slog.String("user", cc.UserID()),
		slog.String("doc", docID), slog.String("client", c.ID))
	s.rooms.LeaveDoc(ctx, c, docID)
	// Document access stays cached. The connection is per project, and the
	// client is already known to be allowed here, so re-joining should not
	// need another round trip.
}

// UpdateClientPosition records where a user's cursor is and tells the room.
func (s *Service) UpdateClientPosition(ctx context.Context, c *socketio.Conn, cc *clientContext, cursor map[string]any) error {
	if c.Disconnected() {
		return nil // do not create a ghost entry in redis
	}
	docID, _ := cursor["doc_id"].(string)

	cc.mu.RLock()
	projectID, user := cc.projectID, cc.user
	cc.mu.RUnlock()

	if !canView[cc.PrivilegeLevel()] || !cc.canAccessDoc(docID) {
		// The editor sends cursor updates before joinProject has completed.
		// That is not an error worth reporting to the user.
		s.log.Debug("silently ignoring unauthorized updateClientPosition",
			slog.String("client", c.ID), slog.String("project", projectID))
		return nil
	}

	cursor["id"] = cc.PublicID
	userID := ""
	if user != nil {
		userID = user.ID
		if user.Email != "" {
			cursor["email"] = user.Email
		}
	}
	if userID != "" {
		cursor["user_id"] = userID
	}

	var err error
	if userID == "" || userID == "anonymous-user" {
		// Anonymous users are not stored: on a popular public link they would
		// flood Redis, and there is no name to show anyway.
		cursor["name"] = ""
	} else {
		cursor["name"] = displayName(user)
		err = s.users.UpdateUserPosition(ctx, projectID, cc.PublicID, user, map[string]any{
			"row": cursor["row"], "column": cursor["column"], "doc_id": cursor["doc_id"],
		})
	}
	s.EmitToRoom(ctx, projectID, "clientTracking.clientUpdated", cursor)
	return err
}

// GetConnectedUsers lists the collaborators in the project.
func (s *Service) GetConnectedUsers(ctx context.Context, c *socketio.Conn, cc *clientContext) ([]ConnectedUser, error) {
	if c.Disconnected() {
		return nil, nil
	}
	if cc.IsRestrictedUser() {
		return []ConnectedUser{}, nil
	}
	projectID := cc.ProjectID()
	if projectID == "" {
		return nil, ErrNotJoined
	}
	if !canView[cc.PrivilegeLevel()] {
		return nil, ErrNotAuthorized
	}

	// Ask every instance to renew its clients, then wait long enough for the
	// answers to land. Entries nobody refreshed are stale and get filtered out
	// by GetConnectedUsers.
	s.EmitToRoom(ctx, projectID, "clientTracking.refresh")
	select {
	case <-time.After(clientRefreshDelay):
	case <-c.Done():
		return nil, nil
	}
	return s.users.GetConnectedUsers(ctx, projectID)
}

// ApplyOtUpdate queues an edit for document-updater.
func (s *Service) ApplyOtUpdate(
	ctx context.Context, c *socketio.Conn, cc *clientContext,
	docID string, update map[string]json.RawMessage,
) error {
	projectID := cc.ProjectID()
	if projectID == "" {
		return ErrNotJoined
	}

	if err := s.assertClientCanApplyUpdate(cc, docID, update); err != nil {
		// Give the client a moment to receive the error before the connection
		// goes away, or it reports a network fault instead of the real reason.
		time.AfterFunc(100*time.Millisecond, c.Close)
		return err
	}

	if raw, ok := update["doc"]; ok {
		var updateDoc string
		if json.Unmarshal(raw, &updateDoc) == nil && updateDoc != docID {
			return CodedError(
				"update.doc must be identical to docId parameter in applyOtUpdate(docId, update)", "")
		}
	}
	encodedDocID, err := json.Marshal(docID)
	if err != nil {
		return err
	}
	update["doc"] = encodedDocID

	meta := map[string]json.RawMessage{}
	if raw, ok := update["meta"]; ok {
		_ = json.Unmarshal(raw, &meta)
	}
	meta["source"], _ = json.Marshal(cc.PublicID)
	meta["user_id"], _ = json.Marshal(cc.UserID())
	// tsRT is the ingestion time, used to measure how long an update takes to
	// come back. It is stripped again before the update reaches any client.
	meta["tsRT"], _ = json.Marshal(monotonicMillis())
	encodedMeta, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	update["meta"] = encodedMeta

	s.log.Debug("sending update to doc updater", slog.String("user", cc.UserID()),
		slog.String("doc", docID), slog.String("project", projectID),
		slog.String("client", c.ID))

	err = s.QueueChange(ctx, projectID, docID, update)
	switch {
	case err == nil:
		return nil
	case err == ErrUpdateTooLarge:
		s.log.Warn("update is too large", slog.String("user", cc.UserID()),
			slog.String("project", projectID), slog.String("doc", docID))
		// The update is acknowledged so the client stops resending it, and
		// then pushed out of sync deliberately: it has to reload rather than
		// keep editing from a state the server never accepted.
		time.AfterFunc(100*time.Millisecond, func() {
			if c.Disconnected() {
				return
			}
			_ = c.Emit("otUpdateError", "update is too large", map[string]any{
				"project_id": projectID, "doc_id": docID, "error": "update is too large",
			})
			c.Close()
		})
		return nil
	default:
		c.Close()
		return err
	}
}

// assertClientCanApplyUpdate picks the permission an update needs from what it
// does: a comment needs only read access, a tracked change needs review
// rights, and anything else needs write access.
func (s *Service) assertClientCanApplyUpdate(cc *clientContext, docID string, update map[string]json.RawMessage) error {
	required := canEdit
	switch {
	case isCommentUpdate(update):
		required = canView
	case hasTrackedChanges(update):
		required = canReview
	}
	if !required[cc.PrivilegeLevel()] || !cc.canAccessDoc(docID) {
		return ErrNotAuthorized
	}
	return nil
}

// isCommentUpdate reports whether every operation in the update is a comment.
func isCommentUpdate(update map[string]json.RawMessage) bool {
	raw, ok := update["op"]
	if !ok {
		return false
	}
	var ops []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &ops); err != nil || len(ops) == 0 {
		return false
	}
	for _, op := range ops {
		if _, isComment := op["c"]; !isComment {
			return false
		}
	}
	return true
}

// hasTrackedChanges reports whether the update is marked as a tracked change.
func hasTrackedChanges(update map[string]json.RawMessage) bool {
	raw, ok := update["meta"]
	if !ok {
		return false
	}
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(raw, &meta); err != nil {
		return false
	}
	_, tracked := meta["tc"]
	return tracked
}

func displayName(user *User) string {
	if user == nil {
		return ""
	}
	switch {
	case user.FirstName != "" && user.LastName != "":
		return user.FirstName + " " + user.LastName
	case user.FirstName != "":
		return user.FirstName
	default:
		return user.LastName
	}
}

var processStart = time.Now()

// monotonicMillis is the Go equivalent of performance.now(): milliseconds
// since the process started, with sub-millisecond precision.
func monotonicMillis() float64 {
	return float64(time.Since(processStart).Nanoseconds()) / 1e6
}
