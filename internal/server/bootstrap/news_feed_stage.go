package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	newsprovider "example.com/stock-analysis-flow/internal/news/adapters/provider"
	newssource "example.com/stock-analysis-flow/internal/news/adapters/source"
	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	newsdomain "example.com/stock-analysis-flow/internal/news/domain"
	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
)

const newsFeedStageOutputRegistryVersion = "1.2.0"

type newsFeedStageComposition struct {
	catalog              *newssource.StaticSourceCatalog
	providerCatalog      *newsprovider.StaticOutputPolicyCatalog
	qualificationCatalog *newssource.StaticAvailabilityQualificationCatalog
	registrations        []newsapp.SourceEndpointRegistration
	sourcePolicies       []newsconfig.FeedSource
	providerPolicies     []newsconfig.ProviderOutputPolicyRegistration
	executor             researchflowapplication.StageExecutor
	registry             *stageoutput.Registry
	resourceRefs         []string
}

func newNewsFeedStageComposition(
	ctx context.Context,
	cfg newsconfig.Config,
	client *platformhttpclient.Client,
	repository *newsstorage.MySQLRepository,
) (*newsFeedStageComposition, error) {
	feedSources := cfg.FeedSources()
	providerPolicies := cfg.ProviderOutputPolicies()
	qualifications := cfg.AvailabilityQualifications()
	if len(feedSources) == 0 {
		if len(providerPolicies) > 0 || len(qualifications) > 0 {
			return nil, errors.New("News G1 qualifications/provider policies require explicit trusted Feed source registrations")
		}
		return nil, nil
	}
	if ctx == nil || client == nil || repository == nil {
		return nil, errors.New("News Feed composition requires startup context, shared Platform HTTP Client and News Repository")
	}

	registrations := newsFeedRegistrations(feedSources)
	providerBindings := newsProviderPolicyBindings(providerPolicies)
	if err := validateNewsG1Bindings(registrations, providerBindings, qualifications); err != nil {
		return nil, fmt.Errorf("validate News G1 source/provider/qualification bindings: %w", err)
	}
	sourceCatalog, err := newssource.NewStaticSourceCatalog(registrations)
	if err != nil {
		return nil, fmt.Errorf("validate trusted News Feed source catalog: %w", err)
	}
	var outputPolicyCatalog *newsprovider.StaticOutputPolicyCatalog
	if len(providerBindings) > 0 {
		outputPolicyCatalog, err = newsprovider.NewStaticOutputPolicyCatalog(providerBindings)
		if err != nil {
			return nil, fmt.Errorf("validate trusted News Provider output policy catalog: %w", err)
		}
	}
	qualificationCatalog, err := newssource.NewStaticAvailabilityQualificationCatalog(qualifications)
	if err != nil {
		return nil, fmt.Errorf("validate trusted News availability qualification catalog: %w", err)
	}
	policies := make(map[string]newsconfig.FeedSource, len(feedSources))
	for _, source := range feedSources {
		if existing, ok := policies[source.SourceID]; ok {
			if existing.SourcePolicyVersionID != source.SourcePolicyVersionID ||
				existing.ExpectedCurrentSourcePolicyVersionID != source.ExpectedCurrentSourcePolicyVersionID {
				return nil, fmt.Errorf("News Feed source %q has conflicting policy version expectations", source.SourceID)
			}
			continue
		}
		policies[source.SourceID] = source
	}

	transport, err := newssource.NewPlatformEndpointTransport(client, registrations)
	if err != nil {
		return nil, fmt.Errorf("bind News Feed endpoints to Platform HTTP Client: %w", err)
	}
	collect, err := newsapp.NewCollectSourceWithPolicyResolver(sourceCatalog, transport, repository)
	if err != nil {
		return nil, fmt.Errorf("create News Feed CollectSource use case: %w", err)
	}
	ingest, err := newsapp.NewIngestCollectedSourceWithProviderPolicies(
		repository, newsFeedIngestID, sourceCatalog, outputPolicyCatalog,
	)
	if err != nil {
		return nil, fmt.Errorf("create News Feed IngestCollectedSource use case: %w", err)
	}
	adapter, err := newsstage.NewFeedCollectionAdapter(registrations, collect, ingest)
	if err != nil {
		return nil, fmt.Errorf("create News Feed collection Stage adapter: %w", err)
	}

	resourceRefs := make([]string, 0, len(registrations))
	for _, registration := range registrations {
		ref := newsstage.SourceEndpointSnapshotRef(registration)
		if ref == "" {
			return nil, errors.New("News Feed registration has an invalid Source/Endpoint identity")
		}
		resourceRefs = append(resourceRefs, ref)
	}
	sort.Strings(resourceRefs)
	policySourceIDs := make([]string, 0, len(policies))
	for sourceID := range policies {
		policySourceIDs = append(policySourceIDs, sourceID)
	}
	sort.Strings(policySourceIDs)

	sourcePolicies := make([]newsconfig.FeedSource, 0, len(policies))
	for _, sourceID := range policySourceIDs {
		sourcePolicies = append(sourcePolicies, policies[sourceID])
	}
	return &newsFeedStageComposition{
		catalog: sourceCatalog, providerCatalog: outputPolicyCatalog,
		qualificationCatalog: qualificationCatalog,
		registrations:        registrations, sourcePolicies: sourcePolicies,
		providerPolicies: providerPolicies,
		executor:         newsFeedStageExecutor{adapter: adapter},
		resourceRefs:     resourceRefs,
	}, nil
}

