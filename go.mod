module bhl-diary

go 1.25.6

replace github.com/mbykov/asr-zipformer-go => ../asr-zipformer-go

replace github.com/mbykov/wshandler-go => ../wshandler-go

require (
	github.com/mbykov/asr-zipformer-go v0.0.0-00010101000000-000000000000
	github.com/mbykov/wshandler-go v0.0.0-00010101000000-000000000000
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/k2-fsa/sherpa-onnx-go v1.12.34 // indirect
	github.com/k2-fsa/sherpa-onnx-go-linux v1.12.34 // indirect
	github.com/k2-fsa/sherpa-onnx-go-macos v1.12.34 // indirect
	github.com/k2-fsa/sherpa-onnx-go-windows v1.12.34 // indirect
)
