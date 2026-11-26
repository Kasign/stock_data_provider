package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
)

const (
	newsAssociationTestRunRef       = "run:researchflow:news-001"
	newsAssociationTestNewsID       = "news:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	newsAssociationUnrelatedNewsID  = "news:sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	newsAssociationTestOutputRef    = "stage_output:sha256:" + "1111111111111111111111111111111111111111111111111111111111111111"
	newsAssociationTestQualityRef   = "stage_output:sha256:" + "2222222222222222222222222222222222222222222222222222222222222222"
	newsAssociationTestFeedStageRef = "stage:news.collect_feeds"
)

type newsAssociationRunReaderStub struct {
	result researchflowapplication.CreateRunResult
	err    error
	calls  int
}

func (stub *newsAssociationRunReaderStub) GetRun(_ context.Context, _ string) (researchflowapplication.CreateRunResult, error) {
	stub.calls++
	return stub.result, stub.err
}

type newsAssociationOutputReaderStub struct {
	content      json.RawMessage
	err          error
	calls        int
	runRefs      []string
	expectations []researchflowapplication.StageOutputExpectation
}

func (stub *newsAssociationOutputReaderStub) Read(
	_ context.Context,
	runRef string,
	expectation researchflowapplication.StageOutputExpectation,
) (researchflowapplication.StageOutputResource, error) {
	stub.calls++
	stub.runRefs = append(stub.runRefs, runRef)
	stub.expectations = append(stub.expectations, expectation)
	if stub.err != nil {
		return researchflowapplication.StageOutputResource{}, stub.err
	}
	return researchflowapplication.StageOutputResource{Content: []byte(stub.content)}, nil
}

type newsAssociationUseCaseStub struct {
	record  newsapp.ResearchEvidenceRecord
	err     error
	calls   int
	request newsapp.GetEvidenceRequest
}

func (stub *newsAssociationUseCaseStub) Execute(
	_ context.Context,
	request newsapp.GetEvidenceRequest,
) (newsapp.ResearchEvidenceRecord, error) {
	stub.calls++
	stub.request = request
	return stub.record, stub.err
}

func newsAssociationStage(runRef, code string, status researchflowdomain.StageStatus) researchflowdomain.Stage {
	outputRef, qualityRef := newsAssociationTestOutputRef, newsAssociationTestQualityRef
	return researchflowdomain.Stage{
		Ref: newsAssociationTestFeedStageRef, RunRef: runRef, Code: code, Status: status, AttemptNo: 1,
		OutputRef: &outputRef, QualitySummaryRef: &qualityRef,
	}
}

func newsAssociationRun(runRef string, stages ...researchflowdomain.Stage) researchflowapplication.CreateRunResult {
	return researchflowapplication.CreateRunResult{
		Run: researchflowdomain.Run{Ref: runRef}, Stages: stages,
	}
}

// newsAssociationFeedPayload 用生产 Validator 自检 fixture，确保测试不是靠伪造 payload 通过。
func newsAssociationFeedPayload(t *testing.T, newsIDs ...string) json.RawMessage {
	t.Helper()
	output := newsstage.FeedCollectionOutput{
		Status: "success", SourceCount: 1, SuccessSourceCount: 1,
		CollectedEntryCount: len(newsIDs), IngestedCount: len(newsIDs),
		Sources: []newsstage.FeedSourceOutcome{{
			SourceID: "source-1", EndpointID: "endpoint-1", Status: "success",
			CollectedCount: len(newsIDs), IngestedCount: len(newsIDs), NewsIDs: newsIDs,
		}},
	}
	payload, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := newsstage.ValidateFeedCollectionPayload(payload); err != nil {
		t.Fatalf("News Feed fixture does not pass the production Validator: %v", err)
	}
	return payload
}

func newsAssociationRequest(newsID string) newsapp.GetEvidenceRequest {
	return newsapp.GetEvidenceRequest{NewsID: newsID}
}

func newsAssociationVerifier(
	run researchflowapplication.CreateRunResult,
	content json.RawMessage,
) (newsEvidenceAssociationVerifier, *newsAssociationOutputReaderStub) {
	outputs := &newsAssociationOutputReaderStub{content: content}
	return newsEvidenceAssociationVerifier{
		runs:    &newsAssociationRunReaderStub{result: run},
		outputs: outputs,
	}, outputs
}

