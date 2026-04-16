package main

import (
	"flag"
	"fmt"
	"log"
	"time"
	"os"
	// "strings"

    "github.com/mbykov/asr-zipformer-go"
)

func main() {
	modelDir := flag.String("model", "../../Models/streaming-zipformer-small-ru-vosk-int8", "путь к папке модели")
	wavPath := flag.String("wav", "../../Models/example.wav", "путь к аудио файлу")
	flag.Parse()

	engine, err := asr.New(asr.Config{ModelDir: *modelDir, SampleRate: 16000})
	if err != nil {
		log.Fatal(err)
	}
	defer engine.Close()

	audioData, _ := os.ReadFile(*wavPath)
	rawPCM := audioData[44:]
	chunkSize := 3200 // 100ms

	fmt.Printf("🎙️ Обработка %s...\n", *wavPath)

	for i := 0; i < len(rawPCM); i += chunkSize {
		end := i + chunkSize
		if end > len(rawPCM) { end = len(rawPCM) }

		resp := engine.Write(bytesToFloat32(rawPCM[i:end]))

		if resp.Text != "" {
			if resp.Type == "final" {
				fmt.Printf("\r\033[K✅ FINAL: %s\n", resp.Text)
			} else {
				fmt.Printf("\r\033[K⏳ INTERIM: %s", resp.Text)
			}
		}
		// Эмуляция скорости речи для визуализации
		time.Sleep(50 * time.Millisecond)
	}

	// Финальный "выдох"
	last := engine.Finish()
	if last.Text != "" {
		fmt.Printf("\r\033[K✅ FINAL (END): %s\n", last.Text)
	}
	fmt.Println("\n✨ Готово.")
}

func bytesToFloat32(data []byte) []float32 {
	samples := make([]float32, len(data)/2)
	for i := 0; i < len(data); i += 2 {
		sample := int16(data[i]) | int16(data[i+1])<<8
		samples[i/2] = float32(sample) / 32768.0
	}
	return samples
}
