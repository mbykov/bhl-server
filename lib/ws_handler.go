package lib

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024 * 1024,
	WriteBufferSize: 1024 * 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type WebSocketHandler struct {
	sessionManager *SessionManager
	logger         *slog.Logger
}

func NewWebSocketHandler(sm *SessionManager) *WebSocketHandler {
	return &WebSocketHandler{
		sessionManager: sm,
		logger:         slog.Default().With("module", "ws_handler"),
	}
}

func (h *WebSocketHandler) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			h.logger.Error("upgrade failed", "error", err)
			return
		}
		go h.handleConnection(conn)
	}
}

func (h *WebSocketHandler) handleConnection(conn *websocket.Conn) {
	session, err := h.sessionManager.CreateSession(conn)
	if err != nil {
		h.logger.Error("create session failed", "error", err)
		conn.Close()
		return
	}
	defer h.sessionManager.RemoveSession(session)

	for {
		mType, msg, err := conn.ReadMessage()
		if err != nil {
			// Нормальное завершение
			break
		}
		if mType == websocket.BinaryMessage {
			samples := bytesToFloat32Slice(msg)
			if len(samples) > 0 {
				session.AcceptAudio(samples)
			}
		}
	}
}
