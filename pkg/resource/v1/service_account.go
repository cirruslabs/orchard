package v1

type ServiceAccount struct {
	Token string               `json:"token,omitempty"`
	Roles []ServiceAccountRole `json:"roles,omitempty"`

	Meta
}
