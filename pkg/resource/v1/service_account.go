package v1

type ServiceAccount struct {
	Token string
	Roles []ServiceAccountRole

	Meta
}
