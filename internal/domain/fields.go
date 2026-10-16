package domain

// Field-level permission model for the users resource.
//
// "Base" attributes (id, tenant_id, user_type, status timestamps, version…)
// are visible with the action-level permission alone. "Protected" fields
// each require an explicit field grant (users.read.<field> etc.).
// Secret material (password_hash, refresh_token_hash) is never exposed and
// has no permission — it simply does not exist in API responses.

// UserProtectedReadFields are user fields gated by users.read.<field>.
var UserProtectedReadFields = []string{
	"email", "first_name", "last_name", "phone", "status", "internal_notes", "security_flags",
}

// UserProtectedUpdateFields are user fields gated by users.update.<field>.
var UserProtectedUpdateFields = []string{
	"email", "first_name", "last_name", "phone", "status", "internal_notes",
}

// UserSearchableFields are filterable fields gated by users.search.<field>.
var UserSearchableFields = []string{
	"email", "first_name", "last_name", "phone", "status",
}

func Contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
