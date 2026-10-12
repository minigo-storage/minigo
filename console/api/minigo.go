// Copyright 2026 Minigo contributors. Licensed under AGPL-3.0-or-later.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/minio/console/models"
	"github.com/minio/console/pkg"
	"github.com/minio/console/pkg/auth"
	"github.com/minio/console/pkg/utils"
	madmin "github.com/minio/madmin-go/v3"
	minio "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
	"github.com/minio/minio-go/v7/pkg/replication"
)

const minigoAPIPrefix = "/api/v1/minigo/"
const minigoMaxBody = 1 << 20

type minigoError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *minigoError) Error() string     { return e.Message }
func minigoInvalid(message string) error { return &minigoError{400, "InvalidArgument", message} }

func minigoRouteError(notFound bool) error {
	if notFound {
		return &minigoError{404, "NotFound", "Route not found."}
	}
	return &minigoError{405, "MethodNotAllowed", "This method is not supported."}
}

// minigoMiddleware only intercepts the new namespace. The original Console,
// S3, and mc admin contracts are deliberately untouched. A valid encrypted
// session supplies the only credentials; the storage server authorizes each
// operation, including STS and externally authenticated sessions.
func minigoMiddleware(next http.Handler, dispatch func(*http.Request, *models.Principal) (any, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, minigoAPIPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		principal, err := auth.GetClaimsFromTokenInRequest(r)
		if err != nil || principal == nil || principal.STSAccessKeyID == "" || principal.STSSecretAccessKey == "" {
			minigoWriteError(w, &minigoError{401, "Unauthorized", "Sign in to continue."})
			return
		}
		if err = minigoCheckRequest(r); err != nil {
			minigoWriteError(w, err)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			body, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, minigoMaxBody))
			if readErr != nil {
				var limitErr *http.MaxBytesError
				if errors.As(readErr, &limitErr) {
					err = &minigoError{413, "RequestTooLarge", "Request exceeds the 1 MiB limit."}
				} else {
					err = minigoInvalid("Cannot read request body.")
				}
				minigoWriteError(w, err)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		result, err := dispatch(r.WithContext(ctx), principal)
		if err != nil {
			minigoWriteError(w, err)
			return
		}
		if result == nil {
			result = map[string]bool{"ok": true}
		}
		_ = json.NewEncoder(w).Encode(result)
	})
}

// Do not trust forwarded host headers for CSRF. HTTPS origins with the exact
// request host also support TLS termination at a reverse proxy. Mutations must
// be same-origin JSON requests, including DELETE; HTML forms cannot submit them.
func minigoCheckRequest(r *http.Request) error {
	mutation := r.Method != http.MethodGet && r.Method != http.MethodHead
	origin := r.Header.Get("Origin")
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return &minigoError{403, "ForbiddenOrigin", "Cross-origin requests are not allowed."}
	}
	if origin != "" {
		u, err := url.Parse(origin)
		if len(r.Header.Values("Origin")) != 1 || err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || !minigoOriginAllowed(r, u) {
			return &minigoError{403, "ForbiddenOrigin", "Cross-origin requests are not allowed."}
		}
	} else if mutation {
		return &minigoError{403, "ForbiddenOrigin", "A same-origin Origin header is required."}
	}
	if mutation {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			return &minigoError{415, "UnsupportedMediaType", "Use application/json."}
		}
	}
	return nil
}

func minigoOriginAllowed(r *http.Request, origin *url.URL) bool {
	if origin.Scheme != "http" && origin.Scheme != "https" {
		return false
	}
	if strings.EqualFold(origin.Host, r.Host) && (origin.Scheme == "https" || r.TLS == nil) {
		return true
	}
	publicURL, err := url.Parse(getConsoleBrowserRedirectURL())
	return err == nil && publicURL.User == nil && publicURL.Host != "" && strings.EqualFold(publicURL.Host, origin.Host) && publicURL.Scheme == origin.Scheme
}

