package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
)

const resultIDPrefix = "quant_result:sha256:"

var (
	ErrInvalidManifest = errors.New("invalid quant result manifest")
	ErrInvalidPayload  = errors.New("invalid quant result payload")
	schemaVersionRE    = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
	fingerprintRE      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	allowedResultTypes = map[string]struct{}{
		"indicator":  {},
		"factor":     {},
		"trend":      {},
		"market":     {},
		"industry":   {},
		"stock":      {},
		"strategy":   {},
		"backtest":   {},
		"risk":       {},
		"evaluation": {},
	}
)

type Manifest struct {
	ResultID            string   `json:"result_id"`
	ManifestFingerprint string   `json:"manifest_fingerprint"`
	ResultType          string   `json:"result_type"`
	SchemaVersion       string   `json:"schema_version"`
	SystemAsOf          string   `json:"system_as_of"`
	InputSnapshotRefs   []string `json:"input_snapshot_refs"`
	ResultFingerprint   string   `json:"result_fingerprint"`
}

type Input struct {
	ResultType        string
	SchemaVersion     string
	SystemAsOf        time.Time
	InputSnapshotRefs []string
	ResultFingerprint string
}

type fingerprintInput struct {
	ResultType        string   `json:"result_type"`
	SchemaVersion     string   `json:"schema_version"`
	SystemAsOf        string   `json:"system_as_of"`
	InputSnapshotRefs []string `json:"input_snapshot_refs"`
	ResultFingerprint string   `json:"result_fingerprint"`
}

// FingerprintPayload 对强类型结果 payload 做 RFC 8785 JCS + SHA-256。
// 调用方应传入 payload 本身，不应把 AuthorizationRef 或 Manifest Envelope 混入 payload。
func FingerprintPayload(payload json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" || trimmed[0] != '{' || !utf8.Valid(payload) || !json.Valid(payload) {
		return "", fmt.Errorf("%w: payload must be a JSON object", ErrInvalidPayload)
	}
	canonical, err := jcs.Transform(payload)
	if err != nil {
		return "", fmt.Errorf("%w: canonicalize payload: %v", ErrInvalidPayload, err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// BuildWithPayload 根据 payload 计算结果指纹并生成 Manifest。
// 若调用方同时提供 ResultFingerprint，则必须与实际 payload 指纹一致。
func BuildWithPayload(input Input, payload json.RawMessage) (Manifest, error) {
	fingerprint, err := FingerprintPayload(payload)
	if err != nil {
		return Manifest{}, err
	}
	if input.ResultFingerprint != "" && input.ResultFingerprint != fingerprint {
		return Manifest{}, fmt.Errorf("%w: supplied result_fingerprint does not match payload", ErrInvalidPayload)
	}
	input.ResultFingerprint = fingerprint
	return Build(input)
}

// Build 按 QNT-CTR-005 生成不可变 Manifest；快照引用会排序，重复或空引用直接拒绝。
func Build(input Input) (Manifest, error) {
	if _, ok := allowedResultTypes[input.ResultType]; !ok {
		return Manifest{}, invalid("result_type")
	}
	if !schemaVersionRE.MatchString(input.SchemaVersion) {
		return Manifest{}, invalid("schema_version")
	}
	if input.SystemAsOf.IsZero() {
		return Manifest{}, invalid("system_as_of")
	}
	if !fingerprintRE.MatchString(input.ResultFingerprint) {
		return Manifest{}, invalid("result_fingerprint")
	}
	if len(input.InputSnapshotRefs) == 0 {
		return Manifest{}, invalid("input_snapshot_refs must not be empty")
	}

	refs := append([]string(nil), input.InputSnapshotRefs...)
	for _, ref := range refs {
		if strings.TrimSpace(ref) == "" || strings.TrimSpace(ref) != ref {
			return Manifest{}, invalid("input_snapshot_refs contains blank or padded reference")
		}
	}
	sort.Strings(refs)
	for i := 1; i < len(refs); i++ {
		if refs[i] == refs[i-1] {
			return Manifest{}, invalid("input_snapshot_refs contains duplicate reference")
		}
	}

	manifest := Manifest{
		ResultType:        input.ResultType,
		SchemaVersion:     input.SchemaVersion,
		SystemAsOf:        input.SystemAsOf.UTC().Format(time.RFC3339Nano),
		InputSnapshotRefs: refs,
		ResultFingerprint: input.ResultFingerprint,
	}
	fingerprint, err := manifest.calculateFingerprint()
	if err != nil {
		return Manifest{}, err
	}
	manifest.ManifestFingerprint = fingerprint
	manifest.ResultID = resultIDPrefix + fingerprint
	return manifest, nil
}

// Verify 检查 Manifest 的内容寻址身份、时间、快照引用排序及指纹。
func (m Manifest) Verify() error {
	if _, ok := allowedResultTypes[m.ResultType]; !ok {
		return invalid("result_type")
	}
	if !schemaVersionRE.MatchString(m.SchemaVersion) {
		return invalid("schema_version")
	}
	if !strings.HasSuffix(m.SystemAsOf, "Z") {
		return invalid("system_as_of must be UTC")
	}
	parsedAsOf, err := time.Parse(time.RFC3339Nano, m.SystemAsOf)
	if err != nil || parsedAsOf.UTC().Format(time.RFC3339Nano) != m.SystemAsOf {
		return invalid("system_as_of must use canonical UTC RFC3339Nano")
	}
	if len(m.InputSnapshotRefs) == 0 {
		return invalid("input_snapshot_refs must not be empty")
	}
	for i, ref := range m.InputSnapshotRefs {
		if strings.TrimSpace(ref) == "" || strings.TrimSpace(ref) != ref {
			return invalid("input_snapshot_refs contains blank or padded reference")
		}
		if i > 0 && m.InputSnapshotRefs[i-1] >= ref {
			return invalid("input_snapshot_refs must be sorted and unique")
		}
	}
	if !fingerprintRE.MatchString(m.ResultFingerprint) || !fingerprintRE.MatchString(m.ManifestFingerprint) {
		return invalid("fingerprint")
	}
	expected, err := m.calculateFingerprint()
	if err != nil {
		return err
	}
	if expected != m.ManifestFingerprint || m.ResultID != resultIDPrefix+expected {
		return invalid("content-addressed identity mismatch")
	}
	return nil
}

// VerifyPayload 同时验证 Manifest 自身身份和其绑定的 payload 内容。
func (m Manifest) VerifyPayload(payload json.RawMessage) error {
	if err := m.Verify(); err != nil {
		return err
	}
	fingerprint, err := FingerprintPayload(payload)
	if err != nil {
		return err
	}
	if fingerprint != m.ResultFingerprint {
		return fmt.Errorf("%w: payload fingerprint mismatch", ErrInvalidPayload)
	}
	return nil
}

func (m Manifest) calculateFingerprint() (string, error) {
	input := fingerprintInput{
		ResultType:        m.ResultType,
		SchemaVersion:     m.SchemaVersion,
		SystemAsOf:        m.SystemAsOf,
		InputSnapshotRefs: m.InputSnapshotRefs,
		ResultFingerprint: m.ResultFingerprint,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("%w: encode fingerprint input: %v", ErrInvalidManifest, err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: canonicalize fingerprint input: %v", ErrInvalidManifest, err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func invalid(field string) error {
	return fmt.Errorf("%w: %s", ErrInvalidManifest, field)
}
