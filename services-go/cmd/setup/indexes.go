package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// The indexes every service relies on.
//
// They used to be a set of migrations run by a Node tool at startup: one file
// per change, going back years. What survives of that is the end state, which
// is the only part a new deployment needs -- the intermediate steps were about
// getting old data from one shape to another, and there is no old data on a
// site being installed now.
//
// Creating an index that is already there is not an error, so this runs on
// every start and is the answer for an existing deployment too.

type indexSpec struct {
	collection  string
	name        string
	keys        bson.D
	unique      bool
	sparse      bool
	partial     string
	expireAfter int
}

// projectHistoryChunks is not here: the history store creates its own, and two
// different uniqueness rules on one collection is one of them refusing writes.
var indexes = []indexSpec{
	{collection: "docSnapshots", name: "project_id_1", keys: bson.D{{Key: "project_id", Value: 1}}},
	{collection: "docSnapshots", name: "ts_1", keys: bson.D{{Key: "ts", Value: 1}}, expireAfter: 2592000},
	{collection: "docs", name: "project_id_1", keys: bson.D{{Key: "project_id", Value: 1}}},
	{collection: "docs", name: "project_id_deleted_deletedAt_1", keys: bson.D{{Key: "project_id", Value: 1}, {Key: "deleted", Value: 1}, {Key: "deletedAt", Value: -1}}},
	{collection: "messages", name: "room_id_1_timestamp_-1", keys: bson.D{{Key: "room_id", Value: 1}, {Key: "timestamp", Value: -1}}},
	{collection: "notifications", name: "expires_1", keys: bson.D{{Key: "expires", Value: 1}}, expireAfter: 10},
	{collection: "notifications", name: "key_1", keys: bson.D{{Key: "key", Value: 1}}},
	{collection: "notifications", name: "user_id_1", keys: bson.D{{Key: "user_id", Value: 1}}},
	{collection: "oauthAccessTokens", name: "accessToken_1", keys: bson.D{{Key: "accessToken", Value: 1}}, unique: true},
	{collection: "oauthAccessTokens", name: "refreshToken_1", keys: bson.D{{Key: "refreshToken", Value: 1}}, unique: true},
	{collection: "oauthAccessTokens", name: "pat_user_id_1", keys: bson.D{{Key: "user_id", Value: 1}}, partial: "{\"type\": \"pat\"}"},
	{collection: "tags", name: "user_id_1", keys: bson.D{{Key: "user_id", Value: 1}}},
	{collection: "tags", name: "user_name_unique", keys: bson.D{{Key: "user_id", Value: 1}, {Key: "name", Value: 1}}, unique: true},
	{collection: "tags", name: "project_ids_1", keys: bson.D{{Key: "project_ids", Value: 1}}},
	{collection: "projectHistoryFailures", name: "project_id_1", keys: bson.D{{Key: "project_id", Value: 1}}},
	{collection: "projectHistoryLabels", name: "project_id_1", keys: bson.D{{Key: "project_id", Value: 1}}},
	{collection: "projectHistoryLabels", name: "user_id_1", keys: bson.D{{Key: "user_id", Value: 1}}},
	{collection: "projectHistorySyncState", name: "project_id_1", keys: bson.D{{Key: "project_id", Value: 1}}},
	{collection: "projectHistorySyncState", name: "expiresAt_1", keys: bson.D{{Key: "expiresAt", Value: 1}}, expireAfter: 0},
	{collection: "projects", name: "active_1_lastOpened_1", keys: bson.D{{Key: "active", Value: 1}, {Key: "lastOpened", Value: 1}}},
	{collection: "projects", name: "tokens.readOnly_1", keys: bson.D{{Key: "tokens.readOnly", Value: 1}}, unique: true, partial: "{\"tokens.readOnly\": {\"$exists\": true}}"},
	{collection: "projects", name: "overleaf.history.id_1", keys: bson.D{{Key: "overleaf.history.id", Value: 1}}, unique: true, partial: "{\"overleaf.history.id\": {\"$exists\": true}}"},
	{collection: "projects", name: "tokens.readAndWritePrefix_1", keys: bson.D{{Key: "tokens.readAndWritePrefix", Value: 1}}, unique: true, partial: "{\"tokens.readAndWritePrefix\": {\"$exists\": true}}"},
	{collection: "projects", name: "publicAccesLevel_1", keys: bson.D{{Key: "publicAccesLevel", Value: 1}}},
	{collection: "projects", name: "owner_ref_1", keys: bson.D{{Key: "owner_ref", Value: 1}}},
	{collection: "projects", name: "tokenAccessReadAndWrite_refs_1", keys: bson.D{{Key: "tokenAccessReadAndWrite_refs", Value: 1}}},
	{collection: "projects", name: "readOnly_refs_1", keys: bson.D{{Key: "readOnly_refs", Value: 1}}},
	{collection: "projects", name: "tokenAccessReadOnly_refs_1", keys: bson.D{{Key: "tokenAccessReadOnly_refs", Value: 1}}},
	{collection: "projects", name: "overleaf.id_1", keys: bson.D{{Key: "overleaf.id", Value: 1}}, unique: true, partial: "{\"overleaf.id\": {\"$exists\": true}}"},
	{collection: "projects", name: "collaberator_refs_1", keys: bson.D{{Key: "collaberator_refs", Value: 1}}},
	{collection: "projects", name: "name_1", keys: bson.D{{Key: "name", Value: 1}}},
	{collection: "projects", name: "tokens.readAndWrite_1", keys: bson.D{{Key: "tokens.readAndWrite", Value: 1}}, unique: true, partial: "{\"tokens.readAndWrite\": {\"$exists\": true}}"},
	{collection: "projects", name: "collabratecUsers.user_id_1", keys: bson.D{{Key: "collabratecUsers.user_id", Value: 1}}, sparse: true},
	{collection: "projects", name: "brandVariationId_1", keys: bson.D{{Key: "brandVariationId", Value: 1}}},
	{collection: "projects", name: "overleaf.history.display_1", keys: bson.D{{Key: "overleaf.history.display", Value: 1}}},
	{collection: "projects", name: "overleaf.history.display_1", keys: bson.D{{Key: "overleaf.history.display", Value: 1}}},
	{collection: "projects", name: "reviewer_refs_1", keys: bson.D{{Key: "reviewer_refs", Value: 1}}},
	{collection: "projects", name: "overleaf_backup_pendingChangeAt_1", keys: bson.D{{Key: "overleaf.backup.pendingChangeAt", Value: 1}}, partial: "{\"overleaf.backup.pendingChangeAt\": {\"$exists\": true}}"},
	{collection: "projects", name: "overleaf.backup.lastBackedUpVersion_1", keys: bson.D{{Key: "overleaf.backup.lastBackedUpVersion", Value: 1}}, partial: "{\"overleaf.backup.lastBackedUpVersion\": {\"$in\": [null]}}"},
	{collection: "projects", name: "overleaf.backup.id_1_lastBackedUpVersion_id_1", keys: bson.D{{Key: "overleaf.backup.lastBackedUpVersion", Value: 1}, {Key: "_id", Value: 1}}, partial: "{\"overleaf.backup.lastBackedUpVersion\": {\"$in\": [null]}}"},
	{collection: "projects", name: "overleaf.backup.lastBackedUpVersion_1", keys: bson.D{{Key: "overleaf.backup.lastBackedUpVersion", Value: 1}}, partial: "{\"overleaf.backup.lastBackedUpVersion\": {\"$in\": [null]}}"},
	{collection: "projects", name: "owner_ref_1_lastUpdated_1_debugCopies", keys: bson.D{{Key: "overleaf.isDebugCopyOf", Value: 1}, {Key: "owner_ref", Value: 1}, {Key: "lastUpdated", Value: 1}}, partial: "{\"overleaf.isDebugCopyOf\": {\"$type\": \"objectId\"}}"},
	{collection: "rooms", name: "project_id", keys: bson.D{{Key: "project_id", Value: 1}}},
	{collection: "rooms", name: "project_id", keys: bson.D{{Key: "project_id", Value: 1}}},
	{collection: "rooms", name: "project_id_1_thread_id_1", keys: bson.D{{Key: "project_id", Value: 1}, {Key: "thread_id", Value: 1}}},
	{collection: "users", name: "email_case_insensitive", keys: bson.D{{Key: "email", Value: 1}}, unique: true},
	{collection: "users", name: "has dropbox", keys: bson.D{{Key: "dropbox.access_token.oauth_token_secret", Value: 1}}},
	{collection: "users", name: "overleaf.id_1", keys: bson.D{{Key: "overleaf.id", Value: 1}}, unique: true, partial: "{\"overleaf.id\": {\"$exists\": true}}"},
	{collection: "users", name: "thirdPartyIdentifiers.externalUserId_1_thirdPartyIdentifiers.providerId_1", keys: bson.D{{Key: "thirdPartyIdentifiers.externalUserId", Value: 1}, {Key: "thirdPartyIdentifiers.providerId", Value: 1}}, unique: true, sparse: true},
	{collection: "users", name: "subscription.freeTrialDowngraded_1", keys: bson.D{{Key: "subscription.freeTrialDowngraded", Value: 1}}},
	{collection: "users", name: "signUpDate", keys: bson.D{{Key: "signUpDate", Value: 1}}},
	{collection: "users", name: "emails_email_1", keys: bson.D{{Key: "emails.email", Value: 1}}, unique: true, partial: "{\"emails.email\": {\"$exists\": true}}"},
	{collection: "users", name: "emails_email_case_insensitive", keys: bson.D{{Key: "emails.email", Value: 1}}, unique: true, partial: "{\"emails.email\": {\"$exists\": true}}"},
	{collection: "users", name: "dropbox.access_token.uid_unique", keys: bson.D{{Key: "dropbox.access_token.uid", Value: 1}}, unique: true, sparse: true},
	{collection: "users", name: "password_and_email", keys: bson.D{{Key: "password", Value: 1}, {Key: "email", Value: 1}}},
	{collection: "users", name: "referal_id", keys: bson.D{{Key: "referal_id", Value: 1}}},
	{collection: "users", name: "subscription.freeTrialExpiresAt_1", keys: bson.D{{Key: "subscription.freeTrialExpiresAt", Value: 1}}},
	{collection: "users", name: "auth_token_1", keys: bson.D{{Key: "auth_token", Value: 1}}},
	{collection: "users", name: "email_1", keys: bson.D{{Key: "email", Value: 1}}, unique: true},
	{collection: "users", name: "emails.reversedHostname_1", keys: bson.D{{Key: "emails.reversedHostname", Value: 1}}},
	{collection: "users", name: "affiliationUnchecked_1", keys: bson.D{{Key: "emails.affiliationUnchecked", Value: 1}}, sparse: true},
	{collection: "users", name: "labsProgram_1", keys: bson.D{{Key: "labsProgram", Value: 1}}},
	{collection: "users", name: "labsProgramGalileo_1", keys: bson.D{{Key: "labsProgramGalileo", Value: 1}}},
	{collection: "users", name: "lastActive_1", keys: bson.D{{Key: "lastActive", Value: 1}}},
	{collection: "users", name: "isAdmin_1", keys: bson.D{{Key: "isAdmin", Value: 1}}, partial: "{\"isAdmin\": true}"},
}

