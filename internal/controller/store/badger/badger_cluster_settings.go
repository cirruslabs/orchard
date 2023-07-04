package badger

import (
	"encoding/json"
	"github.com/cirruslabs/orchard/pkg/resource/v1"
)

var ClusterSettingsKey = []byte("/cluster-settings")

func (txn *Transaction) GetClusterSettings() (result *v1.ClusterSettings, err error) {
	defer func() {
		err = mapErr(err)
	}()

	item, err := txn.badgerTxn.Get(ClusterSettingsKey)
	if err != nil {
		return nil, err
	}

	valueBytes, err := item.ValueCopy(nil)
	if err != nil {
		return nil, err
	}

	var clusterSettings v1.ClusterSettings

	err = json.Unmarshal(valueBytes, &clusterSettings)
	if err != nil {
		return nil, err
	}

	return &clusterSettings, nil
}

func (txn *Transaction) SetClusterSettings(clusterSettings v1.ClusterSettings) (err error) {
	defer func() {
		err = mapErr(err)
	}()

	valueBytes, err := json.Marshal(clusterSettings)
	if err != nil {
		return err
	}

	return txn.badgerTxn.Set(ClusterSettingsKey, valueBytes)
}
