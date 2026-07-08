package auth

import "errors"

var (
	// ErrDeviceCode means the device authorization response was empty.
	ErrDeviceCode = errors.New("device code: empty response")
	// ErrDeviceDeclined means the user declined the authorization request.
	ErrDeviceDeclined = errors.New("device code: authorization declined")
	// ErrDeviceExpired means the device code expired before authorization.
	ErrDeviceExpired = errors.New("device code: expired before authorization")
	// ErrToken means the token request was rejected by the identity platform.
	ErrToken = errors.New("token request failed")
)
