module bhl-server

go 1.25.6

require (
	github.com/gorilla/websocket v1.5.3
	github.com/mbykov/bhl-gigaam-go v0.0.0-00010101000000-000000000000
	github.com/mbykov/bhl-vosk-sherpa-go v0.0.0-00010101000000-000000000000
	github.com/mbykov/command-go-levenshtein v0.0.0-00010101000000-000000000000
	github.com/yalue/onnxruntime_go v1.27.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/k2-fsa/sherpa-onnx-go v1.12.27 // indirect
	github.com/k2-fsa/sherpa-onnx-go-linux v1.12.27 // indirect
	github.com/k2-fsa/sherpa-onnx-go-macos v1.12.27 // indirect
	github.com/k2-fsa/sherpa-onnx-go-windows v1.12.27 // indirect
	github.com/madelynnblue/go-dsp v1.0.0 // indirect
)

replace (
	github.com/mbykov/bhl-gigaam-go => ../bhl-gigaam-go
	github.com/mbykov/bhl-vosk-sherpa-go => ../bhl-vosk-sherpa-go
	github.com/mbykov/command-go-levenshtein => ../command-go-levenshtein
)
