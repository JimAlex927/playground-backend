package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"gopkg.in/yaml.v3"
)

var (
	nacosMu        sync.RWMutex
	nacosOverrides = map[string]string{}
)

type NacosLoadResult struct {
	Enabled   bool
	Loaded    bool
	Server    string
	DataID    string
	Group     string
	ItemCount int
}

func LoadNacosOverrides() (NacosLoadResult, error) {
	source := readNacosSourceConfig()
	result := NacosLoadResult{
		Enabled: source.Enabled,
		Server:  strings.Join(source.ServerAddrs, ","),
		DataID:  source.DataID,
		Group:   source.Group,
	}
	if !source.Enabled {
		return result, nil
	}

	serverConfigs, err := buildNacosServerConfigs(source.ServerAddrs)
	if err != nil {
		return result, err
	}

	clientConfig := constant.ClientConfig{
		NamespaceId:         source.NamespaceID,
		TimeoutMs:           uint64(source.Timeout.Milliseconds()),
		NotLoadCacheAtStart: true,
		LogDir:              filepath.Clean(source.LogDir),
		CacheDir:            filepath.Clean(source.CacheDir),
		LogLevel:            "info",
		Username:            source.Username,
		Password:            source.Password,
	}

	configClient, err := clients.NewConfigClient(vo.NacosClientParam{
		ClientConfig:  &clientConfig,
		ServerConfigs: serverConfigs,
	})
	if err != nil {
		return result, fmt.Errorf("create nacos config client: %w", err)
	}

	content, err := configClient.GetConfig(vo.ConfigParam{
		DataId: source.DataID,
		Group:  source.Group,
	})
	if err != nil {
		return result, fmt.Errorf("load nacos config dataId=%s group=%s: %w", source.DataID, source.Group, err)
	}

	values, err := parseNacosContent(source.Format, content)
	if err != nil {
		return result, err
	}

	nacosMu.Lock()
	nacosOverrides = values
	nacosMu.Unlock()

	result.Loaded = true
	result.ItemCount = len(values)
	return result, nil
}

func lookupValue(key string) (string, bool) {
	if value, ok := os.LookupEnv(key); ok {
		return value, true
	}

	nacosMu.RLock()
	defer nacosMu.RUnlock()
	value, ok := nacosOverrides[key]
	return value, ok
}

func readNacosSourceConfig() NacosConfig {
	return NacosConfig{
		Enabled:     readRawBool("PLAYGROUND_NACOS_ENABLED", false),
		ServerAddrs: readRawList("PLAYGROUND_NACOS_SERVER_ADDRS", []string{"http://127.0.0.1:8848"}),
		NamespaceID: readRawString("PLAYGROUND_NACOS_NAMESPACE_ID", ""),
		Group:       readRawString("PLAYGROUND_NACOS_GROUP", "DEFAULT_GROUP"),
		DataID:      readRawString("PLAYGROUND_NACOS_DATA_ID", "playground-backend.properties"),
		Username:    readRawString("PLAYGROUND_NACOS_USERNAME", "nacos"),
		Password:    readRawString("PLAYGROUND_NACOS_PASSWORD", "nacos"),
		Format:      strings.ToLower(readRawString("PLAYGROUND_NACOS_FORMAT", "properties")),
		Timeout:     readRawDuration("PLAYGROUND_NACOS_TIMEOUT", 5*time.Second),
		LogDir:      readRawString("PLAYGROUND_NACOS_LOG_DIR", "storage/nacos/log"),
		CacheDir:    readRawString("PLAYGROUND_NACOS_CACHE_DIR", "storage/nacos/cache"),
	}
}

func readRawString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func readRawBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func readRawDuration(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func readRawList(key string, fallback []string) []string {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	if len(items) == 0 {
		return fallback
	}
	return items
}

func buildNacosServerConfigs(addrs []string) ([]constant.ServerConfig, error) {
	serverConfigs := make([]constant.ServerConfig, 0, len(addrs))
	for _, raw := range addrs {
		host, port, err := parseServerAddr(raw)
		if err != nil {
			return nil, err
		}
		serverConfigs = append(serverConfigs, constant.ServerConfig{
			IpAddr: host,
			Port:   port,
		})
	}
	return serverConfigs, nil
}

func parseServerAddr(raw string) (string, uint64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", 0, fmt.Errorf("nacos server address is empty")
	}

	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", 0, fmt.Errorf("parse nacos server addr %q: %w", raw, err)
		}
		trimmed = parsed.Host
	}

	host, portText, ok := strings.Cut(trimmed, ":")
	if !ok || strings.TrimSpace(host) == "" || strings.TrimSpace(portText) == "" {
		return "", 0, fmt.Errorf("nacos server addr %q must be host:port", raw)
	}

	port, err := strconv.ParseUint(strings.TrimSpace(portText), 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("parse nacos port from %q: %w", raw, err)
	}

	return strings.TrimSpace(host), port, nil
}

func parseNacosContent(format, content string) (map[string]string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "properties", "props":
		return parseProperties(content), nil
	case "yaml", "yml":
		return parseStructuredMap(content, true)
	case "json":
		return parseStructuredMap(content, false)
	default:
		return nil, fmt.Errorf("unsupported nacos config format %q", format)
	}
}

func parseProperties(content string) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}

		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			key, value, ok = strings.Cut(trimmed, ":")
			if !ok {
				continue
			}
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" {
			values[key] = value
		}
	}
	return values
}

func parseStructuredMap(content string, yamlFormat bool) (map[string]string, error) {
	decoded := make(map[string]any)
	var err error
	if yamlFormat {
		err = yaml.Unmarshal([]byte(content), &decoded)
	} else {
		err = json.Unmarshal([]byte(content), &decoded)
	}
	if err != nil {
		return nil, fmt.Errorf("parse nacos structured config: %w", err)
	}

	values := make(map[string]string, len(decoded))
	for key, value := range decoded {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		values[trimmedKey] = stringifyNacosValue(value)
	}
	return values, nil
}

func stringifyNacosValue(value any) string {
	switch value := value.(type) {
	case nil:
		return ""
	case string:
		return value
	case bool:
		return strconv.FormatBool(value)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprint(value)
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			parts = append(parts, stringifyNacosValue(item))
		}
		return strings.Join(parts, ",")
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	}
}
