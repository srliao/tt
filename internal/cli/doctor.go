package cli

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/srliao/tt/internal/ttclient"
)

type doctorConfig struct {
	Path      string            `json:"path"`
	FileFound bool              `json:"file_found"`
	FileMode  string            `json:"file_mode,omitempty"`
	URL       string            `json:"url"`
	Sources   map[string]string `json:"sources"`
	CFAccess  bool              `json:"cf_access"`
}

type doctorServer struct {
	Reachable bool   `json:"reachable"`
	AuthOK    bool   `json:"auth_ok"`
	Version   string `json:"version,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
	Time      string `json:"time,omitempty"`
}

type doctorReport struct {
	OK       bool         `json:"ok"`
	Config   doctorConfig `json:"config"`
	Server   doctorServer `json:"server"`
	Warnings []string     `json:"warnings"`
}

func newDoctorCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check config, connectivity and auth",
		Long: `Check where the config came from, that the server is reachable and the
credentials are accepted, and report the server's version, time zone and
clock. Run this first when any other command fails with exit code 4.

Output: {ok: true,
 config: {path, file_found, file_mode?, url, sources: {field: "env"|"file"}, cf_access: bool},
 server: {reachable, auth_ok, version, timezone, time},
 warnings: [string]}
On failure the same report is in the stderr error's details.`,
		Example: `  tt-cli doctor`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := s.config()
			if err != nil {
				return err
			}
			rep := doctorReport{
				Config: doctorConfig{
					Path:      cfg.Path,
					FileFound: cfg.FileFound,
					URL:       cfg.URL,
					Sources:   cfg.Source,
					CFAccess:  cfg.CFClientID != "" && cfg.CFClientSecret != "",
				},
				Warnings: []string{},
			}
			if cfg.FileFound {
				rep.Config.FileMode = fmt.Sprintf("%04o", cfg.FileMode)
				if cfg.FileMode&0o077 != 0 {
					rep.Warnings = append(rep.Warnings, fmt.Sprintf(
						"config file mode is %04o; it may hold a secret, run: chmod 0600 %s", cfg.FileMode, cfg.Path))
				}
			}
			if (cfg.CFClientID == "") != (cfg.CFClientSecret == "") {
				rep.Warnings = append(rep.Warnings, "only one of cf_client_id / cf_client_secret is set; no auth headers are sent")
			}

			c, err := s.api()
			if err != nil {
				return withReport(err, rep)
			}
			var v struct {
				Version  string `json:"version"`
				Timezone string `json:"timezone"`
			}
			hdr, err := c.Do(cmd.Context(), http.MethodGet, "/version", nil, nil, &v)
			if err != nil {
				var ae *ttclient.APIError
				rep.Server.Reachable = errors.As(err, &ae) && ae.Status != 0
				return withReport(err, rep)
			}
			rep.Server = doctorServer{Reachable: true, AuthOK: true, Version: v.Version, Timezone: v.Timezone}
			if t, err := http.ParseTime(hdr.Get("Date")); err == nil {
				rep.Server.Time = t.UTC().Format(time.RFC3339)
			}
			rep.OK = true
			return s.out(rep)
		},
	}
}

// withReport attaches the doctor report to a failure's details.
func withReport(err error, rep doctorReport) error {
	e := asError(err)
	details := map[string]any{}
	maps.Copy(details, e.Details)
	details["config"] = rep.Config
	details["server"] = rep.Server
	details["warnings"] = rep.Warnings
	return &Error{Code: e.Code, Message: e.Message, Details: details, Exit: e.Exit}
}
