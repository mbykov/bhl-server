package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "bhl-server/lib"
    ort "github.com/yalue/onnxruntime_go"
)

func main() {
    startTime := time.Now()
    log.Println("🚀 Запуск сервера голосового дневника...")

    // Шаг 1: Загрузка конфига
    log.Println("📁 Шаг 1/4: Загрузка конфигурации...")
    cfg, err := lib.LoadConfig("config.yaml")
    if err != nil {
        log.Fatal("❌ Ошибка загрузки config.yaml:", err)
    }
    log.Printf("   ✅ Конфиг загружен: порт %s", cfg.Server.Port)

    // Шаг 1.5: Запуск проверок системы
    log.Println("🔍 Запуск проверок системы...")
    results := lib.RunAllChecks(cfg)
    lib.PrintCheckResults(results)

    // Если проверки не пройдены и требуется остановка
    if cfg.Checks.RequireAllModules {
        allOK := true
        for _, r := range results {
            if !r.OK {
                allOK = false
                break
            }
        }
        if !allOK {
            log.Fatal("❌ Критические проверки не пройдены. Исправьте ошибки и запустите снова.")
        }
    }

    // Шаг 2: Инициализация ONNX Runtime
    log.Println("⚙️ Инициализация ONNX Runtime...")
    ort.SetSharedLibraryPath("/home/michael/go/ort/lib/libonnxruntime.so")
    if err := ort.InitializeEnvironment(); err != nil {
        log.Fatal("❌ Ошибка инициализации ONNX Runtime:", err)
    }
    log.Println("   ✅ ONNX Runtime инициализирован")

    // Шаг 3: Загрузка моделей
    log.Println("🤖 Шаг 3/4: Инициализация моделей...")
    models, err := lib.LoadModels(cfg)
    if err != nil {
        ort.DestroyEnvironment()
        log.Fatal("❌ Ошибка загрузки моделей:", err)
    }

    // Шаг 4: Создание WebSocket обработчика
    log.Println("🔧 Шаг 4/4: Настройка WebSocket обработчика...")
    wsHandler := lib.NewWSHandler(cfg, models)
    http.HandleFunc("/", wsHandler.Handle)

    // Создаем HTTP сервер
    server := &http.Server{
        Addr: ":" + cfg.Server.Port,
        Handler: nil,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 30 * time.Second,
    }

    // Запуск сервера в горутине
    go func() {
        log.Printf("🌐 Запуск HTTPS сервера на порту %s...", cfg.Server.Port)
        if err := server.ListenAndServeTLS(cfg.Server.Cert, cfg.Server.Key); err != nil && err != http.ErrServerClosed {
            log.Fatal("❌ Ошибка сервера:", err)
        }
    }()

    totalTime := time.Since(startTime)
    log.Printf("✅ Сервер полностью запущен за %v", totalTime)
    log.Println("📝 Ожидание подключений...")

    // Graceful shutdown с контекстом
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    log.Println("🛑 Получен сигнал остановки, завершение работы...")

    // Создаем контекст с таймаутом для graceful shutdown
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // 1. Останавливаем HTTP сервер (перестаем принимать новые соединения)
    log.Println("📡 Остановка HTTP сервера...")
    if err := server.Shutdown(ctx); err != nil {
        log.Printf("⚠️ Ошибка при остановке сервера: %v", err)
    }

    // 2. Закрываем все WebSocket сессии
    log.Println("🔌 Закрытие всех WebSocket соединений...")
    wsHandler.CloseAllSessions()

    // 3. Даем время на завершение текущих запросов
    time.Sleep(500 * time.Millisecond)

    // 4. Закрываем модели (Go-обертки)
    log.Println("🧹 Освобождение ресурсов моделей...")
    models.Close()

    // 5. Даем время C-коду завершиться
    time.Sleep(500 * time.Millisecond)

    // 6. Только после всего уничтожаем ONNX Runtime
    log.Println("🔄 Завершение работы ONNX Runtime...")
    ort.DestroyEnvironment()

    log.Println("👋 Сервер остановлен")
}
