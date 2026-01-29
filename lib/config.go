package lib

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Config - основная структура конфигурации
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Models   ModelsConfig   `mapstructure:"models"`
    Commands CommandConfig  `mapstructure:"commands"`  // Новый раздел
	Decoding DecodingConfig `mapstructure:"decoding"`
	Endpoint EndpointConfig `mapstructure:"endpoint"`
}

// Новый раздел для команд
type CommandConfig struct {
    CommandFile      string `mapstructure:"command_file"`
    CommandThreshold int    `mapstructure:"command_threshold"`
}

// ServerConfig - конфигурация сервера
type ServerConfig struct {
	Port string `mapstructure:"port"`
	Cert string `mapstructure:"cert"`
	Key  string `mapstructure:"key"`
    CommandFile string `mapstructure:"command_file"` // новое поле
}

// ModelsConfig - конфигурация всех моделей
type ModelsConfig struct {
	Vosk       ModelVoskConfig       `mapstructure:"vosk"`
	Punct      ModelPunctConfig      `mapstructure:"punct"`      // Для пунктуации
	Classifier ModelClassifierConfig `mapstructure:"classifier"` // Для классификации команд
}

// ModelVoskConfig - конфигурация модели Vosk
type ModelVoskConfig struct {
	Encoder   string `mapstructure:"encoder"`
	Decoder   string `mapstructure:"decoder"`
	Joiner    string `mapstructure:"joiner"`
	Tokens    string `mapstructure:"tokens"`
	ModelType string `mapstructure:"model_type"`
}

// ModelPunctConfig - конфигурация модели пунктуации
type ModelPunctConfig struct {
	ModelPath string `mapstructure:"model_path"`
	OnnxPath  string `mapstructure:"onnx_path"`
}

// ModelClassifierConfig - конфигурация модели классификации команд
type ModelClassifierConfig struct {
	ModelPath string `mapstructure:"model_path"`
}

// DecodingConfig - конфигурация декодирования
type DecodingConfig struct {
	NumThreads     int    `mapstructure:"num_threads"`
	Provider       string `mapstructure:"provider"`
	DecodingMethod string `mapstructure:"decoding_method"`
	MaxActivePaths int    `mapstructure:"max_active_paths"`
}

// EndpointConfig - конфигурация endpoint detection
type EndpointConfig struct {
	Enable                   bool    `mapstructure:"enable"`
	Rule1MinTrailingSilence  float32 `mapstructure:"rule1_min_trailing_silence"`
	Rule2MinTrailingSilence  float32 `mapstructure:"rule2_min_trailing_silence"`
	Rule3MinUtteranceLength  float32 `mapstructure:"rule3_min_utterance_length"`
}

