package asr

import (
	"fmt"
	"sync"

    "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

type Config struct {
	ModelDir   string
	SampleRate int
}

type Response struct {
	Type string `json:"type"` // "interim" или "final"
	Text string `json:"text"`
}

type ASRModule struct {
	recognizer    *sherpa_onnx.OnlineRecognizer
	stream        *sherpa_onnx.OnlineStream
	mu            sync.Mutex
	lastSentFinal string // Для исключения дублей в Finish()
    lastSentInterim string // <--- для фильтрации дублей
}

func New(cfg Config) (*ASRModule, error) {
	config := sherpa_onnx.OnlineRecognizerConfig{}
	config.ModelConfig.Transducer.Encoder = cfg.ModelDir + "/encoder.int8.onnx"
	config.ModelConfig.Transducer.Decoder = cfg.ModelDir + "/decoder.onnx"
	config.ModelConfig.Transducer.Joiner = cfg.ModelDir + "/joiner.int8.onnx"
	config.ModelConfig.Tokens = cfg.ModelDir + "/tokens.txt"
	config.ModelConfig.ModelType = "zipformer2"
	config.ModelConfig.NumThreads = 2
	config.ModelConfig.Provider = "cpu"
	config.FeatConfig.SampleRate = cfg.SampleRate
	config.FeatConfig.FeatureDim = 80
	config.EnableEndpoint = 1
	config.Rule1MinTrailingSilence = 1.2
	config.DecodingMethod = "greedy_search"

	recognizer := sherpa_onnx.NewOnlineRecognizer(&config)
	if recognizer == nil {
		return nil, fmt.Errorf("failed to create recognizer")
	}

	return &ASRModule{
		recognizer: recognizer,
		stream:     sherpa_onnx.NewOnlineStream(recognizer),
	}, nil
}

func (m *ASRModule) Write(pcm []float32) Response {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stream.AcceptWaveform(16000, pcm)
	for m.recognizer.IsReady(m.stream) {
		m.recognizer.Decode(m.stream)
	}

	res := m.recognizer.GetResult(m.stream)

	// 1. Проверка на Final (по паузе)
	if m.recognizer.IsEndpoint(m.stream) {
		text := res.Text
		m.lastSentFinal = text
		m.lastSentInterim = "" // Сбрасываем промежуточный при фиксации фразы
		m.recognizer.Reset(m.stream)
		return Response{Type: "final", Text: text}
	}

	// 2. ФИЛЬТР ДУБЛИКАТОВ ДЛЯ INTERIM
	// Если текст пустой или точно такой же, как мы уже отправляли — шлем пустой ответ
	if res.Text == "" || res.Text == m.lastSentInterim {
		return Response{}
	}

	// Обновляем состояние и отправляем новый текст
	m.lastSentInterim = res.Text
	return Response{Type: "interim", Text: res.Text}
}

func (m *ASRModule) Finish() Response {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stream.InputFinished()
	for m.recognizer.IsReady(m.stream) {
		m.recognizer.Decode(m.stream)
	}

	res := m.recognizer.GetResult(m.stream)
	// Если текст пустой или совпадает с последним финалом (уже отправленным по паузе)
	if res.Text == "" || res.Text == m.lastSentFinal {
		return Response{}
	}

	return Response{Type: "final", Text: res.Text}
}

func (m *ASRModule) Close() {
	if m.stream != nil {
		sherpa_onnx.DeleteOnlineStream(m.stream)
	}
	if m.recognizer != nil {
		sherpa_onnx.DeleteOnlineRecognizer(m.recognizer)
	}
}
