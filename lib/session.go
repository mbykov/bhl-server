package lib

import (
    "encoding/json"
    "log"
    "strings"
    "time"

    "github.com/mbykov/bhl-command-go"
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
    audioForPhrase := s.currentAudio
    s.currentAudio = make([]byte, 0) // начинаем новую фразу

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

    // TODO: отправить audioForPhrase в GigaAM для пунктуации
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
    if err != nil {
        log.Printf("[%s] ❌ Ошибка GigaAM: %v", s.id, err)
        s.sendToBrowser("final", text)
        return
    }

    s.sendToBrowser("final", result.Text)
    log.Printf("[%s] ✅ Отправлен результат GigaAM: %q", s.id, result.Text)
}

func (s *Session) findCommand(text string) *command.CommandMapping {
    if s.models.Command == nil || !s.cfg.Command.Enabled {
        return nil
    }

    words := strings.Fields(text)
    if len(words) < s.cfg.Command.MinWords {
        return nil
    }

    cmd, err := s.models.Command.FindCommand(text)
    if err != nil {
        log.Printf("[%s] ⚠️ Ошибка поиска команды: %v", s.id, err)
        return nil
    }

    return cmd
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

func (s *Session) sendCommand(cmd *command.CommandMapping, text string) {
    msg := map[string]interface{}{
        "type": "command",
        "text": text,
        "name": cmd.Name,
    }
    if cmd.Score > 0 {
        msg["score"] = cmd.Score
    }

    data, _ := json.Marshal(msg)
    s.sendChan <- data
}

func (s *Session) Close() {
    log.Printf("[%s] 🔚 Закрытие сессии", s.id)
    s.vosk.Close()
}
