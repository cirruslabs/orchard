package badger

import (
	"encoding/json"
	"fmt"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/dgraph-io/badger/v3"
	"path"
	"time"
)

const SpaceEvents = "/events"

func scopePrefix(scope []string) []byte {
	keyParts := []string{SpaceEvents}
	keyParts = append(keyParts, scope...)
	return []byte(path.Join(keyParts...))
}
func (txn *Transaction) AppendEvents(events []v1.Event, scope ...string) (err error) {
	defer func() {
		err = mapErr(err)
	}()

	injectionTime := time.Now().UnixNano()

	for index, event := range events {
		valueBytes, err := json.Marshal(event)
		if err != nil {
			return err
		}
		eventUID := fmt.Sprintf("/%d-%d-%06d",
			event.Timestamp,
			injectionTime, // to preserve order in case two sequential batches have events with the same timestamp
			index,         // to preserve order in case a batch of events has some events with the same timestamp
		)

		eventKey := scopePrefix(scope)
		eventKey = append(eventKey, []byte(eventUID)...)

		err = txn.badgerTxn.Set(eventKey, valueBytes)
		if err != nil {
			return err
		}
	}
	return nil
}

func (txn *Transaction) ListEvents(scope ...string) (_ []v1.Event, err error) {
	defer func() {
		err = mapErr(err)
	}()

	// Declare an empty, non-nil slice to
	// return [] when no events are found
	result := []v1.Event{}

	it := txn.badgerTxn.NewIterator(badger.IteratorOptions{
		Prefix: scopePrefix(scope),
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
		Prefix:         scopePrefix(scope),
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
