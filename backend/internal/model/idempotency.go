package model

import "time"

// IdempotencyKey is a single document in the Mongo `idempotency_keys` collection
// (T2.6). `_id` IS the client- or server-derived key and is therefore naturally
// unique (Mongo enforces it, surfacing an E11000 duplicate-key error on conflict —
// the signal that the key has already been recorded by this process or another
// instance). `ExpireAt` backs the TTL index (expireAfterSeconds=0) that auto-expires
// entries at their stored timestamp, so the collection never grows unbounded.
//
// The store layer keeps these documents only in Mongo and does not load them into a
// typed in-memory slice; this type exists so InsertOne / FindOne share one explicit
// schema with the index definition declared in store/mongo_state.go.
type IdempotencyKey struct {
	Key      string    `bson:"_id"`
	ExpireAt time.Time `bson:"expire_at"`
}
