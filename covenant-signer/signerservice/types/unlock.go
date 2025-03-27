package types

type UnlockRequest struct {
	Passphrase string `json:"passphrase,omitempty"`
}

type UnlockResponse struct{}

type LockResponse struct{}
