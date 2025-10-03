package badger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/badger/v3/pb"
)

func (store *Store) WatchVM(ctx context.Context, vmName string) (chan storepkg.WatchMessage[v1.VM], chan error, error) {
	readyCh := make(chan struct{}, 1)
	watchCh := make(chan storepkg.WatchMessage[v1.VM], 1)
	errCh := make(chan error, 1)
	subCtx, subCtxCancel := context.WithCancel(ctx)

	go func() {
		defer subCtxCancel()
		defer close(readyCh)
		defer close(watchCh)

		var initialVM *v1.VM
		var checkedInitialVM bool

		if err := store.db.Subscribe(subCtx, func(kvList *badger.KVList) error {
			if !checkedInitialVM {
				// Notify the caller that we've subscribed, but don't block,
				// because we may observe multiple watch barriers, yet
				// we only need a single barrier to make things work
				select {
				case readyCh <- struct{}{}:
				default:
				}

				// Now that the subscription has started,
				// retrieve the initial VM, if any
				err := store.View(func(txn storepkg.Transaction) error {
					var err error

					initialVM, err = txn.GetVM(vmName)

					return err
				})
				if err != nil && !errors.Is(err, storepkg.ErrNotFound) {
					return err
				}
				if initialVM != nil {
					watchCh <- storepkg.WatchMessage[v1.VM]{
						Type:   storepkg.WatchMessageTypeAdded,
						Object: *initialVM,
					}
				}

				checkedInitialVM = true
			}

			for _, kv := range kvList.GetKv() {
				switch {
				case bytes.Equal(kv.GetKey(), WatchBarrierKey()):
					// We only need watch barriers so that the Subscribe()'s callback
					// is called at least once, thus we can simply do nothing here
				case bytes.Equal(kv.GetKey(), VMKey(vmName)):
					// Skip all KVs with versions before or equal
					// to the initial VM's version, if any
					if initialVM != nil && kv.GetVersion() <= initialVM.Version {
						continue
					}

					if kv.GetValue() == nil {
						// VM was deleted
						watchCh <- storepkg.WatchMessage[v1.VM]{
							Type: storepkg.WatchMessageTypeDeleted,
						}

						initialVM = nil
					} else {
						// VM was created or modified
						var vm v1.VM

						if err := json.Unmarshal(kv.GetValue(), &vm); err != nil {
							return err
						}

						vm.Version = kv.GetVersion()

						var watchMessageType storepkg.WatchMessageType

						if initialVM == nil {
							watchMessageType = storepkg.WatchMessageTypeAdded

							initialVM = &vm
						} else {
							watchMessageType = storepkg.WatchMessageTypeModified
						}

						watchCh <- storepkg.WatchMessage[v1.VM]{
							Type:   watchMessageType,
							Object: vm,
						}
					}
				default:
					// Ignore unexpected keys
					continue
				}
			}

			return nil
		}, []pb.Match{
			{
				Prefix: WatchBarrierKey(),
			},
			{
				Prefix: VMKey(vmName),
			},
		}); err != nil {
			errCh <- err
		}
	}()

	// Trigger the watch barrier so that Subscribe() callback gets invoked
	if err := store.notifyWatchBarrier(); err != nil {
		subCtxCancel()

		return nil, nil, err
	}

	// Wait for the Subscribe() callback to be invoked
Outer:
	for {
		select {
		case <-readyCh:
			// Subscription has started
			break Outer
		case <-time.After(time.Second):
			// Possible race with late goroutine start, re-issue watch barrier
			if err := store.notifyWatchBarrier(); err != nil {
				subCtxCancel()

				return nil, nil, err
			}
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}

	return watchCh, errCh, nil
}
