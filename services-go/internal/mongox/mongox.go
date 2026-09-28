// Package mongox connects to the same MongoDB instance and database as the
// Node services, using the connection string from the environment.
package mongox

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/config"
)

// Connect dials Mongo and verifies the connection with a ping, mirroring the
// Node services' behaviour of exiting when Mongo is unreachable at startup.
func Connect(ctx context.Context) (*mongo.Client, *mongo.Database, error) {
	uri := config.MongoURL()
	dbName := config.MongoDatabase(uri)
	if dbName == "" {
		return nil, nil, errors.New("mongo connection string does not name a database")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, err
	}
	return client, client.Database(dbName), nil
}
