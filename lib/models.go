package lib


// Файл lib/models.go сейчас не нужен, так как мы не используем hugot для пунктуации. Его можно удалить или заменить на минимальную реализацию:

import (
    "log"
    "github.com/knights-analytics/hugot"
)

// ModelManager управляет моделями для классификации команд (через hugot)
type ModelManager struct {
    Session *hugot.Session
    // Будет использоваться позже для классификации команд
}

func NewModelManager() (*ModelManager, error) {
    // Создаем сессию ONNX Runtime (ORT) только для классификации команд
    session, err := hugot.NewORTSession()
    if err != nil {
        return nil, err
    }
    log.Println("Hugot ORT session created successfully (for command classification)")

    return &ModelManager{
        Session: session,
    }, nil
}

// InitCommandClassifier будет инициализировать модель классификации команд
func (m *ModelManager) InitCommandClassifier(modelPath string) error {
    log.Printf("Command classifier model will be loaded from: %s (future implementation)", modelPath)
    return nil
}

func (m *ModelManager) Destroy() {
    if m.Session != nil {
        if err := m.Session.Destroy(); err != nil {
            log.Printf("Error destroying hugot session: %v", err)
        }
    }
}
