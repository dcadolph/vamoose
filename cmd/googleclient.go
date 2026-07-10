package cmd

import "os"

// builtinGoogleClientID and builtinGoogleClientSecret are the OAuth desktop client
// vamoose ships with, so a user can run "vamoose login" without registering their own
// Google Cloud project. Desktop-client credentials are not confidential: Google issues
// them to installed apps that cannot keep a secret, so security comes from PKCE and the
// consent screen, not from hiding these values. They are empty in source and injected at
// release build with -ldflags "-X github.com/dcadolph/vamoose/cmd.builtinGoogleClientID=...
// -X github.com/dcadolph/vamoose/cmd.builtinGoogleClientSecret=...". A user may override
// both with VAMOOSE_GOOGLE_CLIENT_ID and VAMOOSE_GOOGLE_CLIENT_SECRET.
var (
	builtinGoogleClientID     string
	builtinGoogleClientSecret string
)

// googleClientCreds returns the OAuth desktop client id and secret, reading the
// environment through os.Getenv. See googleClientCredsFrom for the resolution rules.
func googleClientCreds() (id, secret string, ok bool) {
	return googleClientCredsFrom(os.Getenv)
}

// googleClientCredsFrom returns the OAuth desktop client id and secret, preferring the
// user's environment override and falling back to the built-in client. The override is
// all-or-nothing: if either VAMOOSE_GOOGLE_CLIENT_ID or VAMOOSE_GOOGLE_CLIENT_SECRET is
// set, only that pair is used, so a user's own client is never mixed with the built-in
// one. ok reports whether a complete client id and secret were resolved.
func googleClientCredsFrom(getenv func(string) string) (id, secret string, ok bool) {
	envID := getenv("VAMOOSE_GOOGLE_CLIENT_ID")
	envSecret := getenv("VAMOOSE_GOOGLE_CLIENT_SECRET")
	if envID != "" || envSecret != "" {
		return envID, envSecret, envID != "" && envSecret != ""
	}
	return builtinGoogleClientID, builtinGoogleClientSecret, builtinGoogleClientID != "" && builtinGoogleClientSecret != ""
}
