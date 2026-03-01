package main

import (
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "bhl-server/lib"
)

func main() {
    log.Println("🚀 Запуск сервера...")

    cfg, err := lib.LoadConfig("config.yaml")
    if err != nil {
        log.Fatal("❌ Ошибка загрузки config.yaml:", err)
    }
    log.Println("✅ Конфиг загружен")

    wsHandler := lib.NewWSHandler(cfg)
    http.HandleFunc("/", wsHandler.Handle)

    log.Printf("🌐 Сервер слушает порт %s с TLS", cfg.Server.Port)

    go func() {
        if err := http.ListenAndServeTLS(":"+cfg.Server.Port,
            cfg.Server.Cert, cfg.Server.Key, nil); err != nil {
            log.Fatal("❌ Ошибка сервера:", err)
        }
    }()

    log.Println("✅ Сервер успешно запущен")

    // Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    log.Println("🛑 Получен сигнал остановки, завершение работы...")
    time.Sleep(1 * time.Second)
    log.Println("👋 Сервер остановлен")
}
