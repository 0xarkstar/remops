package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/security"
)

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]string{"version": s.version})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	profile := s.profileFromRequest(r)
	if err := security.CheckPermission(profile, config.LevelViewer); err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	host := r.URL.Query().Get("host")
	tag := r.URL.Query().Get("tag")

	if host != "" {
		if err := security.ValidateHostName(host); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	hosts := s.resolveHosts(host, tag)
	if len(hosts) == 0 {
		jsonResponse(w, http.StatusOK, map[string]any{"hosts": []any{}, "message": "no hosts matched"})
		return
	}

	type hostStatus struct {
		Host   string `json:"host"`
		Output string `json:"output,omitempty"`
		Error  string `json:"error,omitempty"`
	}

	var results []hostStatus
	for _, h := range hosts {
		res, err := s.transport.Exec(r.Context(), h, `docker ps --format '{{json .}}'`)
		if err != nil {
			results = append(results, hostStatus{Host: h, Error: err.Error()})
			continue
		}
		results = append(results, hostStatus{Host: h, Output: security.SanitizeOutput(res.Stdout)})
	}

	jsonResponse(w, http.StatusOK, map[string]any{"hosts": results})
}

func (s *Server) handleServiceLogs(w http.ResponseWriter, r *http.Request) {
	profile := s.profileFromRequest(r)
	if err := security.CheckPermission(profile, config.LevelViewer); err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	name := r.PathValue("name")
	if err := security.ValidateServiceName(name); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	svc, ok := s.config.Services[name]
	if !ok {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("unknown service: %s", name))
		return
	}
	if err := security.ValidateContainerName(svc.Container); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	cmd := "docker logs " + svc.Container
	if tail := r.URL.Query().Get("tail"); tail != "" {
		if n, err := strconv.Atoi(tail); err == nil && n > 0 {
			cmd += fmt.Sprintf(" --tail %d", n)
		}
	}
	if since := r.URL.Query().Get("since"); since != "" {
		if err := security.DetectShellInjection(since); err != nil {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("invalid since value: %v", err))
			return
		}
		cmd += " --since " + since
	}

	res, err := s.transport.Exec(r.Context(), svc.Host, cmd)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	combined := res.Stdout
	if res.Stderr != "" {
		combined += res.Stderr
	}
	jsonResponse(w, http.StatusOK, map[string]string{"logs": security.SanitizeOutput(combined)})
}

// handleServiceAction returns a handler for restart/stop/start operations.
func (s *Server) handleServiceAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profile := s.profileFromRequest(r)
		if err := security.CheckPermission(profile, config.LevelOperator); err != nil {
			jsonError(w, http.StatusForbidden, err.Error())
			return
		}

		name := r.PathValue("name")
		if err := security.ValidateServiceName(name); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}

		var body struct {
			Confirm bool `json:"confirm"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if !body.Confirm {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("confirm must be true to %s service", action))
			return
		}

		svc, ok := s.config.Services[name]
		if !ok {
			jsonError(w, http.StatusNotFound, fmt.Sprintf("unknown service: %s", name))
			return
		}
		if err := security.ValidateContainerName(svc.Container); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}

		// Approval flow.
		if s.approver != nil {
			timeout := 5 * time.Minute
			if s.config.Approval != nil {
				timeout = s.config.Approval.EffectiveTimeout()
			}
			approvalCtx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			desc := fmt.Sprintf("HTTP API: %s service %s on %s", action, name, svc.Host)
			approved, err := s.approver.RequestApproval(approvalCtx, desc)
			if err != nil {
				jsonError(w, http.StatusGatewayTimeout, fmt.Sprintf("approval request failed: %v", err))
				return
			}
			if !approved {
				jsonError(w, http.StatusForbidden, "operation denied by approver")
				return
			}
		}

		// Rate limiting.
		if s.rateLimiter != nil {
			if err := s.rateLimiter.Check(svc.Host); err != nil {
				jsonError(w, http.StatusTooManyRequests, err.Error())
				return
			}
		}

		cmd := fmt.Sprintf("docker %s %s", action, svc.Container)
		res, err := s.transport.Exec(r.Context(), svc.Host, cmd)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if s.rateLimiter != nil {
			if err := s.rateLimiter.Record(svc.Host, cmd); err != nil {
				fmt.Fprintf(os.Stderr, "api: rate limiter record: %v\n", err)
			}
		}
		if s.auditLogger != nil {
			if err := s.auditLogger.Log(security.AuditEntry{
				Command: action,
				Host:    svc.Host,
				Service: name,
				Profile: profile.String(),
				Result:  "success",
			}); err != nil {
				fmt.Fprintf(os.Stderr, "api: audit log: %v\n", err)
			}
		}

		jsonResponse(w, http.StatusOK, map[string]any{
			"action":    action,
			"service":   name,
			"container": svc.Container,
			"host":      svc.Host,
			"exit_code": res.ExitCode,
			"output":    security.SanitizeOutput(res.Stdout),
		})
	}
}

func (s *Server) handleHostInfo(w http.ResponseWriter, r *http.Request) {
	profile := s.profileFromRequest(r)
	if err := security.CheckPermission(profile, config.LevelViewer); err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	name := r.PathValue("name")
	if err := security.ValidateHostName(name); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, ok := s.config.Hosts[name]; !ok {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("unknown host: %s", name))
		return
	}

	var results []map[string]string
	for _, cmd := range []string{"uname -a", "docker version --format '{{json .}}'"} {
		res, err := s.transport.Exec(r.Context(), name, cmd)
		if err != nil {
			results = append(results, map[string]string{"command": cmd, "error": err.Error()})
			continue
		}
		results = append(results, map[string]string{"command": cmd, "output": security.SanitizeOutput(res.Stdout)})
	}
	jsonResponse(w, http.StatusOK, map[string]any{"host": name, "info": results})
}

func (s *Server) handleHostDisk(w http.ResponseWriter, r *http.Request) {
	profile := s.profileFromRequest(r)
	if err := security.CheckPermission(profile, config.LevelViewer); err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	name := r.PathValue("name")
	if err := security.ValidateHostName(name); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, ok := s.config.Hosts[name]; !ok {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("unknown host: %s", name))
		return
	}

	res, err := s.transport.Exec(r.Context(), name, "df -h --output=target,size,used,avail,pcent")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"host": name, "disk": security.SanitizeOutput(res.Stdout)})
}

func (s *Server) handleHostExec(w http.ResponseWriter, r *http.Request) {
	profile := s.profileFromRequest(r)
	if err := security.CheckPermission(profile, config.LevelAdmin); err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	name := r.PathValue("name")
	if err := security.ValidateHostName(name); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, ok := s.config.Hosts[name]; !ok {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("unknown host: %s", name))
		return
	}

	var body struct {
		Command string `json:"command"`
		Confirm bool   `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Command == "" {
		jsonError(w, http.StatusBadRequest, "command is required")
		return
	}
	if !body.Confirm {
		jsonError(w, http.StatusBadRequest, "confirm must be true to execute command")
		return
	}

	if s.auditLogger != nil {
		if err := s.auditLogger.Log(security.AuditEntry{
			Command: body.Command,
			Host:    name,
			Profile: profile.String(),
			Result:  "exec",
		}); err != nil {
			fmt.Fprintf(os.Stderr, "api: audit log: %v\n", err)
		}
	}

	res, err := s.transport.Exec(r.Context(), name, body.Command)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	text := res.Stdout
	if res.Stderr != "" {
		text += res.Stderr
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"host":      name,
		"exit_code": res.ExitCode,
		"output":    security.SanitizeOutput(text),
	})
}

