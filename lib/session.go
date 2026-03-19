package lib

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "os"
    "runtime"
    "strings"
    "sync"
    "sync/atomic"
    "time"

    simplecmd "github.com/mbykov/command-go-levenshtein"
    qwen "github.com/michael/bhl-qwen-go"
    "github.com/mbykov/isMath"
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

    // Диагностика
    commandsFound   int64
    framesProcessed int64
    audioBytesTotal int64
    lastMetricsTime time.Time
    done            chan struct{}

    // Контекст для Qwen и isMath
    lastFinalText   string                // последний финальный текст (для isMath)
    lastQwenCommand *qwen.CommandResponse // последний результат Qwen
    qwenMu          sync.Mutex
    debugLog        *log.Logger
}

func NewSession(id string, sendChan chan<- []byte, cfg *Config, models *Models) (*Session, error) {
    log.Printf("[%s] 🆕 Создание новой сессии", id)

    vosk, err := NewVoskProcessor(cfg, id)
    if err != nil {
        return nil, err
    }

    // Инициализируем debug логгер
    var debugLogger *log.Logger
    if os.Getenv("DEBUG") == "1" || (cfg.Qwen.Enabled && cfg.Qwen.Debug) {
        debugLogger = log.New(os.Stdout, fmt.Sprintf("[DEBUG:%s] ", id),
            log.LstdFlags|log.Lmicroseconds)
    } else {
        debugLogger = log.New(io.Discard, "", 0)
    }

    s := &Session{
        id:              id,
        vosk:            vosk,
        sendChan:        sendChan,
        models:          models,
        cfg:             cfg,
        currentAudio:    make([]byte, 0),
        commandsFound:   0,
        framesProcessed: 0,
        audioBytesTotal: 0,
        lastMetricsTime: time.Now(),
        done:            make(chan struct{}),
        lastFinalText:   "",
        lastQwenCommand: nil,
        debugLog:        debugLogger,
    }

    log.Printf("[%s] ✅ Сессия инициализирована", id)
    return s, nil
}

func (s *Session) HandleAudio(pcm []byte) {
    // Если это начало новой фразы
    if len(s.currentAudio) == 0 {
        s.phraseStart = time.Now()
        s.debugLog.Printf("[%s] 🎤 Начало новой фразы", s.id)
    }

    // Накапливаем аудио
    s.currentAudio = append(s.currentAudio, pcm...)
    atomic.AddInt64(&s.audioBytesTotal, int64(len(pcm)))

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

    atomic.AddInt64(&s.framesProcessed, 1)

    if !result.IsFinal {
        // ПРОМЕЖУТОЧНЫЙ - просто шлем в браузер
        s.sendToBrowser("interim", text)
        s.debugLog.Printf("[%s] 🔄 Interim: %q", s.id, text)
        return
    }

    // ФИНАЛ! Vosk распознал текст
    log.Printf("[%s] 🎯 Vosk final: %q", s.id, text)

    // Сохраняем аудио для GigaAM
    audioForPhrase := make([]byte, len(s.currentAudio))
    copy(audioForPhrase, s.currentAudio)
    s.currentAudio = make([]byte, 0)
    audioForPhrase = trimAudioSilence(audioForPhrase, 16000)

    // Запоминаем предыдущий текст для isMath перед обновлением
    previousText := s.lastFinalText

    // Обновляем lastFinalText для следующей фразы
    s.lastFinalText = text

    // Отправляем текст в simple-command, передавая предыдущий текст как контекст
    s.processWithSimpleCommand(text, audioForPhrase, previousText)
}

