# crash-reporter-client

Reference implementation of crash reporter client

## Build

Execute from project root to build client binary for your platform. Binaries will be output to `./bin` directory.

For Windows (x86):
```bash
GOOS=windows GOARCH=386 go build -o ./bin/crash-reporter-client_windows_i386.exe ./cmd/crash-reporter-client
``` 

For Windows (x64):
```bash
GOOS=windows GOARCH=amd64 go build -o ./bin/crash-reporter-client_windows_amd64.exe ./cmd/crash-reporter-client
``` 

For Linux (x64):
```bash
GOOS=linux GOARCH=amd64 go build -o ./bin/crash-reporter-client_linux_amd64.bin ./cmd/crash-reporter-client
``` 

For MacOS (Apple Silicon):
```bash
GOOS=darwin GOARCH=arm64 go build -o ./bin/crash-reporter-client_darwin_arm64.bin ./cmd/crash-reporter-client
```

## Usage

```bash
./crash-reporter-client -h
./crash-reporter-client -r
./crash-reporter-client -c "path/to/config.yaml"
./crash-reporter-client -s critical -d '{"message":"panic"}'
```

## Configuration
The client reads its configuration from `config.yaml` in the current working directory by default. You can specify a different configuration file using the `-c` flag.

Example `config.yaml`:
```yaml
baseurl: https://api.reports.com    # Backend API URL
project: my_project                 # Project name
instance: ""                        # Instance name (filled in by client during registration)
token: ""                           # Authentication token (filled in by client during registration)
logfile: false                      # Enable/disable logging to file
autoregister: false                 # Enable/disable automatic registration on startup and retry on failure
fields:                             # Custom fields to include in crash reports
  - host: test-machine
  - service: web
```