func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	profile := s.profileFromRequest(r)
	if err := security.CheckPermission(profile, config.LevelViewer); err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	type hostCheck struct {
		Host    string `json:"host"`
		Online  bool   `json:"online"`
		Latency string `json:"latency,omitempty"`
		Docker  string `json:"docker,omitempty"`
		Error   string `json:"error,omitempty"`
	}

	var results []hostCheck
	for name := range s.config.Hosts {
		pr, err := s.transport.Ping(r.Context(), name)
		if err != nil || !pr.Online {
			results = append(results, hostCheck{Host: name, Online: false, Error: "unreachable"})
			continue
		}
		hc := hostCheck{Host: name, Online: true, Latency: pr.Latency.String()}
		res, err := s.transport.Exec(r.Context(), name, "docker info --format '{{.ServerVersion}}'")
		if err != nil {
			hc.Docker = "error: " + err.Error()
		} else {
			hc.Docker = strings.TrimSpace(res.Stdout)
		}
		results = append(results, hc)
	}
	jsonResponse(w, http.StatusOK, map[string]any{"hosts": results})
}

func (s *Server) handleDBQuery(w http.ResponseWriter, r *http.Request) {
	profile := s.profileFromRequest(r)
	if err := security.CheckPermission(profile, config.LevelViewer); err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	serviceName := r.PathValue("service")
	if err := security.ValidateServiceName(serviceName); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Query == "" {
		jsonError(w, http.StatusBadRequest, "query is required")
		return
	}

	svc, ok := s.config.Services[serviceName]
	if !ok {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("unknown service: %s", serviceName))
		return
	}
	if svc.DB == nil {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("service %q has no db config", serviceName))
		return
	}

	if security.IsWriteQuery(body.Query) {
		if err := security.CheckPermission(profile, config.LevelOperator); err != nil {
			jsonError(w, http.StatusForbidden, "write query requires operator permission")
			return
		}
	}

	if err := security.ValidateContainerName(svc.Container); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	db := svc.DB
	escaped := escapeSingleQuotes(body.Query)
	var cmd string
	switch db.Engine {
	case "postgresql", "postgres":
		cmd = fmt.Sprintf("docker exec %s psql -U %s -d %s -c '%s'",
			svc.Container, db.User, db.Database, escaped)
	case "mysql":
		cmd = fmt.Sprintf("docker exec -e MYSQL_PWD='%s' %s mysql -u %s %s -e '%s'",
			escapeSingleQuotes(db.Password), svc.Container, db.User, db.Database, escaped)
	default:
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("unsupported db engine %q", db.Engine))
		return
	}

	res, err := s.transport.Exec(r.Context(), svc.Host, cmd)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	combined := res.Stdout
	if res.Stderr != "" {
		combined += res.Stderr
	}
	jsonResponse(w, http.StatusOK, map[string]string{"result": security.SanitizeOutput(combined)})
}

// resolveHosts returns host names based on optional filters.
func (s *Server) resolveHosts(host, tag string) []string {
	if s.config == nil {
		return nil
	}
	if host != "" {
		if _, ok := s.config.Hosts[host]; ok {
			return []string{host}
		}
		return nil
	}
	if tag != "" {
		return s.config.HostsByTag(tag)
	}
	return s.config.AllHostNames()
}

// escapeSingleQuotes escapes single quotes for use inside single-quoted shell arguments.
func escapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}
