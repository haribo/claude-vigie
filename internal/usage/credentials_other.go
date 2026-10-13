//go:build !darwin

package usage

// readCredentials returns Claude Code's stored credentials: on Linux, the file.
func readCredentials() ([]byte, error) { return readCredentialsFile() }
