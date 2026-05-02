## How to build

### 1. Build for android
```bash
GOOS=android GOARCH=arm64 go build -buildmode=pie -o ./build/fileserver_android ./cmd/cli/
```
