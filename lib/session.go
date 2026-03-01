package lib

import (
    "encoding/json"
    "log"
    "sync"
    "time"
)

const (
    finalTimeout = 800 * time.Millisecond // ждём 0.8 секунды после последнего промежутка
)

type Session struct {
    id              string
    vosk            *VoskProcessor
    sendChan        chan<- []byte
    stopChan        chan struct{}
    mu              sync.Mutex

    lastInterimText string
    lastFinalText   string
    lastInterimTime time.Time
    pendingFinal    string // текст, который ждёт подтверждения
}

func NewSession(id string, sendChan chan<- []byte, cfg *Config) (*Session, error) {
    log.Printf("[%s] 🆕 Создание новой сессии", id)

    vosk, err := NewVoskProcessor(cfg, id)
    if err != nil {
        return nil, err
    }

    s := &Session{
        id:              id,
        vosk:            vosk,
        sendChan:        sendChan,
        stopChan:        make(chan struct{}),
        lastInterimTime: time.Now(),
    }

    go s.processResults()
    return s, nil
}

func (s *Session) HandleAudio(pcm []byte) {
    s.vosk.WriteAudio(pcm)
}

func (s *Session) processResults() {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-s.stopChan:
            return
        case <-ticker.C:
            s.mu.Lock()

            result, err := s.vosk.GetResult()
            now := time.Now()

            if err == nil && result.Text != "" {
                // Есть новый текст от VOSK
                if result.IsFinal {
                    // ФИНАЛ от VOSK
                    if result.Text != s.lastFinalText {
                        s.lastFinalText = result.Text
                        s.pendingFinal = ""
                        log.Printf("[%s] ✅ %s", s.id, result.Text)
                        s.sendMessage("final", result.Text)
                    }
                } else {
                    // ПРОМЕЖУТОЧНЫЙ - обновляем время и текст
                    if result.Text != s.lastInterimText {
                        s.lastInterimText = result.Text
                        s.lastInterimTime = now
                        s.pendingFinal = result.Text
                        log.Printf("[%s] 🔄 %s", s.id, result.Text)
                        s.sendMessage("intermediate", result.Text)
                    }
                }
            }

            // Проверяем, не пора ли отправить принудительный финал
            if s.pendingFinal != "" && now.Sub(s.lastInterimTime) > finalTimeout {
                if s.pendingFinal != s.lastFinalText {
                    s.lastFinalText = s.pendingFinal
                    log.Printf("[%s] ⏱️ %s", s.id, s.pendingFinal)
                    s.sendMessage("final", s.pendingFinal)
                }
                s.pendingFinal = ""
            }

            s.mu.Unlock()
        }
    }
}

func (s *Session) sendMessage(msgType, text string) {
    msg := map[string]string{
        "type": msgType,
        "text": text,
    }
    data, _ := json.Marshal(msg)

    select {
    case s.sendChan <- data:
    default:
    }
}

func (s *Session) Close() {
    close(s.stopChan)
    s.vosk.Close()
}
