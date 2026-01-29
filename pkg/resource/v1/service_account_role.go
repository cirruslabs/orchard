package v1

import (
	"errors"
	"fmt"
)

var ErrUnsupportedServiceAccountRole = errors.New("unsupported service account role")

type ServiceAccountRole string

const (
	ServiceAccountRoleComputeRead    ServiceAccountRole = "compute:read"
	ServiceAccountRoleComputeWrite   ServiceAccountRole = "compute:write"
	ServiceAccountRoleComputeConnect ServiceAccountRole = "compute:connect"
	ServiceAccountRoleAdminRead      ServiceAccountRole = "admin:read"
	ServiceAccountRoleAdminWrite     ServiceAccountRole = "admin:write"
)

func NewServiceAccountRole(name string) (ServiceAccountRole, error) {
	switch name {
	case string(ServiceAccountRoleComputeRead):
		return ServiceAccountRoleComputeRead, nil
	case string(ServiceAccountRoleComputeWrite):
		return ServiceAccountRoleComputeWrite, nil
	case string(ServiceAccountRoleComputeConnect):
		return ServiceAccountRoleComputeConnect, nil
	case string(ServiceAccountRoleAdminRead):
		return ServiceAccountRoleAdminRead, nil
	case string(ServiceAccountRoleAdminWrite):
		return ServiceAccountRoleAdminWrite, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedServiceAccountRole, name)
	}
}

func AllServiceAccountRoles() []ServiceAccountRole {
	return []ServiceAccountRole{
		ServiceAccountRoleComputeRead,
		ServiceAccountRoleComputeWrite,
		ServiceAccountRoleComputeConnect,
		ServiceAccountRoleAdminRead,
		ServiceAccountRoleAdminWrite,
	}
}
