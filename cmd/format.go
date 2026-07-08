package cmd

import (
	"fmt"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// personLabel renders a person as "Name <email>", or just the email when the
// display name is unknown.
func personLabel(p calendar.Person) string {
	if p.Name == "" {
		return p.Email
	}
	return fmt.Sprintf("%s <%s>", p.Name, p.Email)
}
