package config

import (
	"encoding/json"
	"testing"
)

func TestUnmarshalDefaultsAutoFetchAudio(t *testing.T) {
	var c Config
	if err := json.Unmarshal([]byte(`{"theme":"dark"}`), &c); err != nil {
		t.Fatal(err)
	}
	if !c.AutoFetchAudio {
		t.Fatal("expected AutoFetchAudio to default to true when the key is absent")
	}
}

func TestUnmarshalAutoFetchAudioFalse(t *testing.T) {
	var c Config
	if err := json.Unmarshal([]byte(`{"auto_fetch_audio":false}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.AutoFetchAudio {
		t.Fatal("expected AutoFetchAudio to be false")
	}
}

func TestDefaultsAutoFetchAudio(t *testing.T) {
	if !Defaults().AutoFetchAudio {
		t.Fatal("expected Defaults().AutoFetchAudio to be true")
	}
}
