package voskpunct

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"sync"

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

type Punctuator struct {
	session   *ort.DynamicSession[int64, float32]
	tokenizer *tokenizer.FullTokenizer
	mu        sync.Mutex
}

type Config struct {
	ModelDir string // путь к директории с recasepunc.onnx и vocab.txt
}

// New создаёт новый экземпляр пунктуатора
func New(cfg Config) (*Punctuator, error) {
	// Сохраняем текущую директорию
	originalWd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// Переключаемся в директорию модели
	if err := os.Chdir(cfg.ModelDir); err != nil {
		return nil, err
	}
	// Возвращаемся после загрузки модели
	defer os.Chdir(originalWd)

	// Загрузка ONNX Runtime
	ortHome := os.Getenv("ORT_HOME")
	if ortHome == "" {
		return nil, nil // возвращаем nil, пунктуация будет пропущена
	}

	ortLib := filepath.Join(ortHome, "lib", "libonnxruntime.so")
	ort.SetSharedLibraryPath(ortLib)
	if !ort.IsInitialized() {
		if err := ort.InitializeEnvironment(); err != nil {
			return nil, err
		}
	}

	// Загрузка модели
	session, err := ort.NewDynamicSession[int64, float32](
		"recasepunc.onnx",
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"logits"},
	)
	if err != nil {
		return nil, err
	}

	// Загрузка токенизатора
	vocabPath := filepath.Join(cfg.ModelDir, "vocab.txt")
	vocab, err := tokenizer.FromFile(vocabPath)
	if err != nil {
		session.Destroy()
		return nil, err
	}
	tk := tokenizer.NewFullTokenizer(vocab, 128, false)

	return &Punctuator{
		session:   session,
		tokenizer: tk,
	}, nil
}

// Process добавляет пунктуацию к тексту
func (p *Punctuator) Process(text string) string {
	if p == nil || p.session == nil || p.tokenizer == nil {
		return text
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Токенизация
	encoding := p.tokenizer.Tokenize(text)

	// Находим реальную длину (до [SEP])
	realLen := 0
	for i, token := range encoding.Tokens {
		realLen = i + 1
		if token == "[SEP]" {
			break
		}
	}

	seqLen := int64(len(encoding.TokenIDs))

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
	if err := p.session.Run([]*ort.Tensor[int64]{in1, in2, in3}, []*ort.Tensor[float32]{outLogits}); err != nil {
		return text // при ошибке возвращаем исходный текст
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
	capitalizeNext := false

	for _, ch := range finalText {
		if capitalizeNext && unicode.IsLetter(ch) {
			builder.WriteRune(unicode.ToUpper(ch))
			capitalizeNext = false
		} else {
			builder.WriteRune(ch)
		}

		if ch == '.' || ch == '?' || ch == '!' {
			capitalizeNext = true
		}
	}

	return builder.String()
}

// Close закрывает ресурсы
func (p *Punctuator) Close() {
	if p != nil && p.session != nil {
		p.session.Destroy()
	}
}

func toInt64Slice(in []int32) []int64 {
	out := make([]int64, len(in))
	for i, v := range in {
		out[i] = int64(v)
	}
	return out
}
