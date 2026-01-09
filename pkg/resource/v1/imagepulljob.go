package v1

type ImagePullJob struct {
	Meta

	// UID is a useful field for avoiding data races within a single Name.
	//
	// It is populated by the Controller when receiving a POST request.
	UID string `json:"uid,omitempty"`

	Image string `json:"image,omitempty"`

	Labels Labels `json:"labels,omitempty"`

	ImagePullJobState
}

func (imagePullJob *ImagePullJob) SetVersion(version uint64) {
	imagePullJob.Version = version
}

func (imagePullJob *ImagePullJob) OwnerReference() OwnerReference {
	return OwnerReference{
		Kind: KindImagePullJob,
		Name: imagePullJob.Name,
		UID:  imagePullJob.UID,
	}
}

type ImagePullJobState struct {
	Conditions  []Condition `json:"conditions,omitempty"`
	Progressing int64       `json:"progressing,omitempty"`
	Succeeded   int64       `json:"succeeded,omitempty"`
	Failed      int64       `json:"failed,omitempty"`
	Total       int64       `json:"total,omitempty"`
}
