package main

var DefConfig Config

type Config struct {
	Host        string `json:"host"`
	PprofHost   string `json:"pprof_host" yaml:"pprof_host" mapstructure:"pprof_host"`
	Secret      string `json:"secret"`
	AdminSecret string `json:"adminsecret"`
	DB          string `json:"db"`
	DBLog       bool   `json:"dblog"`
	GoNum       int    `json:"gonum"`

	Redis  RedisConfig  `json:"redis" yaml:"redis" mapstructure:"redis"`
	Client ClientConfig `json:"client" yaml:"client" mapstructure:"client"`
}

type RedisConfig struct {
	Enable  bool   `json:"enable" yaml:"enable" mapstructure:"enable"`
	Host    string `json:"host" yaml:"host" mapstructure:"host"`
	Name    string `json:"name" yaml:"name" mapstructure:"name"`
	Channel string `json:"channel" yaml:"channel" mapstructure:"channel"`
}

type ClientConfig struct {
	ReadMessageSizeLimit int64 `json:"read_message_size_limit" yaml:"read_message_size_limit" mapstructure:"read_message_size_limit"`
	Compression          bool  `json:"compression" yaml:"compression" mapstructure:"compression"`
	CompressionLevel     int   `json:"compression_level" yaml:"compression_level" mapstructure:"compression_level"`
	ReadBufferSize       int   `json:"read_buffer_size" yaml:"read_buffer_size" mapstructure:"read_buffer_size"`
	WriteBufferSize      int   `json:"write_buffer_size" yaml:"write_buffer_size" mapstructure:"write_buffer_size"`
}
