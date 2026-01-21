package servicerole

import (
	"errors"
	domainrole "starter-kit/internal/domain/role"
	"starter-kit/internal/dto"
	interfacemenu "starter-kit/internal/interfaces/menu"
	interfacepermission "starter-kit/internal/interfaces/permission"
	interfacerole "starter-kit/internal/interfaces/role"
	"starter-kit/pkg/filter"
	"starter-kit/utils"
	"time"
)

type RoleService struct {
	RoleRepo       interfacerole.RepoRoleInterface
	PermissionRepo interfacepermission.RepoPermissionInterface
	MenuRepo       interfacemenu.RepoMenuInterface
}

func NewRoleService(
	roleRepo interfacerole.RepoRoleInterface,
	permissionRepo interfacepermission.RepoPermissionInterface,
	menuRepo interfacemenu.RepoMenuInterface,
) *RoleService {
	return &RoleService{
		RoleRepo:       roleRepo,
		PermissionRepo: permissionRepo,
		MenuRepo:       menuRepo,
	}
}

func (s *RoleService) Create(req dto.RoleCreate) (domainrole.Role, error) {
	existing, _ := s.RoleRepo.GetByName(req.Name)
	if existing.Id != "" {
		return domainrole.Role{}, errors.New("role with this name already exists")
	}

	data := domainrole.Role{
		Id:          utils.CreateUUID(),
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		IsSystem:    false,
		CreatedAt:   time.Now(),
	}

	if err := s.RoleRepo.Store(data); err != nil {
		return domainrole.Role{}, err
	}

	return data, nil
}

func (s *RoleService) GetByID(id string) (domainrole.Role, error) {
	return s.RoleRepo.GetByID(id)
}

func (s *RoleService) GetByIDWithDetails(id string) (dto.RoleWithDetails, error) {
	role, err := s.RoleRepo.GetByID(id)
	if err != nil {
		return dto.RoleWithDetails{}, err
	}

	permissionIds, err := s.RoleRepo.GetRolePermissions(id)
	if err != nil {
		return dto.RoleWithDetails{}, err
	}

	menuIds, err := s.RoleRepo.GetRoleMenus(id)
	if err != nil {
		return dto.RoleWithDetails{}, err
	}

	updatedAt := ""
	if role.UpdatedAt != nil {
		updatedAt = role.UpdatedAt.Format(time.RFC3339)
	}

	return dto.RoleWithDetails{
		Id:            role.Id,
		Name:          role.Name,
		DisplayName:   role.DisplayName,
		Description:   role.Description,
		IsSystem:      role.IsSystem,
		PermissionIds: permissionIds,
		MenuIds:       menuIds,
		CreatedAt:     role.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     updatedAt,
	}, nil
}

func (s *RoleService) GetAll(params filter.BaseParams, currentUserRole string) ([]domainrole.Role, int64, error) {
	roles, total, err := s.RoleRepo.GetAll(params)
	if err != nil {
		return nil, 0, err
	}

	if currentUserRole != utils.RoleSuperAdmin {
		filteredRoles := make([]domainrole.Role, 0)
		for _, role := range roles {
			if role.Name != utils.RoleSuperAdmin {
				filteredRoles = append(filteredRoles, role)
			}
		}
		superadminCount := int64(len(roles) - len(filteredRoles))
		return filteredRoles, total - superadminCount, nil
	}

	return roles, total, nil
}

func (s *RoleService) Update(id string, req dto.RoleUpdate) (domainrole.Role, error) {
	role, err := s.RoleRepo.GetByID(id)
	if err != nil {
		return domainrole.Role{}, err
	}

	if role.IsSystem {
		return domainrole.Role{}, errors.New("cannot update system roles")
	}

	if req.DisplayName != "" {
		role.DisplayName = req.DisplayName
	}
	role.Description = req.Description
	now := time.Now()
	role.UpdatedAt = &now

	if err := s.RoleRepo.Update(role); err != nil {
		return domainrole.Role{}, err
	}

	return role, nil
}

func (s *RoleService) Delete(id string) error {
	role, err := s.RoleRepo.GetByID(id)
	if err != nil {
		return err
	}

	if role.IsSystem {
		return errors.New("cannot delete system roles")
	}

	return s.RoleRepo.Delete(id)
}

func (s *RoleService) AssignPermissions(roleId string, req dto.AssignPermissions, currentUserRole string) error {
	role, err := s.RoleRepo.GetByID(roleId)
	if err != nil {
		return err
	}

	if role.IsSystem {
		if currentUserRole != utils.RoleSuperAdmin && currentUserRole != utils.RoleAdmin {
			return errors.New("access denied: only superadmin and admin can modify system roles")
		}

		if role.Name == utils.RoleSuperAdmin && currentUserRole != utils.RoleSuperAdmin {
			return errors.New("access denied: cannot modify superadmin role")
		}
	}

	for _, permId := range req.PermissionIds {
		if _, err := s.PermissionRepo.GetByID(permId); err != nil {
			return errors.New("invalid permission ID: " + permId)
		}
	}

	return s.RoleRepo.AssignPermissions(roleId, req.PermissionIds)
}

func (s *RoleService) AssignMenus(roleId string, req dto.AssignMenus, currentUserRole string) error {
	role, err := s.RoleRepo.GetByID(roleId)
	if err != nil {
		return err
	}

	if role.IsSystem {
		if currentUserRole != utils.RoleSuperAdmin && currentUserRole != utils.RoleAdmin {
			return errors.New("access denied: only superadmin and admin can modify system roles")
		}

		if role.Name == utils.RoleSuperAdmin && currentUserRole != utils.RoleSuperAdmin {
			return errors.New("access denied: cannot modify superadmin role")
		}
	}

	for _, menuId := range req.MenuIds {
		if _, err := s.MenuRepo.GetByID(menuId); err != nil {
			return errors.New("invalid menu ID: " + menuId)
		}
	}

	return s.RoleRepo.AssignMenus(roleId, req.MenuIds)
}

func (s *RoleService) GetRolePermissions(roleId string) ([]string, error) {
	return s.RoleRepo.GetRolePermissions(roleId)
}

func (s *RoleService) GetRoleMenus(roleId string) ([]string, error) {
	return s.RoleRepo.GetRoleMenus(roleId)
}

var _ interfacerole.ServiceRoleInterface = (*RoleService)(nil)
