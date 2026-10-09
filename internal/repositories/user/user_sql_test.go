package repositoryuser

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type emailConnector struct{ rows [][]driver.Value }

func (c emailConnector) Connect(context.Context) (driver.Conn, error) {
	return &emailConn{rows: c.rows}, nil
}
func (c emailConnector) Driver() driver.Driver { return emailDriver{} }

type emailDriver struct{}

func (emailDriver) Open(string) (driver.Conn, error) { return &emailConn{}, nil }

type emailConn struct {
	driver.Conn
	rows [][]driver.Value
}

func (c *emailConn) Close() error { return nil }
func (c *emailConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &emailRows{rows: c.rows}, nil
}

type emailRows struct{ rows [][]driver.Value }

func (*emailRows) Columns() []string { return []string{"id", "email"} }
func (*emailRows) Close() error      { return nil }
func (r *emailRows) Next(values []driver.Value) error {
	if len(r.rows) == 0 {
		return io.EOF
	}
	copy(values, r.rows[0])
	r.rows = r.rows[1:]
	return nil
}

func TestGetByEmailRejectsAmbiguousAccounts(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		rows := [][]driver.Value{{"user-1", "User@example.com"}, {"user-2", "user@example.com"}}[:count]
		db := sql.OpenDB(emailConnector{rows: rows})
		gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{DisableAutomaticPing: true})
		if err != nil {
			t.Fatal(err)
		}
		user, err := NewUserRepo(gormDB).GetByEmail("user@example.com")
		db.Close()
		switch count {
		case 0:
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Errorf("missing email: %v", err)
			}
		case 1:
			if err != nil || user.Id != "user-1" {
				t.Errorf("unique email: %v, %v", user, err)
			}
		case 2:
			if err == nil || user.Id != "" {
				t.Error("ambiguous email selected an account")
			}
		}
	}
}
