package lib

import (
    "context"
    "encoding/binary"   // <-- добавлено
    "encoding/json"
    "fmt"
    "log/slog"
    "math"
    "strings"
    "sync"
    "time"

    "github.com/gorilla/websocket"
    sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
    "github.com/mbykov/bhl-command-go"
)

// GigaJob – задание для GigaAM.
type GigaJob struct {
	Audio   []float32
	Segment int
}

// HybridSession – одна WebSocket-сессия.
type HybridSession struct {
	ID     string
	Conn   *websocket.Conn
	Logger *slog.Logger

	// Vosk (online)
	VoskRecognizer *sherpa.OnlineRecognizer
	VoskStream     *sherpa.OnlineStream

	// GigaAM (offline) – общий для всех сессий
	GigaRecognizer *sherpa.OfflineRecognizer

	// Поиск команд – глобальный объект
	CommandEngine *command.SearchEngine
	CommandThr    float32

	// Очередь заданий для GigaAM
	JobChan chan GigaJob

	// Буфер аудио текущей фразы
	AudioBuffer []float32
	Segment     int
	LastText    string

	// Управление горутинами
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Исходящие сообщения (writePump)
	outgoing chan []byte

	mu sync.RWMutex // защищает AudioBuffer, Segment, LastText, VoskStream
}

// NewHybridSession создаёт новую сессию.
func NewHybridSession(
	conn *websocket.Conn,
	voskRec *sherpa.OnlineRecognizer,
	voskStream *sherpa.OnlineStream,
	gigaRec *sherpa.OfflineRecognizer,
	cmdEngine *command.SearchEngine,
	cmdThr float32,
) *HybridSession {
	ctx, cancel := context.WithCancel(context.Background())
	sess := &HybridSession{
		ID:             fmt.Sprintf("%s-%d", conn.RemoteAddr().String(), time.Now().UnixNano()),
		Conn:           conn,
		Logger:         slog.Default().With("session", conn.RemoteAddr().String()),
		VoskRecognizer: voskRec,
		VoskStream:     voskStream,
		GigaRecognizer: gigaRec,
		CommandEngine:  cmdEngine,
		CommandThr:     cmdThr,
		JobChan:        make(chan GigaJob, 20),
		AudioBuffer:    make([]float32, 0, 16000*10),
		Segment:        0,
		outgoing:       make(chan []byte, 100),
		ctx:            ctx,
		cancel:         cancel,
	}

	// Запуск воркеров
	sess.wg.Add(2)
	go sess.gigaWorker()
	go sess.writePump()

	sess.Logger.Info("session created")
	return sess
}

// gigaWorker – обрабатывает задания из JobChan.
func (s *HybridSession) gigaWorker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			s.Logger.Debug("gigaWorker stopped by context")
			return
		case job, ok := <-s.JobChan:
			if !ok {
				s.Logger.Debug("gigaWorker stopped, channel closed")
				return
			}
			s.processGigaJob(job)
		}
	}
}

// processGigaJob – выполнить одно задание GigaAM.
func (s *HybridSession) processGigaJob(job GigaJob) {
	// Проверка тишины
	rms := rmsEnergy(job.Audio)
	if rms < 0.002 {
		s.Logger.Debug("skip silent audio", "segment", job.Segment, "rms", rms)
		return
	}

	// Создаём OfflineStream (без пула для простоты)
	stream := sherpa.NewOfflineStream(s.GigaRecognizer)
	if stream == nil {
		s.Logger.Error("failed to create OfflineStream")
		return
	}
	defer sherpa.DeleteOfflineStream(stream)

	// Защита от паники в C++ коде
	defer func() {
		if r := recover(); r != nil {
			s.Logger.Error("panic in GigaAM", "recover", r)
		}
	}()

	stream.AcceptWaveform(16000, job.Audio)
	s.GigaRecognizer.Decode(stream)
	res := stream.GetResult()
	if res.Text == "" {
		s.Logger.Debug("GigaAM returned empty text", "segment", job.Segment)
		return
	}

	text := strings.TrimSpace(res.Text)
	s.Logger.Info("GigaAM final", "segment", job.Segment, "text", text)

	// Отправляем результат клиенту
	s.SendResponse(map[string]interface{}{
		"text":    text,
		"segment": job.Segment,
		"type":    "final",
	})
}

// writePump – отправляет сообщения клиенту.
func (s *HybridSession) writePump() {
    defer s.wg.Done()
    for {
        select {
        case <-s.ctx.Done():
            s.Logger.Debug("writePump stopped by context")
            return
        case msg, ok := <-s.outgoing:
            if !ok {
                s.Logger.Debug("writePump stopped, channel closed")
                return
            }
            s.Logger.Debug("writePump: writing message", "data", string(msg))
            if err := s.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
                s.Logger.Error("write error", "error", err)
                s.cancel()
                return
            }
            s.Logger.Debug("writePump: message sent successfully")
        }
    }
}


