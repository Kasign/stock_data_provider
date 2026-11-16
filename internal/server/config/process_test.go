package config

import (
	"net/http"
	"strings"
	"testing"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

var relayRequiredTestValues = map[string]string{
	"NOTIFICATION_EVENT_MAX_ENVELOPE_BYTES":"8192",
	"NOTIFICATION_EVENT_MAX_PAYLOAD_BYTES":"4096",
	"NOTIFICATION_EVENT_MAX_JSON_DEPTH":"16",
}

func TestLoadProcessDefaults(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == "PLATFORM_ADMIN_TOKEN" { return strings.Repeat("a", 43), true }
		if value, ok := relayRequiredTestValues[key]; ok { return value, true }
		return "", false
	})
	cfg, err := LoadProcess(env)
	if err != nil { t.Fatalf("LoadProcess() error = %v", err) }
	if cfg.LogLevel != "info" || cfg.AnalysisTimeout != 5*time.Minute ||
		cfg.AnalysisWorkers != 2 || cfg.AnalysisMaxQueued != 100 ||
		cfg.Role != RoleAll || cfg.HTTP.Address != ":8092" ||
		cfg.ResearchNotificationRelay.BatchSize != 20 || cfg.ResearchNotificationRelay.EventLimits.MaxJSONDepth != 16 ||
		cfg.ResearchflowCapacity.Configured || cfg.OwnerSession.Enabled {
		t.Fatalf("unexpected process defaults: %+v", cfg)
	}
}

func TestLoadProcessWorkerSkipsHTTPAndSecurity(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == "SERVER_ROLE" { return "worker", true }
		if value, ok := relayRequiredTestValues[key]; ok { return value, true }
		return "", false
	})
	cfg, err := LoadProcess(env)
	if err != nil { t.Fatalf("LoadProcess() error = %v", err) }
	if cfg.Role != RoleWorker || cfg.HTTP.Address != "" || cfg.Security.OwnerToken.Configured() || cfg.OwnerSession.Enabled {
		t.Fatalf("worker process unexpectedly loaded HTTP/security config: %+v", cfg)
	}
}

func TestLoadProcessRejectsInvalidProcessSettings(t *testing.T) {
	tests := []struct{name,key,raw string}{
		{"log level","LOG_LEVEL","trace"}, {"analysis timeout","ANALYSIS_TIMEOUT","0s"},
		{"workers","ANALYSIS_WORKERS","0"}, {"queue smaller than workers","ANALYSIS_MAX_QUEUED","1"},
		{"migration timeout","MIGRATION_TIMEOUT","0s"},
	}
	for _, tt := range tests { t.Run(tt.name,func(t *testing.T){
		env:=platformconfig.NewEnvWithLookup(func(key string)(string,bool){
			if key==tt.key{return tt.raw,true}; if key=="SERVER_ROLE"{return "worker",true}
			if value,ok:=relayRequiredTestValues[key];ok{return value,true}; return "",false
		})
		if _,err:=LoadProcess(env);err==nil{t.Fatal("LoadProcess() error = nil, want validation error")}
	})}
}

func TestLoadProcessReturnsStableSnapshot(t *testing.T) {
	values:=map[string]string{"LOG_LEVEL":"debug","ANALYSIS_WORKERS":"4","ANALYSIS_MAX_QUEUED":"200","SERVER_ROLE":"worker",
		"NOTIFICATION_EVENT_MAX_ENVELOPE_BYTES":"8192","NOTIFICATION_EVENT_MAX_PAYLOAD_BYTES":"4096","NOTIFICATION_EVENT_MAX_JSON_DEPTH":"16"}
	env:=platformconfig.NewEnvWithLookup(func(key string)(string,bool){v,ok:=values[key];return v,ok})
	cfg,err:=LoadProcess(env);if err!=nil{t.Fatalf("LoadProcess() error = %v",err)}
	values["LOG_LEVEL"],values["ANALYSIS_WORKERS"]="error","8"
	if cfg.LogLevel!="debug"||cfg.AnalysisWorkers!=4{t.Fatalf("process snapshot changed: %+v",cfg)}
}

func TestLoadOwnerSessionDisabledByDefault(t *testing.T) {
	cfg,err:=LoadOwnerSession(platformconfig.NewEnvWithLookup(func(string)(string,bool){return "",false}))
	if err!=nil||cfg.Enabled{t.Fatalf("LoadOwnerSession() = %+v, %v; want disabled",cfg,err)}
}

func TestLoadOwnerSessionRequiresExplicitArgonCosts(t *testing.T) {
	values:=map[string]string{"OWNER_SESSION_ENABLED":"true","OWNER_SESSION_PUBLIC_ORIGIN":"https://stocks.example.com"}
	_,err:=LoadOwnerSession(platformconfig.NewEnvWithLookup(func(key string)(string,bool){v,ok:=values[key];return v,ok}))
	if err==nil||!strings.Contains(err.Error(),"OWNER_SESSION_ARGON2_MEMORY_KIB"){t.Fatalf("error = %v, want explicit Argon2 cost",err)}
}

func TestLoadOwnerSessionValidatesAndLoadsExplicitCosts(t *testing.T) {
	values:=map[string]string{"OWNER_SESSION_ENABLED":"true","OWNER_SESSION_PUBLIC_ORIGIN":"https://stocks.example.com",
		"OWNER_SESSION_ARGON2_MEMORY_KIB":"65536","OWNER_SESSION_ARGON2_ITERATIONS":"3","OWNER_SESSION_ARGON2_PARALLELISM":"2"}
	cfg,err:=LoadOwnerSession(platformconfig.NewEnvWithLookup(func(key string)(string,bool){v,ok:=values[key];return v,ok}))
	if err!=nil{t.Fatalf("LoadOwnerSession() error = %v",err)}
	if !cfg.Enabled||!cfg.Cookie.Secure||cfg.Cookie.SameSite!=http.SameSiteLaxMode||
		cfg.IdleTTL!=12*time.Hour||cfg.AbsoluteTTL!=7*24*time.Hour||cfg.Argon2id.Memory!=65536 {
		t.Fatalf("unexpected Owner Session config: %+v",cfg)
	}
}

func TestLoadOwnerSessionRejectsInsecureHTTPSCookie(t *testing.T) {
	values:=map[string]string{"OWNER_SESSION_ENABLED":"true","OWNER_SESSION_PUBLIC_ORIGIN":"https://stocks.example.com",
		"OWNER_SESSION_COOKIE_SECURE":"false","OWNER_SESSION_ARGON2_MEMORY_KIB":"65536",
		"OWNER_SESSION_ARGON2_ITERATIONS":"3","OWNER_SESSION_ARGON2_PARALLELISM":"2"}
	_,err:=LoadOwnerSession(platformconfig.NewEnvWithLookup(func(key string)(string,bool){v,ok:=values[key];return v,ok}))
	if err==nil||!strings.Contains(err.Error(),"HTTPS Origin"){t.Fatalf("error = %v, want HTTPS Secure rejection",err)}
}
