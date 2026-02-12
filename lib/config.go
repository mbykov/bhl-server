package lib

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port string `yaml:"port"`
		Cert string `yaml:"cert"`
		Key  string `yaml:"key"`
	} `yaml:"server"`

	Models struct {
		Vosk struct {
			Encoder   string `yaml:"encoder"`
			Decoder   string `yaml:"decoder"`
			Joiner    string `yaml:"joiner"`
			Tokens    string `yaml:"tokens"`
			ModelType string `yaml:"model_type"`
		} `yaml:"vosk"`
		Giga struct {
			Encoder string `yaml:"encoder"`
			Decoder string `yaml:"decoder"`
			Joiner  string `yaml:"joiner"`
			Tokens  string `yaml:"tokens"`
		} `yaml:"gigaam"`
	} `yaml:"models"`

	Command struct {
		Enabled       bool    `yaml:"enabled"`
		OnnxPath      string  `yaml:"onnx_path"`
		TokenizerPath string  `yaml:"tokenizer_path"`
		OnnxLibPath   string  `yaml:"onnx_lib_path"`
		Threshold     float32 `yaml:"threshold"`
		CommandsJSON  string  `yaml:"commands_json"`
	} `yaml:"command"`

	Decoding struct {
		NumThreads     int    `yaml:"num_threads"`
		Provider       string `yaml:"provider"`
		DecodingMethod string `yaml:"decoding_method"`
		MaxActivePaths int    `yaml:"max_active_paths"`
	} `yaml:"decoding"`

	Endpoint struct {
		Enable                  bool    `yaml:"enable"`
		Rule1MinTrailingSilence float32 `yaml:"rule1_min_trailing_silence"`
		Rule2MinTrailingSilence float32 `yaml:"rule2_min_trailing_silence"`
		Rule3MinUtteranceLength float32 `yaml:"rule3_min_utterance_length"`
	} `yaml:"endpoint"`
}

func LoadConfig() (*Config, error) {
	configPath := "config.yaml"
	if path := os.Getenv("BHL_CONFIG"); path != "" {
		configPath = path
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, err
	}

	configDir := filepath.Dir(absPath)

	// Server certs
	config.Server.Cert = resolvePath(config.Server.Cert, configDir)
	config.Server.Key = resolvePath(config.Server.Key, configDir)

	// Vosk
	config.Models.Vosk.Encoder = resolvePath(config.Models.Vosk.Encoder, configDir)
	config.Models.Vosk.Decoder = resolvePath(config.Models.Vosk.Decoder, configDir)
	config.Models.Vosk.Joiner = resolvePath(config.Models.Vosk.Joiner, configDir)
	config.Models.Vosk.Tokens = resolvePath(config.Models.Vosk.Tokens, configDir)

	// GigaAM
	config.Models.Giga.Encoder = resolvePath(config.Models.Giga.Encoder, configDir)
	config.Models.Giga.Decoder = resolvePath(config.Models.Giga.Decoder, configDir)
	config.Models.Giga.Joiner = resolvePath(config.Models.Giga.Joiner, configDir)
	config.Models.Giga.Tokens = resolvePath(config.Models.Giga.Tokens, configDir)

	// Command models
	config.Command.OnnxPath = resolvePath(config.Command.OnnxPath, configDir)
	config.Command.TokenizerPath = resolvePath(config.Command.TokenizerPath, configDir)
	config.Command.OnnxLibPath = resolvePath(config.Command.OnnxLibPath, configDir)
	config.Command.CommandsJSON = resolvePath(config.Command.CommandsJSON, configDir)

	return config, nil
}

func resolvePath(path, baseDir string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}