// processWithSimpleCommand отправляет текст в simple-command для определения команды
func (s *Session) processWithSimpleCommand(text string, audio []byte, previousText string) {
    s.debugLog.Printf("[%s] 🔄 Simple-command анализ: %q (предыдущий: %q)",
        s.id, text, previousText)

    // Ищем команду через Левенштейн
    cmd, external := s.findCommand(text)

    if cmd == nil {
        // Не команда - отправляем в GigaAM для пунктуации
        s.debugLog.Printf("[%s] 📝 Не команда, отправляем в GigaAM", s.id)
        s.processWithGigaAM(text, audio)
        return
    }

    // Нашли команду!
    atomic.AddInt64(&s.commandsFound, 1)

    if external {
        // External команда - отправляем в Qwen с предыдущим текстом как контекст
        s.debugLog.Printf("[%s] 🔄 External команда: %s -> Qwen", s.id, cmd.Name)
        s.processWithQwen(cmd, text, audio, previousText)
        return
    }

    // Обычная команда - сразу в браузер
    s.debugLog.Printf("[%s] ✅ Локальная команда: %s", s.id, cmd.Name)
    s.sendCommand(cmd, text)
}

// processWithQwen отправляет external команду в Qwen с isMath проверкой
func (s *Session) processWithQwen(cmd *simplecmd.CommandDefinition, text string, audio []byte, previousText string) {
    s.debugLog.Printf("[%s] 🤖 Qwen обработка: cmd=%s, text=%q, контекст=%q",
        s.id, cmd.Name, text, previousText)

    var req qwen.CommandRequest

    switch cmd.Name {
    case "createLatex":
        if !ismath.Check(previousText) {
            s.debugLog.Printf("[%s] ❌ CREATE: контекст не математический: %q", s.id, previousText)
            s.processWithGigaAM(text, audio)
            return
        }

        // Контекст из Vosk - type="final"
        req = qwen.CommandRequest{
            CurrentText: text,
            Context: &qwen.CommandContext{
                Type: "final",
                Text: previousText,
            },
        }

    case "editLatex":
        if !ismath.Check(text) {
            s.debugLog.Printf("[%s] ❌ EDIT: текущий текст не математический: %q", s.id, text)
            s.processWithGigaAM(text, audio)
            return
        }

        if s.lastQwenCommand == nil {
            s.debugLog.Printf("[%s] ❌ EDIT: нет контекста предыдущей команды", s.id)
            s.processWithGigaAM(text, audio)
            return
        }

        // Контекст из предыдущей команды Qwen - type="command"
        req = qwen.CommandRequest{
            CurrentText: text,
            Context: &qwen.CommandContext{
                Type:   "command",
                Text:   s.lastQwenCommand.Text,
                Script: s.lastQwenCommand.Script,
            },
        }
    }

    s.callQwen(req, audio, cmd.Name)
}

// callQwen выполняет вызов к Qwen модели
func (s *Session) callQwen(req qwen.CommandRequest, audio []byte, cmdName string) {
    s.qwenMu.Lock()
    defer s.qwenMu.Unlock()

    s.debugLog.Printf("[%s] 🤖 Вызов Qwen API: cmd=%s, current=%q, context=%+v",
        s.id, cmdName, req.CurrentText, req.Context)

    ctx := context.Background()
    if s.cfg.Qwen.TimeoutSec > 0 {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, time.Duration(s.cfg.Qwen.TimeoutSec)*time.Second)
        defer cancel()
    }

    start := time.Now()
    resp, err := s.models.Qwen.Resolve(ctx, req)
    duration := time.Since(start)

    if err != nil {
        s.debugLog.Printf("[%s] ❌ Qwen ошибка: %v (за %v)", s.id, err, duration)
        s.processWithGigaAM(req.CurrentText, audio)
        return
    }

    s.debugLog.Printf("[%s] ✅ Qwen ответ: type=%s, name=%s, script=%q (за %v)",
        s.id, resp.Type, resp.Name, resp.Script, duration)

    if resp.Type == "final" {
        // Qwen решил, что это не команда - в GigaAM
        s.debugLog.Printf("[%s] 📝 Qwen вернул final, отправляем в GigaAM", s.id)
        s.processWithGigaAM(resp.Text, audio)
        return
    }

    // Это команда - сохраняем и отправляем в браузер
    if cmdName == "createLatex" {
        // После CREATE сохраняем для будущих EDIT
        s.lastQwenCommand = resp
        s.debugLog.Printf("[%s] 💾 Сохранен контекст CREATE: %q", s.id, resp.Script)
    }

    // Для EDIT тоже сохраняем, чтобы можно было цепочку правок
    if cmdName == "editLatex" {
        s.lastQwenCommand = resp
        s.debugLog.Printf("[%s] 💾 Обновлен контекст EDIT: %q", s.id, resp.Script)
    }

    s.sendQwenResponse(resp)
}

