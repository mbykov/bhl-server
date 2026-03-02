package main

import (
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "bhl-server/lib"  // Это должен быть правильный путь к модулю
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

    // Шаг 2: Загрузка моделей
    log.Println("🤖 Шаг 2/4: Инициализация моделей...")
    models, err := lib.LoadModels(cfg)
    if err != nil {
        log.Fatal("❌ Ошибка загрузки моделей:", err)
    }
    defer models.Close()

    // Шаг 3: Создание WebSocket обработчика с моделями
    log.Println("🔧 Шаг 3/4: Настройка WebSocket обработчика...")
    wsHandler := lib.NewWSHandler(cfg, models)
    http.HandleFunc("/", wsHandler.Handle)

    // Шаг 4: Запуск HTTPS сервера
    log.Println("🌐 Шаг 4/4: Запуск HTTPS сервера...")

    go func() {
        if err := http.ListenAndServeTLS(":"+cfg.Server.Port,
            cfg.Server.Cert, cfg.Server.Key, nil); err != nil {
            log.Fatal("❌ Ошибка сервера:", err)
        }
    }()

    totalTime := time.Since(startTime)
    log.Printf("✅ Сервер полностью запущен за %v", totalTime)
    log.Println("📝 Ожидание подключений...")

    // Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    log.Println("🛑 Получен сигнал остановки, завершение работы...")
    time.Sleep(1 * time.Second)
    log.Println("👋 Сервер остановлен")
}
