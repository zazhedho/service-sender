package repositorypermission

import (
	"fmt"
	domainpermission "starter-kit/internal/domain/permission"
	interfacepermission "starter-kit/internal/interfaces/permission"
	"starter-kit/pkg/filter"

	"gorm.io/gorm"
)

type repo struct {
	DB *gorm.DB
}

func NewPermissionRepo(db *gorm.DB) interfacepermission.RepoPermissionInterface {
	return &repo{DB: db}
}

func (r *repo) Store(m domainpermission.Permission) error {
	return r.DB.Create(&m).Error
}

func (r *repo) GetByID(id string) (ret domainpermission.Permission, err error) {
	if err = r.DB.Where("id = ?", id).First(&ret).Error; err != nil {
		return domainpermission.Permission{}, err
	}
	return ret, nil
}

func (r *repo) GetByName(name string) (ret domainpermission.Permission, err error) {
	if err = r.DB.Where("name = ?", name).First(&ret).Error; err != nil {
		return domainpermission.Permission{}, err
	}
	return ret, nil
}

func (r *repo) GetAll(params filter.BaseParams) (ret []domainpermission.Permission, totalData int64, err error) {
	query := r.DB.Model(&domainpermission.Permission{}).Debug()

	if params.Search != "" {
		searchPattern := "%" + params.Search + "%"
		query = query.Where("LOWER(name) LIKE LOWER(?) OR LOWER(display_name) LIKE LOWER(?) OR LOWER(description) LIKE LOWER(?) OR LOWER(resource) LIKE LOWER(?)", searchPattern, searchPattern, searchPattern, searchPattern)
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
			"resource":     true,
			"action":       true,
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

func (r *repo) Update(m domainpermission.Permission) error {
	return r.DB.Save(&m).Error
}

func (r *repo) Delete(id string) error {
	return r.DB.Where("id = ?", id).Delete(&domainpermission.Permission{}).Error
}

func (r *repo) GetByResource(resource string) (ret []domainpermission.Permission, err error) {
	if err = r.DB.Where("resource = ?", resource).Find(&ret).Error; err != nil {
		return nil, err
	}
	return ret, nil
}

func (r *repo) GetUserPermissions(userId string) (ret []domainpermission.Permission, err error) {
	var user struct {
		RoleId *string
		Role   string
	}
	if err = r.DB.Table("users").Select("role_id, role").Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, err
	}

	if user.RoleId != nil && *user.RoleId != "" {
		query := `
			SELECT DISTINCT p.*
			FROM permissions p
			INNER JOIN role_permissions rp ON p.id = rp.permission_id
			INNER JOIN roles r ON rp.role_id = r.id
			INNER JOIN users u ON u.role_id = r.id
			WHERE u.id = ? AND p.deleted_at IS NULL
			ORDER BY p.resource, p.action
		`
		if err = r.DB.Raw(query, userId).Scan(&ret).Error; err != nil {
			return nil, err
		}
	} else if user.Role != "" {
		query := `
			SELECT DISTINCT p.*
			FROM permissions p
			INNER JOIN role_permissions rp ON p.id = rp.permission_id
			INNER JOIN roles r ON rp.role_id = r.id
			WHERE r.name = ? AND p.deleted_at IS NULL
			ORDER BY p.resource, p.action
		`
		if err = r.DB.Raw(query, user.Role).Scan(&ret).Error; err != nil {
			return nil, err
		}
	}

	return ret, nil
}
