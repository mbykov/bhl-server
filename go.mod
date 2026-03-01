module bhl-server

go 1.25.6

require (
	github.com/gorilla/websocket v1.5.0
	github.com/mbykov/bhl-vosk-sherpa-go v0.0.0-00010101000000-000000000000
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/k2-fsa/sherpa-onnx-go v1.12.27 // indirect
	github.com/k2-fsa/sherpa-onnx-go-linux v1.12.27 // indirect
	github.com/k2-fsa/sherpa-onnx-go-macos v1.12.27 // indirect
	github.com/k2-fsa/sherpa-onnx-go-windows v1.12.27 // indirect
)

replace github.com/mbykov/bhl-vosk-sherpa-go => ../bhl-vosk-sherpa-go
