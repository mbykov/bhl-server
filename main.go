package main

import (
    "context"
    "log/slog"
    "net/http"   // <-- добавлено
    "os"
    "os/signal"
    "syscall"
    "time"

    "bhl/lib"
    "github.com/mbykov/bhl-command-go"
    sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

func main() {
	// 1. Настройка логирования
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	// 2. Загрузка конфигурации
	config, err := lib.LoadConfig()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	logger.Info("config loaded",
		"port", config.Server.Port,
		"vosk_encoder", config.Models.Vosk.Encoder,
		"giga_encoder", config.Models.Giga.Encoder)

	// 3. Инициализация GigaAM (один экземпляр для всех сессий)
	gigaRec := initGigaRecognizer(config)
	if gigaRec == nil {
		logger.Error("failed to create GigaAM recognizer")
		os.Exit(1)
	}
	defer sherpa.DeleteOfflineRecognizer(gigaRec)

	// 4. Инициализация модуля поиска команд
	var cmdEngine *command.SearchEngine
	var cmdThr float32
	if config.Command.Enabled {
		cmdEngine, err = initCommandEngine(config)
		if err != nil {
			logger.Error("failed to init command engine", "error", err)
			os.Exit(1)
		}
		defer cmdEngine.Close()
		cmdThr = config.Command.Threshold
		logger.Info("command engine initialized", "threshold", cmdThr)
	} else {
		logger.Info("command engine disabled")
	}

	// 5. Создание менеджера сессий
	sessionManager := lib.NewSessionManager(config, gigaRec, cmdEngine, cmdThr)

	// 6. Настройка HTTP маршрута
	wsHandler := lib.NewWebSocketHandler(sessionManager)
	http.HandleFunc("/", wsHandler.Handler())

	// 7. Запуск сервера (SSL опционально)
	addr := ":" + config.Server.Port
	useSSL := checkSSLCertificates(config.Server.Cert, config.Server.Key)

	serverCtx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if useSSL {
		logger.Info("starting secure server", "addr", "https://localhost"+addr)
		go func() {
			if err := http.ListenAndServeTLS(addr, config.Server.Cert, config.Server.Key, nil); err != nil {
				logger.Error("secure server failed", "error", err)
				stop()
			}
		}()
	} else {
		logger.Info("starting server (no SSL)", "addr", "http://localhost"+addr)
		go func() {
			if err := http.ListenAndServe(addr, nil); err != nil {
				logger.Error("server failed", "error", err)
				stop()
			}
		}()
	}

	// 8. Ожидание сигнала завершения
	<-serverCtx.Done()
	logger.Info("shutting down gracefully...")

	// 9. Завершение всех сессий
	sessionManager.Shutdown()

	// 10. Даём время на закрытие соединений
	time.Sleep(500 * time.Millisecond)
	logger.Info("server stopped")
}

// initGigaRecognizer создаёт общий OfflineRecognizer.
func initGigaRecognizer(cfg *lib.Config) *sherpa.OfflineRecognizer {
	gConfig := sherpa.OfflineRecognizerConfig{}
	gConfig.FeatConfig.SampleRate = 16000
	gConfig.FeatConfig.FeatureDim = 64
	gConfig.ModelConfig.Transducer.Encoder = cfg.Models.Giga.Encoder
	gConfig.ModelConfig.Transducer.Decoder = cfg.Models.Giga.Decoder
	gConfig.ModelConfig.Transducer.Joiner = cfg.Models.Giga.Joiner
	gConfig.ModelConfig.Tokens = cfg.Models.Giga.Tokens
	gConfig.ModelConfig.NumThreads = cfg.Decoding.NumThreads
	gConfig.ModelConfig.ModelType = "nemo_transducer"
	return sherpa.NewOfflineRecognizer(&gConfig)
}

// initCommandEngine инициализирует SearchEngine и загружает команды.
func initCommandEngine(cfg *lib.Config) (*command.SearchEngine, error) {
	engine, err := command.NewSearchEngine(
		cfg.Command.OnnxPath,
		cfg.Command.TokenizerPath,
		cfg.Command.OnnxLibPath,
		cfg.Command.Threshold,
	)
	if err != nil {
		return nil, err
	}
	if err := engine.LoadCommands(cfg.Command.CommandsJSON); err != nil {
		engine.Close()
		return nil, err
	}
	return engine, nil
}

// checkSSLCertificates проверяет существование файлов сертификатов.
func checkSSLCertificates(certPath, keyPath string) bool {
	if certPath == "" || keyPath == "" {
		return false
	}
	_, errCert := os.Stat(certPath)
	_, errKey := os.Stat(keyPath)
	return errCert == nil && errKey == nil
}