type newsPolicyPublication struct {
	PolicyType             string
	PolicyRef              string
	PolicyVersion          string
	ExpectedCurrentVersion string
	Payload                []byte
}

// publishNewsFeedPolicies runs only after the complete Server/Researchflow
// composition has passed in-memory validation. Every policy pointer remains
// protected by its own expected-current CAS transaction in the News Repository.
func publishNewsFeedPolicies(
	ctx context.Context,
	sources []newsconfig.FeedSource,
	providerPolicies []newsconfig.ProviderOutputPolicyRegistration,
	repository *newsstorage.MySQLRepository,
) error {
	publications, err := newsFeedPolicyPublications(sources, providerPolicies)
	if err != nil {
		return err
	}
	if len(publications) == 0 {
		return nil
	}
	if ctx == nil || repository == nil {
		return errors.New("News Feed policy publication dependencies are required")
	}
	publisher, err := newsapp.NewPublishNewsPolicyVersion(repository)
	if err != nil {
		return fmt.Errorf("create News Feed policy publisher: %w", err)
	}
	for _, publication := range publications {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := publisher.ExecuteIfCurrent(
			ctx, publication.PolicyType, publication.PolicyRef, publication.PolicyVersion,
			publication.ExpectedCurrentVersion, publication.Payload,
		); err != nil {
			return fmt.Errorf("publish News policy %s/%s@%s with expected current %q: %w",
				publication.PolicyType, publication.PolicyRef, publication.PolicyVersion,
				publication.ExpectedCurrentVersion, err)
		}
	}
	return nil
}

