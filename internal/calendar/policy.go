package calendar

import "github.com/yourchurch/churchplanner/internal/platform/auth"

// Blockout permissions live HERE and nowhere else. Handlers, SQL filters and
// templates all derive from these functions.
//
//	            see        add            edit / remove
//	admin       all        for anyone     any
//	manager     all        for anyone*    any
//	others      own only   for self       own only
//
// * set managersMayAddForOthers to false to restrict adding-for-others to admins.

const managersMayAddForOthers = true

// canManageAllBlockouts: may see, edit and remove EVERYONE's blockouts.
// Tied to auth.CanManagePeople (admin or manager) on purpose, so the calendar
// follows the same notion of "who manages people" as the rest of the app.
func canManageAllBlockouts(role string) bool { return auth.CanManagePeople(role) }

// canEditBlockout covers both editing and removing.
func canEditBlockout(role, actorID, ownerID string) bool {
	if canManageAllBlockouts(role) {
		return true
	}
	return actorID != "" && actorID == ownerID
}

// canPickPerson: may the add-form target someone other than the actor?
func canPickPerson(role string) bool {
	return role == auth.RoleAdmin || (managersMayAddForOthers && role == auth.RoleManager)
}

func canAddBlockoutFor(role, actorID, targetID string) bool {
	if actorID == "" || targetID == "" {
		return false
	}
	return actorID == targetID || canPickPerson(role)
}
