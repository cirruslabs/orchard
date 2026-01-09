//nolint:dupl // maybe we'll figure out how to make DB resource accessors generic in the future
package badger

import (
	"path"

	"github.com/cirruslabs/orchard/pkg/resource/v1"
)

const SpaceImagePulls = "/imagepulls"

func ImagePullKey(name string) []byte {
	return []byte(path.Join(SpaceImagePulls, name))
}

func (txn *Transaction) GetImagePull(name string) (*v1.ImagePull, error) {
	return genericGet[v1.ImagePull](txn, ImagePullKey(name))
}

func (txn *Transaction) SetImagePull(pull v1.ImagePull) error {
	return genericSet[v1.ImagePull](txn, ImagePullKey(pull.Name), pull)
}

func (txn *Transaction) DeleteImagePull(name string) error {
	return genericDelete(txn, ImagePullKey(name))
}

func (txn *Transaction) ListImagePulls() ([]v1.ImagePull, error) {
	return genericList[v1.ImagePull](txn, []byte(SpaceImagePulls))
}