func TestNewsEvidenceAssociationMatchesFeedOutput(t *testing.T) {
	payload := newsAssociationFeedPayload(t, "news:0001", newsAssociationTestNewsID)
	run := newsAssociationRun(newsAssociationTestRunRef,
		newsAssociationStage(newsAssociationTestRunRef, newsstage.CollectFeedsStageCode, researchflowdomain.StageSucceeded))
	verifier, outputs := newsAssociationVerifier(run, payload)

	if err := verifier.VerifyEvidenceAssociation(context.Background(), newsAssociationTestRunRef, newsAssociationRequest(newsAssociationTestNewsID)); err != nil {
		t.Fatalf("same-Run evidence association must succeed: %v", err)
	}
	if outputs.calls != 1 || outputs.runRefs[0] != newsAssociationTestRunRef {
		t.Fatalf("Stage Output read count=%d runRefs=%v", outputs.calls, outputs.runRefs)
	}
	expectation := outputs.expectations[0]
	if expectation.StageCode != newsstage.CollectFeedsStageCode ||
		expectation.OutputType != newsstage.CollectFeedsOutputType ||
		expectation.SchemaVersion != newsstage.CollectFeedsSchemaVersion ||
		expectation.SchemaRef != newsstage.CollectFeedsPayloadSchemaRef ||
		expectation.QualitySummarySchemaRef != newsstage.CollectFeedsQualitySummarySchemaRef {
		t.Fatalf("News association expectation drifted from the production Registry: %+v", expectation)
	}
	if expectation.PayloadValidator == nil || expectation.QualitySummaryValidator == nil {
		t.Fatal("News association expectation is missing its production Validators")
	}
	if err := expectation.PayloadValidator(payload); err != nil {
		t.Fatalf("production payload Validator rejected its own fixture: %v", err)
	}
}

func TestNewsEvidenceAssociationRejectsUnrelatedOrMissingStage(t *testing.T) {
	payload := newsAssociationFeedPayload(t, newsAssociationTestNewsID)
	cases := []struct {
		name     string
		run      researchflowapplication.CreateRunResult
		newsID   string
		wantRead int
	}{
		{
			name: "different news id",
			run: newsAssociationRun(newsAssociationTestRunRef,
				newsAssociationStage(newsAssociationTestRunRef, newsstage.CollectFeedsStageCode, researchflowdomain.StageSucceeded)),
			newsID: newsAssociationUnrelatedNewsID, wantRead: 1,
		},
		{
			name: "run has no news stage",
			run: newsAssociationRun(newsAssociationTestRunRef,
				newsAssociationStage(newsAssociationTestRunRef, "news.search_candidates", researchflowdomain.StageSucceeded)),
			newsID: newsAssociationTestNewsID, wantRead: 0,
		},
		{
			name: "news stage did not succeed",
			run: newsAssociationRun(newsAssociationTestRunRef,
				newsAssociationStage(newsAssociationTestRunRef, newsstage.CollectFeedsStageCode, researchflowdomain.StageFailed)),
			newsID: newsAssociationTestNewsID, wantRead: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verifier, outputs := newsAssociationVerifier(tc.run, payload)
			err := verifier.VerifyEvidenceAssociation(context.Background(), newsAssociationTestRunRef, newsAssociationRequest(tc.newsID))
			if !errors.Is(err, newsapp.ErrEvidenceAssociationNotFound) {
				t.Fatalf("error = %v, want ErrEvidenceAssociationNotFound", err)
			}
			if outputs.calls != tc.wantRead {
				t.Fatalf("Stage Output reads = %d, want %d", outputs.calls, tc.wantRead)
			}
		})
	}
}