func minigoDecode(r *http.Request, into any) error {
	d := json.NewDecoder(r.Body)
	var raw json.RawMessage
	if err := d.Decode(&raw); err != nil || len(raw) == 0 || raw[0] != '{' {
		return minigoInvalid("Supply a valid JSON object with supported fields.")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return minigoInvalid("Supply exactly one JSON object.")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(into); err != nil {
		return minigoInvalid("Supply a valid JSON object with supported fields.")
	}
	return nil
}

func minigoWriteError(w http.ResponseWriter, err error) {
	var local *minigoError
	if errors.As(err, &local) {
		w.WriteHeader(local.Status)
		_ = json.NewEncoder(w).Encode(local)
		return
	}
	status, code := 502, "UpstreamUnavailable"
	var adminError madmin.ErrorResponse
	var s3Error minio.ErrorResponse
	if errors.As(err, &adminError) {
		code = adminError.Code
		status = 400
	}
	if errors.As(err, &s3Error) {
		code = s3Error.Code
		status = s3Error.StatusCode
	}
	if status < 400 || status > 599 {
		status = 502
	}
	switch code {
	case "AccessDenied", "AllAccessDisabled", "AdminActionNotAllowed", "XMinioAdminAccessDenied":
		status = 403
	case "InvalidAccessKeyId", "InvalidToken", "ExpiredToken", "SignatureDoesNotMatch":
		status = 401
	case "NoSuchBucket", "NoSuchUser", "NoSuchGroup", "XMinioAdminNoSuchUser", "XMinioAdminNoSuchGroup", "XMinioAdminNoSuchPolicy", "NoSuchLifecycleConfiguration", "ReplicationConfigurationNotFoundError":
		status = 404
	case "BucketNotEmpty":
		status = 409
	case "InvalidArgument", "InvalidRequest", "InvalidBucketName", "MalformedXML":
		status = 400
	case "MethodNotAllowed":
		status = 405
	case "NotImplemented", "XMinioAdminNotImplemented":
		status = 501
	case "InternalError", "XMinioAdminConfigBadJSON", "XMinioServerNotInitialized":
		status = 502
	}
	message := "The storage server could not complete the request."
	if status == 401 {
		message = "Your session is no longer authorized. Sign in again."
	}
	if status == 403 {
		message = "Your account does not have permission for this operation."
	}
	if status == 404 {
		message = "The requested resource or configuration does not exist."
	}
	if status == 400 {
		message = "The storage server rejected these settings. Check the supplied values."
	}
	if status == 409 {
		message = "The resource is not empty or conflicts with its current state."
	}
	if status == 405 || status == 501 {
		message = "This operation is not supported by the storage server."
	}
	// Do not expose upstream messages: some include configuration values/secrets.
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(minigoError{status, code, message})
}

func minigoDispatch(r *http.Request, p *models.Principal) (any, error) {
	parts := strings.Split(strings.TrimPrefix(r.URL.EscapedPath(), minigoAPIPrefix), "/")
	for i, part := range parts {
		decoded, err := url.PathUnescape(part)
		if err != nil || decoded == "" || decoded == "." || decoded == ".." {
			return nil, &minigoError{404, "NotFound", "Route not found."}
		}
		parts[i] = decoded
	}
	admin, err := NewMinioAdminClient(r.Context(), p)
	if err != nil {
		return nil, err
	}
	switch parts[0] {
	case "server":
		if len(parts) != 1 {
			return nil, minigoRouteError(true)
		}
		if r.Method != "GET" {
			break
		}
		info, err := admin.ServerInfo(r.Context())
		if err != nil {
			return nil, err
		}
		return minigoServerInfo(info), nil
	case "users":
		return minigoUsers(r, admin, parts[1:])
	case "groups":
		return minigoGroups(r, admin, parts[1:])
	case "policies":
		return minigoPolicies(r, admin, parts[1:])
	case "policy-associations":
		if len(parts) != 1 {
			return nil, minigoRouteError(true)
		}
		if r.Method != "POST" {
			break
		}
		var body struct {
			Policies []string `json:"policies"`
			User     string   `json:"user"`
			Group    string   `json:"group"`
			Action   string   `json:"action"`
		}
		if err := minigoDecode(r, &body); err != nil {
			return nil, err
		}
		if len(body.Policies) == 0 || (body.User == "") == (body.Group == "") {
			return nil, minigoInvalid("Choose policies and exactly one user or group.")
		}
		req := madmin.PolicyAssociationReq{Policies: body.Policies, User: body.User, Group: body.Group}
		if body.Action == "attach" {
			return admin.AttachPolicy(r.Context(), req)
		}
		if body.Action == "detach" {
			return admin.DetachPolicy(r.Context(), req)
		}
		return nil, minigoInvalid("Action must be attach or detach.")
	case "service-accounts":
		return minigoServiceAccounts(r, admin, parts[1:])
	case "config":
		return minigoConfig(r, admin, parts[1:])
	case "buckets":
		client, err := minigoS3Client(p, utils.ClientIPFromContext(r.Context()))
		if err != nil {
			return nil, err
		}
		return minigoBucketSettings(r, client, parts[1:])
	default:
		return nil, &minigoError{404, "NotFound", "Route not found."}
	}
	return nil, &minigoError{405, "MethodNotAllowed", "This route or method is not supported."}
}

// The pinned minio-go SDK's lifecycle DELETE does not check HTTP status. A
// narrow transport guard prevents a rejected deletion being reported as success
// while retaining normal SDK signing, STS credentials, retries and S3 handling.
type minigoS3StatusTransport struct{ http.RoundTripper }

func (t minigoS3StatusTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.RoundTripper.RoundTrip(r)
	if err != nil || r.Method != http.MethodDelete || !r.URL.Query().Has("lifecycle") || response.StatusCode == http.StatusOK || response.StatusCode == http.StatusNoContent {
		return response, err
	}
	defer response.Body.Close()
	upstream := minio.ErrorResponse{Code: "UpstreamError"}
	_ = xml.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&upstream)
	upstream.StatusCode = response.StatusCode
	return nil, upstream
}

