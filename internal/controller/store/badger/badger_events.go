package badger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"time"

	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/dgraph-io/badger/v3"
)

const SpaceEvents = "/events"

func scopePrefix(scope []string) []byte {
	keyParts := []string{SpaceEvents}
	keyParts = append(keyParts, scope...)
	return []byte(path.Join(keyParts...))
}

func prefixEnd(prefix []byte) []byte {
	end := make([]byte, len(prefix)+1)
	copy(end, prefix)
	end[len(prefix)] = 0xFF
	return end
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
	page, err := txn.ListEventsPage(storepkg.ListOptions{}, scope...)
	if err != nil {
		return nil, err
	}

	return page.Items, nil
}

func (txn *Transaction) ListEventsPage(options storepkg.ListOptions, scope ...string) (
	result storepkg.Page[v1.Event],
	err error,
) {
	defer func() {
		err = mapErr(err)
	}()

	// Declare an empty, non-nil slice to
	// return [] when no events are found
	result.Items = []v1.Event{}

	prefix := scopePrefix(scope)
	itOptions := badger.DefaultIteratorOptions
	itOptions.Prefix = prefix
	itOptions.Reverse = options.Order == storepkg.ListOrderDesc

	it := txn.badgerTxn.NewIterator(itOptions)
	defer it.Close()

	if len(options.Cursor) > 0 {
		it.Seek(options.Cursor)
		if it.ValidForPrefix(prefix) && bytes.Equal(it.Item().Key(), options.Cursor) {
			it.Next()
		}
	} else if itOptions.Reverse {
		it.Seek(prefixEnd(prefix))
	} else {
		it.Rewind()
	}

	for it.ValidForPrefix(prefix) {
		item := it.Item()

		eventBytes, err := item.ValueCopy(nil)
		if err != nil {
			return result, err
		}

		var event v1.Event

		if err := json.Unmarshal(eventBytes, &event); err != nil {
			return result, err
		}

		result.Items = append(result.Items, event)

		if options.Limit > 0 && len(result.Items) >= options.Limit {
			lastKey := item.KeyCopy(nil)
			it.Next()
			if it.ValidForPrefix(prefix) {
				result.NextCursor = lastKey
			}
			break
		}

		it.Next()
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
