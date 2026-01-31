package main

import (
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "bhl/lib"
    rupunct "github.com/mbykov/rupunct-go"
    command "github.com/mbykov/command-go"
)

func main() {
    // Загрузка конфигурации
    config, err := lib.LoadConfig()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // log.Printf("=== CONFIGURATION ===")
	// log.Printf("Punctuation model: %s", config.Models.Punct.OnnxPath)
	// log.Printf("Command file: %s", config.Commands.CommandFile)
	// log.Printf("Command threshold: %d", config.Commands.CommandThreshold)
	// log.Printf("=====================")

    // 1. Создаем пунктуатор НАПРЯМУЮ
    var punctuator rupunct.Punctuator
    if config.Models.Punct.ModelPath != "" && config.Models.Punct.OnnxPath != "" {
        log.Printf("Loading punctuation model from: %s", config.Models.Punct.ModelPath)

        punctuator, err = rupunct.NewPunctuator(
            config.Models.Punct.ModelPath,
            config.Models.Punct.OnnxPath,
        )
        if err != nil {
            log.Printf("ERROR: Failed to initialize punctuation model: %v", err)
            log.Printf("Punctuation will be disabled")
            punctuator = nil
        } else {
            log.Println("✓ Punctuation model loaded successfully")
            defer punctuator.Close()
        }
    } else {
        log.Println("ℹ️  Punctuation model paths not provided, skipping punctuation")
    }

    // 1. Создаем менеджер сессий Vosk
    sessionManager := lib.NewSessionManager()

    // 3. Создаем менеджер моделей для команд (через hugot)
    modelManager, err := lib.NewModelManager()
    if err != nil {
        log.Printf("Warning: Failed to create model manager for hugot: %v", err)
        // Не фатальная ошибка - команды пока не обрабатываем
    }
    if modelManager != nil {
        defer modelManager.Destroy()

        // Инициализируем классификатор команд если путь указан
        if config.Models.Classifier.ModelPath != "" {
            if err := modelManager.InitCommandClassifier(config.Models.Classifier.ModelPath); err != nil {
                log.Printf("Warning: Failed to init command classifier: %v", err)
            }
        }
    }


    if _, err := os.Stat(config.Commands.CommandFile); os.IsNotExist(err) {
        log.Printf("ERROR: Command definitions file not found: %s", config.Commands.CommandFile)
    } else {
        log.Printf("Command definitions file found: %s", config.Commands.CommandFile)
    }

    // 2. Создаем определитель команд
    var commandResolver *command.CommandResolver
    if config.Commands.CommandFile != "" {
        log.Printf("Loading command definitions from: %s", config.Commands.CommandFile)
        commandResolver, err = command.NewResolver(
            config.Commands.CommandFile,
            config.Commands.CommandThreshold,
        )
        if err != nil {
            log.Printf("ERROR: Failed to load command resolver: %v", err)
            commandResolver = nil
        } else {
            log.Println("✓ Command resolver loaded successfully")
        }
    }

    // Создаем CommandExecutor (заглушку)
    commandExecutor := &lib.CommandExecutor{}

    // 4. Создаем WebSocket обработчик
    // wsHandler := lib.NewWebSocketHandler(config, sessionManager, punctuator)
    wsHandler := lib.NewWebSocketHandler(
        config,
        sessionManager,
        punctuator,
        commandResolver,
        commandExecutor,
    )


    // 5. Настройка HTTP маршрутов "/ws"
    http.HandleFunc("/", wsHandler.Handler())

    // Проверка SSL сертификатов
    useSSL := checkSSLCertificates(config.Server.Cert, config.Server.Key)

    // Запуск сервера
    addr := ":" + config.Server.Port

    if useSSL {
        log.Printf("Starting secure ASR WebSocket server on https://%s", addr)
        log.Printf("SSL certificate: %s", config.Server.Cert)
        log.Printf("SSL key: %s", config.Server.Key)

        log.Printf("Available addresses:")
        log.Printf("  https://localhost%s", addr)
        log.Printf("  https://127.0.0.1%s", addr)
        log.Printf("  https://tma.local%s", addr)

        go func() {
            if err := http.ListenAndServeTLS(addr, config.Server.Cert, config.Server.Key, nil); err != nil {
                log.Fatalf("Failed to start secure server: %v", err)
            }
        }()
    } else {
        log.Printf("Starting ASR WebSocket server on http://%s", addr)
        log.Printf("Warning: Running without SSL. For secure connections, provide --cert and --key parameters")

        go func() {
            if err := http.ListenAndServe(addr, nil); err != nil {
                log.Fatalf("Failed to start server: %v", err)
            }
        }()
    }

    // Ожидание сигнала завершения
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    log.Printf("Server started successfully. Press Ctrl+C to stop.")

    // Ожидание сигнала завершения
    <-sigChan
    log.Println("Shutting down server...")

    // Даем время для завершения операций
    time.Sleep(1 * time.Second)
    log.Println("Server stopped.")
}

// checkSSLCertificates проверяет наличие SSL сертификатов
func checkSSLCertificates(certPath, keyPath string) bool {
    if certPath == "" || keyPath == "" {
        return false
    }

    if _, err := os.Stat(certPath); os.IsNotExist(err) {
        log.Printf("SSL certificate not found: %s", certPath)
        return false
    }

    if _, err := os.Stat(keyPath); os.IsNotExist(err) {
        log.Printf("SSL key not found: %s", keyPath)
        return false
    }

    return true
}
