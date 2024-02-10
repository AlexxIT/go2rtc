package app

import (
	"bytes"
	"github.com/AlexxIT/go2rtc/pkg/expr"
	"log"
	"os"
	"reflect"
	"testing"
)

// TestLoadConfig tests the LoadConfig function.
func TestLoadConfig(t *testing.T) {
	// Redirect log output to buffer for testing.
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	type Config struct {
		Key1 string `yaml:"key1"`
		Key2 string `yaml:"key2"`
		Key3 string `yaml:"key3"`
	}

	tests := []struct {
		name        string
		configs     [][]byte
		want        Config
		expectError bool
	}{
		{
			name: "Valid configs",
			configs: [][]byte{
				[]byte("key1: value1\nkey2: value2"),
				[]byte("key3: value3"),
			},
			want: Config{
				Key1: "value1",
				Key2: "value2",
				Key3: "value3",
			},
			expectError: false,
		},
		{
			name: "Invalid config",
			configs: [][]byte{
				[]byte("key1: value1"),
				[]byte("invalid_yaml"),
			},
			want: Config{
				Key1: "value1",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the configs with the test case's configs.
			configs = tt.configs

			var got Config
			LoadConfig(&got)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s: LoadConfig() got = %v, want %v", tt.name, got, tt.want)
			}

			containsError := bytes.Contains(buf.Bytes(), []byte("read config"))
			if containsError != tt.expectError {
				t.Errorf("%s: LoadConfig() expected error = %v, but got = %v", tt.name, tt.expectError, containsError)
			}

			// Clear buffer after each test case.
			buf.Reset()
		})
	}
}

// Test_processConfig tests the processConfig function.
func Test_processConfig(t *testing.T) {
	type args struct {
		data []byte
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name:    "No expressions",
			args:    args{data: []byte("config: value")},
			want:    []byte("config: value"),
			wantErr: false,
		},
		{
			name:    "Simple expression",
			args:    args{data: []byte(`config: ${{ "value" }}`)},
			want:    []byte("config: value"),
			wantErr: false,
		},
		{
			name:    "Math expression",
			args:    args{data: []byte(`config: ${{ 2+2 }}`)},
			want:    []byte("config: 4"),
			wantErr: false,
		},
		{
			name:    "Invalid expression",
			args:    args{data: []byte(`config: ${{ invalid }}`)},
			want:    []byte(`config: ${{ invalid }}`),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expr.ProcessConfig(tt.args.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("%s: processConfig() error = %v, wantErr %v", tt.name, err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s: processConfig() got = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
