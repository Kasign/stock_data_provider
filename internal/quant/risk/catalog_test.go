package risk

import (
	"errors"
	"reflect"
	"testing"
)

func TestDefaultRuleCatalogIsValid(t *testing.T) {
	catalog := DefaultRuleCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatalf("DefaultRuleCatalog().Validate() error = %v", err)
	}
	if catalog.Version != RuleCatalogVersion {
		t.Fatalf("catalog version = %q, want %q", catalog.Version, RuleCatalogVersion)
	}
}

// 目录加载出的规则集必须与实际执行入口使用的规则集完全一致，
// 否则"目录说要跑什么"与"实际跑了什么"会漂移。
func TestDefaultRuleCatalogLoadsTradabilityRuleSetIdentically(t *testing.T) {
	loaded, err := DefaultRuleCatalog().Load(TradabilityRuleSetVersion)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(loaded, TradabilityRuleSet()) {
		t.Fatalf("loaded rule set = %+v, want %+v", loaded, TradabilityRuleSet())
	}
}

func TestDefaultRuleCatalogRejectsUnpublishedRuleSetVersion(t *testing.T) {
	if _, err := DefaultRuleCatalog().Load("qnt-risk-unknown/1.0.0"); !errors.Is(err, ErrInvalidRuleCatalog) {
		t.Fatalf("Load() error = %v, want ErrInvalidRuleCatalog", err)
	}
}

func TestDefaultRuleCatalogRegistersInputQualityWithoutPublishingIt(t *testing.T) {
	catalog := DefaultRuleCatalog()
	var found bool
	for _, entry := range catalog.Entries {
		if entry.Definition.Code != InputQualityGateCode {
			continue
		}
		found = true
		if entry.Status != RuleEntryRegistered {
			t.Fatalf("input quality status = %q, want registered_not_published", entry.Status)
		}
		if entry.Definition.Severity != SeverityBlock || !entry.Definition.Required {
			t.Fatalf("input quality rule = %+v, want block + required", entry.Definition)
		}
	}
	if !found {
		t.Fatal("input quality rule is not registered")
	}
	// 未发布意味着不能让任何已发布规则集引用它。
	for _, published := range catalog.Published {
		for _, code := range published.RuleCodes {
			if code == InputQualityGateCode {
				t.Fatalf("rule set %q published unpublished rule %q", published.Version, code)
			}
		}
	}
}

// 缺规则的典型误用：把占位主题直接写进规则集。必须加载即失败，而不是少跑门禁。
func TestRuleCatalogRejectsPendingRuleInRuleSet(t *testing.T) {
	catalog := DefaultRuleCatalog()
	catalog.Published = append([]PublishedRuleSet{{
		Version:        "qnt-risk-pending/1.0.0",
		CatalogVersion: catalog.Version,
		RuleCodes:      []string{catalog.Pending[0].Code},
	}}, catalog.Published...)
	if err := catalog.Validate(); !errors.Is(err, ErrInvalidRuleCatalog) {
		t.Fatalf("Validate() error = %v, want ErrInvalidRuleCatalog", err)
	}
	if _, err := catalog.Load("qnt-risk-pending/1.0.0"); err == nil {
		t.Fatal("Load() error = nil, want rejection for pending rule")
	}
}

func TestRuleCatalogRejectsRegisteredButUnpublishedInRuleSet(t *testing.T) {
	catalog := DefaultRuleCatalog()
	catalog.Published = append([]PublishedRuleSet{{
		Version:        "qnt-risk-input-quality/1.0.0",
		CatalogVersion: catalog.Version,
		RuleCodes:      []string{InputQualityGateCode},
	}}, catalog.Published...)
	if err := catalog.Validate(); !errors.Is(err, ErrInvalidRuleCatalog) {
		t.Fatalf("Validate() error = %v, want ErrInvalidRuleCatalog", err)
	}
}

func TestRuleCatalogRejectsForeignCatalogVersion(t *testing.T) {
	catalog := DefaultRuleCatalog()
	catalog.Published[0].CatalogVersion = "qnt-risk-catalog/0.9.0"
	if err := catalog.Validate(); !errors.Is(err, ErrInvalidRuleCatalog) {
		t.Fatalf("Validate() error = %v, want ErrInvalidRuleCatalog", err)
	}
}

func TestRuleCatalogRejectsEntryWithoutSource(t *testing.T) {
	catalog := DefaultRuleCatalog()
	catalog.Entries[0].Source = " "
	if err := catalog.Validate(); !errors.Is(err, ErrInvalidRuleCatalog) {
		t.Fatalf("Validate() error = %v, want ErrInvalidRuleCatalog", err)
	}
}

func TestRuleCatalogRejectsBlockRuleThatIsNotRequired(t *testing.T) {
	catalog := DefaultRuleCatalog()
	catalog.Entries[0].Definition.Required = false
	if err := catalog.Validate(); !errors.Is(err, ErrInvalidRuleCatalog) {
		t.Fatalf("Validate() error = %v, want ErrInvalidRuleCatalog", err)
	}
}

func TestRuleCatalogRejectsDuplicateEntry(t *testing.T) {
	catalog := DefaultRuleCatalog()
	catalog.Entries = append(catalog.Entries, catalog.Entries[0])
	if err := catalog.Validate(); !errors.Is(err, ErrInvalidRuleCatalog) {
		t.Fatalf("Validate() error = %v, want ErrInvalidRuleCatalog", err)
	}
}

func TestRuleCatalogRejectsPendingCodeThatIsAlsoRegistered(t *testing.T) {
	catalog := DefaultRuleCatalog()
	catalog.Pending = append(catalog.Pending, PendingRule{
		Topic: "duplicate", Code: TradabilityGateCode, Reason: "占位编码不得与已登记规则重名",
	})
	if err := catalog.Validate(); !errors.Is(err, ErrInvalidRuleCatalog) {
		t.Fatalf("Validate() error = %v, want ErrInvalidRuleCatalog", err)
	}
}

func TestRuleCatalogPendingTopicsExposeGaps(t *testing.T) {
	catalog := DefaultRuleCatalog()
	topics := catalog.PendingTopics()
	if len(topics) != len(catalog.Pending) || len(topics) == 0 {
		t.Fatalf("PendingTopics() = %v, want %d items", topics, len(catalog.Pending))
	}
	topics[0] = "mutated"
	if catalog.PendingTopics()[0] == "mutated" {
		t.Fatal("PendingTopics() returned a slice aliased with catalog state")
	}
}
