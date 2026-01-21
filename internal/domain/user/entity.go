package domainuser

import (
	"time"

	"gorm.io/gorm"
)

func (Users) TableName() string {
	return "users"
}

type Users struct {
	Id        string         `json:"id" gorm:"column:id;primaryKey"`
	Name      string         `json:"name" gorm:"column:name"`
	Email     string         `json:"email,omitempty" gorm:"column:email"`
	Phone     string         `json:"phone,omitempty" gorm:"column:phone"`
	Password  string         `json:"-" gorm:"column:password"`
	Role      string         `json:"role,omitempty" gorm:"column:role"`
	RoleId    *string        `json:"role_id,omitempty" gorm:"column:role_id"`
	CreatedAt time.Time      `json:"created_at,omitempty" gorm:"column:created_at"`
	UpdatedAt *time.Time     `json:"updated_at,omitempty" gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}
