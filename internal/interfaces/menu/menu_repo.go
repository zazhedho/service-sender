package interfacemenu

import (
	domainmenu "starter-kit/internal/domain/menu"
	"starter-kit/pkg/filter"
)

type RepoMenuInterface interface {
	Store(m domainmenu.MenuItem) error
	GetByID(id string) (domainmenu.MenuItem, error)
	GetByName(name string) (domainmenu.MenuItem, error)
	GetAll(params filter.BaseParams) ([]domainmenu.MenuItem, int64, error)
	Update(m domainmenu.MenuItem) error
	Delete(id string) error

	GetActiveMenus() ([]domainmenu.MenuItem, error)
	GetUserMenus(userId string) ([]domainmenu.MenuItem, error)
}
