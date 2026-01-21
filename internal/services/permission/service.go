package servicepermission

import (
	"errors"
	domainpermission "starter-kit/internal/domain/permission"
	"starter-kit/internal/dto"
	interfacepermission "starter-kit/internal/interfaces/permission"
	"starter-kit/pkg/filter"
	"starter-kit/utils"
	"time"
)

type PermissionService struct {
	PermissionRepo interfacepermission.RepoPermissionInterface
}

func NewPermissionService(permissionRepo interfacepermission.RepoPermissionInterface) *PermissionService {
	return &PermissionService{
		PermissionRepo: permissionRepo,
	}
}

func (s *PermissionService) Create(req dto.PermissionCreate) (domainpermission.Permission, error) {
	existing, _ := s.PermissionRepo.GetByName(req.Name)
	if existing.Id != "" {
		return domainpermission.Permission{}, errors.New("permission with this name already exists")
	}

	data := domainpermission.Permission{
		Id:          utils.CreateUUID(),
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Resource:    req.Resource,
		Action:      req.Action,
		CreatedAt:   time.Now(),
	}

	if err := s.PermissionRepo.Store(data); err != nil {
		return domainpermission.Permission{}, err
	}

	return data, nil
}

func (s *PermissionService) GetByID(id string) (domainpermission.Permission, error) {
	return s.PermissionRepo.GetByID(id)
}

func (s *PermissionService) GetAll(params filter.BaseParams) ([]domainpermission.Permission, int64, error) {
	return s.PermissionRepo.GetAll(params)
}

func (s *PermissionService) GetByResource(resource string) ([]domainpermission.Permission, error) {
	return s.PermissionRepo.GetByResource(resource)
}

func (s *PermissionService) GetUserPermissions(userId string) ([]domainpermission.Permission, error) {
	return s.PermissionRepo.GetUserPermissions(userId)
}

func (s *PermissionService) Update(id string, req dto.PermissionUpdate) (domainpermission.Permission, error) {
	permission, err := s.PermissionRepo.GetByID(id)
	if err != nil {
		return domainpermission.Permission{}, err
	}

	if req.DisplayName != "" {
		permission.DisplayName = req.DisplayName
	}
	permission.Description = req.Description
	if req.Resource != "" {
		permission.Resource = req.Resource
	}
	if req.Action != "" {
		permission.Action = req.Action
	}
	now := time.Now()
	permission.UpdatedAt = &now

	if err := s.PermissionRepo.Update(permission); err != nil {
		return domainpermission.Permission{}, err
	}

	return permission, nil
}

func (s *PermissionService) Delete(id string) error {
	return s.PermissionRepo.Delete(id)
}

var _ interfacepermission.ServicePermissionInterface = (*PermissionService)(nil)
