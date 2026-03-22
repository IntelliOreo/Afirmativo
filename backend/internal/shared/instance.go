package shared

import (
	"os"
	"strings"
)

// InstanceMetadata describes the current runtime instance for health and telemetry.
type InstanceMetadata struct {
	ID       string
	Service  string
	Revision string
	Hostname string
}

// CurrentInstanceMetadata resolves instance identity from the current runtime environment.
func CurrentInstanceMetadata() InstanceMetadata {
	return resolveInstanceMetadata(os.Getenv, os.Hostname)
}

func resolveInstanceMetadata(getenv func(string) string, hostnameFn func() (string, error)) InstanceMetadata {
	service := strings.TrimSpace(getenv("K_SERVICE"))
	revision := strings.TrimSpace(getenv("K_REVISION"))
	hostname := strings.TrimSpace(getenv("HOSTNAME"))
	if hostname == "" && hostnameFn != nil {
		if resolved, err := hostnameFn(); err == nil {
			hostname = strings.TrimSpace(resolved)
		}
	}

	id := strings.TrimSpace(getenv("INSTANCE_ID"))
	if id == "" {
		switch {
		case service != "" || revision != "":
			parts := make([]string, 0, 3)
			if service != "" {
				parts = append(parts, service)
			}
			if revision != "" {
				parts = append(parts, revision)
			}
			if hostname != "" {
				parts = append(parts, hostname)
			}
			id = strings.Join(parts, "/")
		case hostname != "":
			id = hostname
		default:
			id = "unknown"
		}
	}

	return InstanceMetadata{
		ID:       id,
		Service:  service,
		Revision: revision,
		Hostname: hostname,
	}
}
