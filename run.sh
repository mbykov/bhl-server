#!/usr/bin/env bash
set -ex

# Проверяем наличие go
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed or not in PATH"
    exit 1
fi

# Очищаем кэш зависимостей
# go clean -modcache

# Загружаем зависимости
echo "Downloading dependencies..."
go mod tidy

# Собираем приложение с тегом ORT для ONNX Runtime
echo "Building application..."
go build -tags ORT -o bhl .

# Запускаем приложение
echo "Starting voice assistant..."
./bhl
