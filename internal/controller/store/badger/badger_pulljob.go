//nolint:dupl // maybe we'll figure out how to make DB resource accessors generic in the future
package badger

import (
	"path"

	"github.com/cirruslabs/orchard/pkg/resource/v1"
)

const SpaceImagePullJobs = "/imagepulljobs"

func ImagePullJobKey(name string) []byte {
	return []byte(path.Join(SpaceImagePullJobs, name))
}

func (txn *Transaction) GetImagePullJob(name string) (*v1.ImagePullJob, error) {
	return genericGet[v1.ImagePullJob](txn, ImagePullJobKey(name))
}

func (txn *Transaction) SetImagePullJob(pull v1.ImagePullJob) error {
	return genericSet[v1.ImagePullJob](txn, ImagePullJobKey(pull.Name), pull)
}

func (txn *Transaction) DeleteImagePullJob(name string) error {
	return genericDelete(txn, ImagePullJobKey(name))
}

func (txn *Transaction) ListImagePullJobs() ([]v1.ImagePullJob, error) {
	return genericList[v1.ImagePullJob](txn, []byte(SpaceImagePullJobs))
}
