package googleauth

import "errors"

var (
	// ErrAuthorization means the consent screen returned an error.
	ErrAuthorization = errors.New("authorization failed")
	// ErrState means the redirect state did not match, a possible forgery.
	ErrState = errors.New("authorization state mismatch")
	// ErrNoCode means the redirect carried no authorization code.
	ErrNoCode = errors.New("no authorization code received")
	// ErrToken means the token request was rejected by Google.
	ErrToken = errors.New("token request failed")
)
