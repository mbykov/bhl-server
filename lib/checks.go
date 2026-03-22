package lib

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"
)

// CheckResult результат проверки
type CheckResult struct {
    Name    string
    OK      bool
    Error   string
    Warning string
}

// RunAllChecks запускает все проверки при старте
func RunAllChecks(cfg *Config) []CheckResult {
    results := []CheckResult{}

    // 1. Проверка наличия всех модулей (директорий)
    results = append(results, checkModulesExist())

    // 2. Проверка моделей Vosk
    results = append(results, checkVoskModel(cfg))

    // 3. Проверка модели GigaAM
    if cfg.GigaAM.Enabled {
        results = append(results, checkGigaAMModel(cfg))
    }

    // 4. Проверка Ollama и модели Qwen
    if cfg.Qwen.Enabled && cfg.Checks.RequireOllama {
        results = append(results, checkOllama(cfg))
    }

    return results
}

// checkModulesExist проверяет наличие всех необходимых модулей
func checkModulesExist() CheckResult {
    result := CheckResult{Name: "Модули проекта"}

    required := []string{
        "../bhl-vosk-sherpa-go",
        "../command-qwen-gguf",
        "../gigaam-ort-go",
        "../isMath",
        "../simple-command",
    }

    missing := []string{}
    for _, module := range required {
        if _, err := os.Stat(module); err != nil {
            missing = append(missing, module)
        }
    }

    if len(missing) == 0 {
        result.OK = true
    } else {
        result.OK = false
        result.Error = fmt.Sprintf("Отсутствуют модули: %v", missing)
    }

    return result
}

// checkVoskModel проверяет наличие модели Vosk
func checkVoskModel(cfg *Config) CheckResult {
    result := CheckResult{Name: "Vosk модель"}

    // Проверяем только что папка существует и не пуста
    info, err := os.Stat(cfg.Vosk.ModelPath)
    if err != nil {
        result.OK = false
        result.Error = fmt.Sprintf("Модель Vosk не найдена: %v", err)
        return result
    }

    if !info.IsDir() {
        result.OK = false
        result.Error = "Путь к Vosk модели не является директорией"
        return result
    }

    // Проверяем, что директория не пуста
    entries, err := os.ReadDir(cfg.Vosk.ModelPath)
    if err != nil {
        result.OK = false
        result.Error = fmt.Sprintf("Не удается прочитать директорию модели: %v", err)
        return result
    }

    if len(entries) == 0 {
        result.OK = false
        result.Error = "Директория модели Vosk пуста"
        return result
    }

    result.OK = true
    result.Warning = fmt.Sprintf("Найдено %d файлов/папок", len(entries))
    return result
}

// checkGigaAMModel проверяет наличие модели GigaAM
func checkGigaAMModel(cfg *Config) CheckResult {
    result := CheckResult{Name: "GigaAM модель"}

    if _, err := os.Stat(cfg.GigaAM.ModelPath); err != nil {
        result.OK = false
        result.Error = fmt.Sprintf("Модель GigaAM не найдена: %v", err)
        return result
    }

    // Проверяем наличие onnx файлов
    pattern := filepath.Join(cfg.GigaAM.ModelPath, "*.onnx")
    matches, err := filepath.Glob(pattern)
    if err != nil || len(matches) == 0 {
        result.OK = false
        result.Error = "В модели GigaAM нет .onnx файлов"
        return result
    }

    result.OK = true
    result.Warning = fmt.Sprintf("Найдено %d onnx файлов", len(matches))
    return result
}

