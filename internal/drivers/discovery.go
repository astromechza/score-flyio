package drivers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultScoreDriversFile = `file://.score-drivers.yaml`

func DiscoverDrivers() ([]Driver, error) {
	fileSource := os.Getenv("SCORE_DRIVERS_CONFIG")
	isDefault := fileSource == ""
	if fileSource == "" {
		fileSource = defaultScoreDriversFile
	} else if !strings.Contains(fileSource, "://") {
		if strings.HasPrefix(fileSource, "/") {
			fileSource = "file://" + fileSource
		} else if strings.HasPrefix(fileSource, "~") {
			wd, _ := os.UserHomeDir()
			fileSource = "file://" + strings.Replace(fileSource, "~", wd, 1)
		} else {
			wd, _ := os.Getwd()
			fileSource = "file://" + path.Clean(wd+"/"+fileSource)
		}
	}
	slog.Info(fmt.Sprintf("Discovering resource drivers from %s..", fileSource))

	parsedUri, err := url.Parse(fileSource)
	if err != nil {
		return nil, fmt.Errorf("failed to decode drivers config source: '%s': %w", fileSource, err)
	}

	switch parsedUri.Scheme {
	case "file":
		f, err := os.Open(parsedUri.Path)
		if err != nil {
			if os.IsNotExist(err) && isDefault {
				return []Driver{}, nil
			}
			return nil, fmt.Errorf("failed to read score drivers from '%s': %w", fileSource, err)
		}
		defer f.Close()
		var out []Driver
		if err := yaml.NewDecoder(f).Decode(&out); err != nil {
			return nil, fmt.Errorf("failed to decode score drivers: %w", err)
		}
		return out, nil
	case "http":
		resp, err := http.Get(fileSource)
		if err != nil {
			return nil, fmt.Errorf("failed to make a request to discover drivers on '%s': %w", fileSource, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("request to discovery drivers returned non 200: %d", resp.StatusCode)
		}
		var out []Driver
		if err := yaml.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, fmt.Errorf("failed to decode score drivers: %w", err)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported drivers source type '%s'", parsedUri.Scheme)
	}
}

func ValidateDrivers(drivers []Driver) error {
	driverErrors := make([]error, 0)
	for i, driver := range drivers {
		if driver.Type == "" {
			driverErrors = append(driverErrors, fmt.Errorf("%d: type not defined", i))
		} else if driver.Class == "" {
			driverErrors = append(driverErrors, fmt.Errorf("%d: class not defined, did you mean 'default'", i))
		} else if driver.Uri == "" {
			driverErrors = append(driverErrors, fmt.Errorf("%d: driver uri not defined", i))
		} else if _, err := url.Parse(driver.Uri); err != nil {
			slog.Info("u: " + driver.Uri)
			slog.Info("e: " + err.Error())
			driverErrors = append(driverErrors, fmt.Errorf("%d: failed to parse driver uri: %w", i, err))
		}
	}
	return errors.Join(driverErrors...)
}

func DiscoverAndValidateDrivers() ([]Driver, error) {
	drivers, err := DiscoverDrivers()
	if err != nil {
		return nil, err
	}
	if err := ValidateDrivers(drivers); err != nil {
		return nil, err
	}
	return drivers, nil
}
