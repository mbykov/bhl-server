package lib

import (
    "encoding/json"
    "log"
    "strings"
    "time"
    "sync/atomic"
    "runtime"

    "github.com/mbykov/command-go-levenshtein"
    // "github.com/mbykov/gigaam-ort-go"
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
    done            chan struct{}  // для остановки горутин
}

func NewSession(id string, sendChan chan<- []byte, cfg *Config, models *Models) (*Session, error) {
    log.Printf("[%s] 🆕 Создание новой сессии", id)

    vosk, err := NewVoskProcessor(cfg, id)
    if err != nil {
        return nil, err
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
    }

    log.Printf("[%s] ✅ Сессия инициализирована", id)
    return s, nil
}

func (s *Session) HandleAudio(pcm []byte) {
    // Если это начало новой фразы
    if len(s.currentAudio) == 0 {
        s.phraseStart = time.Now()
        log.Printf("[%s] 🎤 Начало новой фразы", s.id)
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
        log.Printf("[%s] 🔄 Interim: %q", s.id, text)
        return
    }

    // ФИНАЛ! Обрабатываем результат
    log.Printf("[%s] 🎯 Final: %q", s.id, text)

    // Сохраняем аудио для этой фразы
    audioForPhrase := make([]byte, len(s.currentAudio))
    copy(audioForPhrase, s.currentAudio)
    s.currentAudio = make([]byte, 0) // очищаем для новой фразы

    // Обрезаем тишину
    audioForPhrase = trimAudioSilence(audioForPhrase, 16000)

    // Проверяем, не команда ли это
    cmd := s.findCommand(text)
    if cmd != nil {
        // Это команда found
        atomic.AddInt64(&s.commandsFound, 1)
        s.sendCommand(cmd, text)
        log.Printf("[%s] ✅ Команда: %s", s.id, cmd.Name)
    } else {
        // Обычный текст - отправляем в GigaAM если доступен
        if s.models.GigaAM != nil && s.cfg.GigaAM.Enabled {
            s.processWithGigaAM(text, audioForPhrase)
        } else {
            s.sendToBrowser("final", text)
        }
    }

    // Замеряем время обработки фразы
    log.Printf("[%s] ⏱️ Фраза обработана за %v (аудио: %d байт)",
        s.id, time.Since(s.phraseStart), len(audioForPhrase))
}

func (s *Session) processWithGigaAM(text string, audio []byte) {
    log.Printf("[%s] 🔄 Отправка в GigaAM (текст: %q, аудио: %d байт)",
        s.id, text, len(audio))

    // Используем ProcessAudio
    result, err := s.models.GigaAM.ProcessAudio(audio)

    if err != nil {
        log.Printf("[%s] ❌ Ошибка GigaAM: %v", s.id, err)
        s.sendToBrowser("final", text)
        return
    }

    if strings.TrimSpace(result.Text) == "" {
        // Пустой результат - используем оригинал
        log.Printf("[%s] ⚠️ GigaAM вернул пустую строку", s.id)
        s.sendToBrowser("final", text)
        return
    }

    s.sendToBrowser("correct", result.Text)
    log.Printf("[%s] ✅ Отправлен результат GigaAM: %q", s.id, result.Text)
}

func (s *Session) findCommand(text string) *command.CommandDefinition {
    if s.models.Command == nil || !s.cfg.Command.Enabled {
        return nil
    }

    words := strings.Fields(text)
    if len(words) < s.cfg.Command.MinWords {
        return nil
    }

    cmdName, external := s.models.Command.Resolve(text)
    _ = external // игнорируем, так как пока не используем

    if cmdName == "" {
        return nil
    }

    cmdDef := s.models.Command.GetCommand(cmdName)
    return cmdDef
}

func (s *Session) sendToBrowser(msgType, text string) {
    msg := map[string]string{
        "type": msgType,
        "text": text,
    }
    data, _ := json.Marshal(msg)

    select {
    case s.sendChan <- data:
    default:
        log.Printf("[%s] ⚠️ Канал отправки переполнен", s.id)
    }
}

func (s *Session) sendCommand(cmd *command.CommandDefinition, text string) {
    msg := map[string]interface{}{
        "type": "command",
        "text": text,
        "name": cmd.Name,
    }
    // Поле External есть и в CommandDefinition
    if cmd.External {
        msg["external"] = true
    }

    data, _ := json.Marshal(msg)

    select {
    case s.sendChan <- data:
    default:
        log.Printf("[%s] ⚠️ Канал отправки переполнен", s.id)
    }
}

