package config

import "time"

const (
	defaultUrl     = "http://127.0.0.1:9791"
	defaultTimeout = 2 * time.Second
)

type RemoteSignerCfg struct {
	URL     string        `long:"url" description:"URL of the remote signer"`
	Timeout time.Duration `long:"timeout" description:"client when making requests to the remote signer"`
}

func DefaultRemoteSignerConfig() RemoteSignerCfg {
	return RemoteSignerCfg{
		URL:     defaultUrl,
		Timeout: defaultTimeout,
	}
}
