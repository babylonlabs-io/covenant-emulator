package config

import "time"

const (
	defaultUrl     = "http://127.0.0.1:9791"
	defaultTimeout = 2 * time.Second
)

type RemoteSignerCfg struct {
	URL     string        `long:"url" description:"URL of the remote signer"`
	Timeout time.Duration `long:"timeout" description:"client when making requests to the remote signer"`
	HMACKey string        `long:"hmackey" description:"HMAC key for authenticating requests to the remote signer - can be a direct key or reference to a cloud secret manager. Leave empty to disable HMAC authentication."`
}

func DefaultRemoteSignerConfig() RemoteSignerCfg {
	return RemoteSignerCfg{
		URL:     defaultUrl,
		Timeout: defaultTimeout,
		HMACKey: "",
	}
}