func minigoS3Client(p *models.Principal, clientIP string) (*minio.Client, error) {
	client, err := minio.New(getMinIOEndpoint(), &minio.Options{
		Creds:     getConsoleCredentialsFromSession(p),
		Secure:    getMinIOEndpointIsSecure(),
		Transport: minigoS3StatusTransport{PrepareSTSClientTransport(clientIP)},
	})
	if err != nil {
		return nil, err
	}
	client.SetAppInfo(globalAppName, pkg.Version)
	return client, nil
}

func minigoServerInfo(info madmin.InfoMessage) any {
	servers := make([]any, 0, len(info.Servers))
	for _, s := range info.Servers {
		drives := make([]any, 0, len(s.Disks))
		for _, d := range s.Disks {
			drives = append(drives, map[string]any{"state": d.State, "totalSpace": d.TotalSpace, "usedSpace": d.UsedSpace, "availableSpace": d.AvailableSpace})
		}
		servers = append(servers, map[string]any{"endpoint": s.Endpoint, "state": s.State, "version": s.Version, "uptime": s.Uptime, "drives": drives})
	}
	return map[string]any{"mode": info.Mode, "region": info.Region, "deploymentId": info.DeploymentID, "buckets": info.Buckets.Count, "objects": info.Objects.Count, "usageBytes": info.Usage.Size, "servers": servers}
}

func minigoSafeUser(name string, u madmin.UserInfo) any {
	return map[string]any{"accessKey": name, "status": u.Status, "policyName": u.PolicyName, "memberOf": u.MemberOf, "updatedAt": u.UpdatedAt}
}

