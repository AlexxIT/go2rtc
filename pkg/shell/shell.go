package shell

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"

	"github.com/AlexxIT/go2rtc/pkg/yaml"
)

var (
	secretReplacer *strings.Replacer
	secretValues   map[string]bool // Tracker für alle bekannten Secret-Werte
	secretMutex    sync.RWMutex
)

func QuoteSplit(s string) []string {
	var a []string

	for len(s) > 0 {
		switch c := s[0]; c {
		case '\t', '\n', '\r', ' ': // unicode.IsSpace
			s = s[1:]
		case '"', '\'': // quote chars
			if i := strings.IndexByte(s[1:], c); i > 0 {
				a = append(a, s[1:i+1])
				s = s[i+2:]
			} else {
				return nil // error
			}
		default:
			i := strings.IndexAny(s, "\t\n\r ")
			if i > 0 {
				a = append(a, s[:i])
				s = s[i:]
			} else {
				a = append(a, s)
				s = ""
			}
		}
	}

	return a
}

// ReplaceEnvVars - support format ${CAMERA_PASSWORD} and ${RTSP_USER:admin}
func ReplaceEnvVars(text string) string {
	var cfg struct {
		Env     map[string]string            `yaml:"env"`
		Secrets map[string]map[string]string `yaml:"secrets"`
	}

	yaml.Unmarshal([]byte(text), &cfg)

	buildSecretReplacer(cfg)

	re := regexp.MustCompile(`\${([^}{]+)}`)
	return re.ReplaceAllStringFunc(text, func(match string) string {
		key := match[2 : len(match)-1]

		var def string
		var dok bool

		if i := strings.IndexByte(key, ':'); i > 0 {
			key, def = key[:i], key[i+1:]
			dok = true
		}

		if dir, vok := os.LookupEnv("CREDENTIALS_DIRECTORY"); vok {
			value, err := os.ReadFile(filepath.Join(dir, key))
			if err == nil {
				return strings.TrimSpace(string(value))
			}
		}

		if value, vok := os.LookupEnv(key); vok {
			return value
		}

		if cfg.Env != nil {
			if value, ok := cfg.Env[key]; ok {
				return value
			}
		}

		if cfg.Secrets != nil {
			for secretName, secretValues := range cfg.Secrets {
				for k, v := range secretValues {
					name := fmt.Sprintf("%s_%s", secretName, k)
					if key == name {
						return v
					}
				}
			}
		}

		if dok {
			return def
		}

		return match
	})
}

func RunUntilSignal() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	println("exit with signal:", (<-sigs).String())
}

func Redact(text string) string {
	secretMutex.RLock()
	defer secretMutex.RUnlock()
	
	if secretReplacer == nil {
		return text
	}
	
	return secretReplacer.Replace(text)
}

func buildSecretReplacer(cfg struct {
	Env     map[string]string            `yaml:"env"`
	Secrets map[string]map[string]string `yaml:"secrets"`
}) {
	secretMutex.Lock()
	defer secretMutex.Unlock()
	
	if secretValues == nil {
		secretValues = make(map[string]bool)
	}
	
	var newSecrets []string
	
	if dir, ok := os.LookupEnv("CREDENTIALS_DIRECTORY"); ok {
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					value, err := os.ReadFile(filepath.Join(dir, entry.Name()))
					if err == nil {
						cleanValue := strings.TrimSpace(string(value))
						if len(cleanValue) > 0 && !secretValues[cleanValue] {
							secretValues[cleanValue] = true
							newSecrets = append(newSecrets, cleanValue)
						}
					}
				}
			}
		}
	}
	
	if cfg.Env != nil {
		for _, value := range cfg.Env {
			if len(value) > 0 && !secretValues[value] {
				secretValues[value] = true
				newSecrets = append(newSecrets, value)
			}
		}
	}
	
	if cfg.Secrets != nil {
		for _, secretMap := range cfg.Secrets {
			for _, value := range secretMap {
				if len(value) > 0 && !secretValues[value] {
					secretValues[value] = true
					newSecrets = append(newSecrets, value)
				}
			}
		}
	}
	
	if len(newSecrets) > 0 {
		rebuildReplacer()
	}
}

func rebuildReplacer() {
	var replacements []string
	
	for secret := range secretValues {
		replacements = append(replacements, secret, "*****")
	}
	
	if len(replacements) > 0 {
		secretReplacer = strings.NewReplacer(replacements...)
	} else {
		secretReplacer = nil
	}
}