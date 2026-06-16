package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Storage   StorageConfig   `mapstructure:"storage"`
	MiMo      MiMoConfig      `mapstructure:"mimo"`
	RTVC      RTVCConfig      `mapstructure:"rtvc"`
	Synthesis SynthesisConfig `mapstructure:"synthesis"`
	Library   LibraryConfig   `mapstructure:"library"`
	LLM       LLMConfig       `mapstructure:"llm"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Host string `mapstructure:"host"`
}

type StorageConfig struct {
	UploadDir string `mapstructure:"upload_dir"`
	OutputDir string `mapstructure:"output_dir"`
	DBPath    string `mapstructure:"db_path"`
}

type MiMoConfig struct {
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
}

type RTVCConfig struct {
	PythonPath string `mapstructure:"python_path"`
	ProjectDir string `mapstructure:"project_dir"`
	Enabled    bool   `mapstructure:"enabled"`
}

type SynthesisConfig struct {
	MaxConcurrency int     `mapstructure:"max_concurrency"`
	ChapterGap     float64 `mapstructure:"chapter_gap"`
	DefaultFormat  string  `mapstructure:"default_format"`
	SampleRate     int     `mapstructure:"sample_rate"`
}

type LibraryConfig struct {
	ProjectPath string `mapstructure:"project_path"`
}

type LLMConfig struct {
	BaseURL string `mapstructure:"base_url"`
	APIKey  string `mapstructure:"api_key"`
	Model   string `mapstructure:"model"`
	Timeout int    `mapstructure:"timeout"` // seconds
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("storage.upload_dir", "./data/uploads")
	v.SetDefault("storage.output_dir", "./data/output")
	v.SetDefault("storage.db_path", "./data/audiobook.db")
	v.SetDefault("mimo.base_url", "https://api.xiaomimimo.com/v1")
		v.SetDefault("synthesis.max_concurrency", 2)
		v.SetDefault("synthesis.chapter_gap", 1.5)
		v.SetDefault("synthesis.default_format", "mp3")
		v.SetDefault("synthesis.sample_rate", 24000)
		v.SetDefault("library.project_path", "")
		v.SetDefault("llm.timeout", 120)

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(path)
	v.AddConfigPath(".")

	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
