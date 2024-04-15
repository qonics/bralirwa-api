package config

type LogConfig struct {
	Level   string
	Maxsize int64
	Backups int
}
type Config struct {
	Port int
	Log  LogConfig
}

func InitializeConfig() *Config {
	return &Config{
		Port: 50051,
		Log: LogConfig{
			Level:   "debug",
			Maxsize: 100 * 1024 * 1024,
			Backups: 7,
		},
	}
}
