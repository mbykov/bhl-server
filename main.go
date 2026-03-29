package main

import (
    "context"      // <--- Добавлено
	"log/slog"
    "net/http"
    "os"
    "log"
    "os/signal"   // <--- Добавлено
    "syscall"     // <--- Добавлено
    "time"

	"github.com/mbykov/wshandler-go" // замените на ваш путь
	"github.com/mbykov/asr-zipformer-go"   // ваш модуль ASR
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port string `yaml:"port"`
		Cert string `yaml:"cert"`
		Key  string `yaml:"key"`
	} `yaml:"server"`

	// Вместо просто asr.Config, опиши структуру с тегами здесь
	ASR struct {
		ModelDir   string `yaml:"model_dir"`
		SampleRate int    `yaml:"sample_rate"`
	} `yaml:"asr"`
}

func main() {
    // Настройка логгера на уровень DEBUG
    opts := &slog.HandlerOptions{Level: slog.LevelDebug}
    logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
    slog.SetDefault(logger)
	log.Println("🚀 Запуск нового сервера дневника...")

	// 1. Загрузка конфига
	f, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Fatalf("❌ Ошибка чтения конфига: %v", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(f, &cfg); err != nil {
		log.Fatalf("❌ Ошибка парсинга конфига: %v", err)
	}

	// Проверка порта
	if cfg.Server.Port == "" {
		cfg.Server.Port = "6006" // Дефолтный безопасный порт
	}

    // Преобразуем локальную структуру в ту, которую ждет asr.New
    asrParams := asr.Config{
        ModelDir:   cfg.ASR.ModelDir,
        SampleRate: cfg.ASR.SampleRate,
    }

    // Для отладки — если здесь увидишь пустоту, значит YAML не распарсился
    log.Printf("🔍 Проверка пути модели: '%s'", asrParams.ModelDir)

	// 2. Инициализация хендлера
    wsHandler := wshandler.NewWSHandler(asrParams)

	// 3. Настройка HTTP сервера
	mux := http.NewServeMux()
	mux.HandleFunc("/", wsHandler.Handle)

	server := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// 4. Запуск сервера
	go func() {
		log.Printf("🌐 Запуск HTTPS на порту %s...", cfg.Server.Port)
		if err := server.ListenAndServeTLS(cfg.Server.Cert, cfg.Server.Key); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Ошибка сервера: %v", err)
		}
	}()

	// Graceful shutdown (как в старом сервере)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Останавливаем сервер...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("⚠️ Ошибка при остановке: %v", err)
	}

	log.Println("👋 Сервер остановлен")
}