// checkOllama проверяет доступность Ollama и наличие модели
func checkOllama(cfg *Config) CheckResult {
    result := CheckResult{Name: "Ollama"}

    // 1. Проверяем, запущен ли ollama (короткий таймаут)
    client := &http.Client{
        Timeout: 2 * time.Second,
    }

    resp, err := client.Get("http://localhost:11434/api/tags")
    if err != nil {
        result.OK = false
        result.Error = fmt.Sprintf("Ollama не доступен (запустите 'ollama serve'): %v", err)
        return result
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        result.OK = false
        result.Error = fmt.Sprintf("Ollama вернул статус %d", resp.StatusCode)
        return result
    }

    // 2. Проверяем наличие модели
    var tags struct {
        Models []struct {
            Name string `json:"name"`
        } `json:"models"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
        result.OK = false
        result.Error = fmt.Sprintf("Не удалось прочитать список моделей: %v", err)
        return result
    }

    // Логируем все найденные модели
    fmt.Printf("📋 Найденные модели Ollama:\n")
    for _, m := range tags.Models {
        fmt.Printf("   - %s\n", m.Name)
    }

    // Ищем нашу модель
    modelFound := false
    searchName := cfg.Qwen.ModelName
    searchBase := strings.TrimSuffix(searchName, ":latest")

    for _, m := range tags.Models {
        if m.Name == searchName ||
           m.Name == searchBase ||
           m.Name == searchBase+":latest" ||
           strings.Contains(m.Name, searchBase) {
            modelFound = true
            break
        }
    }

    if !modelFound {
        modelNames := make([]string, len(tags.Models))
        for i, m := range tags.Models {
            modelNames[i] = m.Name
        }
        result.OK = false
        result.Error = fmt.Sprintf("Модель '%s' не найдена. Доступные: %v",
            searchName, modelNames)
        return result
    }

    // 3. Упрощенный тестовый запрос с БОЛЬШИМ таймаутом
    fmt.Println("⏳ Проверка работы модели (может занять несколько секунд)...")

    // Создаем клиент с большим таймаутом для теста модели
    testClient := &http.Client{
        Timeout: 10 * time.Second, // Увеличили до 10 секунд
    }

    // Максимально легкий запрос
    testPayload := map[string]interface{}{
        "model": cfg.Qwen.ModelName,
        "messages": []map[string]string{
            {"role": "user", "content": "test"}, // Одно слово
        },
        "stream": false,
        "options": map[string]interface{}{
            "num_predict": 1,     // Генерировать только 1 токен
            "temperature": 0.0,    // Минимум вычислений
        },
    }

    jsonData, _ := json.Marshal(testPayload)

    start := time.Now()
    testResp, err := testClient.Post("http://localhost:11434/api/chat",
        "application/json", bytes.NewReader(jsonData))

    if err != nil {
        result.OK = false
        result.Error = fmt.Sprintf("Модель не отвечает за %v: %v",
            time.Since(start), err)
        return result
    }
    defer testResp.Body.Close()

    if testResp.StatusCode != http.StatusOK {
        result.OK = false
        result.Error = fmt.Sprintf("Модель вернула ошибку: %d", testResp.StatusCode)
        return result
    }

    fmt.Printf("✅ Модель ответила за %v\n", time.Since(start))
    result.OK = true
    return result
}

// PrintCheckResults выводит результаты проверок
func PrintCheckResults(results []CheckResult) {
    line := strings.Repeat("=", 60)
    fmt.Println("\n" + line)
    fmt.Println("🔍 ПРОВЕРКА СИСТЕМЫ ПЕРЕД ЗАПУСКОМ")
    fmt.Println(line)

    allOK := true
    for _, r := range results {
        status := "✅"
        if !r.OK {
            status = "❌"
            allOK = false
        }

        fmt.Printf("%s %s\n", status, r.Name)
        if r.Error != "" {
            fmt.Printf("   ⚠️  Ошибка: %s\n", r.Error)
        }
        if r.Warning != "" {
            fmt.Printf("   ℹ️  %s\n", r.Warning)
        }
    }

    fmt.Println(strings.Repeat("=", 59))
    if allOK {
        fmt.Println("✅ Все проверки пройдены, можно запускать сервер")
    } else {
        fmt.Println("❌ Есть проблемы, исправьте перед запуском")
    }
    fmt.Println(strings.Repeat("=", 59))
}
