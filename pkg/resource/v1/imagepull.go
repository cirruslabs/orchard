package v1

type ImagePull struct {
	Meta

	// UID is a useful field for avoiding data races within a single Name.
	//
	// It is populated by the Controller when receiving a POST request.
	UID string `json:"uid,omitempty"`

	OwnerReferences []OwnerReference `json:"ownerReferences,omitempty"`

	Image string `json:"image,omitempty"`

	Worker string `json:"worker,omitempty"`

	PullState
}

func (pull *ImagePull) SetVersion(version uint64) {
	pull.Version = version
}

type PullState struct {
	Conditions []Condition `json:"conditions,omitempty"`
}
