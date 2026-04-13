package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/sipeed/picoclaw/pkg/credential"
)

func TestLoadSecurityValue(t *testing.T) {
	type valueStruct struct {
		Url     string        `json:"url,omitempty"      yaml:"-"`
		Token   *SecureString `json:"token,omitempty"    yaml:"token,omitempty"    env:"PICO_TOKEN"`
		ApiKeys SecureStrings `json:"api_keys,omitempty" yaml:"api_keys,omitempty" env:"PICO_API_KEYS"`
	}

	type testStruct struct {
		Pico *valueStruct `json:"pico,omitempty" yaml:"pico,omitempty"`
	}

	v1 := &testStruct{
		Pico: &valueStruct{
			Url:     "https://example.com",
			Token:   NewSecureString("token1"),
			ApiKeys: SecureStrings{NewSecureString("api-key1"), NewSecureString("api-key2")},
		},
	}
	bytes, err := yaml.Marshal(v1)
	assert.NoError(t, err)
	jsonBytes, err := json.Marshal(v1)
	assert.NoError(t, err)
	const want = `pico:
    token: token1
    api_keys:
        - api-key1
        - api-key2
`
	const jsonPost = `{"pico":{"url":"https://example.com","token":"token0"}}`
	v0 := &testStruct{}
	err = json.Unmarshal([]byte(jsonPost), v0)
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com", v0.Pico.Url)
	assert.Equal(t, "token0", v0.Pico.Token.String())

	const jsonWant = `{"pico":{"url":"https://example.com","token":"[NOT_HERE]","api_keys":"[NOT_HERE]"}}`
	assert.Equal(t, want, string(bytes))
	assert.Equal(t, jsonWant, string(jsonBytes))

	v2 := &testStruct{}
	err = json.Unmarshal(jsonBytes, v2)
	assert.NoError(t, err)
	err = yaml.Unmarshal(bytes, v2)
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com", v2.Pico.Url)
	if v2.Pico.Token != nil {
		assert.Equal(t, "token1", v2.Pico.Token.String())
		assert.Equal(t, "token1", v2.Pico.Token.raw)
	}

	v2.Pico.Token = NewSecureString("token1")
	v2.Pico.Token.raw = "abc"
	err = yaml.Unmarshal(bytes, v2)
	assert.NoError(t, err)
	assert.Equal(t, "token1", v2.Pico.Token.raw)

	os.Setenv("PICO_TOKEN", "token_env")
	err = env.Parse(v2)
	assert.NoError(t, err)
	assert.NotNil(t, v2.Pico.Token)
	assert.Equal(t, "token1", v2.Pico.Token.String())

	v3 := &testStruct{Pico: &valueStruct{}}
	err = env.Parse(v3)
	assert.NoError(t, err)
	if v3.Pico.Token != nil {
		assert.Equal(t, "token_env", v3.Pico.Token.String())
	}

	type toolsStruct struct {
		Pico valueStruct `json:"pico,omitempty" yaml:"pico,omitempty"`
	}

	type testStruct2 struct {
		Tools toolsStruct `json:"tools,omitempty" yaml:",inline"`
	}

	v4 := &testStruct2{
		Tools: toolsStruct{
			Pico: valueStruct{
				Url:     "https://example.com",
				Token:   NewSecureString("token1"),
				ApiKeys: SecureStrings{NewSecureString("api-key1"), NewSecureString("api-key2")},
			},
		},
	}
	bytes, err = yaml.Marshal(v4)
	assert.NoError(t, err)
	assert.Equal(t, want, string(bytes))
	jsonBytes, err = json.Marshal(v4)
	assert.NoError(t, err)
	assert.Equal(
		t,
		`{"tools":{"pico":{"url":"https://example.com","token":"[NOT_HERE]","api_keys":"[NOT_HERE]"}}}`,
		string(jsonBytes),
	)

	v5 := &testStruct2{}
	err = json.Unmarshal(jsonBytes, v5)
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com", v5.Tools.Pico.Url)
	err = yaml.Unmarshal(bytes, v5)
	assert.NoError(t, err)
	assert.NotNil(t, v5.Tools.Pico.Token)
	assert.Equal(t, "token1", v5.Tools.Pico.Token.raw)

	dir := t.TempDir()
	sshKeyPath := filepath.Join(dir, "codex-claw_ed25519.key")
	if err = os.WriteFile(sshKeyPath, []byte("fake-ssh-key-material\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	const passphrase = "test-passphrase-32bytes-long-ok!"

	t.Setenv(credential.SSHKeyPathEnvVar, sshKeyPath)

	t.Setenv(credential.PassphraseEnvVar, passphrase)

	v5.Tools.Pico.Token.Set("newtoken1")
	v5.Tools.Pico.ApiKeys[0].Set("newapi-key1")
	bytes, err = yaml.Marshal(v5)
	assert.NoError(t, err)
	t.Logf("yaml: %s", string(bytes))

	v6 := &testStruct2{}
	err = yaml.Unmarshal(bytes, v6)
	assert.NoError(t, err)
	assert.NotNil(t, v6.Tools.Pico.Token)
	assert.Equal(t, "newtoken1", v6.Tools.Pico.Token.String())
}

func TestConfigStructs_DoNotExposeLegacyEnvPrefixes(t *testing.T) {
	const legacyEnvPrefix = "PICO" + "CLAW_"

	var bad []string
	visited := map[reflect.Type]bool{}

	var walk func(reflect.Type, string)
	walk = func(rt reflect.Type, path string) {
		for {
			switch rt.Kind() {
			case reflect.Pointer, reflect.Slice, reflect.Array:
				rt = rt.Elem()
			case reflect.Map:
				walk(rt.Elem(), path+"{}")
				return
			default:
				goto done
			}
		}
	done:
		if rt.Kind() != reflect.Struct || visited[rt] {
			return
		}
		visited[rt] = true

		for i := range rt.NumField() {
			field := rt.Field(i)
			fieldPath := path + "." + field.Name
			if envTag := field.Tag.Get("env"); strings.HasPrefix(envTag, legacyEnvPrefix) {
				bad = append(bad, fieldPath+" env="+envTag)
			}
			if envPrefix := field.Tag.Get("envPrefix"); strings.HasPrefix(envPrefix, legacyEnvPrefix) {
				bad = append(bad, fieldPath+" envPrefix="+envPrefix)
			}
			walk(field.Type, fieldPath)
		}
	}

	walk(reflect.TypeOf(Config{}), "Config")
	walk(reflect.TypeOf(GatewayConfig{}), "GatewayConfig")

	if len(bad) > 0 {
		t.Fatalf("legacy PicoClaw env tags remain:\n%s", strings.Join(bad, "\n"))
	}
}