func newsFeedPolicyPublications(
	sources []newsconfig.FeedSource,
	providerPolicies []newsconfig.ProviderOutputPolicyRegistration,
) ([]newsPolicyPublication, error) {
	publications := make([]newsPolicyPublication, 0, len(sources)*2+len(providerPolicies))
	seen := make(map[string]newsPolicyPublication, cap(publications))
	add := func(publication newsPolicyPublication) error {
		key := publication.PolicyType + "\x00" + publication.PolicyRef
		if previous, exists := seen[key]; exists {
			if previous.PolicyVersion != publication.PolicyVersion ||
				previous.ExpectedCurrentVersion != publication.ExpectedCurrentVersion ||
				string(previous.Payload) != string(publication.Payload) {
				return fmt.Errorf("News Feed policy %s/%s has conflicting version expectations",
					publication.PolicyType, publication.PolicyRef)
			}
			return nil
		}
		seen[key] = publication
		publications = append(publications, publication)
		return nil
	}
	for _, source := range sources {
		payload, err := json.Marshal(source.SourcePolicy)
		if err != nil {
			return nil, fmt.Errorf("encode News Feed source policy %q: %w", source.SourceID, err)
		}
		if err := add(newsPolicyPublication{
			PolicyType: newsdomain.NewsPolicyTypeSource, PolicyRef: source.SourceID,
			PolicyVersion:          source.SourcePolicyVersionID,
			ExpectedCurrentVersion: source.ExpectedCurrentSourcePolicyVersionID,
			Payload:                payload,
		}); err != nil {
			return nil, err
		}
		if source.EndpointPolicy == nil {
			continue
		}
		endpointPayload, err := json.Marshal(source.EndpointPolicy)
		if err != nil {
			return nil, fmt.Errorf("encode News Feed endpoint policy %q: %w", source.EndpointID, err)
		}
		if err := add(newsPolicyPublication{
			PolicyType: newsdomain.NewsPolicyTypeEndpoint, PolicyRef: source.EndpointID,
			PolicyVersion:          source.EndpointPolicyVersionID,
			ExpectedCurrentVersion: source.ExpectedCurrentEndpointPolicyVersionID,
			Payload:                endpointPayload,
		}); err != nil {
			return nil, err
		}
	}
	for _, policy := range providerPolicies {
		payload, err := json.Marshal(policy.Policy)
		if err != nil {
			return nil, fmt.Errorf("encode News Provider output policy %q: %w", policy.ProviderCode, err)
		}
		if err := add(newsPolicyPublication{
			PolicyType: newsdomain.NewsPolicyTypeProviderOutput, PolicyRef: policy.PolicyRef,
			PolicyVersion:          policy.PolicyVersion,
			ExpectedCurrentVersion: policy.ExpectedCurrentPolicyVersion,
			Payload:                payload,
		}); err != nil {
			return nil, err
		}
	}
	sort.Slice(publications, func(i, j int) bool {
		if publications[i].PolicyType != publications[j].PolicyType {
			return publications[i].PolicyType < publications[j].PolicyType
		}
		return publications[i].PolicyRef < publications[j].PolicyRef
	})
	return publications, nil
}

func newsFeedIngestID() string {
	value, err := (researchflowIDGenerator{}).NewRef(context.Background(), "news")
	if err != nil {
		return ""
	}
	return value
}

type newsFeedStageExecutor struct {
	adapter *newsstage.FeedCollectionAdapter
}

