package badger

import (
	"encoding/json"
	"fmt"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/dgraph-io/badger/v3"
	"math/rand"
	"path"
)

const SpaceEvents = "/events"

func ScopePrefix(scope []string) []byte {
	keyParts := []string{SpaceEvents}
	keyParts = append(keyParts, scope...)
	return []byte(path.Join(keyParts...))
}
func (txn *Transaction) AppendEvent(event v1.Event, scope ...string) (err error) {
	defer func() {
		err = mapErr(err)
	}()

	for index, event := range events {
		valueBytes, err := json.Marshal(event)
		if err != nil {
			return err
		}
		eventUID := fmt.Sprintf("/%d-%d-%d",
			event.Timestamp,
			index,         // to preserve order in case a batch of events has some events with the same timestamp
			rand.Uint32(), // extra cautions to avoid collisions in case several batches of events are appended at the same time
		)

		eventKey := ScopePrefix(scope)
		eventKey = append(eventKey, []byte(eventUID)...)

		err = txn.badgerTxn.Set(eventKey, valueBytes)
		if err != nil {
			return err
		}
	}
	return nil
}

func (txn *Transaction) ListEvents(scope ...string) (result []v1.Event, err error) {
	defer func() {
		err = mapErr(err)
	}()

	it := txn.badgerTxn.NewIterator(badger.IteratorOptions{
		Prefix: ScopePrefix(scope),
	})
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()

		eventBytes, err := item.ValueCopy(nil)
		if err != nil {
			return nil, err
		}

		var event v1.Event

		if err := json.Unmarshal(eventBytes, &event); err != nil {
			return nil, err
		}

		result = append(result, event)
	}

	return result, nil
}

func (txn *Transaction) DeleteEvents(scope ...string) (err error) {
	defer func() {
		err = mapErr(err)
	}()

	it := txn.badgerTxn.NewIterator(badger.IteratorOptions{
		Prefix:         ScopePrefix(scope),
		AllVersions:    false,
		PrefetchValues: false, // only need keys
	})
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		keyToDelete := it.Item().KeyCopy(nil)
		err := txn.badgerTxn.Delete(keyToDelete)
		if err != nil {
			return err
		}
	}

	return nil
}
