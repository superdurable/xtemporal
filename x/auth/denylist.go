package auth

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/superdurable/xtemporal/common/authorization"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/log/tag"
	"github.com/superdurable/xtemporal/common/metrics"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	EnvFile             = "XTEMPORAL_JTI_DENYLIST_FILE"
	EnvPollInterval     = "XTEMPORAL_JTI_DENYLIST_POLL_INTERVAL"
	EnvStartupMode      = "XTEMPORAL_JTI_DENYLIST_STARTUP_MODE"
	defaultPollInterval = 30 * time.Second
)

var reloadFailure = metrics.NewCounterDef("xtemporal_jti_denylist_reload_failure")

type Source interface {
	Load(context.Context) (map[string]int64, error)
}

type FileSource struct{ Path string }

type entry struct {
	JTI string `json:"jti"`
	Exp int64  `json:"exp"`
}

func (s FileSource) Load(_ context.Context) (_ map[string]int64, retErr error) {
	file, err := os.Open(s.Path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); retErr == nil {
			retErr = err
		}
	}()

	entries := make(map[string]int64)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	now := time.Now().Unix()
	for line := 1; scanner.Scan(); line++ {
		var item entry
		data := scanner.Bytes()
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if item.JTI == "" || item.Exp <= 0 {
			return nil, fmt.Errorf("line %d: jti and positive exp are required", line)
		}
		if item.Exp > now {
			entries[item.JTI] = item.Exp
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

type Denylist struct {
	mu         sync.RWMutex
	entries    map[string]int64
	loaded     bool
	source     Source
	logger     log.Logger
	metrics    metrics.Handler
	failClosed bool
}

func NewDenylist(source Source, logger log.Logger, handler metrics.Handler, failClosed bool) *Denylist {
	return &Denylist{source: source, logger: logger, metrics: handler, failClosed: failClosed}
}

func (d *Denylist) Reload(ctx context.Context) {
	entries, err := d.source.Load(ctx)
	if err != nil {
		d.logger.Error("unable to load JTI denylist", tag.Error(err))
		reloadFailure.With(d.metrics).Record(1)
		return
	}
	d.mu.Lock()
	d.entries = entries
	d.loaded = true
	d.mu.Unlock()
}

func (d *Denylist) Poll(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.Reload(ctx)
		}
	}
}

func (d *Denylist) Revoked(jti string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if !d.loaded && d.failClosed {
		return true
	}
	return d.entries[jti] > time.Now().Unix()
}

type Mapper struct {
	delegate authorization.ClaimMapper
	denylist *Denylist
}

func NewMapper(delegate authorization.ClaimMapper, denylist *Denylist) *Mapper {
	return &Mapper{delegate: delegate, denylist: denylist}
}

func (m *Mapper) GetClaims(info *authorization.AuthInfo) (*authorization.Claims, error) {
	if info == nil {
		return nil, status.Error(codes.Unauthenticated, "missing JWT")
	}
	claims, err := m.delegate.GetClaims(info)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(info.AuthToken, " ", 2)
	if len(parts) != 2 {
		return nil, status.Error(codes.Unauthenticated, "missing JWT")
	}
	token, _, err := new(jwt.Parser).ParseUnverified(parts[1], jwt.MapClaims{})
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid JWT claims")
	}
	tokenClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "invalid JWT claims")
	}
	jti, ok := tokenClaims["jti"].(string)
	if !ok || jti == "" {
		return nil, status.Error(codes.Unauthenticated, "JWT jti required")
	}
	exp, ok := tokenClaims["exp"].(float64)
	if !ok || exp <= 0 {
		return nil, status.Error(codes.Unauthenticated, "JWT exp required")
	}
	if m.denylist.Revoked(jti) {
		return nil, status.Error(codes.Unauthenticated, "JWT revoked")
	}
	return claims, nil
}

func ConfigFromEnv() (FileSource, time.Duration, bool, error) {
	path := os.Getenv(EnvFile)
	if path == "" {
		return FileSource{}, 0, false, fmt.Errorf("%s is required", EnvFile)
	}
	interval := defaultPollInterval
	if raw := os.Getenv(EnvPollInterval); raw != "" {
		var err error
		interval, err = time.ParseDuration(raw)
		if err != nil || interval <= 0 {
			return FileSource{}, 0, false, fmt.Errorf("%s must be a positive duration", EnvPollInterval)
		}
	}
	failClosed := true
	switch os.Getenv(EnvStartupMode) {
	case "", "fail-closed":
	case "empty":
		failClosed = false
	default:
		return FileSource{}, 0, false, fmt.Errorf("%s must be fail-closed or empty", EnvStartupMode)
	}
	return FileSource{Path: path}, interval, failClosed, nil
}
