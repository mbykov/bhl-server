package wshandler

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"log/slog"

	"github.com/gorilla/websocket"
	"github.com/mbykov/asr-zipformer-go"
	"github.com/mbykov/vosk-punct"
)

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

type WSHandler struct {
	upgrader    websocket.Upgrader
	asrCfg      asr.Config
	punctuator  *voskpunct.Punctuator
}

func NewWSHandler(cfg asr.Config, p *voskpunct.Punctuator) *WSHandler {
	return &WSHandler{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		asrCfg:     cfg,
		punctuator: p,
	}
}

func (h *WSHandler) Handle(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	engine, err := asr.New(h.asrCfg)
	if err != nil {
		logger.Error("ASR Init failed", "err", err)
		return
	}
	defer engine.Close()

	logger.Info("New session", "remote", r.RemoteAddr)

	for {
		mt, message, err := conn.ReadMessage()
		if err != nil {
			final := engine.Finish()
			if final.Text != "" {
				// Применяем пунктуацию к финальному тексту
				text := h.processText(final.Text)
				sendJSON(conn, asr.Response{Type: final.Type, Text: text})
			}
			logger.Info("Session closed", "remote", r.RemoteAddr)
			break
		}

		if mt == websocket.BinaryMessage {
			pcm := bytesToFloat32Slice(message)
			logger.Debug("Received audio", "bytes", len(message), "samples", len(pcm))

			resp := engine.Write(pcm)

			if resp.Text != "" {
				// Применяем пунктуацию к тексту
				text := h.processText(resp.Text)
				logger.Info("ASR Result", "type", resp.Type, "text", text)
				sendJSON(conn, asr.Response{Type: resp.Type, Text: text})
			}
		} else if mt == websocket.TextMessage {
			logger.Info("Control message", "msg", string(message))
		}
	}
}

// processText применяет пунктуацию, если доступен пунктуатор
func (h *WSHandler) processText(text string) string {
	if h.punctuator == nil {
		return text
	}
	return h.punctuator.Process(text)
}

func sendJSON(conn *websocket.Conn, data interface{}) {
	msg, _ := json.Marshal(data)
	conn.WriteMessage(websocket.TextMessage, msg)
}

func bytesToFloat32Slice(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	samples := make([]float32, len(b)/4)
	for i := 0; i < len(b); i += 4 {
		bits := binary.LittleEndian.Uint32(b[i:i+4])
		samples[i/4] = math.Float32frombits(bits)
	}
	return samples
}
