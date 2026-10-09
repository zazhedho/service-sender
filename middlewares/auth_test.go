package middlewares

import (
	"testing"

	domainpermission "service-sender/internal/domain/permission"
)

func TestHasPermissionMatchesResourceAndAction(t *testing.T) {
	permissions := []domainpermission.Permission{{Resource: "users", Action: "read"}}
	tests := []struct {
		name     string
		resource string
		action   string
		want     bool
	}{
		{name: "matching permission", resource: "users", action: "read", want: true},
		{name: "different resource", resource: "roles", action: "read"},
		{name: "different action", resource: "users", action: "write"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := hasPermission(permissions, test.resource, test.action); got != test.want {
				t.Fatalf("hasPermission(%q, %q) = %t, want %t", test.resource, test.action, got, test.want)
			}
		})
	}
}
