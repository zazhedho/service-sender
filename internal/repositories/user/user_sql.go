package repositoryuser

import (
	"fmt"
	domainuser "starter-kit/internal/domain/user"
	interfaceuser "starter-kit/internal/interfaces/user"
	"starter-kit/pkg/filter"

	"gorm.io/gorm"
)

type repo struct {
	DB *gorm.DB
}

func NewUserRepo(db *gorm.DB) interfaceuser.RepoUserInterface {
	return &repo{DB: db}
}

func (r *repo) Store(m domainuser.Users) error {
	return r.DB.Create(&m).Error
}

func (r *repo) GetByEmail(email string) (ret domainuser.Users, err error) {
	if err = r.DB.Where("email = ?", email).First(&ret).Error; err != nil {
		return domainuser.Users{}, err
	}

	return ret, nil
}

func (r *repo) GetByPhone(phone string) (ret domainuser.Users, err error) {
	if err = r.DB.Where("phone = ?", phone).First(&ret).Error; err != nil {
		return domainuser.Users{}, err
	}

	return ret, nil
}

func (r *repo) GetByID(id string) (ret domainuser.Users, err error) {
	if err = r.DB.Where("id = ?", id).First(&ret).Error; err != nil {
		return domainuser.Users{}, err
	}
	return ret, nil
}

func (r *repo) GetAll(params filter.BaseParams) (ret []domainuser.Users, totalData int64, err error) {
	query := r.DB.Model(&domainuser.Users{}).Debug()

	if params.Search != "" {
		searchPattern := "%" + params.Search + "%"
		query = query.Where("LOWER(name) LIKE LOWER(?) OR LOWER(email) LIKE LOWER(?) OR LOWER(phone) LIKE LOWER(?)", searchPattern, searchPattern, searchPattern)
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
			"name":       true,
			"email":      true,
			"phone":      true,
			"role":       true,
			"created_at": true,
			"updated_at": true,
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

func (r *repo) Update(m domainuser.Users) error {
	return r.DB.Save(&m).Error
}

func (r *repo) Delete(id string) error {
	return r.DB.Where("id = ?", id).Delete(&domainuser.Users{}).Error
}
