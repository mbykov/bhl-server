package lib

import (
    "encoding/json"
    "log"
    "strings"
    "time"
    // "fmt"
    // "os"

    "github.com/mbykov/command-go-levenshtein"
    "github.com/mbykov/bhl-gigaam-go"  // добавьте эту строку
)

type Session struct {
    id          string
    vosk        *VoskProcessor
    sendChan    chan<- []byte
    models      *Models
    cfg         *Config

    // Текущая фраза
    currentAudio []byte
    phraseStart  time.Time
}

func NewSession(id string, sendChan chan<- []byte, cfg *Config, models *Models) (*Session, error) {
    log.Printf("[%s] 🆕 Создание новой сессии", id)

    vosk, err := NewVoskProcessor(cfg, id)
    if err != nil {
        return nil, err
    }

    s := &Session{
        id:         id,
        vosk:       vosk,
        sendChan:   sendChan,
        models:     models,
        cfg:        cfg,
        currentAudio: make([]byte, 0),
    }

    log.Printf("[%s] ✅ Сессия инициализирована", id)
    return s, nil
}

func (s *Session) HandleAudio(pcm []byte) {
    // Если это начало новой фразы
    if len(s.currentAudio) == 0 {
        s.phraseStart = time.Now()
        log.Printf("[%s] 🎤 Начало новой фразы", s.id)
    }

    // Накапливаем аудио
    s.currentAudio = append(s.currentAudio, pcm...)

    // Отправляем в Vosk
    s.vosk.WriteAudio(pcm)

    // Проверяем результат
    s.checkVosk()
}

func (s *Session) checkVosk() {
    result, err := s.vosk.GetResult()
    if err != nil || result.Text == "" {
        return
    }

    text := strings.TrimSpace(result.Text)
    if text == "" {
        return
    }

    if !result.IsFinal {
        // ПРОМЕЖУТОЧНЫЙ - просто шлем в браузер
        s.sendToBrowser("interim", text)
        log.Printf("[%s] 🔄 Interim: %q", s.id, text)
        return
    }

    // ФИНАЛ! Обрабатываем результат
    log.Printf("[%s] 🎯 Final: %q", s.id, text)

    // Сохраняем аудио для этой фразы (позже отправим в Giga)
    // audioForPhrase := s.currentAudio
    // s.currentAudio = make([]byte, 0) // начинаем новую фразу
    audioForPhrase := make([]byte, len(s.currentAudio))
    copy(audioForPhrase, s.currentAudio)
    s.currentAudio = make([]byte, 0) // очищаем сразу

    audioForPhrase = trimAudioSilence(audioForPhrase, 16000)

    // Проверяем, не команда ли это
    cmd := s.findCommand(text)
    if cmd != nil {
        // Это команда
        s.sendCommand(cmd, text)
        log.Printf("[%s] ✅ Команда: %s", s.id, cmd.Name)
    } else {
        // Обычный текст
        // s.sendToBrowser("final", text)
        go s.processWithGigaAM(text, audioForPhrase)
        log.Printf("[%s] 📝 Текст: %s", s.id, text)
    }

    // Замеряем время обработки фразы
    log.Printf("[%s] ⏱️ Фраза обработана за %v (аудио: %d байт)",
        s.id, time.Since(s.phraseStart), len(audioForPhrase))
}

