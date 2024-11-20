package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/viper"
)

const (
	folderPermissions = 0750
)

type Config struct {
	Server  ServerConfig  `mapstructure:"server-config"`
	Metrics MetricsConfig `mapstructure:"metrics"`
}

func DefaultConfig() *Config {
	return &Config{
		Server:  *DefaultServerConfig(),
		Metrics: *DefaultMetricsConfig(),
	}
}

type ParsedConfig struct {
	ServerConfig  *ParsedServerConfig
	MetricsConfig *ParsedMetricsConfig
}

func (cfg *Config) Parse() (*ParsedConfig, error) {
	serverConfig, err := cfg.Server.Parse()

	if err != nil {
		return nil, err
	}

	metricsConfig, err := cfg.Metrics.Parse()

	if err != nil {
		return nil, err
	}

	return &ParsedConfig{
		ServerConfig:  serverConfig,
		MetricsConfig: metricsConfig,
	}, nil
}

const defaultConfigTemplate = `# This is a TOML config file.
# For more information, see https://github.com/toml-lang/toml

[server-config]
# The address to listen on
host = "{{ .Server.Host }}"

# The port to listen on
port = {{ .Server.Port }}

# Read timeout in seconds
read-timeout = {{ .Server.ReadTimeout }}

# Write timeout in seconds
write-timeout = {{ .Server.WriteTimeout }}

# Idle timeout in seconds
idle-timeout = {{ .Server.IdleTimeout }}

# Max content length in bytes
max-content-length = {{ .Server.MaxContentLength }}

[metrics]
# The prometheus server host
host = "{{ .Metrics.Host }}"
# The prometheus server port
port = {{ .Metrics.Port }}
`

var configTemplate *template.Template

func init() {
	var err error
	tmpl := template.New("configFileTemplate").Funcs(template.FuncMap{
		"StringsJoin": strings.Join,
	})
	if configTemplate, err = tmpl.Parse(defaultConfigTemplate); err != nil {
		panic(err)
	}
}

func writeConfigToFile(configFilePath string, config *Config) error {
	var buffer bytes.Buffer

	if err := configTemplate.Execute(&buffer, config); err != nil {
		panic(err)
	}

	return os.WriteFile(configFilePath, buffer.Bytes(), 0o600)
}

func WriteConfigToFile(pathToConfFile string, conf *Config) error {
	dirPath, _ := filepath.Split(pathToConfFile)

	if _, err := os.Stat(pathToConfFile); os.IsNotExist(err) {
		if err := os.MkdirAll(dirPath, folderPermissions); err != nil {
			return fmt.Errorf("couldn't make config: %v", err)
		}

		if err := writeConfigToFile(pathToConfFile, conf); err != nil {
			return fmt.Errorf("could config to the file: %v", err)
		}
	}
	return nil
}

func fileNameWithoutExtension(fileName string) string {
	return strings.TrimSuffix(fileName, filepath.Ext(fileName))
}

func GetConfig(pathToConfFile string) (*Config, error) {
	dir, file := filepath.Split(pathToConfFile)
	configName := fileNameWithoutExtension(file)
	viper.SetConfigName(configName)
	viper.AddConfigPath(dir)
	viper.SetConfigType("toml")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	conf := DefaultConfig()
	if err := viper.Unmarshal(conf); err != nil {
		return nil, err
	}

	return conf, nil
}
