package store

import v1 "github.com/cirruslabs/orchard/pkg/resource/v1"

type Store interface {
	View(cb func(txn Transaction) error) error
	Update(cb func(txn Transaction) error) error
}

type Transaction interface {
	GetVM(name string) (result *v1.VM, err error)
	SetVM(vm v1.VM) (err error)
	DeleteVM(name string) (err error)
	ListVMs() (result []v1.VM, err error)

	GetWorker(name string) (result *v1.Worker, err error)
	SetWorker(worker v1.Worker) (err error)
	DeleteWorker(name string) (err error)
	ListWorkers() (result []v1.Worker, err error)

	GetServiceAccount(name string) (result *v1.ServiceAccount, err error)
	SetServiceAccount(serviceAccount *v1.ServiceAccount) (err error)
	DeleteServiceAccount(name string) (err error)
	ListServiceAccounts() (result []*v1.ServiceAccount, err error)

	AppendEvents(event []v1.Event, scope ...string) (err error)
	ListEvents(scope ...string) (result []v1.Event, err error)
	DeleteEvents(scope ...string) (err error)
}
