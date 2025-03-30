package types

// UnlockRequest represents a request to unlock the key.
// Note: When using the file keyring backend, the passphrase will be prompted
// interactively in the terminal, so it doesn't need to be included in this request.
type UnlockRequest struct {
	Passphrase string `json:"passphrase,omitempty"`
}

type UnlockResponse struct{}

type LockResponse struct{}
