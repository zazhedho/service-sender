package interfacerole

import (
	domainrole "service-otp/internal/domain/role"
	"service-otp/pkg/filter"
)

type RepoRoleInterface interface {
	Store(m domainrole.Role) error
	GetByID(id string) (domainrole.Role, error)
	GetByName(name string) (domainrole.Role, error)
	GetAll(params filter.BaseParams) ([]domainrole.Role, int64, error)
	Update(m domainrole.Role) error
	Delete(id string) error

	AssignPermissions(roleId string, permissionIds []string) error
	RemovePermissions(roleId string, permissionIds []string) error
	GetRolePermissions(roleId string) ([]string, error)

	AssignMenus(roleId string, menuIds []string) error
	RemoveMenus(roleId string, menuIds []string) error
	GetRoleMenus(roleId string) ([]string, error)
}
