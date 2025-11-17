package badger

import (
	"path"

	"github.com/cirruslabs/orchard/pkg/resource/v1"
)

const SpaceServiceAccounts = "/service-accounts"

func ServiceAccountKey(name string) []byte {
	return []byte(path.Join(SpaceServiceAccounts, name))
}

func (txn *Transaction) GetServiceAccount(name string) (*v1.ServiceAccount, error) {
	return genericGet[v1.ServiceAccount](txn, ServiceAccountKey(name))
}

func (txn *Transaction) SetServiceAccount(serviceAccount *v1.ServiceAccount) error {
	return genericSet[v1.ServiceAccount](txn, ServiceAccountKey(serviceAccount.Name), *serviceAccount)
}

func (txn *Transaction) DeleteServiceAccount(name string) error {
	return genericDelete(txn, ServiceAccountKey(name))
}

func (txn *Transaction) ListServiceAccounts() ([]v1.ServiceAccount, error) {
	return genericList[v1.ServiceAccount](txn, []byte(SpaceServiceAccounts))
}