// SendResponse ставит JSON-сообщение в очередь отправки.
func (s *HybridSession) SendResponse(data interface{}) {
    jsonData, err := json.Marshal(data)
    if err != nil {
        s.Logger.Error("failed to marshal response", "error", err)
        return
    }
    s.Logger.Debug("SendResponse: queueing message", "data", string(jsonData))
    select {
    case s.outgoing <- jsonData:
        s.Logger.Debug("SendResponse: message queued successfully")
    case <-s.ctx.Done():
        s.Logger.Debug("SendResponse: context done, message dropped")
    default:
        s.Logger.Warn("outgoing channel full, message dropped")
    }
}

// Close освобождает ресурсы сессии.
func (s *HybridSession) Close() {
	s.Logger.Info("closing session")
	s.cancel()                  // сигнал воркерам
	close(s.JobChan)            // завершаем gigaWorker
	close(s.outgoing)           // завершаем writePump
	s.wg.Wait()                 // ждём завершения горутин

	// Удаляем C++ объекты Vosk
	sherpa.DeleteOnlineStream(s.VoskStream)
	sherpa.DeleteOnlineRecognizer(s.VoskRecognizer)
	// GigaRecognizer общий – не удаляем здесь
	s.Logger.Info("session closed")
}

// --- Обработка аудиоданных ---

// AcceptAudio принимает очередной фрагмент аудио.
func (s *HybridSession) AcceptAudio(samples []float32) {
	s.mu.Lock()
	// Буферизация
	s.AudioBuffer = append(s.AudioBuffer, samples...)

	// Передаём в Vosk
	s.VoskStream.AcceptWaveform(16000, samples)
	for s.VoskRecognizer.IsReady(s.VoskStream) {
		s.VoskRecognizer.Decode(s.VoskStream)
	}

	res := s.VoskRecognizer.GetResult(s.VoskStream)
	text := strings.TrimSpace(res.Text)
	isEndpoint := s.VoskRecognizer.IsEndpoint(s.VoskStream)

	// Отправляем промежуточный результат, если текст изменился
	if text != "" && text != s.LastText {
		s.LastText = text
		s.mu.Unlock()
		s.SendResponse(map[string]interface{}{
			"text":    text,
			"segment": s.Segment,
			"type":    "intermediate",
		})
	} else {
		s.mu.Unlock()
	}

	// Обработка endpoint – делаем после отпускания мьютекса, чтобы не блокировать
	if isEndpoint {
		s.handleEndpoint(text)
	}
}

// handleEndpoint вызывается при обнаружении конца фразы.
func (s *HybridSession) handleEndpoint(voskText string) {
	s.mu.Lock()
	// Если фраза пустая или слишком короткая – просто сброс
	if len(s.AudioBuffer) < 3200 || voskText == "" {
		// s.Logger.Debug("endpoint ignored (empty or too short)")
		s.AudioBuffer = s.AudioBuffer[:0]
		s.VoskRecognizer.Reset(s.VoskStream)
		s.LastText = ""
		s.mu.Unlock()
		return
	}

	// Копируем аудиобуфер (владеем копией, можно отдать воркеру)
	phraseAudio := make([]float32, len(s.AudioBuffer))
	copy(phraseAudio, s.AudioBuffer)
	segment := s.Segment
	s.Segment++
	s.AudioBuffer = s.AudioBuffer[:0]
	s.VoskRecognizer.Reset(s.VoskStream)
	s.LastText = ""
	s.mu.Unlock()

	s.Logger.Info("endpoint detected",
		"segment", segment,
		"text", voskText,
		"audio_samples", len(phraseAudio))

	// Поиск команды
	if s.CommandEngine != nil {
		if cmd, err := s.CommandEngine.FindCommand(voskText); err != nil {
			s.Logger.Error("command search error", "error", err)
		} else if cmd != nil && cmd.Score >= s.CommandThr {
			s.Logger.Info("command found",
				"name", cmd.Name,
				"score", cmd.Score,
				"segment", segment)
			// Отправляем команду клиенту
			s.SendResponse(map[string]interface{}{
				"name":     cmd.Name,
				"synonyms": cmd.Synonyms,
				"external": cmd.External,
				"score":    cmd.Score,
				"segment":  segment,
				"type":     "command",
                "text":     cmd.Name, // "⚡ " + cmd.Name, // временно для отладки
			})
			s.Logger.Info("AFTER command send",
				"name", cmd.Name,
				"score", cmd.Score,
				"segment", segment)
			// Команда обработана, GigaAM не вызываем
			return
		}
	}

	// Если команда не найдена – отправляем в GigaAM
	select {
	case s.JobChan <- GigaJob{Audio: phraseAudio, Segment: segment}:
		s.Logger.Debug("queued to GigaAM", "segment", segment)
	case <-s.ctx.Done():
		s.Logger.Debug("session closed, dropping job", "segment", segment)
	default:
		// Очередь переполнена – блокируем, пока не освободится место (или контекст не отменят)
		select {
		case s.JobChan <- GigaJob{Audio: phraseAudio, Segment: segment}:
			s.Logger.Debug("queued to GigaAM after blocking", "segment", segment)
		case <-s.ctx.Done():
			s.Logger.Debug("session closed while waiting for queue", "segment", segment)
		}
	}
}

