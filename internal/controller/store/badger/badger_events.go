//nolint:dupl // maybe we'll figure out how to make DB resource accessors generic in the future
package badger

import (
	"encoding/binary"
	"encoding/json"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/dgraph-io/badger/v3"
	"math/rand"
	"path"
)

const SpaceEvents = "/events"

func NewEventKey(event v1.Event, scope ...string) []byte {
	key := ScopePrefix(scope)
	timestampBytes := make([]byte, 8+4)
	binary.BigEndian.PutUint64(timestampBytes, event.Timestamp)
	// append random bytes to deduplicate events with the same timestamp if any
	binary.BigEndian.PutUint32(timestampBytes, rand.Uint32())
	key = append(key, timestampBytes...)
	return key
}

func ScopePrefix(scope []string) []byte {
	keyParts := []string{SpaceEvents}
	keyParts = append(keyParts, scope...)
	return []byte(path.Join(keyParts...))
}
func (txn *Transaction) AppendEvent(event v1.Event, scope ...string) (err error) {
	defer func() {
		err = mapErr(err)
	}()

	key := NewEventKey(event, scope...)

	valueBytes, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return txn.badgerTxn.Set(key, valueBytes)
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
