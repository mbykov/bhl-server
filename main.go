package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/mbykov/wshandler-go"
	"github.com/mbykov/asr-zipformer-go"
	"github.com/mbykov/vosk-punct"  // добавляем импорт
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port string `yaml:"port"`
		Cert string `yaml:"cert"`
		Key  string `yaml:"key"`
	} `yaml:"server"`

	ASR struct {
		ModelDir   string `yaml:"model_dir"`
		SampleRate int    `yaml:"sample_rate"`
	} `yaml:"asr"`

	// Добавляем секцию пунктуации
	Punctuation struct {
		ModelDir string `yaml:"model_dir"`
	} `yaml:"punctuation"`
}

func main() {
	// Настройка логгера
	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
	slog.SetDefault(logger)
	log.Println("🚀 Запуск сервера дневника...")

	// 1. Загрузка конфига
	f, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Fatalf("❌ Ошибка чтения конфига: %v", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(f, &cfg); err != nil {
		log.Fatalf("❌ Ошибка парсинга конфига: %v", err)
	}

	if cfg.Server.Port == "" {
		cfg.Server.Port = "6006"
	}

	// 2. Инициализация ASR
	asrParams := asr.Config{
		ModelDir:   cfg.ASR.ModelDir,
		SampleRate: cfg.ASR.SampleRate,
	}
	log.Printf("🔍 ASR модель: '%s'", asrParams.ModelDir)

	// 3. Инициализация пунктуатора
	var punctuator *voskpunct.Punctuator
	if cfg.Punctuation.ModelDir != "" {
		log.Printf("🔍 Загрузка пунктуатора из: %s", cfg.Punctuation.ModelDir)
		punctuator, err = voskpunct.New(voskpunct.Config{
			ModelDir: cfg.Punctuation.ModelDir,
		})
		if err != nil {
			log.Printf("⚠️ Ошибка загрузки пунктуатора: %v (пунктуация будет отключена)", err)
			punctuator = nil
		} else {
			log.Println("✅ Пунктуатор загружен")
		}
	} else {
		log.Println("⚠️ Пунктуация не настроена в config.yaml")
	}

	// 4. Инициализация хендлера с передачей пунктуатора
	wsHandler := wshandler.NewWSHandler(asrParams, punctuator)

	// 5. Настройка HTTP сервера
	mux := http.NewServeMux()
	mux.HandleFunc("/", wsHandler.Handle)

	server := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// 6. Запуск сервера
	go func() {
		log.Printf("🌐 Запуск HTTPS на порту %s...", cfg.Server.Port)
		if err := server.ListenAndServeTLS(cfg.Server.Cert, cfg.Server.Key); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Ошибка сервера: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Останавливаем сервер...")

	// Закрываем пунктуатор
	if punctuator != nil {
		punctuator.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("⚠️ Ошибка при остановке: %v", err)
	}

	log.Println("👋 Сервер остановлен")
}