func (e newsFeedStageExecutor) ExecuteStage(
	ctx context.Context,
	request researchflowapplication.StageExecutionRequest,
) (researchflowapplication.StageExecutionResult, error) {
	if request.Stage.Code != newsstage.CollectFeedsStageCode {
		return researchflowapplication.StageExecutionResult{}, fmt.Errorf("News Feed Stage Executor does not support stage code %q", request.Stage.Code)
	}
	policyRegistry, policyErr := resourcepolicy.NewRegistry()
	if policyErr != nil || e.adapter == nil ||
		request.ResourcePolicy.PolicyRef != resourcepolicy.NewsFeedConfiguredSourcesRef ||
		len(request.ResourcePolicy.AllowedProviderCodes) != 0 ||
		len(request.ResourcePolicy.AllowedSourceEndpointRefs) == 0 ||
		policyRegistry.ValidateSnapshot(request.ResourcePolicy) != nil {
		return failedNewsFeedStage("news_feed_resource_policy_invalid"), nil
	}
	output, err := e.adapter.Execute(ctx, request.Run.Ref, request.Stage.Code, request.ResourcePolicy.AllowedSourceEndpointRefs)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return researchflowapplication.StageExecutionResult{}, ctxErr
		}
		return failedNewsFeedStage("news_feed_adapter_failed"), nil
	}
	providerResult := &stageoutput.ProviderResult{
		OutputType: output.OutputType, SchemaVersion: output.SchemaVersion,
		Payload: output.Payload, QualitySummary: output.QualitySummary,
		DiagnosticRefs: []string{},
	}
	var payload newsstage.FeedCollectionOutput
	if err := json.Unmarshal(output.Payload, &payload); err != nil {
		return failedNewsFeedStage("news_feed_output_invalid"), nil
	}
	switch payload.Status {
	case "success", "empty":
		return researchflowapplication.StageExecutionResult{
			AttemptStatus:  researchflowdomain.AttemptSucceeded,
			StageStatus:    researchflowdomain.StageSucceeded,
			ProviderResult: providerResult,
		}, nil
	case "partial":
		if output.Retryable {
			return researchflowapplication.StageExecutionResult{
				AttemptStatus:  researchflowdomain.AttemptFailed,
				StageStatus:    researchflowdomain.StageRetryWait,
				ProviderResult: providerResult,
				ReasonCodes:    []string{"news_feed_retryable_source_failure"},
			}, nil
		}
		return researchflowapplication.StageExecutionResult{
			AttemptStatus:  researchflowdomain.AttemptDegraded,
			StageStatus:    researchflowdomain.StageDegraded,
			ProviderResult: providerResult,
			ReasonCodes:    output.ReasonCodes,
		}, nil
	case "failed_retryable":
		return researchflowapplication.StageExecutionResult{
			AttemptStatus:  researchflowdomain.AttemptFailed,
			StageStatus:    researchflowdomain.StageRetryWait,
			ProviderResult: providerResult,
			ReasonCodes:    []string{"news_feed_retryable_source_failure"},
		}, nil
	case "failed":
		reason := payload.FailureCode
		if reason == "" {
			reason = "news_feed_collection_failed"
		}
		return failedNewsFeedStageWithOutput(reason, providerResult), nil
	default:
		return failedNewsFeedStage("news_feed_status_invalid"), nil
	}
}

func failedNewsFeedStage(reasonCode string) researchflowapplication.StageExecutionResult {
	return researchflowapplication.StageExecutionResult{
		AttemptStatus: researchflowdomain.AttemptFailed,
		StageStatus:   researchflowdomain.StageFailed,
		ReasonCodes:   []string{reasonCode},
	}
}

func failedNewsFeedStageWithOutput(reasonCode string, output *stageoutput.ProviderResult) researchflowapplication.StageExecutionResult {
	result := failedNewsFeedStage(reasonCode)
	result.ProviderResult = output
	return result
}

func newsFeedRegistrations(feedSources []newsconfig.FeedSource) []newsapp.SourceEndpointRegistration {
	registrations := make([]newsapp.SourceEndpointRegistration, 0, len(feedSources))
	for _, source := range feedSources {
		registrations = append(registrations, newsapp.SourceEndpointRegistration{
			SourceID: source.SourceID, EndpointID: source.EndpointID, Enabled: true,
			URL: source.URL, PublisherName: source.PublisherName, PublisherDomain: source.PublisherDomain,
			AccessMethod: source.AccessMethod, SourcePolicy: source.SourcePolicy,
			SourcePolicyVersionID:         source.SourcePolicyVersionID,
			EndpointPolicy:                source.EndpointPolicy,
			EndpointPolicyVersionID:       source.EndpointPolicyVersionID,
			ProviderCode:                  source.ProviderCode,
			ProviderOutputPolicyRef:       source.ProviderOutputPolicyRef,
			ProviderOutputPolicyVersionID: source.ProviderOutputPolicyVersionID,
			URLPolicy:                     source.URLPolicy,
			MaxResponseBytes:              source.MaxResponseBytes,
			RequestParams:                 source.RequestParams,
		})
	}
	return registrations
}

