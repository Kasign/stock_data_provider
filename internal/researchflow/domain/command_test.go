package domain

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func commandAt(now time.Time) CreateRunCommand {
	return CreateRunCommand{SchemaVersion:"1",IdempotencyKey:"key-1",TemplateRef:"stock_research",TemplateVersion:"1",
		SystemAsOf:now.Add(-time.Minute),DeadlineAt:now.Add(time.Hour),InputSnapshotRefs:[]string{"snapshot-b","snapshot-a"},
		Parameters:json.RawMessage(`{"symbol":"600000"}`),CorrelationID:"corr-1"}
}

func TestValidateCreateRunCommandBoundaries(t *testing.T) {
	now:=time.Date(2026,11,6,10,0,0,0,time.UTC)
	if err:=ValidateCreateRunCommand(commandAt(now),now,false);err!=nil{t.Fatal(err)}
	cmd:=commandAt(now);cmd.DeadlineAt=now
	if err:=ValidateCreateRunCommand(cmd,now,false);!errors.Is(err,ErrInvalidCommand){t.Fatalf("expected deadline rejection, got %v",err)}
	cmd=commandAt(now);cmd.SystemAsOf=now.Add(time.Second)
	if err:=ValidateCreateRunCommand(cmd,now,false);err==nil{t.Fatal("future system_as_of accepted")}
	cmd=commandAt(now);cmd.InputSnapshotRefs=[]string{}
	if err:=ValidateCreateRunCommand(cmd,now,false);err==nil{t.Fatal("empty snapshots accepted without template permission")}
	if err:=ValidateCreateRunCommand(cmd,now,true);err!=nil{t.Fatal(err)}
	cmd=commandAt(now);cmd.Parameters=json.RawMessage(`[]`)
	if err:=ValidateCreateRunCommand(cmd,now,false);err==nil{t.Fatal("non-object parameters accepted")}
	cmd=commandAt(now);cmd.InputSnapshotRefs=[]string{" snapshot-a"}
	if err:=ValidateCreateRunCommand(cmd,now,false);err==nil{t.Fatal("snapshot reference with surrounding whitespace accepted")}
	cmd=commandAt(now);cmd.InputSnapshotRefs=[]string{"snapshot-a","snapshot-a"}
	if err:=ValidateCreateRunCommand(cmd,now,false);err==nil{t.Fatal("duplicate snapshot references accepted")}
}

func TestRequestFingerprintCanonicalizesParametersAndExcludesTracing(t *testing.T) {
	now:=time.Date(2026,11,6,10,0,0,0,time.UTC)
	a,b:=commandAt(now),commandAt(now)
	b.Parameters=json.RawMessage(`{ "symbol" : "600000" }`)
	b.CorrelationID="different-correlation"
	requestID:="request-2";b.OriginatingRequestID=&requestID
	fa,err:=RequestFingerprint("caller-a",a);if err!=nil{t.Fatal(err)}
	fb,err:=RequestFingerprint("caller-a",b);if err!=nil{t.Fatal(err)}
	if fa!=fb{t.Fatal("semantic-equivalent request produced different fingerprint")}
	if fa != "b6f556621d19608b4f6a71afe8c568ad855e320415bf83bf4c9d89d65d19b861" { t.Fatalf("RequestFingerprint golden mismatch: %s", fa) }
	fc,err:=RequestFingerprint("caller-b",a);if err!=nil{t.Fatal(err)}
	if fa==fc{t.Fatal("caller scope missing from fingerprint")}
	if err:=ValidateIdempotentReplay(fa,fc);!errors.Is(err,ErrIdempotencyConflict){t.Fatalf("expected idempotency conflict, got %v",err)}
}


func TestRequestFingerprintUsesJCSNumberAndUnicodeRules(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	a, b := commandAt(now), commandAt(now)
	a.Parameters = json.RawMessage(`{"value":1.0,"negative_zero":-0,"unicode":"\u0065"}`)
	b.Parameters = json.RawMessage(`{ "unicode":"e", "negative_zero":0.0, "value":1 }`)
	fa, err := RequestFingerprint("caller-a", a)
	if err != nil { t.Fatal(err) }
	fb, err := RequestFingerprint("caller-a", b)
	if err != nil { t.Fatal(err) }
	if fa != fb { t.Fatalf("JCS-equivalent numeric/Unicode parameters differ: %s != %s", fa, fb) }
}

func TestRequestFingerprintRejectsDuplicateJSONKeys(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	cmd := commandAt(now)
	cmd.Parameters = json.RawMessage(`{"symbol":"600000","symbol":"000001"}`)
	if _, err := RequestFingerprint("caller-a", cmd); err == nil {
		t.Fatal("duplicate JSON keys must be rejected by JCS")
	}
}


func TestRequestFingerprintRejectsNonUTCTimestampsBeforeReplay(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	base := commandAt(now)
	invalid := []struct {
		name string
		mutate func(*CreateRunCommand)
	}{
		{name:"zero system_as_of", mutate:func(cmd *CreateRunCommand) { cmd.SystemAsOf = time.Time{} }},
		{name:"zero deadline_at", mutate:func(cmd *CreateRunCommand) { cmd.DeadlineAt = time.Time{} }},
		{name:"offset system_as_of", mutate:func(cmd *CreateRunCommand) { cmd.SystemAsOf = cmd.SystemAsOf.In(time.FixedZone("UTC+8", 8*60*60)) }},
		{name:"offset deadline_at", mutate:func(cmd *CreateRunCommand) { cmd.DeadlineAt = cmd.DeadlineAt.In(time.FixedZone("UTC+8", 8*60*60)) }},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			cmd := base
			tc.mutate(&cmd)
			if _, err := RequestFingerprint("caller-a", cmd); err == nil {
				t.Fatal("invalid timestamp accepted for idempotency fingerprint")
			}
		})
	}
}

func TestRequestFingerprintCanonicalizesSnapshotSetAndRejectsInvalidRefs(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	a, b := commandAt(now), commandAt(now)
	a.InputSnapshotRefs = []string{"snapshot-b", "snapshot-a"}
	b.InputSnapshotRefs = []string{"snapshot-a", "snapshot-b"}
	fa, err := RequestFingerprint("caller-a", a)
	if err != nil { t.Fatal(err) }
	fb, err := RequestFingerprint("caller-a", b)
	if err != nil { t.Fatal(err) }
	if fa != fb { t.Fatal("snapshot set order changed request fingerprint") }

	invalid := []struct {
		name string
		refs []string
	}{
		{name:"nil", refs:nil},
		{name:"empty reference", refs:[]string{""}},
		{name:"surrounding whitespace", refs:[]string{" snapshot-a"}},
		{name:"duplicate", refs:[]string{"snapshot-a","snapshot-a"}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			cmd := commandAt(now)
			cmd.InputSnapshotRefs = tc.refs
			if _, err := RequestFingerprint("caller-a", cmd); err == nil {
				t.Fatal("invalid input_snapshot_refs accepted for fingerprint")
			}
		})
	}
}
