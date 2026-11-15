package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

// Validation capabilities never imply support for executing installations.
func ServiceInstallationCapabilities() []string {
	return []string{"service.install.decode.v1", "service.install.plan.v1", "service.egg-catalog.validate.v2"}
}

type ServiceCatalogInstall struct {
	Mode           string `json:"mode"`
	PackageManager string `json:"packageManager"`
}
type ServiceCatalogStartup struct {
	Mode       string  `json:"mode"`
	Entrypoint *string `json:"entrypoint,omitempty"`
}

// ServiceInstallCatalog is the non-secret portion of Panel contract v2.
// Variable values are intentionally not accepted by this descriptor.
type ServiceInstallCatalog struct {
	WorkingDirectory string                `json:"workingDirectory"`
	Install          ServiceCatalogInstall `json:"install"`
	Startup          ServiceCatalogStartup `json:"startup"`
	ProfileID        *string               `json:"profileId"`
	RuntimeVersion   *string               `json:"runtimeVersion"`
}

func DecodeServiceInstallCatalog(data []byte) (ServiceInstallCatalog, error) {
	var value ServiceInstallCatalog
	if len(data) > 16384 {
		return value, errors.New("catalog descriptor is too large")
	}
	if err := RejectDuplicateJSONKeys(data); err != nil {
		return value, errors.New("duplicate catalog descriptor keys")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return ServiceInstallCatalog{}, errors.New("invalid catalog descriptor")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ServiceInstallCatalog{}, errors.New("trailing catalog data")
	}
	if err := ValidateServiceInstallCatalog(value); err != nil {
		return ServiceInstallCatalog{}, err
	}
	return value, nil
}

var catalogEntrypoint = regexp.MustCompile(`^[a-zA-Z0-9_./-]+\.(?:[cm]?js|py)$`)
var catalogProfile = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var catalogRuntime = regexp.MustCompile(`^(?:[0-9]+|3\.(?:11|12|13))$`)

func ValidateServiceInstallCatalog(value ServiceInstallCatalog) error {
	if value.WorkingDirectory != "/app" {
		return errors.New("unsupported catalog working directory")
	}
	node := value.Install.Mode == "nodejs" && value.Install.PackageManager == "npm"
	python := value.Install.Mode == "python" && value.Install.PackageManager == "pip"
	if !node && !python {
		return errors.New("unsupported dependency installer")
	}
	switch value.Startup.Mode {
	case "npm-start":
		if !node || value.Startup.Entrypoint != nil {
			return errors.New("invalid npm startup")
		}
	case "node-file", "python-file":
		if value.Startup.Entrypoint == nil {
			return errors.New("missing startup entrypoint")
		}
		entry := *value.Startup.Entrypoint
		if len(entry) > 160 || strings.HasPrefix(entry, "/") || !catalogEntrypoint.MatchString(entry) {
			return errors.New("invalid startup entrypoint")
		}
		for _, part := range strings.Split(entry, "/") {
			if part == ".." {
				return errors.New("entrypoint escapes working directory")
			}
		}
		if value.Startup.Mode == "python-file" {
			if !python || !strings.HasSuffix(entry, ".py") {
				return errors.New("invalid Python startup")
			}
		} else if !node || strings.HasSuffix(entry, ".py") {
			return errors.New("invalid Node startup")
		}
	default:
		return errors.New("unsupported startup mode")
	}
	if value.ProfileID != nil && !catalogProfile.MatchString(*value.ProfileID) {
		return errors.New("invalid profile ID")
	}
	if value.RuntimeVersion != nil && (len(*value.RuntimeVersion) > 20 || !catalogRuntime.MatchString(*value.RuntimeVersion)) {
		return errors.New("invalid runtime version")
	}
	return nil
}