func minigoUsers(r *http.Request, admin *madmin.AdminClient, parts []string) (any, error) {
	ctx := r.Context()
	if len(parts) == 0 && r.Method == "GET" {
		users, err := admin.ListUsers(ctx)
		if err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(users))
		for k := range users {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		result := make([]any, 0, len(keys))
		for _, k := range keys {
			result = append(result, minigoSafeUser(k, users[k]))
		}
		return map[string]any{"users": result}, nil
	}
	if len(parts) != 1 {
		return nil, minigoRouteError(len(parts) > 1)
	}
	name := parts[0]
	switch r.Method {
	case "GET":
		u, err := admin.GetUserInfo(ctx, name)
		if err != nil {
			return nil, err
		}
		return minigoSafeUser(name, u), nil
	case "PUT", "PATCH":
		var body struct {
			SecretKey string               `json:"secretKey"`
			Status    madmin.AccountStatus `json:"status"`
		}
		if err := minigoDecode(r, &body); err != nil {
			return nil, err
		}
		if body.Status == "" && r.Method == "PUT" {
			body.Status = madmin.AccountEnabled
		}
		if body.Status != madmin.AccountEnabled && body.Status != madmin.AccountDisabled {
			return nil, minigoInvalid("Status must be enabled or disabled.")
		}
		if r.Method == "PATCH" {
			if body.SecretKey != "" {
				return nil, minigoInvalid("Use PUT to change a secret key.")
			}
			return nil, admin.SetUserStatus(ctx, name, body.Status)
		}
		if body.SecretKey == "" {
			return nil, minigoInvalid("A secret key is required.")
		}
		return nil, admin.SetUser(ctx, name, body.SecretKey, body.Status)
	case "DELETE":
		return nil, admin.RemoveUser(ctx, name)
	}
	return nil, &minigoError{405, "MethodNotAllowed", "This method is not supported."}
}

func minigoGroups(r *http.Request, admin *madmin.AdminClient, parts []string) (any, error) {
	ctx := r.Context()
	if len(parts) == 0 && r.Method == "GET" {
		groups, err := admin.ListGroups(ctx)
		if groups == nil {
			groups = []string{}
		}
		sort.Strings(groups)
		return map[string]any{"groups": groups}, err
	}
	if len(parts) != 1 {
		return nil, minigoRouteError(len(parts) > 1)
	}
	name := parts[0]
	switch r.Method {
	case "GET":
		return admin.GetGroupDescription(ctx, name)
	case "PUT":
		var body struct {
			Members []string `json:"members"`
			Remove  bool     `json:"remove"`
		}
		if err := minigoDecode(r, &body); err != nil {
			return nil, err
		}
		if len(body.Members) == 0 {
			return nil, minigoInvalid("Choose at least one member. Use DELETE to remove an empty group.")
		}
		return nil, admin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{Group: name, Members: body.Members, IsRemove: body.Remove})
	case "PATCH":
		var body struct {
			Status madmin.GroupStatus `json:"status"`
		}
		if err := minigoDecode(r, &body); err != nil {
			return nil, err
		}
		if body.Status != madmin.GroupEnabled && body.Status != madmin.GroupDisabled {
			return nil, minigoInvalid("Status must be enabled or disabled.")
		}
		return nil, admin.SetGroupStatus(ctx, name, body.Status)
	case "DELETE":
		return nil, admin.UpdateGroupMembers(ctx, madmin.GroupAddRemove{Group: name, IsRemove: true})
	}
	return nil, &minigoError{405, "MethodNotAllowed", "This method is not supported."}
}

func minigoPolicies(r *http.Request, admin *madmin.AdminClient, parts []string) (any, error) {
	ctx := r.Context()
	if len(parts) == 0 && r.Method == "GET" {
		policies, err := admin.ListCannedPolicies(ctx)
		if err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(policies))
		for k := range policies {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		result := make([]any, 0, len(keys))
		for _, k := range keys {
			result = append(result, map[string]any{"name": k, "document": policies[k]})
		}
		return map[string]any{"policies": result}, nil
	}
	if len(parts) != 1 {
		return nil, minigoRouteError(len(parts) > 1)
	}
	name := parts[0]
	switch r.Method {
	case "GET":
		policy, err := admin.InfoCannedPolicy(ctx, name)
		if err != nil {
			return nil, err
		}
		return map[string]any{"name": name, "document": json.RawMessage(policy)}, nil
	case "PUT":
		var body struct {
			Document json.RawMessage `json:"document"`
		}
		if err := minigoDecode(r, &body); err != nil {
			return nil, err
		}
		if len(body.Document) == 0 || body.Document[0] != '{' {
			return nil, minigoInvalid("Document must be a policy JSON object.")
		}
		return nil, admin.AddCannedPolicy(ctx, name, body.Document)
	case "DELETE":
		return nil, admin.RemoveCannedPolicy(ctx, name)
	}
	return nil, &minigoError{405, "MethodNotAllowed", "This method is not supported."}
}

