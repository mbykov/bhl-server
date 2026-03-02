package lib

import (
    "os"
    "gopkg.in/yaml.v3"
)

type Config struct {
    Server struct {
        Port string `yaml:"port"`
        Cert string `yaml:"cert"`
        Key  string `yaml:"key"`
    } `yaml:"server"`

    Vosk struct {
        ModelPath   string `yaml:"model_path"`
        SampleRate  int    `yaml:"sample_rate"`
        FeatureDim  int    `yaml:"feature_dim"`
        ChunkMs     int    `yaml:"chunk_ms"`
    } `yaml:"vosk"`

    Command struct {
        Enabled      bool    `yaml:"enabled"`
        ModelPath    string  `yaml:"model_path"`
        TokenizerPath string `yaml:"tokenizer_path"`
        OrtLibPath   string  `yaml:"ort_lib_path"`
        CommandsPath string  `yaml:"commands_path"`
        Threshold    float32 `yaml:"threshold"`
        MinWords     int     `yaml:"min_words"`
    } `yaml:"command"`

    Giga struct {
        Enabled   bool   `yaml:"enabled"`
        ModelPath string `yaml:"model_path"`
    } `yaml:"giga"`

    Qwen struct {
        Enabled   bool   `yaml:"enabled"`
        ModelPath string `yaml:"model_path"`
    } `yaml:"qwen"`

    Logging struct {
        Verbose bool `yaml:"verbose"`
        Profile bool `yaml:"profile"`
    } `yaml:"logging"`
}

func LoadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    var cfg Config
    err = yaml.Unmarshal(data, &cfg)
    return &cfg, err
}
