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
