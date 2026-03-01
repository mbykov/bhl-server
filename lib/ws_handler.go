package lib

import (
    "encoding/binary"
    "log"
    "math"
    "net/http"
    "sync"

    "github.com/gorilla/websocket"
)

type WSHandler struct {
    upgrader websocket.Upgrader
    sessions sync.Map
    cfg      *Config
}

func NewWSHandler(cfg *Config) *WSHandler {
    log.Println("🔧 Создание WebSocket обработчика")
    return &WSHandler{
        upgrader: websocket.Upgrader{
            CheckOrigin: func(r *http.Request) bool { return true },
            ReadBufferSize:  1024 * 1024,
            WriteBufferSize: 1024 * 1024,
        },
        cfg: cfg,
    }
}

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

func float32ToInt16PCM(samples []float32) []byte {
    pcm := make([]byte, len(samples)*2)
    for i, sample := range samples {
        val := int16(sample * 32767)
        pcm[i*2] = byte(val)
        pcm[i*2+1] = byte(val >> 8)
    }
    return pcm
}

func (h *WSHandler) Handle(w http.ResponseWriter, r *http.Request) {
    sessionID := r.RemoteAddr
    log.Printf("📡 Новое WebSocket соединение от %s", sessionID)

    conn, err := h.upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("❌ WebSocket upgrade error: %v", err)
        return
    }
    defer conn.Close()

    sendChan := make(chan []byte, 100)

    sess, err := NewSession(sessionID, sendChan, h.cfg)
    if err != nil {
        log.Printf("[%s] ❌ Failed to create session: %v", sessionID, err)
        return
    }
    defer sess.Close()

    h.sessions.Store(sess.id, sess)
    defer h.sessions.Delete(sess.id)

    // Отправка результатов клиенту
    go func() {
        for msg := range sendChan {
            if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
                return
            }
        }
    }()

    // Приём аудио от клиента
    for {
        _, message, err := conn.ReadMessage()
        if err != nil {
            break
        }

        floatSamples := bytesToFloat32Slice(message)
        if len(floatSamples) > 0 {
            pcmData := float32ToInt16PCM(floatSamples)
            sess.HandleAudio(pcmData)
        }
    }
}
