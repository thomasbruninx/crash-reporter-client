# crash-reporter-client

Reference implementation of crash reporter client

## Build

```bash
go build ./cmd/crash-reporter-client
```

## Usage

```bash
./crash-reporter-client -h
./crash-reporter-client -r
./crash-reporter-client -s critical -d '{"message":"panic"}'
```

Config file is `config.yaml` in current working directory.