func newsProviderPolicyBindings(
	policies []newsconfig.ProviderOutputPolicyRegistration,
) []newsapp.ProviderOutputPolicyBinding {
	bindings := make([]newsapp.ProviderOutputPolicyBinding, 0, len(policies))
	for _, policy := range policies {
		bindings = append(bindings, newsapp.ProviderOutputPolicyBinding{
			ProviderCode:  policy.ProviderCode,
			PolicyRef:     policy.PolicyRef,
			PolicyVersion: policy.PolicyVersion,
			Policy:        policy.Policy,
		})
	}
	return bindings
}

func validateNewsG1Bindings(
	registrations []newsapp.SourceEndpointRegistration,
	providerBindings []newsapp.ProviderOutputPolicyBinding,
	qualifications []newsapp.AvailabilitySourceQualification,
) error {
	providerByCode := make(map[string]newsapp.ProviderOutputPolicyBinding, len(providerBindings))
	for _, binding := range providerBindings {
		providerByCode[binding.ProviderCode] = binding
	}
	registrationByEndpoint := make(map[string]newsapp.SourceEndpointRegistration, len(registrations))
	for _, registration := range registrations {
		registrationByEndpoint[registration.SourceID+"\x00"+registration.EndpointID] = registration
		if registration.ProviderCode == "" {
			if registration.ProviderOutputPolicyRef != "" || registration.ProviderOutputPolicyVersionID != "" {
				return errors.New("News Feed registration has a partial provider policy binding")
			}
			continue
		}
		binding, exists := providerByCode[registration.ProviderCode]
		if !exists || binding.PolicyRef != registration.ProviderOutputPolicyRef ||
			binding.PolicyVersion != registration.ProviderOutputPolicyVersionID {
			return fmt.Errorf("News Feed registration %s/%s has no matching provider policy binding",
				registration.SourceID, registration.EndpointID)
		}
	}
	for _, qualification := range qualifications {
		registration, exists := registrationByEndpoint[qualification.SourceID+"\x00"+qualification.EndpointID]
		if !exists || registration.SourcePolicyVersionID != qualification.SourcePolicyVersionID ||
			registration.EndpointPolicyVersionID != qualification.EndpointPolicyVersionID ||
			registration.ProviderCode != qualification.ProviderCode ||
			registration.ProviderOutputPolicyRef != qualification.ProviderOutputPolicyRef ||
			registration.ProviderOutputPolicyVersionID != qualification.ProviderOutputPolicyVersionID ||
			registration.EndpointPolicy == nil {
			return fmt.Errorf("News qualification %q has no matching trusted registration", qualification.QualificationRef)
		}
		if !newsPolicyAllowsField(registration.SourcePolicy.AllowedMetadataFields, qualification.FieldPath) ||
			!newsPolicyAllowsField(registration.EndpointPolicy.AllowedMetadataFields, qualification.FieldPath) {
			return fmt.Errorf("News qualification %q field path is not authorized by source/endpoint policies", qualification.QualificationRef)
		}
		if qualification.ProviderCode != "" {
			binding, exists := providerByCode[qualification.ProviderCode]
			if !exists || binding.PolicyRef != qualification.ProviderOutputPolicyRef ||
				binding.PolicyVersion != qualification.ProviderOutputPolicyVersionID ||
				!newsPolicyAllowsField(binding.Policy.AllowedMetadataFields, qualification.FieldPath) {
				return fmt.Errorf("News qualification %q has no matching provider policy authorization", qualification.QualificationRef)
			}
		}
	}
	return nil
}

func newsPolicyAllowsField(values []string, fieldPath string) bool {
	for _, value := range values {
		if value == fieldPath {
			return true
		}
	}
	return false
}