func minigoServiceAccounts(r *http.Request, admin *madmin.AdminClient, parts []string) (any, error) {
	ctx := r.Context()
	if len(parts) == 0 {
		switch r.Method {
		case "GET":
			// An omitted user asks MinIO to infer the caller's parent identity.
			// Supplying the parent's name explicitly instead would require the
			// separate permission to list another user's keys for STS sessions.
			return admin.ListServiceAccounts(ctx, r.URL.Query().Get("user"))
		case "POST":
			var body madmin.AddServiceAccountReq
			if err := minigoDecode(r, &body); err != nil {
				return nil, err
			}
			return admin.AddServiceAccount(ctx, body)
		}
	}
	if len(parts) == 1 {
		switch r.Method {
		case "GET":
			return admin.InfoServiceAccount(ctx, parts[0])
		case "PATCH":
			var body madmin.UpdateServiceAccountReq
			if err := minigoDecode(r, &body); err != nil {
				return nil, err
			}
			return nil, admin.UpdateServiceAccount(ctx, parts[0], body)
		case "DELETE":
			return nil, admin.DeleteServiceAccount(ctx, parts[0])
		}
	}
	return nil, minigoRouteError(len(parts) > 1)
}

func minigoBucketSettings(r *http.Request, client *minio.Client, parts []string) (any, error) {
	ctx := r.Context()
	if len(parts) == 1 && r.Method == "DELETE" {
		return nil, client.RemoveBucket(ctx, parts[0])
	}
	if len(parts) != 2 {
		return nil, minigoRouteError(len(parts) > 2)
	}
	bucket := parts[0]
	switch parts[1] {
	case "versioning":
		switch r.Method {
		case "GET":
			return client.GetBucketVersioning(ctx, bucket)
		case "PUT":
			var cfg minio.BucketVersioningConfiguration
			if err := minigoDecode(r, &cfg); err != nil {
				return nil, err
			}
			if cfg.Status != "Enabled" && cfg.Status != "Suspended" {
				return nil, minigoInvalid("Status must be Enabled or Suspended.")
			}
			return nil, client.SetBucketVersioning(ctx, bucket, cfg)
		}
	case "lifecycle":
		switch r.Method {
		case "GET":
			return client.GetBucketLifecycle(ctx, bucket)
		case "PUT":
			var cfg lifecycle.Configuration
			if err := minigoDecode(r, &cfg); err != nil {
				return nil, err
			}
			if len(cfg.Rules) == 0 {
				return nil, minigoInvalid("Supply lifecycle rules or use DELETE to remove the configuration.")
			}
			return nil, client.SetBucketLifecycle(ctx, bucket, &cfg)
		case "DELETE":
			return nil, client.SetBucketLifecycle(ctx, bucket, nil)
		}
	case "replication":
		switch r.Method {
		case "GET":
			return client.GetBucketReplication(ctx, bucket)
		case "PUT":
			var cfg replication.Config
			if err := minigoDecode(r, &cfg); err != nil {
				return nil, err
			}
			if len(cfg.Rules) == 0 {
				return nil, minigoInvalid("Supply replication rules or use DELETE to remove the configuration.")
			}
			return nil, client.SetBucketReplication(ctx, bucket, cfg)
		case "DELETE":
			return nil, client.RemoveBucketReplication(ctx, bucket)
		}
	default:
		return nil, minigoRouteError(true)
	}
	return nil, &minigoError{405, "MethodNotAllowed", "This route or method is not supported."}
}
