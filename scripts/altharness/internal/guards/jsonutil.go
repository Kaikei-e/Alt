package guards

import "encoding/json"

func jsonUnmarshalString(text string, dest *string) error {
	return json.Unmarshal([]byte(text), dest)
}

func jsonNewDecoder(raw []byte) func(*map[string]any) error {
	return func(dest *map[string]any) error {
		return json.Unmarshal(raw, dest)
	}
}
