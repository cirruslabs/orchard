package badger

import (
	"github.com/dgraph-io/badger/v3"
	"github.com/google/uuid"
)

const SpaceWatchBarrier = "/watch-barrier"

func WatchBarrierKey() []byte {
	return []byte(SpaceWatchBarrier)
}

func (store *Store) notifyWatchBarrier() error {
	return store.db.Update(func(txn *badger.Txn) error {
		return txn.Set(WatchBarrierKey(), []byte(uuid.NewString()))
	})
}