// --- Утилиты ---

// bytesToFloat32Slice конвертирует сырые PCM float32 байты.
func bytesToFloat32Slice(data []byte) []float32 {
    if len(data)%4 != 0 {
        data = data[:len(data)-len(data)%4]
    }
    numFloats := len(data) / 4
    floats := make([]float32, numFloats)
    for i := 0; i < numFloats; i++ {
        bits := binary.LittleEndian.Uint32(data[i*4 : i*4+4])
        floats[i] = math.Float32frombits(bits)
    }
    return floats
}

// rmsEnergy вычисляет среднеквадратичную энергию сигнала.
func rmsEnergy(samples []float32) float64 {
	var sum float32
	for _, v := range samples {
		sum += v * v
	}
	return math.Sqrt(float64(sum / float32(len(samples))))
}

// --- SessionManager ---

type SessionManager struct {
	config         *Config
	gigaRecognizer *sherpa.OfflineRecognizer
	commandEngine  *command.SearchEngine
	commandThr     float32
	sessions       map[string]*HybridSession
	mu             sync.RWMutex
	logger         *slog.Logger
}

func NewSessionManager(
	config *Config,
	gigaRec *sherpa.OfflineRecognizer,
	cmdEngine *command.SearchEngine,
	cmdThr float32,
) *SessionManager {
	return &SessionManager{
		config:         config,
		gigaRecognizer: gigaRec,
		commandEngine:  cmdEngine,
		commandThr:     cmdThr,
		sessions:       make(map[string]*HybridSession),
		logger:         slog.Default().With("module", "session_manager"),
	}
}

// CreateSession создаёт новую сессию и инициализирует Vosk.
func (sm *SessionManager) CreateSession(conn *websocket.Conn) (*HybridSession, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Создаём Vosk распознаватель для этой сессии
	vConfig := sherpa.OnlineRecognizerConfig{}
	vConfig.FeatConfig.SampleRate = 16000
	vConfig.FeatConfig.FeatureDim = 80
	vConfig.ModelConfig.Tokens = sm.config.Models.Vosk.Tokens
	vConfig.ModelConfig.NumThreads = sm.config.Decoding.NumThreads
	vConfig.ModelConfig.Debug = 0
	vConfig.ModelConfig.Provider = sm.config.Decoding.Provider
	if sm.config.Models.Vosk.ModelType != "" {
		vConfig.ModelConfig.ModelType = sm.config.Models.Vosk.ModelType
	}
	vConfig.ModelConfig.Transducer.Encoder = sm.config.Models.Vosk.Encoder
	vConfig.ModelConfig.Transducer.Decoder = sm.config.Models.Vosk.Decoder
	vConfig.ModelConfig.Transducer.Joiner = sm.config.Models.Vosk.Joiner
	vConfig.DecodingMethod = sm.config.Decoding.DecodingMethod
	vConfig.MaxActivePaths = sm.config.Decoding.MaxActivePaths
	if sm.config.Endpoint.Enable {
		vConfig.EnableEndpoint = 1
	} else {
		vConfig.EnableEndpoint = 0
	}
	vConfig.Rule1MinTrailingSilence = sm.config.Endpoint.Rule1MinTrailingSilence
	vConfig.Rule2MinTrailingSilence = sm.config.Endpoint.Rule2MinTrailingSilence
	vConfig.Rule3MinUtteranceLength = sm.config.Endpoint.Rule3MinUtteranceLength

	voskRec := sherpa.NewOnlineRecognizer(&vConfig)
	if voskRec == nil {
		return nil, fmt.Errorf("failed to create Vosk recognizer")
	}
	voskStream := sherpa.NewOnlineStream(voskRec)

	sess := NewHybridSession(
		conn,
		voskRec,
		voskStream,
		sm.gigaRecognizer,
		sm.commandEngine,
		sm.commandThr,
	)

	sm.sessions[sess.ID] = sess
	sm.logger.Info("session created", "id", sess.ID, "active", len(sm.sessions))
	return sess, nil
}

// RemoveSession удаляет сессию из менеджера и закрывает её.
func (sm *SessionManager) RemoveSession(sess *HybridSession) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if _, ok := sm.sessions[sess.ID]; ok {
		delete(sm.sessions, sess.ID)
		sess.Close()
		sm.logger.Info("session removed", "id", sess.ID, "active", len(sm.sessions))
	}
}

// Shutdown закрывает все активные сессии.
func (sm *SessionManager) Shutdown() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.logger.Info("shutting down all sessions", "count", len(sm.sessions))
	for _, sess := range sm.sessions {
		sess.Close()
	}
	sm.sessions = make(map[string]*HybridSession)
}
