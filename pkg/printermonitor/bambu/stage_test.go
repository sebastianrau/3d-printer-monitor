package bambu

import "testing"

func TestStageName(t *testing.T) {
	tests := map[any]string{
		float64(2): "Heating bed",
		"14":       "Cleaning nozzle tip",
		0:          "",
		255:        "",
		999:        "",
	}
	for value, want := range tests {
		if got := StageName(value); got != want {
			t.Errorf("StageName(%v) = %q, want %q", value, got, want)
		}
	}
}
