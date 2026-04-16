package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	tokenizer "github.com/Hank-Kuo/go-bert-tokenizer"
	ort "github.com/yalue/onnxruntime_go"
)

// Метки для интерпретации
var puncLabels = map[int]string{
	0: "",
	1: ",",
	2: ".",
	3: "?",
	4: "!",
}

func main() {
	modelDir := "/home/michael/LLM/bhl/Models/vosk-recasepunc-ru-0.22"
	vocabPath := modelDir + "/vocab.txt"

	// Переключаемся в директорию модели
	originalWd, _ := os.Getwd()
	if err := os.Chdir(modelDir); err != nil {
		log.Fatalf("Change dir error: %v", err)
	}
	defer os.Chdir(originalWd)

	// Загрузка ONNX Runtime
	ortHome := os.Getenv("ORT_HOME")
	if ortHome == "" {
		log.Fatal("ORT_HOME environment variable is not set")
	}

	ortLib := filepath.Join(ortHome, "lib", "libonnxruntime.so")
	ort.SetSharedLibraryPath(ortLib)
	if !ort.IsInitialized() {
		if err := ort.InitializeEnvironment(); err != nil {
			log.Fatalf("ONNX init error: %v", err)
		}
	}

	// Загрузка модели
	session, err := ort.NewDynamicSession[int64, float32](
		"recasepunc.onnx",
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"logits"},
	)
	if err != nil {
		log.Fatalf("Session error: %v", err)
	}
	defer session.Destroy()

	// Загрузка токенизатора
	vocab, err := tokenizer.FromFile(vocabPath)
	if err != nil {
		log.Fatalf("Load vocab error: %v", err)
	}
	tk := tokenizer.NewFullTokenizer(vocab, 128, false)

	// Тестовый текст
	inputStr := "привет как дела меня зовут михаил"

	encoding := tk.Tokenize(inputStr)

	// Находим реальную длину (до [SEP])
	realLen := 0
	for i, token := range encoding.Tokens {
		realLen = i + 1
		if token == "[SEP]" {
			break
		}
	}

	seqLen := int64(len(encoding.TokenIDs))

	fmt.Printf("Текст: %s\n", inputStr)
	fmt.Printf("Реальных токенов (до SEP): %d\n", realLen)
	fmt.Printf("Токены: %v\n", encoding.Tokens[:realLen])
	fmt.Println(strings.Repeat("-", 60))

	// Подготовка входов
	shape := ort.NewShape(1, seqLen)
	inputIds := toInt64Slice(encoding.TokenIDs)
	mask := toInt64Slice(encoding.MaskIDs)
	types := toInt64Slice(encoding.TypeIDs)

	in1, _ := ort.NewTensor(shape, inputIds)
	defer in1.Destroy()
	in2, _ := ort.NewTensor(shape, mask)
	defer in2.Destroy()
	in3, _ := ort.NewTensor(shape, types)
	defer in3.Destroy()

	// Выход
	outputShape := ort.NewShape(1, seqLen, 5)
	outputData := make([]float32, 1*int(seqLen)*5)
	outLogits, _ := ort.NewTensor(outputShape, outputData)
	defer outLogits.Destroy()

	// Инференс
	if err := session.Run([]*ort.Tensor[int64]{in1, in2, in3}, []*ort.Tensor[float32]{outLogits}); err != nil {
		log.Fatalf("Inference error: %v", err)
	}

	logits := outLogits.GetData()

	// Пост-обработка
	var result []string

	for i := 0; i < realLen; i++ {
		token := encoding.Tokens[i]

		// Пропускаем специальные токены
		if token == "[CLS]" || token == "[SEP]" {
			continue
		}

		// Находим класс с максимальной вероятностью
		maxIdx := 0
		maxVal := float32(-1e9)
		for j := 0; j < 5; j++ {
			val := logits[i*5+j]
			if val > maxVal {
				maxVal = val
				maxIdx = j
			}
		}

		punc := puncLabels[maxIdx]

		// Обработка subword (##)
		isSubword := strings.HasPrefix(token, "##")
		cleanWord := token
		if isSubword {
			cleanWord = token[2:]
		}

		// Собираем слово
		if isSubword && len(result) > 0 {
			result[len(result)-1] += cleanWord
		} else {
			result = append(result, cleanWord)
		}

		// Добавляем пунктуацию (только если не subword)
		if !isSubword && punc != "" {
			result[len(result)-1] += punc
		}
	}

	// Собираем финальный текст
	finalText := strings.Join(result, " ")

	// Делаем заглавной первую букву предложения
	if len(finalText) > 0 {
		runes := []rune(finalText)
		runes[0] = unicode.ToUpper(runes[0])
		finalText = string(runes)
	}

	// Делаем заглавными буквы после точек, вопросительных и восклицательных знаков
	var builder strings.Builder
	capitalizeNext := false // первая буква уже заглавная

	for _, ch := range finalText {
		if capitalizeNext && unicode.IsLetter(ch) {
			builder.WriteRune(unicode.ToUpper(ch))
			capitalizeNext = false
		} else {
			builder.WriteRune(ch)
		}

		// Если встретили знак конца предложения, следующая буква должна быть заглавной
		if ch == '.' || ch == '?' || ch == '!' {
			capitalizeNext = true
		}
	}

	finalText = builder.String()

	fmt.Println("\nРезультат:")
	fmt.Println(finalText)
}

func toInt64Slice(in []int32) []int64 {
	out := make([]int64, len(in))
	for i, v := range in {
		out[i] = int64(v)
	}
	return out
}