// processWithGigaAM отправляет текст в GigaAM для пунктуации
func (s *Session) processWithGigaAM(text string, audio []byte) {
    if s.models.GigaAM == nil || !s.cfg.GigaAM.Enabled {
        // GigaAM отключен или недоступен - отправляем как есть
        s.debugLog.Printf("[%s] ⚠️ GigaAM отключен, отправляем финал как есть", s.id)
        s.sendToBrowser("final", text)
        return
    }

    s.debugLog.Printf("[%s] 🔄 GigaAM пунктуация: %q (аудио: %d байт)",
        s.id, text, len(audio))

    result, err := s.models.GigaAM.ProcessAudio(audio)
    if err != nil {
        s.debugLog.Printf("[%s] ❌ GigaAM ошибка: %v", s.id, err)
        s.sendToBrowser("final", text)
        return
    }

    if strings.TrimSpace(result.Text) == "" {
        s.debugLog.Printf("[%s] ⚠️ GigaAM вернул пустую строку", s.id)
        s.sendToBrowser("final", text)
        return
    }

    s.debugLog.Printf("[%s] ✅ GigaAM: %q -> %q", s.id, text, result.Text)
    s.sendToBrowser("correct", result.Text)
}

// findCommand ищет команду через simple-command (Левенштейн)
func (s *Session) findCommand(text string) (*simplecmd.CommandDefinition, bool) {
    if s.models.Command == nil || !s.cfg.Command.Enabled {
        return nil, false
    }

    words := strings.Fields(text)
    if len(words) < s.cfg.Command.MinWords {
        return nil, false
    }

    cmdName, external := s.models.Command.Resolve(text)

    if cmdName == "" {
        return nil, false
    }

    cmdDef := s.models.Command.GetCommand(cmdName)
    return cmdDef, external
}

func (s *Session) sendToBrowser(msgType, text string) {
    msg := map[string]string{
        "type": msgType,
        "text": text,
    }
    data, _ := json.Marshal(msg)

    select {
    case s.sendChan <- data:
        s.debugLog.Printf("[%s] 📤 Отправлено: %s", s.id, msgType)
    default:
        log.Printf("[%s] ⚠️ Канал отправки переполнен", s.id)
    }
}

func (s *Session) sendCommand(cmd *simplecmd.CommandDefinition, text string) {
    msg := map[string]interface{}{
        "type": "command",
        "text": text,
        "name": cmd.Name,
    }
    if cmd.External {
        msg["external"] = true
    }

    data, _ := json.Marshal(msg)

    select {
    case s.sendChan <- data:
        s.debugLog.Printf("[%s] 📤 Отправлена команда: %s", s.id, cmd.Name)
    default:
        log.Printf("[%s] ⚠️ Канал отправки переполнен", s.id)
    }
}

func (s *Session) sendQwenResponse(resp *qwen.CommandResponse) {
    msg := map[string]interface{}{
        "type":   "command",
        "name":   resp.Name,
        "script": resp.Script,
        "text":   resp.Text,
    }

    data, err := json.Marshal(msg)
    if err != nil {
        s.debugLog.Printf("[%s] ❌ Ошибка маршалинга ответа Qwen: %v", s.id, err)
        return
    }

    select {
    case s.sendChan <- data:
        s.debugLog.Printf("[%s] 📤 Отправлен Qwen ответ: %s (%s)",
            s.id, resp.Name, resp.Script)
    default:
        log.Printf("[%s] ⚠️ Канал отправки переполнен", s.id)
    }
}

// ============= МЕТОДЫ ДЛЯ КОНТРОЛЬНЫХ СООБЩЕНИЙ =============

