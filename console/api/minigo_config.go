// Copyright 2026 Minigo contributors. Licensed under AGPL-3.0-or-later.
package api

import (
	"net/http"
	"regexp"
	"sort"
	"strings"

	madmin "github.com/minio/madmin-go/v3"
)

// Only non-secret operational settings are exposed. Credential, identity,
// notification, root-access, KMS and replication target configuration remain
// available through mc. Never serialize an entire server configuration.
var minigoConfigKeys = map[string][]string{
	"region":  {"name"},
	"scanner": {"speed"},
	"heal":    {"bitrotscan", "max_sleep", "max_io", "drive_workers"},
	"api":     {"requests_max", "cluster_deadline", "remote_transport_deadline", "list_quorum", "replication_priority", "replication_max_workers", "replication_max_lrg_workers", "transition_workers", "stale_uploads_cleanup_interval", "stale_uploads_expiry", "delete_cleanup_interval", "odirect", "gzip_objects", "sync_events", "object_max_versions"},
}

var minigoConfigValue = regexp.MustCompile(`^[A-Za-z0-9._:+,\-]*$`)

func minigoConfig(r *http.Request, admin *madmin.AdminClient, parts []string) (any, error) {
	if len(parts) == 0 && r.Method == "GET" {
		// HelpConfigKV performs the server's config permission check even for
		// the subsystem list. This endpoint does not infer privileges locally.
		if _, err := admin.HelpConfigKV(r.Context(), "", "", false); err != nil {
			return nil, err
		}
		return map[string]any{"subsystems": []string{"api", "region", "scanner", "heal"}}, nil
	}
	if len(parts) != 1 {
		return nil, minigoRouteError(len(parts) > 1)
	}
	subsystem := parts[0]
	keys, found := minigoConfigKeys[subsystem]
	if !found {
		return nil, minigoInvalid("This configuration subsystem is not exposed in the console. Use mc admin config.")
	}
	allowed := make(map[string]bool, len(keys))
	for _, k := range keys {
		allowed[k] = true
	}
	switch r.Method {
	case "GET":
		data, err := admin.GetConfigKV(r.Context(), subsystem)
		if err != nil {
			return nil, err
		}
		values, err := minigoParseConfig(string(data), subsystem, allowed)
		if err != nil {
			return nil, err
		}
		help, err := admin.HelpConfigKV(r.Context(), subsystem, "", false)
		if err != nil {
			return nil, err
		}
		filtered := make(madmin.HelpKVS, 0, len(help.KeysHelp))
		for _, h := range help.KeysHelp {
			if allowed[h.Key] {
				filtered = append(filtered, h)
			}
		}
		help.KeysHelp = filtered
		return map[string]any{"subsystem": subsystem, "values": values, "help": help}, nil
	case "PUT":
		var body struct {
			Values map[string]string `json:"values"`
		}
		if err := minigoDecode(r, &body); err != nil {
			return nil, err
		}
		kv, err := minigoConfigUpdate(subsystem, body.Values, allowed)
		if err != nil {
			return nil, err
		}
		restart, err := admin.SetConfigKV(r.Context(), kv)
		return map[string]any{"restartRequired": restart}, err
	}
	return nil, &minigoError{405, "MethodNotAllowed", "This method is not supported."}
}

func minigoConfigUpdate(subsystem string, values map[string]string, allowed map[string]bool) (string, error) {
	if len(values) == 0 {
		return "", minigoInvalid("Supply at least one supported setting.")
	}
	keys := make([]string, 0, len(values))
	for key, value := range values {
		if !allowed[key] || len(value) > 256 || !minigoConfigValue.MatchString(value) {
			return "", minigoInvalid("Unsupported setting or value. Values may contain only letters, digits, and . _ : + , -.")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var text strings.Builder
	text.WriteString(subsystem)
	for _, key := range keys {
		text.WriteByte(' ')
		text.WriteString(key)
		text.WriteString("=\"")
		text.WriteString(values[key])
		text.WriteByte('"')
	}
	return text.String(), nil
}

// Tokenize the server's key=value format without interpreting shell syntax.
// In particular, keys mentioned inside a quoted comment are never exposed.
func minigoParseConfig(data, subsystem string, allowed map[string]bool) (map[string]string, error) {
	var tokens []string
	var token strings.Builder
	var quote rune
	escaped := false
	for _, ch := range data {
		if escaped {
			token.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else {
				token.WriteRune(ch)
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			if token.Len() > 0 {
				tokens = append(tokens, token.String())
				token.Reset()
			}
			continue
		}
		token.WriteRune(ch)
	}
	if quote != 0 || escaped {
		return nil, &minigoError{502, "InvalidUpstreamResponse", "The server returned an unsupported configuration format."}
	}
	if token.Len() > 0 {
		tokens = append(tokens, token.String())
	}
	if len(tokens) == 0 || tokens[0] != subsystem {
		return nil, &minigoError{502, "InvalidUpstreamResponse", "The server returned an unsupported configuration format."}
	}
	values := make(map[string]string)
	for _, item := range tokens[1:] {
		key, value, ok := strings.Cut(item, "=")
		if ok && allowed[key] && minigoConfigValue.MatchString(value) {
			values[key] = value
		}
	}
	return values, nil
}