// ============= МЕТОДЫ ДЛЯ КОНТРОЛЬНЫХ СООБЩЕНИЙ =============

// HandleControlMessage обрабатывает управляющие сообщения от клиента
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
            "audio_processed": s.vosk.GetAudioBytes(),
        },
        "server": map[string]interface{}{
            "time":    time.Now().Format(time.RFC3339),
            "uptime":  time.Since(s.phraseStart).String(),
        },
    }

    // Добавляем метрики GigaAM если доступны
    if s.models.GigaAM != nil {
        // Если у GigaAM есть метод GetMetrics, можно добавить
        // metrics["gigaam"] = s.models.GigaAM.GetMetrics()
        metrics["gigaam"] = map[string]interface{}{
            "enabled": true,
            "model":   s.cfg.GigaAM.ModelPath,
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
    // Отправляем только безопасные для клиента параметры конфига
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
        },
    }

    data, _ := json.Marshal(config)

    select {
    case s.sendChan <- data:
    default:
        log.Printf("[%s] ⚠️ Не удалось отправить конфиг", s.id)
    }
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
        },
    }

    data, _ := json.Marshal(stats)

    select {
    case s.sendChan <- data:
    default:
        log.Printf("[%s] ⚠️ Не удалось отправить статистику", s.id)
    }
}

func (s *Session) sendPong() {
    pong := map[string]interface{}{
        "type": "pong",
        "time": time.Now().UnixNano(),
    }
    data, _ := json.Marshal(pong)

    select {
    case s.sendChan <- data:
    default:
        // Не логируем, чтобы не засорять
    }
}

func (s *Session) resetVosk() {
    log.Printf("[%s] 🔄 Принудительный сброс Vosk по запросу клиента", s.id)

    err := s.vosk.Reset()
    if err != nil {
        log.Printf("[%s] ❌ Ошибка сброса Vosk: %v", s.id, err)
        s.sendToBrowser("error", "Failed to reset Vosk: "+err.Error())
        return
    }

    // Очищаем текущий буфер аудио
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
        "type": "gc_result",
        "freed_mb": freedMB,
        "heap_before_mb": memBefore.HeapAlloc / 1024 / 1024,
        "heap_after_mb":  memAfter.HeapAlloc / 1024 / 1024,
    }
    data, _ := json.Marshal(msg)

    select {
    case s.sendChan <- data:
    default:
    }

    // Сразу отправляем обновленные метрики
    s.sendMetrics()
}

// ============= ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ =============

// trimAudioSilence обрезает тишину в начале и конце аудио
// pcm - сырые PCM данные (16-bit, mono)
// sampleRate - частота дискретизации
func trimAudioSilence(pcm []byte, sampleRate int) []byte {
    if len(pcm) < 640 { // меньше 20 мс
        return pcm
    }

    // Параметры для анализа
    frameSize := sampleRate / 100 // 10 мс
    threshold := 500 // порог тишины

    // Конвертируем в int16 для анализа
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

    // Конвертируем обратно в []byte
    trimmed := make([]byte, (end-start)*2)
    for i := start; i < end; i++ {
        trimmed[(i-start)*2] = byte(samples[i])
        trimmed[(i-start)*2+1] = byte(samples[i] >> 8)
    }
    return trimmed
}

// Close закрывает сессию и освобождает ресурсы
func (s *Session) Close() {
    log.Printf("[%s] 🔚 Закрытие сессии", s.id)

    // Сигнализируем о завершении
    close(s.done)

    // Закрываем Vosk
    if s.vosk != nil {
        s.vosk.Close()
    }

    // Очищаем буферы
    s.currentAudio = nil

    log.Printf("[%s] ✅ Сессия закрыта, обработано фреймов: %d, команд: %d, аудио: %.2f KB",
        s.id,
        atomic.LoadInt64(&s.framesProcessed),
        atomic.LoadInt64(&s.commandsFound),
        float64(atomic.LoadInt64(&s.audioBytesTotal))/1024)
}

// GetAudioBytes возвращает количество обработанных байт атомарно
func (v *VoskProcessor) GetAudioBytes() int64 {
    v.mu.Lock()
    defer v.mu.Unlock()
    return v.audioBytes
}
