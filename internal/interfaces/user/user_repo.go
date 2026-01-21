package interfaceuser

import (
	domainuser "starter-kit/internal/domain/user"
	"starter-kit/pkg/filter"
)

type RepoUserInterface interface {
	Store(m domainuser.Users) error
	GetByEmail(email string) (domainuser.Users, error)
	GetByPhone(phone string) (domainuser.Users, error)
	GetByID(id string) (domainuser.Users, error)
	GetAll(params filter.BaseParams) ([]domainuser.Users, int64, error)
	Update(m domainuser.Users) error
	Delete(id string) error
}
