package lib

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

type VoskSession struct {
	ID         string
	Conn       *websocket.Conn
	Recognizer *sherpa.OnlineRecognizer
	Stream     *sherpa.OnlineStream
	Segment    int
	LastText   string
	CreatedAt  time.Time
	mu         sync.RWMutex
}

type SessionManager struct {
	sessions map[string]*VoskSession
	mu       sync.RWMutex
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*VoskSession),
	}
}

func (sm *SessionManager) CreateSession(conn *websocket.Conn, config *Config) (*VoskSession, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Создание распознавателя Vosk
	recognizer := createVoskRecognizer(config)

	// Создание потока
	stream := sherpa.NewOnlineStream(recognizer)

	// Создание сессии
	sessionID := fmt.Sprintf("%s-%d", conn.RemoteAddr().String(), time.Now().UnixNano())
	session := &VoskSession{
		ID:         sessionID,
		Conn:       conn,
		Recognizer: recognizer,
		Stream:     stream,
		Segment:    0,
		CreatedAt:  time.Now(),
	}

	sm.sessions[sessionID] = session
	log.Printf("Created new session: %s", sessionID)

	return session, nil
}

func (sm *SessionManager) GetSession(sessionID string) *VoskSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.sessions[sessionID]
}

func (sm *SessionManager) RemoveSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, exists := sm.sessions[sessionID]; exists {
		// Освобождение ресурсов Vosk
		sherpa.DeleteOnlineStream(session.Stream)
		sherpa.DeleteOnlineRecognizer(session.Recognizer)

		delete(sm.sessions, sessionID)
		log.Printf("Removed session: %s", sessionID)
	}
}

func createVoskRecognizer(config *Config) *sherpa.OnlineRecognizer {
	// Создание конфигурации распознавателя на основе вашего кода
	recognizerConfig := sherpa.OnlineRecognizerConfig{}

	// Настройка параметров признаков
	recognizerConfig.FeatConfig.SampleRate = 16000
	recognizerConfig.FeatConfig.FeatureDim = 80

	// Настройка модели Vosk
	recognizerConfig.ModelConfig.Tokens = config.Models.Vosk.Tokens
	recognizerConfig.ModelConfig.NumThreads = config.Decoding.NumThreads
	recognizerConfig.ModelConfig.Debug = 0
	recognizerConfig.ModelConfig.Provider = config.Decoding.Provider

	if config.Models.Vosk.ModelType != "" {
		recognizerConfig.ModelConfig.ModelType = config.Models.Vosk.ModelType
	}

	// Transducer модель (zipformer)
	recognizerConfig.ModelConfig.Transducer.Encoder = config.Models.Vosk.Encoder
	recognizerConfig.ModelConfig.Transducer.Decoder = config.Models.Vosk.Decoder
	recognizerConfig.ModelConfig.Transducer.Joiner = config.Models.Vosk.Joiner

	// Настройка декодирования
	recognizerConfig.DecodingMethod = config.Decoding.DecodingMethod
	recognizerConfig.MaxActivePaths = config.Decoding.MaxActivePaths

	// Настройка endpoint detection
	if config.Endpoint.Enable {
		recognizerConfig.EnableEndpoint = 1
	} else {
		recognizerConfig.EnableEndpoint = 0
	}
	recognizerConfig.Rule1MinTrailingSilence = config.Endpoint.Rule1MinTrailingSilence
	recognizerConfig.Rule2MinTrailingSilence = config.Endpoint.Rule2MinTrailingSilence
	recognizerConfig.Rule3MinUtteranceLength = config.Endpoint.Rule3MinUtteranceLength

	log.Println("Creating Vosk recognizer...")
	recognizer := sherpa.NewOnlineRecognizer(&recognizerConfig)
	log.Println("Vosk recognizer created successfully!")

	return recognizer
}
