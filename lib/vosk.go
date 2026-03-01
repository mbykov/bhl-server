package lib

import (
    "fmt"
    "log"
    "sync"
    "os"

    "github.com/mbykov/bhl-vosk-sherpa-go/vosk"
)

type VoskProcessor struct {
    module *vosk.ASRModule
    cfg    *Config
    mu     sync.Mutex
    id     string // для идентификации сессии
}

func NewVoskProcessor(cfg *Config, sessionID string) (*VoskProcessor, error) {
    log.Printf("[%s] 🔧 Инициализация VOSK с моделью: %s", sessionID, cfg.Vosk.ModelPath)

    // Проверим, что файлы модели существуют
    encoderPath := cfg.Vosk.ModelPath + "/am-onnx/encoder.onnx"
    if _, err := os.Stat(encoderPath); err != nil {
        return nil, fmt.Errorf("encoder.onnx not found: %w", err)
    }
    log.Printf("[%s] ✅ encoder.onnx найден", sessionID)

    voskCfg := vosk.Config{
        ModelPath:   cfg.Vosk.ModelPath,
        SampleRate:  cfg.Vosk.SampleRate,
        FeatureDim:  cfg.Vosk.FeatureDim,
        ChunkMs:     cfg.Vosk.ChunkMs,
    }

    module, err := vosk.New(voskCfg)
    if err != nil {
        return nil, fmt.Errorf("vosk init error: %w", err)
    }

    log.Printf("[%s] ✅ VOSK инициализирован", sessionID)

    return &VoskProcessor{
        module: module,
        cfg:    cfg,
        id:     sessionID,
    }, nil
}

func (v *VoskProcessor) WriteAudio(pcm []byte) error {
    v.mu.Lock()
    defer v.mu.Unlock()

    log.Printf("[%s] 📥 Запись аудио: %d байт", v.id, len(pcm))

    err := v.module.WriteAudio(pcm)
    if err != nil {
        log.Printf("[%s] ❌ Ошибка записи аудио: %v", v.id, err)
        return err
    }

    // Принудительно вызываем декодирование после каждого чанка
    // (в vosk-go это должно делаться автоматически, но на всякий случай)

    return nil
}

func (v *VoskProcessor) GetResult() (vosk.Result, error) {
    v.mu.Lock()
    defer v.mu.Unlock()

    result, err := v.module.GetResult()
    if err != nil {
        log.Printf("[%s] ❌ Ошибка получения результата: %v", v.id, err)
        return vosk.Result{}, err
    }

    if result.Text != "" {
        log.Printf("[%s] 📝 Результат VOSK: '%s' (финал: %v)", v.id, result.Text, result.IsFinal)
    }

    return result, nil
}

func (v *VoskProcessor) Close() {
    log.Printf("[%s] 🔚 Закрытие VOSK", v.id)
    v.module.Close()
}
