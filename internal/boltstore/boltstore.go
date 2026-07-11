// Package boltstore provides an embedded, transactional key-value store backed by bbolt,
// with values encrypted at rest when a key is set. It is the database backing behind
// vamoose's store interfaces, giving atomic crash-safe writes and multi-tenant scale
// without an external service. One DB file holds a bucket per store.
package boltstore

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/dcadolph/vamoose/internal/secret"
)

// DB is an embedded bbolt database whose values are sealed with AES-256-GCM when an
// encryption key is configured.
type DB struct {
	// bolt is the underlying database handle.
	bolt *bolt.DB
	// box seals and opens values when set; nil keeps them plaintext.
	box *secret.Box
}

// Open opens, creating if needed, the bbolt database at path, encrypting values at rest
// when VAMOOSE_SECRET_KEY is set. Parent directories are created.
func Open(path string) (*DB, error) {
	box, err := secret.FromEnv(os.Getenv)
	if err != nil && !errors.Is(err, secret.ErrNoKey) {
		return nil, err
	}
	if errors.Is(err, secret.ErrNoKey) {
		box = nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}
	return &DB{bolt: db, box: box}, nil
}

// Close closes the database.
func (d *DB) Close() error { return d.bolt.Close() }

// Put stores value under key in bucket in one atomic transaction, encrypting when
// configured.
func (d *DB) Put(bucket, key string, value []byte) error {
	sealed, err := d.seal(value)
	if err != nil {
		return err
	}
	return d.bolt.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return err
		}
		return b.Put([]byte(key), sealed)
	})
}

// Get returns the value under key in bucket, decrypting it. ok is false when absent.
func (d *DB) Get(bucket, key string) (value []byte, ok bool, err error) {
	err = d.bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		v := b.Get([]byte(key))
		if v == nil {
			return nil
		}
		out, oerr := d.open(v)
		if oerr != nil {
			return oerr
		}
		value, ok = out, true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return value, ok, nil
}

// Delete removes key from bucket, succeeding even when it is absent.
func (d *DB) Delete(bucket, key string) error {
	return d.bolt.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(key))
	})
}

// List returns every key and its decrypted value in bucket.
func (d *DB) List(bucket string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := d.bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			dec, derr := d.open(v)
			if derr != nil {
				return derr
			}
			out[string(k)] = dec
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// seal encrypts when a box is set, otherwise copies the plaintext so the caller owns it.
func (d *DB) seal(plaintext []byte) ([]byte, error) {
	if d.box == nil {
		return append([]byte(nil), plaintext...), nil
	}
	return d.box.Seal(plaintext)
}

// open decrypts when a box is set, otherwise copies the stored bytes, which bbolt only
// lends for the transaction's lifetime.
func (d *DB) open(stored []byte) ([]byte, error) {
	if d.box == nil {
		return append([]byte(nil), stored...), nil
	}
	return d.box.Open(stored)
}