func TestNewsEvidenceAssociationFailsClosedOnInconsistentEvidence(t *testing.T) {
	payload := newsAssociationFeedPayload(t, newsAssociationTestNewsID)
	noOutput := newsAssociationStage(newsAssociationTestRunRef, newsstage.CollectFeedsStageCode, researchflowdomain.StageSucceeded)
	noOutput.OutputRef = nil
	noQuality := newsAssociationStage(newsAssociationTestRunRef, newsstage.CollectFeedsStageCode, researchflowdomain.StageSucceeded)
	noQuality.QualitySummaryRef = nil
	foreign := newsAssociationStage("run:researchflow:other", newsstage.CollectFeedsStageCode, researchflowdomain.StageSucceeded)
	valid := newsAssociationStage(newsAssociationTestRunRef, newsstage.CollectFeedsStageCode, researchflowdomain.StageSucceeded)

	cases := []struct {
		name string
		run  researchflowapplication.CreateRunResult
	}{
		{"succeeded stage without output ref", newsAssociationRun(newsAssociationTestRunRef, noOutput)},
		{"succeeded stage without quality ref", newsAssociationRun(newsAssociationTestRunRef, noQuality)},
		{"duplicate stage code", newsAssociationRun(newsAssociationTestRunRef, valid, valid)},
		{"stage belongs to another run", newsAssociationRun(newsAssociationTestRunRef, foreign)},
		{"run aggregate mismatch", newsAssociationRun("run:researchflow:other", valid)},
		{"run without stages", newsAssociationRun(newsAssociationTestRunRef)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verifier, _ := newsAssociationVerifier(tc.run, payload)
			err := verifier.VerifyEvidenceAssociation(context.Background(), newsAssociationTestRunRef, newsAssociationRequest(newsAssociationTestNewsID))
			if !errors.Is(err, researchflowapplication.ErrStageOutputIntegrity) {
				t.Fatalf("error = %v, want fail-closed ErrStageOutputIntegrity (must not degrade to not-found)", err)
			}
		})
	}
}

func TestNewsEvidenceAssociationPropagatesReadFailures(t *testing.T) {
	payload := newsAssociationFeedPayload(t, newsAssociationTestNewsID)
	run := newsAssociationRun(newsAssociationTestRunRef,
		newsAssociationStage(newsAssociationTestRunRef, newsstage.CollectFeedsStageCode, researchflowdomain.StageSucceeded))
	outputs := &newsAssociationOutputReaderStub{content: payload, err: researchflowapplication.ErrStageOutputUnavailable}
	verifier := newsEvidenceAssociationVerifier{runs: &newsAssociationRunReaderStub{result: run}, outputs: outputs}
	err := verifier.VerifyEvidenceAssociation(context.Background(), newsAssociationTestRunRef, newsAssociationRequest(newsAssociationTestNewsID))
	if err == nil || errors.Is(err, newsapp.ErrEvidenceAssociationNotFound) {
		t.Fatalf("read failure must propagate, not degrade to not-found: %v", err)
	}
	if !errors.Is(err, researchflowapplication.ErrStageOutputUnavailable) {
		t.Fatalf("read failure was not preserved: %v", err)
	}
}

func TestNewsEvidenceAssociationRejectsInvalidArguments(t *testing.T) {
	payload := newsAssociationFeedPayload(t, newsAssociationTestNewsID)
	run := newsAssociationRun(newsAssociationTestRunRef,
		newsAssociationStage(newsAssociationTestRunRef, newsstage.CollectFeedsStageCode, researchflowdomain.StageSucceeded))
	for _, runRef := range []string{"", "  ", newsAssociationTestRunRef + " ", " " + newsAssociationTestRunRef} {
		verifier, outputs := newsAssociationVerifier(run, payload)
		if err := verifier.VerifyEvidenceAssociation(context.Background(), runRef, newsAssociationRequest(newsAssociationTestNewsID)); err == nil {
			t.Fatalf("invalid runRef %q must be rejected", runRef)
		}
		if outputs.calls != 0 {
			t.Fatalf("invalid runRef %q still triggered %d Stage Output reads", runRef, outputs.calls)
		}
	}
	for _, newsID := range []string{"", "  ", " news:0001 "} {
		verifier, _ := newsAssociationVerifier(run, payload)
		if err := verifier.VerifyEvidenceAssociation(context.Background(), newsAssociationTestRunRef, newsAssociationRequest(newsID)); err == nil {
			t.Fatalf("invalid newsID %q must be rejected", newsID)
		}
	}
	failing := newsEvidenceAssociationVerifier{
		runs:    &newsAssociationRunReaderStub{result: run, err: errors.New("run store unavailable")},
		outputs: &newsAssociationOutputReaderStub{content: payload},
	}
	if err := failing.VerifyEvidenceAssociation(context.Background(), newsAssociationTestRunRef, newsAssociationRequest(newsAssociationTestNewsID)); err == nil ||
		errors.Is(err, newsapp.ErrEvidenceAssociationNotFound) {
		t.Fatalf("Run read failure must propagate: %v", err)
	}
}