func (s *Session) HandleControlMessage(msg map[string]interface{}) {
    msgType, ok := msg["type"].(string)
    if !ok {
        return
    }

    switch msgType {
    case "get_metrics":
        s.sendMetrics()
    case "get_config":
        s.sendConfig()
    case "get_stats":
        s.sendStats()
    case "ping":
        s.sendPong()
    case "reset_vosk":
        s.resetVosk()
    case "gc":
        s.forceGC()
    }
}

func (s *Session) sendMetrics() {
    var mem runtime.MemStats
    runtime.ReadMemStats(&mem)

    metrics := map[string]interface{}{
        "memory": map[string]interface{}{
            "alloc_mb":       mem.Alloc / 1024 / 1024,
            "total_alloc_mb": mem.TotalAlloc / 1024 / 1024,
            "sys_mb":         mem.Sys / 1024 / 1024,
            "heap_mb":        mem.HeapAlloc / 1024 / 1024,
            "goroutines":     runtime.NumGoroutine(),
            "gc_cycles":      mem.NumGC,
            "gc_pause_total_ms": mem.PauseTotalNs / 1_000_000,
        },
        "session": map[string]interface{}{
            "id":              s.id,
            "uptime_sec":      time.Since(s.phraseStart).Seconds(),
            "audio_buffered":  len(s.currentAudio),
            "commands_found":  atomic.LoadInt64(&s.commandsFound),
            "frames_processed": atomic.LoadInt64(&s.framesProcessed),
            "audio_bytes_total": atomic.LoadInt64(&s.audioBytesTotal),
        },
        "vosk": map[string]interface{}{
            "audio_processed": atomic.LoadInt64(&s.audioBytesTotal),
        },
        "server": map[string]interface{}{
            "time":    time.Now().Format(time.RFC3339),
            "uptime":  time.Since(s.phraseStart).String(),
        },
    }

    if s.models.GigaAM != nil {
        metrics["gigaam"] = map[string]interface{}{
            "enabled": true,
            "model":   s.cfg.GigaAM.ModelPath,
        }
    }

    if s.models.Qwen != nil {
        metrics["qwen"] = map[string]interface{}{
            "enabled":      true,
            "model":        s.cfg.Qwen.ModelName,
            "has_context":  s.lastQwenCommand != nil,
        }
    }

    response := map[string]interface{}{
        "type":      "metrics",
        "timestamp": time.Now(),
        "metrics":   metrics,
    }

    data, _ := json.Marshal(response)

    select {
    case s.sendChan <- data:
    default:
        log.Printf("[%s] ⚠️ Не удалось отправить метрики: канал переполнен", s.id)
    }
}

func (s *Session) sendConfig() {
    config := map[string]interface{}{
        "type": "config",
        "config": map[string]interface{}{
            "vosk": map[string]interface{}{
                "sample_rate": s.cfg.Vosk.SampleRate,
                "chunk_ms":    s.cfg.Vosk.ChunkMs,
            },
            "command": map[string]interface{}{
                "enabled":   s.cfg.Command.Enabled,
                "min_words": s.cfg.Command.MinWords,
                "threshold": s.cfg.Command.Threshold,
            },
            "gigaam": map[string]interface{}{
                "enabled":     s.cfg.GigaAM.Enabled,
                "sample_rate": s.cfg.GigaAM.SampleRate,
                "num_threads": s.cfg.GigaAM.NumThreads,
            },
            "qwen": map[string]interface{}{
                "enabled": s.cfg.Qwen.Enabled,
                "model":   s.cfg.Qwen.ModelName,
            },
        },
    }

    data, _ := json.Marshal(config)
    s.sendChan <- data
}

func (s *Session) sendStats() {
    stats := map[string]interface{}{
        "type": "stats",
        "stats": map[string]interface{}{
            "session_id":       s.id,
            "commands_found":   atomic.LoadInt64(&s.commandsFound),
            "frames_processed": atomic.LoadInt64(&s.framesProcessed),
            "audio_bytes":      atomic.LoadInt64(&s.audioBytesTotal),
            "audio_buffered":   len(s.currentAudio),
            "qwen_context":     s.lastQwenCommand != nil,
        },
    }

    data, _ := json.Marshal(stats)
    s.sendChan <- data
}

