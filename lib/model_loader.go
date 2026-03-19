package lib

import (
    "fmt"
    "log"
    "sync"
    "time"

    "github.com/mbykov/command-go-levenshtein"
    "github.com/mbykov/bhl-vosk-sherpa-go/vosk"
    "github.com/mbykov/gigaam-ort-go"
    qwen "github.com/michael/bhl-qwen-go"
)

type Models struct {
    Vosk     *vosk.ASRModule
    Command  *command.CommandResolver
    GigaAM   *gigaam.GigaAMModule
    Qwen     *qwen.CommandResolver  // добавляем Qwen
    closeMu  sync.Mutex
    closed   bool
}

func LoadModels(cfg *Config) (*Models, error) {
    models := &Models{}

    // Vosk
    log.Println("  🎤 Загрузка Vosk модуля...")
    voskCfg := vosk.Config{
        ModelPath:  cfg.Vosk.ModelPath,
        SampleRate: cfg.Vosk.SampleRate,
        FeatureDim: cfg.Vosk.FeatureDim,
        ChunkMs:    cfg.Vosk.ChunkMs,
    }
    voskModule, err := vosk.New(voskCfg)
    if err != nil {
        return nil, fmt.Errorf("ошибка загрузки Vosk: %v", err)
    }
    models.Vosk = voskModule
    log.Println("  ✅ Vosk модуль загружен")

    // Command (Левенштейн)
    if cfg.Command.Enabled {
        log.Println("  🔧 Загрузка Command модуля (Левенштейн)...")
        threshold := 3
        if cfg.Command.Threshold > 0 {
            threshold = cfg.Command.Threshold
        }
        cmdResolver, err := command.NewResolver(cfg.Command.CommandsFile, threshold)
        if err != nil {
            return nil, fmt.Errorf("ошибка загрузки Command: %v", err)
        }
        models.Command = cmdResolver
        log.Println("  ✅ Command модуль загружен")
    }

    // GigaAM
    if cfg.GigaAM.Enabled {
        log.Println("  📝 Загрузка GigaAM модуля...")
        gigaamCfg := gigaam.Config{
            ModelPath:  cfg.GigaAM.ModelPath,
            SampleRate: cfg.GigaAM.SampleRate,
            FeatureDim: cfg.GigaAM.FeatureDim,
            NumThreads: cfg.GigaAM.NumThreads,
            Provider:   cfg.GigaAM.Provider,
        }
        gigaamModule, err := gigaam.New(gigaamCfg)
        if err != nil {
            return nil, fmt.Errorf("ошибка загрузки GigaAM: %v", err)
        }
        models.GigaAM = gigaamModule
        log.Println("  ✅ GigaAM модуль загружен")
    }

    // Qwen
    if cfg.Qwen.Enabled {
        log.Println("  🤖 Загрузка Qwen модуля...")
        qwenModule, err := qwen.NewResolver(cfg.Qwen.ConfigPath)
        if err != nil {
            return nil, fmt.Errorf("ошибка загрузки Qwen: %v", err)
        }
        models.Qwen = qwenModule
        log.Println("  ✅ Qwen модуль загружен")
    }

    return models, nil
}

func (m *Models) Close() {
    m.closeMu.Lock()
    defer m.closeMu.Unlock()

    if m.closed {
        return
    }
    m.closed = true

    log.Println("🔄 Начало закрытия моделей...")

    // Закрываем в обратном порядке загрузки
    if m.Qwen != nil {
        log.Println("  🤖 Закрытие Qwen...")
        m.Qwen.Close()
    }

    if m.GigaAM != nil {
        log.Println("  📝 Закрытие GigaAM...")
        m.GigaAM.Close()
    }

    if m.Command != nil {
        log.Println("  🔧 Закрытие Command...")
        // если есть метод Close - вызвать
    }

    if m.Vosk != nil {
        log.Println("  🎤 Закрытие Vosk...")
        m.Vosk.Close()
    }

    // Даем время на завершение C-вызовов
    time.Sleep(100 * time.Millisecond)

    log.Println("✅ Все модели закрыты")
}
