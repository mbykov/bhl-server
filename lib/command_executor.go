package lib

import (
    "bytes"
    "log"
    "os/exec"
    "strings"
    "time"
)

// CommandExecutor выполняет команды
type CommandExecutor struct{}

// NewCommandExecutor создает новый исполнитель команд
func NewCommandExecutor() *CommandExecutor {
    return &CommandExecutor{}
}

// Execute выполняет команду и возвращает результат
func (e *CommandExecutor) Execute(commandName string, originalText string) (string, bool, error) {
    log.Printf("Executing command: %s", commandName)

    switch commandName {
    case "getTime":
        // Заглушка: выполняем команду date
        return e.executeDateCommand()

    // Здесь будут добавляться другие команды:
    // case "latex":
    //     return e.executeLatexCommand(originalText)

    default:
        // Для команд без специальной обработки возвращаем подтверждение
        return "Команда '" + commandName + "' выполнена", false, nil
    }
}

// executeDateCommand выполняет команду date
func (e *CommandExecutor) executeDateCommand() (string, bool, error) {
    // Вариант 1: Использовать системную команду date
    cmd := exec.Command("date")
    var out bytes.Buffer
    cmd.Stdout = &out

    if err := cmd.Run(); err != nil {
        // Если команда date не сработала, используем Go time
        log.Printf("Failed to run date command: %v, using Go time", err)
        currentTime := time.Now().Format("2006-01-02 15:04:05")
        return "Текущее время: " + currentTime, true, nil
    }

    result := strings.TrimSpace(out.String())
    return "Текущее время: " + result, true, nil
}

// executeExternalCommand выполняет внешнюю команду
func (e *CommandExecutor) executeExternalCommand(cmd *exec.Cmd) (string, bool, error) {
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    if err := cmd.Run(); err != nil {
        log.Printf("External command failed: %v, stderr: %s", err, stderr.String())
        return "", false, err
    }

    result := strings.TrimSpace(stdout.String())
    return result, true, nil
}
