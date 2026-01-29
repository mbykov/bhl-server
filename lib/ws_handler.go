package lib

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	rupunct "github.com/mbykov/rupunct-go"
	command "github.com/mbykov/command-go"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024 * 1024,
	WriteBufferSize: 1024 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		allowedOrigins := []string{
			"https://localhost:6006", "https://127.0.0.1:6006", "https://tma.local:6006",
			"http://localhost:6006", "http://127.0.0.1:6006", "http://tma.local:6006",
		}

		for _, allowed := range allowedOrigins {
			if origin == allowed {
				return true
			}
		}
		log.Printf("Allowing origin: %s", origin)
		return true
	},
}

type WebSocketHandler struct {
	config           *Config
	sessionManager   *SessionManager
	punctuator       rupunct.Punctuator
	commandResolver  *command.CommandResolver
	commandExecutor  *CommandExecutor
}

func NewWebSocketHandler(config *Config, sessionManager *SessionManager,
						 punctuator rupunct.Punctuator,
						 commandResolver *command.CommandResolver,
						 commandExecutor *CommandExecutor) *WebSocketHandler {
	return &WebSocketHandler{
		config:           config,
		sessionManager:   sessionManager,
		punctuator:       punctuator,
		commandResolver:  commandResolver,
		commandExecutor:  commandExecutor,
	}
}

func (h *WebSocketHandler) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Failed to upgrade connection: %v", err)
			return
		}

		go h.handleConnection(conn)
	}
}

func (h *WebSocketHandler) handleConnection(conn *websocket.Conn) {
	defer func() {
		conn.Close()
		log.Println("WebSocket connection closed")
	}()

	remoteAddr := conn.RemoteAddr().String()
	log.Printf("New WebSocket connection from %s", remoteAddr)

	session, err := h.sessionManager.CreateSession(conn, h.config)
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		return
	}
	defer h.sessionManager.RemoveSession(session.ID)

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error from %s: %v", remoteAddr, err)
			}
			break
		}

		switch messageType {
		case websocket.TextMessage:
			msg := strings.TrimSpace(string(message))
			h.handleTextMessage(session, remoteAddr, msg)
		case websocket.BinaryMessage:
			h.handleAudioData(session, remoteAddr, message)
		default:
			log.Printf("Unsupported message type from %s: %v", remoteAddr, messageType)
		}
	}
}

func (h *WebSocketHandler) handleTextMessage(session *VoskSession, remoteAddr, msg string) {
	if msg == "Done" {
		log.Printf("Received 'Done' signal from %s", remoteAddr)

		// Просто отправляем финальный ответ с "Done"
		resp := map[string]interface{}{
			"text":    "Done",
			"segment": session.Segment,
			"type":    "final",
		}

		h.sendResponse(session.Conn, resp)
	}
}

func (h *WebSocketHandler) handleAudioData(session *VoskSession, remoteAddr string, data []byte) {
	session.mu.Lock()
	defer session.mu.Unlock()

	// Преобразование байтов в float32 сэмплы
	samples := bytesToFloat32Slice(data)
	if len(samples) == 0 {
		return
	}

	// Отправляем сэмплы в распознаватель
	session.Stream.AcceptWaveform(16000, samples)

	// Декодирование если готово
	for session.Recognizer.IsReady(session.Stream) {
		session.Recognizer.Decode(session.Stream)
	}

	// Получение промежуточного результата
	result := session.Recognizer.GetResult(session.Stream)
	text := result.Text

	// Если текст пустой - пропускаем
	if text == "" {
		return
	}

	// Очищаем текст от лишних пробелов
	text = strings.TrimSpace(text)

	// Отправка промежуточного результата в браузер (только если текст изменился)
	if text != "" && session.LastText != text {
		session.LastText = text

		resp := map[string]interface{}{
			"text":    text,
			"segment": session.Segment,
			"type":    "intermediate",
		}

		h.sendResponse(session.Conn, resp)
		log.Printf("Intermediate result from %s: '%s'", remoteAddr, text)
	}

	// Проверка endpoint
	if session.Recognizer.IsEndpoint(session.Stream) {
		log.Printf("=== ENDPOINT DETECTED ===")
		log.Printf("Session: %s, Segment: %d", session.ID, session.Segment)
		log.Printf("Text before processing: '%s'", text)

		// Формируем ответ
		var resp map[string]interface{}

		// 1. Сначала проверяем на наличие команды (равна строке)
		var commandName string
		var commandResult string
		var hasCommand bool

		if h.commandResolver != nil {
			log.Printf("Checking for command in: '%s'", text)

			cmd, external := h.commandResolver.Resolve(text)
			if cmd != "" {
				hasCommand = true
				commandName = cmd
				log.Printf("✓ COMMAND DETECTED: %s (external: %v)", commandName, external)

				// Выполняем команду
				if h.commandExecutor != nil {
					resultText, _, err := h.commandExecutor.Execute(commandName, text)
					if err != nil {
						log.Printf("Command execution error: %v", err)
						commandResult = "Ошибка выполнения команды"
					} else {
						commandResult = resultText
						log.Printf("Command result: %s", commandResult)
					}
				} else {
					log.Printf("WARNING: CommandExecutor is nil")
				}
			} else {
				log.Printf("✗ NO COMMAND FOUND for: '%s'", text)
			}
		} else {
			log.Printf("WARNING: CommandResolver is nil")
		}

		if hasCommand {
			// Ответ с командой
			resp = map[string]interface{}{
				"text":     text, // Оригинальный текст (без пунктуации)
				"segment":  session.Segment,
				"type":     "final",
				"command":  commandName,
			}

			if commandResult != "" {
				resp["result"] = commandResult
			}

			log.Printf("Sending command response: %v", resp)
		} else {
			// Ответ с пунктуацией (только если команда не найдена)
			finalText := text
			if h.punctuator != nil {
				punctuated, err := h.punctuator.Predict(text)
				if err != nil {
					log.Printf("Punctuation error: %v", err)
				} else {
					finalText = punctuated
					log.Printf("Punctuated: '%s' → '%s'", text, finalText)
				}
			}

			resp = map[string]interface{}{
				"text":    finalText,
				"segment": session.Segment,
				"type":    "final",
			}

			log.Printf("Sending punctuation response: %v", resp)
		}

		h.sendResponse(session.Conn, resp)

		// Увеличиваем счетчик сегментов
		session.Segment++

		// Сброс для следующей фразы
		session.Recognizer.Reset(session.Stream)
		session.LastText = ""

		log.Printf("=== ENDPOINT PROCESSING COMPLETE ===\n")
	}
}

func bytesToFloat32Slice(data []byte) []float32 {
	if len(data) == 0 {
		return []float32{}
	}

	if len(data)%4 != 0 {
		log.Printf("Warning: Data length %d not divisible by 4, truncating", len(data))
		data = data[:len(data)-len(data)%4]
	}

	numFloats := len(data) / 4
	floats := make([]float32, numFloats)

	for i := 0; i < numFloats; i++ {
		start := i * 4
		bits := binary.LittleEndian.Uint32(data[start:start+4])
		floats[i] = math.Float32frombits(bits)
	}

	return floats
}

func (h *WebSocketHandler) sendResponse(conn *websocket.Conn, data interface{}) {
	jsonResp, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshaling response: %v", err)
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, jsonResp); err != nil {
		log.Printf("Error sending response: %v", err)
	}
}
