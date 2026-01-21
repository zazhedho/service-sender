package repositorymenu

import (
	"fmt"
	domainmenu "starter-kit/internal/domain/menu"
	interfacemenu "starter-kit/internal/interfaces/menu"
	"starter-kit/pkg/filter"

	"gorm.io/gorm"
)

type repo struct {
	DB *gorm.DB
}

func NewMenuRepo(db *gorm.DB) interfacemenu.RepoMenuInterface {
	return &repo{DB: db}
}

func (r *repo) Store(m domainmenu.MenuItem) error {
	return r.DB.Create(&m).Error
}

func (r *repo) GetByID(id string) (ret domainmenu.MenuItem, err error) {
	if err = r.DB.Where("id = ?", id).First(&ret).Error; err != nil {
		return domainmenu.MenuItem{}, err
	}
	return ret, nil
}

func (r *repo) GetByName(name string) (ret domainmenu.MenuItem, err error) {
	if err = r.DB.Where("name = ?", name).First(&ret).Error; err != nil {
		return domainmenu.MenuItem{}, err
	}
	return ret, nil
}

func (r *repo) GetAll(params filter.BaseParams) (ret []domainmenu.MenuItem, totalData int64, err error) {
	query := r.DB.Model(&domainmenu.MenuItem{}).Debug()

	if params.Search != "" {
		searchPattern := "%" + params.Search + "%"
		query = query.Where("LOWER(name) LIKE LOWER(?) OR LOWER(display_name) LIKE LOWER(?) OR LOWER(path) LIKE LOWER(?)", searchPattern, searchPattern, searchPattern)
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
			"path":         true,
			"order_index":  true,
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

func (r *repo) Update(m domainmenu.MenuItem) error {
	return r.DB.Save(&m).Error
}

func (r *repo) Delete(id string) error {
	return r.DB.Where("id = ?", id).Delete(&domainmenu.MenuItem{}).Error
}

func (r *repo) GetActiveMenus() (ret []domainmenu.MenuItem, err error) {
	if err = r.DB.Where("is_active = ?", true).Order("order_index ASC").Find(&ret).Error; err != nil {
		return nil, err
	}
	return ret, nil
}

func (r *repo) GetUserMenus(userId string) (ret []domainmenu.MenuItem, err error) {
	var user struct {
		RoleId *string
		Role   string
	}
	if err = r.DB.Table("users").Select("role_id, role").Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, err
	}

	if user.RoleId != nil && *user.RoleId != "" {
		query := `
			SELECT DISTINCT m.*
			FROM menu_items m
			INNER JOIN role_menus rm ON m.id = rm.menu_item_id
			INNER JOIN roles r ON rm.role_id = r.id
			INNER JOIN users u ON u.role_id = r.id
			WHERE u.id = ? AND m.is_active = true AND m.deleted_at IS NULL
			ORDER BY m.order_index ASC
		`
		if err = r.DB.Raw(query, userId).Scan(&ret).Error; err != nil {
			return nil, err
		}
	} else if user.Role != "" {
		query := `
			SELECT DISTINCT m.*
			FROM menu_items m
			INNER JOIN role_menus rm ON m.id = rm.menu_item_id
			INNER JOIN roles r ON rm.role_id = r.id
			WHERE r.name = ? AND m.is_active = true AND m.deleted_at IS NULL
			ORDER BY m.order_index ASC
		`
		if err = r.DB.Raw(query, user.Role).Scan(&ret).Error; err != nil {
			return nil, err
		}
	}

	return ret, nil
}
