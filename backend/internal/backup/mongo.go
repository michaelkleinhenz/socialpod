package backup

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// MongoStore is the Store over a MongoDB database.
type MongoStore struct{ DB *mongo.Database }

func (s MongoStore) CollectionNames(ctx context.Context) ([]string, error) {
	// Views have no documents of their own to back up or restore.
	return s.DB.ListCollectionNames(ctx, bson.M{"type": "collection"})
}

func (s MongoStore) Each(ctx context.Context, collection string, fn func(doc bson.Raw) error) error {
	cur, err := s.DB.Collection(collection).Find(ctx, bson.M{})
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		if err := fn(cur.Current); err != nil {
			return err
		}
	}
	return cur.Err()
}

func (s MongoStore) Count(ctx context.Context, collection string) (int64, error) {
	return s.DB.Collection(collection).CountDocuments(ctx, bson.M{})
}

func (s MongoStore) Clear(ctx context.Context, collection string) error {
	_, err := s.DB.Collection(collection).DeleteMany(ctx, bson.M{})
	return err
}

func (s MongoStore) Insert(ctx context.Context, collection string, docs []interface{}) error {
	_, err := s.DB.Collection(collection).InsertMany(ctx, docs)
	return err
}
