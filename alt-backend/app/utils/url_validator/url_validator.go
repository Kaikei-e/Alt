package url_validator

import (
	"alt/utils/security"
	"errors"
	"net/url"
)

// IsAllowedURL checks if the URL is allowed (scheme http/https, no userinfo, port absent or 80/443, not private/special host).
// It delegates directly to the security.URLSecurityValidator used during registration to ensure identical policy enforcement.
func IsAllowedURL(u *url.URL) error {
	if u == nil {
		return errors.New("nil URL")
	}
	return security.NewURLSecurityValidator().ValidateParsedRSSURL(u)
}
