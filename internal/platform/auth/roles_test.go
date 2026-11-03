package auth

import "testing"

func TestRoleCapabilities(t *testing.T) {
	tests := []struct {
		role             string
		managePeople     bool
		assignPrivileged bool
		editContent      bool
		viewPlans        bool
	}{
		{RoleAdmin, true, true, true, true},
		{RoleManager, true, false, false, true},
		{RoleEditor, false, false, true, true},
		{RoleViewer, false, false, false, true},
		{RoleScheduledViewer, false, false, false, false},
		{"unknown", false, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			if got := CanManagePeople(tt.role); got != tt.managePeople {
				t.Errorf("CanManagePeople() = %v, want %v", got, tt.managePeople)
			}
			if got := CanAssignPrivilegedRoles(tt.role); got != tt.assignPrivileged {
				t.Errorf("CanAssignPrivilegedRoles() = %v, want %v", got, tt.assignPrivileged)
			}
			if got := CanEditContent(tt.role); got != tt.editContent {
				t.Errorf("CanEditContent() = %v, want %v", got, tt.editContent)
			}
			if got := CanViewPlans(tt.role); got != tt.viewPlans {
				t.Errorf("CanViewPlans() = %v, want %v", got, tt.viewPlans)
			}
		})
	}
}
