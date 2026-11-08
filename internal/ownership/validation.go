package ownership

import "regexp"

var authorizedServiceIDPattern = regexp.MustCompile(
	`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`,
)

// ValidServiceID reports whether id has the format
// accepted for service ownership authorization.
func ValidServiceID(id string) bool {
	return authorizedServiceIDPattern.MatchString(id)
}
