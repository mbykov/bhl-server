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
        ModelPath  string `yaml:"model_path"`
        TestWav    string `yaml:"test_wav"`
        SampleRate int    `yaml:"sample_rate"`
        FeatureDim int    `yaml:"feature_dim"`
        ChunkMs    int    `yaml:"chunk_ms"`
    } `yaml:"vosk"`

    Command struct {
        Enabled      bool   `yaml:"enabled"`
        CommandsFile string `yaml:"commands_file"`
        MinWords     int    `yaml:"min_words"`
        Threshold    int    `yaml:"threshold"`
    } `yaml:"command"`

    GigaAM struct {
        Enabled    bool   `yaml:"enabled"`
        ModelPath  string `yaml:"model_path"`
        SampleRate int    `yaml:"sample_rate"`
        FeatureDim int    `yaml:"feature_dim"`
        NumThreads int    `yaml:"num_threads"`
        Provider   string `yaml:"provider"`
    } `yaml:"gigaam"`

    // Новая секция для Qwen
    Qwen struct {
        Enabled     bool   `yaml:"enabled"`
        ConfigPath  string `yaml:"config_path"`   // путь к config.yaml модуля Qwen
        ModelName   string `yaml:"model_name"`     // для проверки в Ollama
        TimeoutSec  int    `yaml:"timeout_sec"`    // таймаут на весь вызов
        Debug       bool   `yaml:"debug"`          // подробное логирование
    } `yaml:"qwen"`

    // Проверка модулей при старте
    Checks struct {
        RequireAllModules bool `yaml:"require_all_modules"` // требовать ли все модули
        RequireOllama     bool `yaml:"require_ollama"`      // требовать ли Ollama
    } `yaml:"checks"`
}

func LoadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var cfg Config
    err = yaml.Unmarshal(data, &cfg)
    if err != nil {
        return nil, err
    }

    return &cfg, nil
}
