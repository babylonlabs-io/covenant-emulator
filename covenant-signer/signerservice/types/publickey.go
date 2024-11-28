package types

type GetPublicKeyResponse struct {
	// hex encoded 33 byte public key
	PublicKey string `json:"public_key_hex"`
}

