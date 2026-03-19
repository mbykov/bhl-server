package lib

import (
    "encoding/binary"
    "encoding/json"
    "log"
    "math"
    "net/http"
    "sync"
    "time"

    "github.com/gorilla/websocket"
)

// Конвертация байт в float32 (для аудио)
func bytesToFloat32Slice(b []byte) []float32 {
    if len(b)%4 != 0 {
        return nil
    }
    samples := make([]float32, len(b)/4)
    for i := 0; i < len(b); i += 4 {
        bits := binary.LittleEndian.Uint32(b[i : i+4])
        samples[i/4] = math.Float32frombits(bits)
    }
    return samples
}

// Конвертация float32 в int16 PCM
func float32ToInt16PCM(samples []float32) []byte {
    pcm := make([]byte, len(samples)*2)
    for i, sample := range samples {
        val := int16(sample * 32767)
        pcm[i*2] = byte(val)
        pcm[i*2+1] = byte(val >> 8)
    }
    return pcm
}

type WSHandler struct {
    upgrader websocket.Upgrader
    sessions sync.Map
    cfg      *Config
    models   *Models
}

func NewWSHandler(cfg *Config, models *Models) *WSHandler {
    log.Println("🔧 Создание WebSocket обработчика")
    return &WSHandler{
        upgrader: websocket.Upgrader{
            CheckOrigin: func(r *http.Request) bool {
                return true // В продакшене нужно ограничить
            },
            ReadBufferSize:  64 * 1024,  // 64KB вместо 1MB
            WriteBufferSize: 64 * 1024,  // 64KB вместо 1MB
        },
        cfg:    cfg,
        models: models,
    }
}

func (h *WSHandler) Handle(w http.ResponseWriter, r *http.Request) {
    sessionID := r.RemoteAddr
    log.Printf("[%s] 📡 Новое WebSocket соединение", sessionID)

    // Upgrade HTTP to WebSocket
    conn, err := h.upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("[%s] ❌ WebSocket upgrade error: %v", sessionID, err)
        return
    }

    // Канал для отправки сообщений клиенту с буфером
    sendChan := make(chan []byte, 100)

    // Создаем сессию
    sess, err := NewSession(sessionID, sendChan, h.cfg, h.models)
    if err != nil {
        log.Printf("[%s] ❌ Failed to create session: %v", sessionID, err)
        conn.Close()
        return
    }

    // Сохраняем сессию
    h.sessions.Store(sess.id, sess)

    // Гарантированно удаляем сессию и закрываем соединение при выходе
    defer func() {
        h.sessions.Delete(sess.id)
        sess.Close()
        conn.Close()
        log.Printf("[%s] 👋 Соединение закрыто", sessionID)
    }()

    // Запускаем горутину для отправки сообщений клиенту
    go func() {
        for {
            select {
            case msg, ok := <-sendChan:
                if !ok {
                    // Канал закрыт - завершаем горутину
                    return
                }

                // Устанавливаем таймаут на запись
                if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
                    log.Printf("[%s] ⚠️ Ошибка установки таймаута записи: %v", sess.id, err)
                    return
                }

                if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
                    log.Printf("[%s] ⚠️ Ошибка отправки сообщения: %v", sess.id, err)
                    return
                }

            case <-sess.done:
                // Сессия завершена - выходим
                return
            }
        }
    }()

    // Устанавливаем таймаут на чтение
    conn.SetReadLimit(32 * 1024 * 1024) // 32MB максимальный размер сообщения

    // Основной цикл чтения сообщений от клиента
    for {
        // Устанавливаем таймаут на чтение
        if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
            log.Printf("[%s] ⚠️ Ошибка установки таймаута чтения: %v", sess.id, err)
            break
        }

        _, message, err := conn.ReadMessage()
        if err != nil {
            // Нормальное закрытие соединения
            if websocket.IsCloseError(err, websocket.CloseNormalClosure,
                websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
                log.Printf("[%s] ℹ️ Соединение закрыто клиентом", sess.id)
            } else {
                log.Printf("[%s] ⚠️ Ошибка чтения: %v", sess.id, err)
            }
            break
        }

        // Пробуем распарсить как JSON для контрольных сообщений
        var msg map[string]interface{}
        if err := json.Unmarshal(message, &msg); err == nil {
            // Это JSON - возможно контрольное сообщение
            if msgType, ok := msg["type"].(string); ok {
                // Проверяем, что это не аудио, а управляющее сообщение
                switch msgType {
                case "get_metrics", "get_config", "get_stats", "ping", "reset_vosk", "gc":
                    sess.HandleControlMessage(msg)
                    continue // пропускаем обработку как аудио
                }
            }
        }

        // Если не контрольное сообщение - обрабатываем как аудио
        floatSamples := bytesToFloat32Slice(message)
        if len(floatSamples) > 0 {
            // Ограничиваем размер аудио пакета для безопасности
            if len(floatSamples) > 16000*10 { // больше 10 секунд
                log.Printf("[%s] ⚠️ Слишком большой аудио пакет: %d сэмплов",
                    sess.id, len(floatSamples))
                continue
            }

            pcmData := float32ToInt16PCM(floatSamples)
            sess.HandleAudio(pcmData)
        } else {
            log.Printf("[%s] ⚠️ Получено не аудио и не JSON сообщение", sess.id)
        }
    }
}

// GetStats возвращает статистику по всем сессиям
func (h *WSHandler) GetStats() map[string]interface{} {
    stats := map[string]interface{}{
        "active_sessions": 0,
        "sessions":        []string{},
    }

    h.sessions.Range(func(key, value interface{}) bool {
        stats["active_sessions"] = stats["active_sessions"].(int) + 1
        stats["sessions"] = append(stats["sessions"].([]string), key.(string))
        return true
    })

    return stats
}

// CloseAllSessions закрывает все активные сессии
func (h *WSHandler) CloseAllSessions() {
    log.Println("🔌 Закрытие всех активных сессий...")

    var wg sync.WaitGroup
    h.sessions.Range(func(key, value interface{}) bool {
        wg.Add(1)
        go func(sess *Session) {
            defer wg.Done()
            sess.Close()
        }(value.(*Session))
        return true
    })

    // Ждем завершения всех сессий
    wg.Wait()

    // Очищаем map
    h.sessions.Range(func(key, value interface{}) bool {
        h.sessions.Delete(key)
        return true
    })

    log.Printf("✅ Все сессии закрыты")
}