func TestNewsEvidenceStageAssociationSpecsAreClosedAndComplete(t *testing.T) {
	if len(newsEvidenceStageAssociationSpecs) != 1 {
		t.Fatalf("spec table has %d entries, want exactly the registered News Evidence Stage", len(newsEvidenceStageAssociationSpecs))
	}
	spec, supported := newsEvidenceStageAssociationSpecFor(newsstage.CollectFeedsStageCode)
	if !supported {
		t.Fatal("news.collect_feeds is missing from the association spec table")
	}
	if strings.TrimSpace(spec.PayloadSchemaRef) == "" || strings.TrimSpace(spec.QualitySummarySchemaRef) == "" ||
		spec.PayloadValidator == nil || spec.QualitySummaryValidator == nil || spec.EvidenceIDs == nil {
		t.Fatalf("News association spec is incomplete: %+v", spec)
	}
	// search_candidates 只承载候选发现，不承载 Evidence 身份：未登记即拒绝。
	if _, supported := newsEvidenceStageAssociationSpecFor(newsstage.SearchCandidatesStageCode); supported {
		t.Fatal("news.search_candidates must not be registered as an Evidence-bearing Stage")
	}
}

func TestNewsFeedAssociationEvidenceIDsOnlyFromSuccessfulSources(t *testing.T) {
	raw := json.RawMessage(`{"sources":[
		{"source_id":"a","status":"success","news_ids":["news:0001","news:0002"]},
		{"source_id":"b","status":"failed","news_ids":["news:0003"]},
		{"source_id":"c","status":"empty","news_ids":["news:0004"]}
	]}`)
	ids, err := newsFeedAssociationEvidenceIDs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "news:0001" || ids[1] != "news:0002" {
		t.Fatalf("extracted IDs = %v, want only successful source IDs", ids)
	}
}

func TestResearchflowNewsEvidenceReaderVerifiesBeforeRead(t *testing.T) {
	payload := newsAssociationFeedPayload(t, newsAssociationTestNewsID)
	run := newsAssociationRun(newsAssociationTestRunRef,
		newsAssociationStage(newsAssociationTestRunRef, newsstage.CollectFeedsStageCode, researchflowdomain.StageSucceeded))
	verifier, _ := newsAssociationVerifier(run, payload)
	useCase := &newsAssociationUseCaseStub{record: newsapp.ResearchEvidenceRecord{
		NewsID: newsAssociationTestNewsID, AuthorizationStatus: "allowed",
	}}
	reader := &researchflowNewsEvidenceReader{verifier: verifier, read: useCase}

	// 无关联时不得触达 News 授权读取用例。
	if _, err := reader.Read(context.Background(), newsAssociationTestRunRef, newsAssociationRequest(newsAssociationUnrelatedNewsID)); !errors.Is(err, newsapp.ErrEvidenceAssociationNotFound) {
		t.Fatalf("unrelated read error = %v, want ErrEvidenceAssociationNotFound", err)
	}
	if useCase.calls != 0 {
		t.Fatalf("News GetEvidence was called %d times for an unrelated Evidence", useCase.calls)
	}
	// 同 Run 关联成立时委托既有用例，且不改写返回 DTO。
	record, err := reader.Read(context.Background(), newsAssociationTestRunRef, newsAssociationRequest(newsAssociationTestNewsID))
	if err != nil {
		t.Fatalf("authorized read failed: %v", err)
	}
	if useCase.calls != 1 || record.NewsID != newsAssociationTestNewsID || record.AuthorizationStatus != "allowed" {
		t.Fatalf("delegated read = calls %d record %+v", useCase.calls, record)
	}
	if record.RelationStatus != "" {
		t.Fatalf("wrapper must not forge RelationStatus: %q", record.RelationStatus)
	}
}
