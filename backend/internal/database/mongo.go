package database

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoDB struct {
	Client   *mongo.Client
	Database *mongo.Database
}

func Connect(uri, dbName string) (*MongoDB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	db := client.Database(dbName)
	mdb := &MongoDB{Client: client, Database: db}
	mdb.ensureIndexes()
	return mdb, nil
}

func (m *MongoDB) ensureIndexes() {
	ctx := context.Background()

	m.Users().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	m.Users().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "apiToken", Value: 1}},
	})

	m.Posts().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "userId", Value: 1}}},
		{Keys: bson.D{{Key: "teamId", Value: 1}}},
		{Keys: bson.D{{Key: "scheduledAt", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}}},
		{Keys: bson.D{{Key: "scheduledAt", Value: 1}, {Key: "status", Value: 1}}},
	})

	m.Teams().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "apiToken", Value: 1}},
	})

	m.Uploads().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "filename", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	m.Suffixes().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "userId", Value: 1}}},
		{Keys: bson.D{{Key: "teamId", Value: 1}}},
	})

	m.Watermarks().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "userId", Value: 1}}},
		{Keys: bson.D{{Key: "teamId", Value: 1}}},
	})

	m.Mentions().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "userId", Value: 1}}},
		{Keys: bson.D{{Key: "teamId", Value: 1}}},
	})

	m.ConventionQueues().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "userId", Value: 1}}},
		{Keys: bson.D{{Key: "teamId", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}}},
	})

	m.ConventionQueueItems().Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "queueId", Value: 1}}},
		{Keys: bson.D{{Key: "queueId", Value: 1}, {Key: "status", Value: 1}}},
		{Keys: bson.D{{Key: "queueId", Value: 1}, {Key: "sortOrder", Value: 1}}},
	})

	log.Println("MongoDB indexes ensured")
}

func (m *MongoDB) Users() *mongo.Collection {
	return m.Database.Collection("users")
}

func (m *MongoDB) Posts() *mongo.Collection {
	return m.Database.Collection("posts")
}

func (m *MongoDB) SocialAccounts() *mongo.Collection {
	return m.Database.Collection("social_accounts")
}

func (m *MongoDB) Teams() *mongo.Collection {
	return m.Database.Collection("teams")
}

func (m *MongoDB) Uploads() *mongo.Collection {
	return m.Database.Collection("uploads")
}

func (m *MongoDB) Settings() *mongo.Collection {
	return m.Database.Collection("settings")
}

func (m *MongoDB) Suffixes() *mongo.Collection {
	return m.Database.Collection("suffixes")
}

func (m *MongoDB) Watermarks() *mongo.Collection {
	return m.Database.Collection("watermarks")
}

func (m *MongoDB) Mentions() *mongo.Collection {
	return m.Database.Collection("mentions")
}

func (m *MongoDB) ConventionQueues() *mongo.Collection {
	return m.Database.Collection("convention_queues")
}

func (m *MongoDB) ConventionQueueItems() *mongo.Collection {
	return m.Database.Collection("convention_queue_items")
}

func (m *MongoDB) MastodonOAuthStates() *mongo.Collection {
	return m.Database.Collection("mastodon_oauth_states")
}

func (m *MongoDB) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m.Client.Disconnect(ctx)
}
