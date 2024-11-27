package types

type UnlockRequest struct {
	Passphrase string `json:"passphrase"`
}

type UnlockResponse struct{}

type LockResponse struct{}
