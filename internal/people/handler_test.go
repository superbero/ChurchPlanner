package people

import "testing"

func TestValidUserRole(t *testing.T) {
	for _, role := range []string{"admin", "editor", "volunteer", "guest"} {
		if !validUserRole(role) {
			t.Errorf("validUserRole(%q) = false", role)
		}
	}
	for _, role := range []string{"", "leader", "owner", "ADMIN"} {
		if validUserRole(role) {
			t.Errorf("validUserRole(%q) = true", role)
		}
	}
}
