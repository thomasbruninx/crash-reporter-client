package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	openapi "github.com/thomasbruninx/crash-reporter-go"
	"gopkg.in/yaml.v3"
)

type Config struct {
	BaseURL      string                 `yaml:"baseurl"`
	Project      string                 `yaml:"project"`
	Instance     string                 `yaml:"instance"`
	Token        string                 `yaml:"token"`
	LogFile      bool                   `yaml:"logfile"`
	AutoRegister bool                   `yaml:"autoregister"`
	Metadata     map[string]interface{} `yaml:"-"`
}

var reservedKeys = map[string]struct{}{
	"baseurl":      {},
	"project":      {},
	"instance":     {},
	"token":        {},
	"logfile":      {},
	"autoregister": {},
	"fields":       {},
}

func main() {
	severity := flag.String("severity", "", "Severity: low|medium|high|critical")
	severityShort := flag.String("s", "", "Severity: low|medium|high|critical")
	data := flag.String("data", "", "Report metadata as JSON object")
	dataShort := flag.String("d", "", "Report metadata as JSON object")
	configPathLong := flag.String("config", "config.yaml", "Path to YAML config file")
	configPathShort := flag.String("c", "", "Path to YAML config file")
	register := flag.Bool("register", false, "Register instance")
	registerShort := flag.Bool("r", false, "Register instance")
	help := flag.Bool("help", false, "Show help")
	helpShort := flag.Bool("h", false, "Show help")
	flag.Parse()

	if *help || *helpShort || len(os.Args) == 1 {
		printHelp()
		return
	}

	selectedConfigPath := strings.TrimSpace(*configPathLong)
	if strings.TrimSpace(*configPathShort) != "" {
		selectedConfigPath = strings.TrimSpace(*configPathShort)
	}
	cfgPath, err := filepath.Abs(selectedConfigPath)
	if err != nil {
		fatalf(nil, "failed to resolve config path: %v", err)
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		fatalf(nil, "failed to load config.yaml: %v", err)
	}
	logger := newLogger(cfg.LogFile)

	doRegister := *register || *registerShort
	if doRegister {
		if err := require(cfg.BaseURL != "" && cfg.Project != "", "register requires baseurl and project in config.yaml"); err != nil {
			fatalf(logger, "%v", err)
		}
		if err := registerInstance(cfg, logger, cfgPath); err != nil {
			fatalf(logger, "register failed: %v", err)
		}
		logger.info("register successful")
		return
	}

	sev := strings.TrimSpace(*severity)
	if sev == "" {
		sev = strings.TrimSpace(*severityShort)
	}
	payload := strings.TrimSpace(*data)
	if payload == "" {
		payload = strings.TrimSpace(*dataShort)
	}
	if sev == "" || payload == "" {
		fatalf(logger, "posting a report requires -s/--severity and -d/--data")
	}
	if !isSeverity(sev) {
		fatalf(logger, "invalid severity: %s", sev)
	}

	metadata := map[string]interface{}{}
	for k, v := range cfg.Metadata {
		metadata[k] = v
	}
	var cliMeta map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &cliMeta); err != nil {
		fatalf(logger, "-d/--data must be a JSON object: %v", err)
	}
	for k, v := range cliMeta {
		metadata[k] = v
	}

	if cfg.AutoRegister && (cfg.Instance == "" || cfg.Token == "") {
		logger.info("autoregister: missing token/instance; attempting register")
		if err := registerInstance(cfg, logger, cfgPath); err != nil {
			fatalf(logger, "autoregister failed: %v", err)
		}
	}

	if err := require(cfg.BaseURL != "" && cfg.Project != "" && cfg.Instance != "" && cfg.Token != "", "baseurl, project, instance, and token are required"); err != nil {
		fatalf(logger, "%v", err)
	}

	logger.info("posting report")
	err = postReport(cfg, sev, metadata)
	if err == nil {
		logger.info("report posted successfully")
		return
	}

	logger.errorf("report post failed: %v", err)
	if cfg.AutoRegister {
		logger.info("autoregister retry: attempting register and retry once")
		if regErr := registerInstance(cfg, logger, cfgPath); regErr != nil {
			fatalf(logger, "autoregister retry register failed: %v", regErr)
		}
		if retryErr := postReport(cfg, sev, metadata); retryErr != nil {
			fatalf(logger, "report retry failed: %v", retryErr)
		}
		logger.info("report posted successfully after autoregister retry")
		return
	}

	fatalf(logger, "report post failed: %v", err)
}

func printHelp() {
	fmt.Println("crash-reporter-client")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  crash-reporter-client -c|--config <path> [other options]")
	fmt.Println("  crash-reporter-client -r|--register")
	fmt.Println("  crash-reporter-client -s|--severity <low|medium|high|critical> -d|--data '<json object>'")
	fmt.Println("  crash-reporter-client -h|--help")
	fmt.Println()
	fmt.Println("Config file: defaults to ./config.yaml, override with -c/--config")
	fmt.Println("Known keys: baseurl, project, instance, token, logfile, autoregister, fields")
	fmt.Println("Metadata format:")
	fmt.Println("  fields:")
	fmt.Println("    - environment: dev")
	fmt.Println("    - host: local-mac")
	fmt.Println("CLI metadata from -d/--data is merged over config metadata")
}

func loadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, err
	}

	cfg := &Config{Metadata: map[string]interface{}{}}
	if v, ok := raw["baseurl"].(string); ok {
		cfg.BaseURL = strings.TrimSpace(v)
	}
	if v, ok := raw["project"].(string); ok {
		cfg.Project = strings.TrimSpace(v)
	}
	if v, ok := raw["instance"].(string); ok {
		cfg.Instance = strings.TrimSpace(v)
	}
	if v, ok := raw["token"].(string); ok {
		cfg.Token = strings.TrimSpace(v)
	}
	if v, ok := raw["logfile"].(bool); ok {
		cfg.LogFile = v
	}
	if v, ok := raw["autoregister"].(bool); ok {
		cfg.AutoRegister = v
	}
	if fieldsRaw, ok := raw["fields"].([]interface{}); ok {
		for _, item := range fieldsRaw {
			fieldMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			for k, v := range fieldMap {
				cfg.Metadata[k] = v
			}
		}
	}

	for k, v := range raw {
		if _, reserved := reservedKeys[k]; !reserved {
			cfg.Metadata[k] = v
		}
	}
	return cfg, nil
}

func saveConfig(path string, cfg *Config) error {
	out := map[string]interface{}{
		"baseurl":      cfg.BaseURL,
		"project":      cfg.Project,
		"instance":     cfg.Instance,
		"token":        cfg.Token,
		"logfile":      cfg.LogFile,
		"autoregister": cfg.AutoRegister,
	}
	if len(cfg.Metadata) > 0 {
		keys := make([]string, 0, len(cfg.Metadata))
		for k := range cfg.Metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fields := make([]map[string]interface{}, 0, len(keys))
		for _, k := range keys {
			fields = append(fields, map[string]interface{}{k: cfg.Metadata[k]})
		}
		out["fields"] = fields
	}
	b, err := yaml.Marshal(out)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func registerInstance(cfg *Config, logger *Logger, cfgPath string) error {
	logger.info("registering instance for project %s", cfg.Project)
	clientCfg := openapi.NewConfiguration()
	clientCfg.Servers = openapi.ServerConfigurations{{URL: strings.TrimRight(cfg.BaseURL, "/")}}
	api := openapi.NewAPIClient(clientCfg)

	req := api.DefaultAPI.CreateInstanceApiV1InstancePost(context.Background()).
		InstanceCreate(openapi.InstanceCreate{ProjectId: cfg.Project})
	resp, _, err := req.Execute()
	if err != nil {
		return err
	}
	cfg.Instance = resp.GetInstanceUuid()
	cfg.Token = resp.GetToken()
	if err := saveConfig(cfgPath, cfg); err != nil {
		return err
	}
	return nil
}

func postReport(cfg *Config, severity string, metadata map[string]interface{}) error {
	clientCfg := openapi.NewConfiguration()
	clientCfg.Servers = openapi.ServerConfigurations{{URL: strings.TrimRight(cfg.BaseURL, "/")}}
	api := openapi.NewAPIClient(clientCfg)

	ctx := context.WithValue(context.Background(), openapi.ContextAccessToken, cfg.Token)
	_, _, err := api.DefaultAPI.CreateReportApiV1ReportPost(ctx).ReportCreate(openapi.ReportCreate{
		InstanceUuid: cfg.Instance,
		Severity:     severity,
		Metadata:     metadata,
	}).Execute()
	return err
}

func isSeverity(s string) bool {
	switch s {
	case "low", "medium", "high", "critical":
		return true
	default:
		return false
	}
}

func require(condition bool, msg string) error {
	if condition {
		return nil
	}
	return errors.New(msg)
}

type Logger struct {
	file *os.File
}

func newLogger(enabled bool) *Logger {
	l := &Logger{}
	if enabled {
		f, err := os.OpenFile("log.txt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			l.file = f
		}
	}
	return l
}

func (l *Logger) line(level string, format string, args ...interface{}) string {
	msg := fmt.Sprintf(format, args...)
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	return fmt.Sprintf("%s %s %s", ts, level, msg)
}

func (l *Logger) info(format string, args ...interface{}) {
	line := l.line("INFO", format, args...)
	fmt.Fprintln(os.Stdout, line)
	if l != nil && l.file != nil {
		fmt.Fprintln(l.file, line)
	}
}

func (l *Logger) errorf(format string, args ...interface{}) {
	line := l.line("ERROR", format, args...)
	fmt.Fprintln(os.Stderr, line)
	if l != nil && l.file != nil {
		fmt.Fprintln(l.file, line)
	}
}

func fatalf(logger *Logger, format string, args ...interface{}) {
	if logger != nil {
		logger.errorf(format, args...)
	} else {
		ts := time.Now().Format("2006-01-02 15:04:05.000")
		fmt.Fprintf(os.Stderr, "%s ERROR %s\n", ts, fmt.Sprintf(format, args...))
	}
	os.Exit(1)
}
