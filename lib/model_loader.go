package lib

import (
    "fmt"
    "log"

    "github.com/mbykov/bhl-command-go"
    "github.com/mbykov/bhl-vosk-sherpa-go/vosk"
    "github.com/mbykov/bhl-gigaam-sherpa-go"
)

type Models struct {
    Vosk   *vosk.ASRModule
    Command *command.SearchEngine
    GigaAM *gigaam.GigaAMModule
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
        log.Println("  🔧 Загрузка Command модуля...")

        cmdEngine, err := command.NewSearchEngine(
            cfg.Command.Model.OnnxPath,
            cfg.Command.Model.TokenizerPath,
            cfg.Command.Model.LibPath,
            cfg.Command.Model.Threshold,
        )
        if err != nil {
            return nil, fmt.Errorf("ошибка загрузки Command: %v", err)
        }

        // Загружаем команды из JSON
        if err := cmdEngine.LoadCommands("./data/commands.json"); err != nil {
            return nil, fmt.Errorf("ошибка загрузки команд: %v", err)
        }

        models.Command = cmdEngine
        log.Println("  ✅ Command модуль загружен")
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
    if m.Command != nil {
        m.Command.Close()
    }
    if m.GigaAM != nil {
        m.GigaAM.Close()
    }
}