// Добавить метод:
func (s *Session) processWithGigaAM(text string, audio []byte) {
    if s.models.GigaAM == nil {
        // Если GigaAM не загружен, просто отправляем текст
        s.sendToBrowser("final", text)
        return
    }

    log.Printf("[%s] 🔄 Отправка в GigaAM (текст: %q, аудио: %d байт)",
        s.id, text, len(audio))

    // Используем ProcessAudio вместо ProcessText!
    result, err := s.models.GigaAM.ProcessAudio(audio)

    // ДИАГНОСТИКА: пересоздаём модуль после каждой фразы
    oldModule := s.models.GigaAM
    defer func() {
        // Создаём новый модуль для следующей фразы

        // newModule, err := gigaam.New(s.cfg.GigaAM)  // без второго параметра
        gigaamConfig := gigaam.Config{
            ModelPath:  s.cfg.GigaAM.ModelPath,
            SampleRate: s.cfg.GigaAM.SampleRate,
            FeatureDim: s.cfg.GigaAM.FeatureDim,
            NumThreads: s.cfg.GigaAM.NumThreads,
            Provider:   s.cfg.GigaAM.Provider,
            // Debug поле не нужно, если его нет в gigaam.Config
        }
        newModule, err := gigaam.New(gigaamConfig)

        if err != nil {
            log.Printf("[%s] ❌ Ошибка пересоздания GigaAM: %v", s.id, err)
            // Если не удалось создать новый, оставляем старый
            s.models.GigaAM = oldModule
        } else {
            // Заменяем модуль
            s.models.GigaAM = newModule
            // Закрываем старый в фоне, чтобы не блокировать
            go func(m *gigaam.GigaAMModule) {
                m.Close()
            }(oldModule)
            log.Printf("[%s] 🔄 GigaAM модуль пересоздан", s.id)
        }
    }()

    if err != nil {
        log.Printf("[%s] ❌ Ошибка GigaAM: %v", s.id, err)
        s.sendToBrowser("final", text)
        return
    }

    if strings.TrimSpace(result.Text) == "" {
        // Пустой результат - используем оригинал
        log.Printf("[%s] ⚠️ GigaAM вернул пустую строку", s.id)
        s.sendToBrowser("final", text)
        return
    }

    s.sendToBrowser("final", result.Text)
    log.Printf("[%s] ✅ Отправлен результат GigaAM: %q", s.id, result.Text)
}

func (s *Session) findCommand(text string) *command.CommandDefinition {
    if s.models.Command == nil || !s.cfg.Command.Enabled {
        return nil
    }

    words := strings.Fields(text)
    if len(words) < s.cfg.Command.MinWords {
        return nil
    }

    cmdName, external := s.models.Command.Resolve(text)
    _ = external // игнорируем, так как пока не используем

    if cmdName == "" {
        return nil
    }

    cmdDef := s.models.Command.GetCommand(cmdName)
    return cmdDef
}

func (s *Session) sendToBrowser(msgType, text string) {
    msg := map[string]string{
        "type": msgType,
        "text": text,
    }
    data, _ := json.Marshal(msg)

    select {
    case s.sendChan <- data:
    default:
        log.Printf("[%s] ⚠️ Канал отправки переполнен", s.id)
    }
}

func (s *Session) sendCommand(cmd *command.CommandDefinition, text string) {
    msg := map[string]interface{}{
        "type": "command",
        "text": text,
        "name": cmd.Name,
    }
    // Поле External есть и в CommandDefinition
    if cmd.External {
        msg["external"] = true
    }

    data, _ := json.Marshal(msg)
    s.sendChan <- data
}

func (s *Session) Close() {
    log.Printf("[%s] 🔚 Закрытие сессии", s.id)
    s.vosk.Close()
}

// trimAudioSilence обрезает тишину в начале и конце аудио
// pcm - сырые PCM данные (16-bit, mono)
// sampleRate - частота дискретизации
func trimAudioSilence(pcm []byte, sampleRate int) []byte {
    if len(pcm) < 640 { // меньше 20 мс
        return pcm
    }

    // Параметры для анализа
    frameSize := sampleRate / 100 // 10 мс
    threshold := 500 // порог тишины (подберите экспериментально)

    // Конвертируем в int16 для анализа
    samples := make([]int16, len(pcm)/2)
    for i := 0; i < len(pcm); i += 2 {
        samples[i/2] = int16(pcm[i]) | int16(pcm[i+1])<<8
    }

    // Ищем начало речи
    start := 0
    for i := 0; i < len(samples)-frameSize; i += frameSize {
        var maxAmp int16
        for j := 0; j < frameSize; j++ {
            amp := samples[i+j]
            if amp < 0 {
                amp = -amp
            }
            if amp > maxAmp {
                maxAmp = amp
            }
        }
        if maxAmp > int16(threshold) {
            start = i
            break
        }
    }

    // Ищем конец речи
    end := len(samples)
    for i := len(samples) - frameSize; i > start; i -= frameSize {
        var maxAmp int16
        for j := 0; j < frameSize; j++ {
            amp := samples[i+j]
            if amp < 0 {
                amp = -amp
            }
            if amp > maxAmp {
                maxAmp = amp
            }
        }
        if maxAmp > int16(threshold) {
            end = i + frameSize
            break
        }
    }

    // Конвертируем обратно в []byte
    trimmed := make([]byte, (end-start)*2)
    for i := start; i < end; i++ {
        trimmed[(i-start)*2] = byte(samples[i])
        trimmed[(i-start)*2+1] = byte(samples[i] >> 8)
    }
    return trimmed
}
