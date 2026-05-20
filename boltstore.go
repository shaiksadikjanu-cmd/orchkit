package orchkit

import (
	"context"
	"encoding/json"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

// BoltStore is a persistent Store backed by bbolt (embedded key-value DB).
// Data survives process restarts. Single file on disk, no server needed.
//
// Usage:
//
//	store, err := orchkit.NewBoltStore("/tmp/orchkit.db")
//	defer store.Close()
//	orchkit.Run(ctx, flow, store)
type BoltStore struct {
	db     *bolt.DB
	bucket []byte
}

var defaultBucket = []byte("orchkit")

// NewBoltStore opens (or creates) a bbolt database at the given path.
// Call Close() when done.
func NewBoltStore(path string) (*BoltStore, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("boltstore: open %s: %w", path, err)
	}

	// Create the bucket if it doesn't exist.
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(defaultBucket)
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("boltstore: init bucket: %w", err)
	}

	return &BoltStore{db: db, bucket: defaultBucket}, nil
}

// Close releases the database file. Always call this when done.
func (b *BoltStore) Close() error {
	return b.db.Close()
}

func (b *BoltStore) Get(_ context.Context, key string) (any, bool, error) {
	var val any
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(b.bucket)
		raw := bucket.Get([]byte(key))
		if raw == nil {
			return nil
		}
		return json.Unmarshal(raw, &val)
	})
	if err != nil {
		return nil, false, fmt.Errorf("boltstore: get %q: %w", key, err)
	}
	return val, val != nil, nil
}

func (b *BoltStore) Put(_ context.Context, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("boltstore: marshal %q: %w", key, err)
	}
	return b.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(b.bucket).Put([]byte(key), raw)
	})
}

func (b *BoltStore) Snapshot(_ context.Context) (map[string]any, error) {
	out := map[string]any{}
	err := b.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(b.bucket).ForEach(func(k, v []byte) error {
			var val any
			if err := json.Unmarshal(v, &val); err != nil {
				return err
			}
			out[string(k)] = val
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("boltstore: snapshot: %w", err)
	}
	return out, nil
}

// Clear wipes all keys from the store. Useful for testing or restarting a flow.
func (b *BoltStore) Clear() error {
	return b.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(b.bucket); err != nil {
			return err
		}
		_, err := tx.CreateBucket(b.bucket)
		return err
	})
}
