package config

import (
	"fmt"
	"strconv"
)

func envInt(key string, defaultVal int) (int, error) {
	value, err := strconv.Atoi(envOr(key, strconv.Itoa(defaultVal)))
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return value, nil
}

func envIntMin(key string, defaultVal, min int) (int, error) {
	value, err := envInt(key, defaultVal)
	if err != nil {
		return 0, err
	}
	if value < min {
		if min == 1 {
			return 0, fmt.Errorf("%s must be > 0", key)
		}
		return 0, fmt.Errorf("%s must be >= %d", key, min)
	}
	return value, nil
}

func envFloat(key string, defaultVal, min, max float64) (float64, error) {
	value, err := strconv.ParseFloat(envOr(key, strconv.FormatFloat(defaultVal, 'f', -1, 64)), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	if value < min || value > max {
		return 0, fmt.Errorf("%s must be between %g and %g", key, min, max)
	}
	return value, nil
}

func envBool(key string, defaultVal bool) (bool, error) {
	value, err := strconv.ParseBool(envOr(key, strconv.FormatBool(defaultVal)))
	if err != nil {
		return false, fmt.Errorf("invalid %s: %w", key, err)
	}
	return value, nil
}
