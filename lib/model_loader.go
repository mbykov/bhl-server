package lib

import (
    "fmt"
    "log"

    "github.com/mbykov/command-go-levenshtein"
    "github.com/mbykov/bhl-vosk-sherpa-go/vosk"
    "github.com/mbykov/bhl-gigaam-go"
)

type Models struct {
    Vosk   *vosk.ASRModule
    Command *command.CommandResolver
    GigaAM *gigaam.GigaAMModule  // теперь новый тип
}

func LoadModels(cfg *Config) (*Models, error) {
    models := &Models{}

    // Vosk всегда загружаем (он есть в конфиге)
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

    // Загрузка Command Engine (если включен)
    if cfg.Command.Enabled {
        log.Println("  🔧 Загрузка Command модуля (Левенштейн)...")

        // Используем порог из конфига или значение по умолчанию
        threshold := 3
        if cfg.Command.Threshold > 0 {
            threshold = cfg.Command.Threshold
        }

        cmdResolver, err := command.NewResolver(cfg.Command.CommandsFile, threshold)
        if err != nil {
            return nil, fmt.Errorf("ошибка загрузки Command: %v", err)
        }

        models.Command = cmdResolver
        log.Println("  ✅ Command модуль загружен (легковесный)")
    }

    // Загрузка GigaAM (если включен)
    if cfg.GigaAM.Enabled {
        log.Println("  📝 Загрузка GigaAM модуля...")
        gigaamCfg := gigaam.Config{
            ModelPath:  cfg.GigaAM.ModelPath,
            SampleRate: cfg.GigaAM.SampleRate,
            FeatureDim: cfg.GigaAM.FeatureDim,
            NumThreads: cfg.GigaAM.NumThreads,
            Provider:   cfg.GigaAM.Provider,
        }
        // gigaamModule, err := gigaam.New(gigaamCfg, cfg.GigaAM.Debug)
        gigaamModule, err := gigaam.New(gigaamCfg)
        if err != nil {
            return nil, fmt.Errorf("ошибка загрузки GigaAM: %v", err)
        }
        models.GigaAM = gigaamModule
        log.Println("  ✅ GigaAM модуль загружен")
    }

    return models, nil
}

func (m *Models) Close() {
    if m.Vosk != nil {
        m.Vosk.Close()
    }
    // if m.Command != nil {
    //     m.Command.Close()
    // }
    if m.GigaAM != nil {
        m.GigaAM.Close()
    }
}
