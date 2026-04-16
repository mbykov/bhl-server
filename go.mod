module bhl-diary

go 1.25.6

replace github.com/mbykov/asr-zipformer-go => ../asr-zipformer-go

replace github.com/mbykov/wshandler-go => ../wshandler-go

replace github.com/mbykov/vosk-punct => ../vosk-punct

require (
	github.com/mbykov/asr-zipformer-go v0.0.0-00010101000000-000000000000
	github.com/mbykov/vosk-punct v0.0.0-00010101000000-000000000000
	github.com/mbykov/wshandler-go v0.0.0-00010101000000-000000000000
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/Hank-Kuo/go-bert-tokenizer v1.0.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/k2-fsa/sherpa-onnx-go v1.12.34 // indirect
	github.com/k2-fsa/sherpa-onnx-go-linux v1.12.34 // indirect
	github.com/k2-fsa/sherpa-onnx-go-macos v1.12.34 // indirect
	github.com/k2-fsa/sherpa-onnx-go-windows v1.12.34 // indirect
	github.com/yalue/onnxruntime_go v1.27.0 // indirect
	golang.org/x/text v0.25.0 // indirect
)
