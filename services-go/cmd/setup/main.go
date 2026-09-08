// Command setup is what runs before the services do.
//
// Three things the container needs done between starting and being a site:
// the settings an administrator changed have to reach the environment every
// service is built from, the databases have to be reachable, and the TeX Live
// images a project might ask for have to exist. All three used to be scripts
// inside the Node web service, which is the last reason that service was in
// the image.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/api/settings"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/config"
	"github.com/Uniseem/ayakaleaf-pro/services-go/internal/mongox"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: setup environment|check-databases|check-texlive|ensure-indexes")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var err error
	switch os.Args[1] {
	case "environment":
		err = writeEnvironment(ctx)
	case "check-databases":
		err = checkDatabases(ctx)
	case "check-texlive":
		err = checkTexLive(ctx)
	case "ensure-indexes":
		err = ensureIndexes(ctx)
	default:
		fmt.Fprintf(os.Stderr, "setup: no such command %q\n", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

// writeEnvironment materialises the stored settings.
//
// Nothing here is fatal. A site whose database is not up yet should start with
// the environment it was given and fail in the check that exists for that,
// with the message that check has, rather than here.
func writeEnvironment(ctx context.Context) error {
	directory := config.Env("CONTAINER_ENVIRONMENT_DIR", "/etc/container_environment")
	// Not inside that directory: everything in there becomes a variable, and a
	// manifest is not one. On the data volume so it survives the container
	// being replaced, which is when it matters.
	manifest := config.Env("SITE_SETTINGS_MANIFEST",
		"/var/lib/overleaf/data/site-settings-env.json")

	client, db, err := mongox.Connect(ctx)
	if err != nil {
		fmt.Println("Could not read the site settings; starting with the environment as it is")
		return nil
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	var stored struct {
		Values map[string]json.RawMessage `bson:"values"`
	}
	err = db.Collection("siteSettings").
		FindOne(ctx, bson.M{"_id": "site"}).Decode(&stored)
	if err != nil {
		// A site that has never been set up. What it was started with is all
		// there is.
		fmt.Println("No stored site settings yet, leaving the environment alone")
		return nil
	}

	written, err := settings.WriteEnvironment(directory, manifest, stored.Values)
	if err != nil {
		fmt.Println("Could not write the site settings into the environment:", err)
		return nil
	}
	fmt.Printf("Applied %d settings from the database\n", written)
	return nil
}

// checkDatabases refuses to let the container start without them.
//
// Every service would fail one at a time and restart for ever, which is a
// harder thing to read than one message here.
func checkDatabases(ctx context.Context) error {
	client, db, err := mongox.Connect(ctx)
	if err != nil {
		return fmt.Errorf("cannot connect to mongo: %w", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	var hello struct {
		SetName           string `bson:"setName"`
		IsWritablePrimary bool   `bson:"isWritablePrimary"`
	}
	if err := db.RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return fmt.Errorf("mongo did not answer: %w", err)
	}
	if hello.SetName == "" {
		// Not a preference: the application writes several documents at once
		// and needs transactions, which a standalone mongod does not have.
		return fmt.Errorf("mongo is not a replica set, which this application needs")
	}
	if !hello.IsWritablePrimary {
		return fmt.Errorf("mongo is a replica set with no primary, so it cannot take writes")
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr("web"),
		Password: config.RedisPassword("web"),
	})
	defer func() { _ = rdb.Close() }()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("cannot connect to redis: %w", err)
	}

	fmt.Println("Mongo and Redis are both reachable")
	return nil
}

// checkTexLive refuses to start when a project names a TeX Live image the
// deployment no longer offers.
//
// Only with sandboxed compiles: without them every project compiles in the one
// image this container has, and the name a project carries is not used.
func checkTexLive(ctx context.Context) error {
	if os.Getenv("SKIP_TEX_LIVE_CHECK") == "true" {
		fmt.Println("Skipping the TeX Live check")
		return nil
	}
	if os.Getenv("SANDBOXED_COMPILES") != "true" {
		fmt.Println("Compiles are not sandboxed, so there is one image and nothing to check")
		return nil
	}

	offered := map[string]bool{}
	for _, image := range strings.Split(os.Getenv("ALL_TEX_LIVE_DOCKER_IMAGES"), ",") {
		if image = strings.TrimSpace(image); image != "" {
			offered[identity(image)] = true
		}
	}
	if len(offered) == 0 {
		return fmt.Errorf("sandboxed compiles are on but no TeX Live images are listed; " +
			"set them in the admin settings")
	}

	client, db, err := mongox.Connect(ctx)
	if err != nil {
		return fmt.Errorf("cannot connect to mongo: %w", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	projects := db.Collection("projects")
	count, err := projects.CountDocuments(ctx, bson.M{})
	if err != nil {
		return err
	}
	if count == 0 {
		fmt.Println("No projects yet, so no image to be missing")
		return nil
	}

	var inUse []string
	if err := projects.Distinct(ctx, "imageName", bson.M{}).Decode(&inUse); err != nil {
		return err
	}

	var missing []string
	unset := len(inUse) == 0
	for _, image := range inUse {
		if strings.TrimSpace(image) == "" {
			unset = true
			continue
		}
		if !offered[identity(image)] {
			missing = append(missing, image)
		}
	}

	if unset {
		return fmt.Errorf("some projects do not say which TeX Live they use; " +
			"set SKIP_TEX_LIVE_CHECK=true to start anyway")
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("projects use TeX Live images this site no longer offers: %s",
			strings.Join(missing, ", "))
	}

	fmt.Printf("All %d TeX Live images in use are offered\n", len(offered))
	return nil
}

// identity is which TeX Live an image reference names, apart from where it is
// pulled from.
//
// A deployment that moves to a different registry keeps the same TeX Live
// under a different address, and every project made before the move still
// names the old one. Those are not missing images, and refusing to start over
// them turns a change of registry into an outage.
func identity(image string) string {
	if slash := strings.LastIndexByte(image, '/'); slash >= 0 {
		return image[slash+1:]
	}
	return image
}