// LoadConfig загружает конфигурацию из файла и флагов командной строки
func LoadConfig() (*Config, error) {
	// Настройка Viper
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	// Чтение конфигурации из файла
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("Config file not found, using defaults and flags")
		} else {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	// Установка значений по умолчанию
	setDefaults()

	// Привязка флагов командной строки
	bindFlags()

	// Автоматическое чтение переменных окружения
	viper.AutomaticEnv()

	// Разбор конфигурации
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Валидация обязательных параметров
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

func setDefaults() {
	viper.SetDefault("server.port", "6006")
	viper.SetDefault("decoding.num_threads", 1)
	viper.SetDefault("decoding.provider", "cpu")
	viper.SetDefault("decoding.decoding_method", "greedy_search")
	viper.SetDefault("decoding.max_active_paths", 4)
	viper.SetDefault("endpoint.enable", true)
	viper.SetDefault("endpoint.rule1_min_trailing_silence", 2.4)
	viper.SetDefault("endpoint.rule2_min_trailing_silence", 1.2)
	viper.SetDefault("endpoint.rule3_min_utterance_length", 20.0)
}

func bindFlags() {
	// Флаги для Vosk модели
	pflag.String("encoder", "", "Path to the transducer encoder model")
	pflag.String("decoder", "", "Path to the transducer decoder model")
	pflag.String("joiner", "", "Path to the transducer joiner model")
	pflag.String("tokens", "", "Path to the tokens file")
	pflag.String("model-type", "", "Model type for faster loading")

	// Флаги для сервера
	pflag.String("port", "6006", "Port to listen on")
	pflag.String("cert", "", "Path to SSL certificate (PEM)")
	pflag.String("key", "", "Path to SSL private key (PEM)")

	// Флаги для модели пунктуации
	pflag.String("punct-model", "", "Path to RUPunct model directory (contains tokenizer.json)")
	pflag.String("punct-onnx", "", "Path to RUPunct ONNX model file")

	// Флаги для модели классификации команд
	pflag.String("classifier-model", "", "Path to command classifier model")

	// Флаги для обработки
	pflag.Int("num-threads", 1, "Number of threads for NN computation")
	pflag.String("provider", "cpu", "Provider to use (cpu, cuda, coreml)")
	pflag.String("decoding-method", "greedy_search", "Decoding method")
	pflag.Int("max-active-paths", 4, "Max active paths for modified_beam_search")

	// Endpoint detection
	pflag.Bool("enable-endpoint", true, "Enable endpoint detection")
	pflag.Float32("rule1-min-trailing-silence", 2.4, "Rule1 min trailing silence")
	pflag.Float32("rule2-min-trailing-silence", 1.2, "Rule2 min trailing silence")
	pflag.Float32("rule3-min-utterance-length", 20, "Rule3 min utterance length")

    // Флаги для команд
    pflag.String("command-file", "", "Path to command definitions JSON file")
    pflag.Int("command-threshold", 2, "Levenshtein distance threshold for command matching")

	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)
}

func validateConfig(config *Config) error {
	// Проверка обязательных параметров Vosk
	if config.Models.Vosk.Tokens == "" {
		return fmt.Errorf("tokens path is required")
	}

	if config.Models.Vosk.Encoder == "" || config.Models.Vosk.Decoder == "" || config.Models.Vosk.Joiner == "" {
		return fmt.Errorf("encoder, decoder, and joiner paths are required for Vosk model")
	}

    // Предупреждение о файле команд
    if config.Commands.CommandFile == "" {
        log.Println("Warning: Command definitions file not provided, command detection will be disabled")
    } else {
        if _, err := os.Stat(config.Commands.CommandFile); os.IsNotExist(err) {
            log.Printf("Warning: Command definitions file not found: %s", config.Commands.CommandFile)
        }
    }

	// Проверка существования файлов
	checkFile := func(path, desc string) error {
		if path != "" {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return fmt.Errorf("%s does not exist: %s", desc, path)
			}
		}
		return nil
	}

	if err := checkFile(config.Models.Vosk.Encoder, "Encoder model"); err != nil {
		return err
	}
	if err := checkFile(config.Models.Vosk.Decoder, "Decoder model"); err != nil {
		return err
	}
	if err := checkFile(config.Models.Vosk.Joiner, "Joiner model"); err != nil {
		return err
	}
	if err := checkFile(config.Models.Vosk.Tokens, "Tokens file"); err != nil {
		return err
	}

	// Предупреждение о модели пунктуации, но не фатально
	if config.Models.Punct.ModelPath == "" || config.Models.Punct.OnnxPath == "" {
		log.Println("Warning: Punctuation model paths not provided, punctuation will be disabled")
	} else {
		if err := checkFile(config.Models.Punct.ModelPath, "Punctuation model directory"); err != nil {
			log.Printf("Warning: %v", err)
		}
		if err := checkFile(config.Models.Punct.OnnxPath, "Punctuation ONNX model"); err != nil {
			log.Printf("Warning: %v", err)
		}
	}

	// Предупреждение о модели классификации команд
	if config.Models.Classifier.ModelPath == "" {
		log.Println("Warning: Command classifier model path not provided, command detection will be disabled")
	}

	return nil
}