func (s *Session) sendPong() {
    pong := map[string]interface{}{
        "type": "pong",
        "time": time.Now().UnixNano(),
    }
    data, _ := json.Marshal(pong)
    s.sendChan <- data
}

func (s *Session) resetVosk() {
    log.Printf("[%s] 🔄 Принудительный сброс Vosk по запросу клиента", s.id)

    err := s.vosk.Reset()
    if err != nil {
        log.Printf("[%s] ❌ Ошибка сброса Vosk: %v", s.id, err)
        s.sendToBrowser("error", "Failed to reset Vosk: "+err.Error())
        return
    }

    s.currentAudio = make([]byte, 0)
    s.sendToBrowser("system", "Vosk reset completed")
}

func (s *Session) forceGC() {
    log.Printf("[%s] 🧹 Принудительный запуск GC по запросу клиента", s.id)

    var memBefore runtime.MemStats
    runtime.ReadMemStats(&memBefore)

    runtime.GC()

    var memAfter runtime.MemStats
    runtime.ReadMemStats(&memAfter)

    freedMB := (memBefore.HeapAlloc - memAfter.HeapAlloc) / 1024 / 1024

    msg := map[string]interface{}{
        "type":          "gc_result",
        "freed_mb":      freedMB,
        "heap_before_mb": memBefore.HeapAlloc / 1024 / 1024,
        "heap_after_mb":  memAfter.HeapAlloc / 1024 / 1024,
    }
    data, _ := json.Marshal(msg)
    s.sendChan <- data

    s.sendMetrics()
}

// ============= ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ =============

func trimAudioSilence(pcm []byte, sampleRate int) []byte {
    if len(pcm) < 640 {
        return pcm
    }

    frameSize := sampleRate / 100 // 10 мс
    threshold := 500

    samples := make([]int16, len(pcm)/2)
    for i := 0; i < len(pcm); i += 2 {
        samples[i/2] = int16(pcm[i]) | int16(pcm[i+1])<<8
    }

    // Ищем начало речи
    start := 0
    for i := 0; i < len(samples)-frameSize; i += frameSize {
        var maxAmp int16
        for j := 0; j < frameSize; j++ {
            amp := samples[i+j]
            if amp < 0 {
                amp = -amp
            }
            if amp > maxAmp {
                maxAmp = amp
            }
        }
        if maxAmp > int16(threshold) {
            start = i
            break
        }
    }

    // Ищем конец речи
    end := len(samples)
    for i := len(samples) - frameSize; i > start; i -= frameSize {
        var maxAmp int16
        for j := 0; j < frameSize; j++ {
            amp := samples[i+j]
            if amp < 0 {
                amp = -amp
            }
            if amp > maxAmp {
                maxAmp = amp
            }
        }
        if maxAmp > int16(threshold) {
            end = i + frameSize
            break
        }
    }

    trimmed := make([]byte, (end-start)*2)
    for i := start; i < end; i++ {
        trimmed[(i-start)*2] = byte(samples[i])
        trimmed[(i-start)*2+1] = byte(samples[i] >> 8)
    }
    return trimmed
}

// Close закрывает сессию
func (s *Session) Close() {
    log.Printf("[%s] 🔚 Закрытие сессии", s.id)

    select {
    case <-s.done:
        // уже закрыт
    default:
        close(s.done)
    }

    if s.vosk != nil {
        s.vosk.Close()
    }

    s.currentAudio = nil

    log.Printf("[%s] ✅ Сессия закрыта, обработано фреймов: %d, команд: %d, аудио: %.2f KB",
        s.id,
        atomic.LoadInt64(&s.framesProcessed),
        atomic.LoadInt64(&s.commandsFound),
        float64(atomic.LoadInt64(&s.audioBytesTotal))/1024)
}
