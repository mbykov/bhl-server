package lib

import (
    "fmt"
    "log"
    "sync"
    "time"

    "github.com/mbykov/bhl-vosk-sherpa-go/vosk"
)

type VoskProcessor struct {
    module     *vosk.ASRModule
    cfg        *Config
    mu         sync.Mutex
    id         string
    audioBytes int64
    lastLog    time.Time
}

func NewVoskProcessor(cfg *Config, sessionID string) (*VoskProcessor, error) {
    log.Printf("[%s] 🔧 Инициализация VOSK с моделью: %s", sessionID, cfg.Vosk.ModelPath)

    voskCfg := vosk.Config{
        ModelPath:  cfg.Vosk.ModelPath,
        SampleRate: cfg.Vosk.SampleRate,
        FeatureDim: cfg.Vosk.FeatureDim,
    }

    module, err := vosk.New(voskCfg)
    if err != nil {
        return nil, fmt.Errorf("vosk init error: %w", err)
    }

    log.Printf("[%s] ✅ VOSK инициализирован", sessionID)

    return &VoskProcessor{
        module:  module,
        cfg:     cfg,
        id:      sessionID,
        lastLog: time.Now(),
    }, nil
}

func (v *VoskProcessor) WriteAudio(pcm []byte) error {
    v.mu.Lock()
    defer v.mu.Unlock()

    v.audioBytes += int64(len(pcm))

    // Логировать каждые 10 секунд или 1 МБ
    if time.Since(v.lastLog) > 10*time.Second || v.audioBytes > 1024*1024 {
        // log.Printf("[%s] 📊 Vosk обработал всего: %.2f KB", v.id, float64(v.audioBytes)/1024)
        v.lastLog = time.Now()
        v.audioBytes = 0
    }

    return v.module.WriteAudio(pcm)
}

func (v *VoskProcessor) GetResult() (vosk.Result, error) {
    v.mu.Lock()
    defer v.mu.Unlock()

    result, err := v.module.GetResult()
    if err != nil {
        return vosk.Result{}, err
    }

    if result.Text != "" {
        log.Printf("[%s] 📝 VOSK_go %s: %q",
            v.id,
            map[bool]string{true: "final", false: "interim"}[result.IsFinal],
            result.Text)
    }

    return result, nil
}

// Удаляем метод Reset - он не нужен, используем флаг в сессии

func (v *VoskProcessor) Reset() error {
    v.mu.Lock()
    defer v.mu.Unlock()

    log.Printf("[%s] 🔄 Принудительный сброс Vosk", v.id)

    // Создаём новую сессию Vosk через существующий механизм
    // В зависимости от вашего модуля vosk-sherpa-go

    // Вариант 1: если есть метод Reset
    if resetter, ok := interface{}(v.module).(interface{ Reset() error }); ok {
        return resetter.Reset()
    }

    // Вариант 2: пересоздаём модуль
    voskCfg := vosk.Config{
        ModelPath:  v.cfg.Vosk.ModelPath,
        SampleRate: v.cfg.Vosk.SampleRate,
        FeatureDim: v.cfg.Vosk.FeatureDim,
    }

    newModule, err := vosk.New(voskCfg)
    if err != nil {
        return fmt.Errorf("не удалось пересоздать Vosk: %w", err)
    }

    // Закрываем старый
    v.module.Close()
    v.module = newModule

    return nil
}


func (v *VoskProcessor) Close() {
    log.Printf("[%s] 🔚 Закрытие VOSK", v.id)
    v.module.Close()
}
