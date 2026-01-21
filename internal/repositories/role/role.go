package repositoryrole

import (
	"fmt"
	domainrole "starter-kit/internal/domain/role"
	interfacerole "starter-kit/internal/interfaces/role"
	"starter-kit/pkg/filter"
	"starter-kit/utils"

	"gorm.io/gorm"
)

type repo struct {
	DB *gorm.DB
}

func NewRoleRepo(db *gorm.DB) interfacerole.RepoRoleInterface {
	return &repo{DB: db}
}

func (r *repo) Store(m domainrole.Role) error {
	return r.DB.Create(&m).Error
}

func (r *repo) GetByID(id string) (ret domainrole.Role, err error) {
	if err = r.DB.Where("id = ?", id).First(&ret).Error; err != nil {
		return domainrole.Role{}, err
	}
	return ret, nil
}

func (r *repo) GetByName(name string) (ret domainrole.Role, err error) {
	if err = r.DB.Where("name = ?", name).First(&ret).Error; err != nil {
		return domainrole.Role{}, err
	}
	return ret, nil
}

func (r *repo) GetAll(params filter.BaseParams) (ret []domainrole.Role, totalData int64, err error) {
	query := r.DB.Model(&domainrole.Role{}).Debug()

	if params.Search != "" {
		searchPattern := "%" + params.Search + "%"
		query = query.Where("LOWER(name) LIKE LOWER(?) OR LOWER(display_name) LIKE LOWER(?) OR LOWER(description) LIKE LOWER(?)", searchPattern, searchPattern, searchPattern)
	}

	for key, value := range params.Filters {
		if value == nil {
			continue
		}

		switch v := value.(type) {
		case string:
			if v == "" {
				continue
			}
			query = query.Where(fmt.Sprintf("%s = ?", key), v)
		case []string, []int:
			query = query.Where(fmt.Sprintf("%s IN ?", key), v)
		default:
			query = query.Where(fmt.Sprintf("%s = ?", key), v)
		}
	}

	if err := query.Count(&totalData).Error; err != nil {
		return nil, 0, err
	}

	if params.OrderBy != "" && params.OrderDirection != "" {
		validColumns := map[string]bool{
			"name":         true,
			"display_name": true,
			"is_system":    true,
			"created_at":   true,
			"updated_at":   true,
		}

		if _, ok := validColumns[params.OrderBy]; !ok {
			return nil, 0, fmt.Errorf("invalid orderBy column: %s", params.OrderBy)
		}

		query = query.Order(fmt.Sprintf("%s %s", params.OrderBy, params.OrderDirection))
	}

	if err := query.Offset(params.Offset).Limit(params.Limit).Find(&ret).Error; err != nil {
		return nil, 0, err
	}

	return ret, totalData, nil
}

func (r *repo) Update(m domainrole.Role) error {
	return r.DB.Save(&m).Error
}

func (r *repo) Delete(id string) error {
	return r.DB.Where("id = ?", id).Delete(&domainrole.Role{}).Error
}

func (r *repo) AssignPermissions(roleId string, permissionIds []string) error {
	tx := r.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Where("role_id = ?", roleId).Delete(&domainrole.RolePermission{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	for _, permissionId := range permissionIds {
		rolePermission := domainrole.RolePermission{
			Id:           utils.CreateUUID(),
			RoleId:       roleId,
			PermissionId: permissionId,
		}
		if err := tx.Create(&rolePermission).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

func (r *repo) RemovePermissions(roleId string, permissionIds []string) error {
	return r.DB.Where("role_id = ? AND permission_id IN ?", roleId, permissionIds).Delete(&domainrole.RolePermission{}).Error
}

func (r *repo) GetRolePermissions(roleId string) ([]string, error) {
	var rolePermissions []domainrole.RolePermission
	if err := r.DB.Where("role_id = ?", roleId).Find(&rolePermissions).Error; err != nil {
		return nil, err
	}

	permissionIds := make([]string, len(rolePermissions))
	for i, rp := range rolePermissions {
		permissionIds[i] = rp.PermissionId
	}

	return permissionIds, nil
}

func (r *repo) AssignMenus(roleId string, menuIds []string) error {
	tx := r.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Where("role_id = ?", roleId).Delete(&domainrole.RoleMenu{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	for _, menuId := range menuIds {
		roleMenu := domainrole.RoleMenu{
			Id:         utils.CreateUUID(),
			RoleId:     roleId,
			MenuItemId: menuId,
		}
		if err := tx.Create(&roleMenu).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

func (r *repo) RemoveMenus(roleId string, menuIds []string) error {
	return r.DB.Where("role_id = ? AND menu_item_id IN ?", roleId, menuIds).Delete(&domainrole.RoleMenu{}).Error
}

func (r *repo) GetRoleMenus(roleId string) ([]string, error) {
	var roleMenus []domainrole.RoleMenu
	if err := r.DB.Where("role_id = ?", roleId).Find(&roleMenus).Error; err != nil {
		return nil, err
	}

	menuIds := make([]string, len(roleMenus))
	for i, rm := range roleMenus {
		menuIds[i] = rm.MenuItemId
	}

	return menuIds, nil
}
