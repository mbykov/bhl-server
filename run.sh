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

# Проверяем существование файлов моделей
# check_model_file() {
#     if [ ! -f "$1" ]; then
#         echo "Warning: Model file not found: $1"
#     fi
# }

# echo "Checking model files..."
# check_model_file "../Models/sherpa-onnx-streaming-zipformer-small-ru-vosk-2025-08-16/encoder.onnx"
# check_model_file "../Models/sherpa-onnx-streaming-zipformer-small-ru-vosk-2025-08-16/decoder.onnx"
# check_model_file "../Models/sherpa-onnx-streaming-zipformer-small-ru-vosk-2025-08-16/joiner.onnx"
# check_model_file "../Models/sherpa-onnx-streaming-zipformer-small-ru-vosk-2025-08-16/tokens.txt"

# Запускаем приложение
echo "Starting voice assistant..."
./bhl
# ./voice-assistant \
#   --encoder ../Models/sherpa-onnx-streaming-zipformer-small-ru-vosk-2025-08-16/encoder.onnx \
#   --decoder ../Models/sherpa-onnx-streaming-zipformer-small-ru-vosk-2025-08-16/decoder.onnx \
#   --joiner ../Models/sherpa-onnx-streaming-zipformer-small-ru-vosk-2025-08-16/joiner.onnx \
#   --tokens ../Models/sherpa-onnx-streaming-zipformer-small-ru-vosk-2025-08-16/tokens.txt \
#   --punct-model ../Models/RuPunct/small \
#   --punct-onnx ../Models/rupunct_onnx/RuPunct_small.onnx \
#   --model-type zipformer2 \
#   --port 6006 \
#   --cert ../../../bhl/.cert/combined-tma-cert.pem \
#   --key ../../../bhl/.cert/tma.local+2-key.pem