// ensureIndexes creates them all.
//
// One at a time and not in a batch: an index that cannot be built -- because
// the data has a duplicate the uniqueness rule would refuse -- should stop
// that one index, not the rest.
func ensureIndexes(ctx context.Context) error {
	client, db, err := mongox.Connect(ctx)
	if err != nil {
		return fmt.Errorf("cannot connect to mongo: %w", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	created, failed := 0, 0
	for _, spec := range indexes {
		model := mongo.IndexModel{Keys: spec.keys}
		builder := options.Index().SetName(spec.name)
		if spec.unique {
			builder = builder.SetUnique(true)
		}
		if spec.sparse {
			builder = builder.SetSparse(true)
		}
		if spec.expireAfter > 0 {
			builder = builder.SetExpireAfterSeconds(int32(spec.expireAfter))
		}
		if spec.partial != "" {
			var filter bson.M
			if err := json.Unmarshal([]byte(spec.partial), &filter); err == nil {
				builder = builder.SetPartialFilterExpression(filter)
			}
		}
		model.Options = builder

		attempt, done := context.WithTimeout(ctx, 5*time.Minute)
		_, err := db.Collection(spec.collection).Indexes().CreateOne(attempt, model)
		done()
		if err == nil {
			created++
			continue
		}
		var cmdErr mongo.CommandError
		if errors.As(err, &cmdErr) && (cmdErr.Code == 85 || cmdErr.Code == 86) {
			// Already there, under this name or another. Nothing to do.
			continue
		}
		failed++
		fmt.Printf("could not create %s on %s: %v\n", spec.name, spec.collection, err)
	}

	fmt.Printf("Indexes: %d created, %d could not be\n", created, failed)
	if failed > 0 {
		// Not fatal. A missing index is a slow site, and refusing to start
		// over one would be a site nobody can reach at all.
		return nil
	}
	return nil
}
