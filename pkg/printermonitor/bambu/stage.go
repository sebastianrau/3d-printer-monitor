package bambu

import "fmt"

var stageNames = map[int]string{
	1: "Auto bed leveling", 2: "Heating bed", 3: "Vibration compensation",
	4: "Changing filament", 5: "M400 pause", 6: "Filament runout pause",
	7: "Heating hotend", 8: "Calibrating extrusion", 9: "Scanning bed surface",
	10: "Inspecting first layer", 11: "Identifying build plate", 12: "Calibrating micro lidar",
	13: "Homing toolhead", 14: "Cleaning nozzle tip", 15: "Checking extruder temperature",
	16: "Paused by user", 17: "Front cover falling", 18: "Calibrating micro lidar",
	19: "Calibrating extrusion flow", 20: "Nozzle temperature malfunction", 21: "Bed temperature malfunction",
	22: "Unloading filament", 23: "Paused: skipped step", 24: "Loading filament",
	25: "Calibrating motor noise", 26: "Paused: AMS lost", 27: "Paused: low fan speed",
	28: "Chamber temperature control error", 29: "Cooling chamber", 30: "Paused by G-code",
	31: "Motor noise calibration", 32: "Paused: nozzle filament covered", 33: "Paused: cutter error",
	34: "Paused: first layer error", 35: "Paused: nozzle clog", 36: "Checking absolute accuracy",
	37: "Absolute accuracy calibration", 38: "Checking absolute accuracy", 39: "Calibrating nozzle offset",
	40: "High-temperature bed leveling", 41: "Checking quick release", 42: "Checking door and cover",
	43: "Laser calibration", 44: "Checking platform", 45: "Checking camera position",
	46: "Calibrating camera", 47: "Bed leveling phase 1", 48: "Bed leveling phase 2",
	49: "Heating chamber", 50: "Cooling bed", 51: "Printing calibration lines",
	52: "Checking material", 53: "Live-view camera calibration", 54: "Waiting for bed temperature",
	55: "Checking material position", 56: "Cutting module offset calibration", 57: "Measuring surface",
	58: "Thermal preconditioning", 59: "Homing blade holder", 60: "Calibrating camera offset",
	61: "Calibrating blade holder", 62: "Hotend pick-and-place test", 63: "Waiting for chamber temperature",
	64: "Preparing hotend", 65: "Calibrating nozzle clump detection", 66: "Purifying chamber air",
	77: "Preparing AMS",
}

// StageName translates Bambu's MQTT stg_cur code. Empty means that the code is
// printing/idle or is not known to this version of the monitor.
func StageName(value any) string {
	var code int
	switch typed := value.(type) {
	case int:
		code = typed
	case float64:
		code = int(typed)
	case string:
		if _, err := fmt.Sscan(typed, &code); err != nil {
			return ""
		}
	default:
		return ""
	}
	return stageNames[code]
}
