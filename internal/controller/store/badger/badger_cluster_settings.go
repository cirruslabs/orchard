package badger

import (
	"github.com/cirruslabs/orchard/pkg/resource/v1"
)

var ClusterSettingsKey = []byte("/cluster-settings")

func (txn *Transaction) GetClusterSettings() (*v1.ClusterSettings, error) {
	return genericGet[v1.ClusterSettings](txn, ClusterSettingsKey)
}

func (txn *Transaction) SetClusterSettings(clusterSettings v1.ClusterSettings) error {
	return genericSet[v1.ClusterSettings](txn, ClusterSettingsKey, clusterSettings)
}
