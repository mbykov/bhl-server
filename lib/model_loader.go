package lib

import (
    "fmt"
    "log"
    "time"

    "github.com/mbykov/bhl-command-go" // импортируем ваш модуль
)

type Models struct {
    Command *command.SearchEngine
    // Giga и Qwen добавим позже
}

func LoadModels(cfg *Config) (*Models, error) {
    log.Println("🔄 Загрузка моделей...")
    startTime := time.Now()

    models := &Models{}

    // 1. Загрузка Command Search Engine
    if cfg.Command.Enabled {
        log.Println("  📦 Загрузка модели команд (multilingual-e5-small)...")
        cmdStart := time.Now()

        engine, err := command.NewSearchEngine(
            cfg.Command.ModelPath,
            cfg.Command.TokenizerPath,
            cfg.Command.OrtLibPath,
            cfg.Command.Threshold,
        )
        if err != nil {
            return nil, fmt.Errorf("ошибка загрузки command model: %w", err)
        }

        // Загрузка команд и вычисление эмбеддингов
        log.Printf("  📖 Загрузка команд из %s...", cfg.Command.CommandsPath)
        if err := engine.LoadCommands(cfg.Command.CommandsPath); err != nil {
            engine.Close()
            return nil, fmt.Errorf("ошибка загрузки команд: %w", err)
        }

        models.Command = engine
        log.Printf("  ✅ Command model загружена за %v", time.Since(cmdStart))
    } else {
        log.Println("  ⏭️ Command model отключена в конфиге")
    }

    // 2. Заглушки для Giga и Qwen (позже)
    if cfg.Giga.Enabled {
        log.Println("  📦 Загрузка GigaAM (пунктуация)...")
        // TODO: реализовать позже
    }

    if cfg.Qwen.Enabled {
        log.Println("  📦 Загрузка Qwen (выполнение команд)...")
        // TODO: реализовать позже
    }

    log.Printf("✅ Все модели загружены за %v", time.Since(startTime))
    return models, nil
}

func (m *Models) Close() {
    if m.Command != nil {
        m.Command.Close()
    }
    // TODO: закрыть Giga и Qwen
}
