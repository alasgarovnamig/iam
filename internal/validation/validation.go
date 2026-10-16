// Package validation provides small, dependency-free input validators used
// by application-layer command validation.
package validation

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{1,61}[a-z0-9])?$`)

func Email(v string) error {
	if v == "" {
		return fmt.Errorf("email is required")
	}
	if len(v) > 254 {
		return fmt.Errorf("email is too long")
	}
	addr, err := mail.ParseAddress(v)
	if err != nil || addr.Address != v {
		return fmt.Errorf("email is not valid")
	}
	return nil
}

func UUID(v string) error {
	if _, err := uuid.Parse(v); err != nil {
		return fmt.Errorf("must be a valid UUID")
	}
	return nil
}

func Slug(v string) error {
	if !slugRe.MatchString(v) {
		return fmt.Errorf("must be a lowercase slug (a-z, 0-9, hyphen), 2-63 characters")
	}
	return nil
}

func Required(name, v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

func MaxLen(name, v string, max int) error {
	if len(v) > max {
		return fmt.Errorf("%s must be at most %d characters", name, max)
	}
	return nil
}

func OneOf(name, v string, allowed ...string) error {
	for _, a := range allowed {
		if v == a {
			return nil
		}
	}
	return fmt.Errorf("%s must be one of: %s", name, strings.Join(allowed, ", "))
}
