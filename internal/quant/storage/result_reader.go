package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	platformauthorization "example.com/stock-analysis-flow/internal/platform/authorization"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	"example.com/stock-analysis-flow/internal/quant/result"
)

var ErrResultRecordNotFound = errors.New("Quant result record not found")

type quantResultRecordReader interface {
	GetRecord(ctx context.Context, resultID string) (result.Record, error)
}

// MySQLAuthorizedResultReader joins the current Quant policy head, the exact
// Researchflow Run/Stage association, and the immutable Quant record.
// The policy-head row lock remains held until all three facts are verified.
type MySQLAuthorizedResultReader struct {
	policyGuard  quantapplication.ResultAccessPolicyReadGuard
	records      quantResultRecordReader
	associations quantapplication.ResultAssociationVerifier
}

func NewMySQLAuthorizedResultReader(
	policyGuard quantapplication.ResultAccessPolicyReadGuard,
	records quantResultRecordReader,
	associations quantapplication.ResultAssociationVerifier,
) (*MySQLAuthorizedResultReader, error) {
	if policyGuard == nil || records == nil || associations == nil {
		return nil, quantapplication.ErrInvalidGetResultRequest
	}
	return &MySQLAuthorizedResultReader{
		policyGuard: policyGuard, records: records, associations: associations,
	}, nil
}

var _ quantapplication.AuthorizedResultReader = (*MySQLAuthorizedResultReader)(nil)

func (reader *MySQLAuthorizedResultReader) ReadAuthorized(
	ctx context.Context,
	request quantapplication.ReadRequest,
) (quantapplication.AuthorizedRecord, error) {
	if reader == nil || reader.policyGuard == nil || reader.records == nil ||
		reader.associations == nil || ctx == nil ||
		!validAuditRunRef(request.RunRef) ||
		request.SubjectRef != "service:researchflow" ||
		!validQuantResultRef(request.ResourceRef) ||
		request.PurposeCode != "quant_analysis" || request.ContentLevel != "full" ||
		request.At.IsZero() || request.At.Location() != time.UTC {
		return quantapplication.AuthorizedRecord{}, quantapplication.ErrInvalidGetResultRequest
	}
	if err := ctx.Err(); err != nil {
		return quantapplication.AuthorizedRecord{}, err
	}

	var authorized quantapplication.AuthorizedRecord
	err := reader.policyGuard.WithCurrentResultAccessPolicy(ctx, func(state quantapplication.ResultAccessPolicyState) error {
		if state.Version == 0 || !state.Enabled || state.Snapshot == nil {
			return quantapplication.ErrAccessDenied
		}
		snapshot := cloneAuthorizedPolicySnapshot(*state.Snapshot)
		if snapshot.SchemaVersion != "1.0.0" || snapshot.SnapshotRef == "" ||
			len(snapshot.Policies) != 1 ||
			snapshot.Policies[0].PolicyType != "quant_result_access" ||
			snapshot.Policies[0].PolicyRef != "quant-result-access" ||
			snapshot.Policies[0].PolicyVersion != strconv.FormatUint(state.Version, 10) {
			return quantapplication.ErrAuthorizationUnavailable
		}

		if err := reader.associations.VerifyResultAssociation(ctx, request.RunRef, request.ResourceRef); err != nil {
			if errors.Is(err, quantapplication.ErrResultAssociationNotFound) {
				return quantapplication.ErrAccessDenied
			}
			return errors.Join(quantapplication.ErrAuthorizationUnavailable, err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		record, err := reader.records.GetRecord(ctx, request.ResourceRef)
		if err != nil {
			return errors.Join(quantapplication.ErrAuthorizationUnavailable, err)
		}
		record, err = record.ValidatedSnapshot()
		if err != nil || record.Manifest.ResultID != request.ResourceRef {
			return quantapplication.ErrInvalidStoredRecord
		}

		authorizationRef, err := newQuantAuthorizationRef(request, snapshot)
		if err != nil {
			return errors.Join(quantapplication.ErrAuthorizationUnavailable, err)
		}
		authorized = quantapplication.AuthorizedRecord{
			Record: record, AuthorizationRef: authorizationRef, PolicySnapshot: snapshot,
		}
		return nil
	})
	if err != nil {
		return quantapplication.AuthorizedRecord{}, err
	}
	if err := ctx.Err(); err != nil {
		return quantapplication.AuthorizedRecord{}, err
	}
	return authorized, nil
}

func newQuantAuthorizationRef(
	request quantapplication.ReadRequest,
	snapshot quantapplication.PolicySnapshot,
) (quantapplication.AuthorizationRef, error) {
	randomValue := make([]byte, 32)
	if _, err := rand.Read(randomValue); err != nil {
		return quantapplication.AuthorizationRef{}, fmt.Errorf("generate Quant authorization reference: %w", err)
	}
	ref := quantapplication.AuthorizationRef{
		SchemaVersion:    "1.0.0",
		AuthorizationRef: "authorization:quant:" + base64.RawURLEncoding.EncodeToString(randomValue),
		PolicyOwner:      "quant", PolicyVersion: snapshot.SnapshotRef, Decision: "allowed",
		PurposeCode: request.PurposeCode, ContentLevel: request.ContentLevel,
		SubjectRef: request.SubjectRef, ResourceRef: request.ResourceRef,
		DecidedAt: request.At.UTC().Format(time.RFC3339Nano),
		ExpiresAt: nil, ExpiresAtPresent: true, Conditions: []string{},
	}
	refJSON, err := json.Marshal(ref)
	if err != nil {
		return quantapplication.AuthorizationRef{}, err
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return quantapplication.AuthorizationRef{}, err
	}
	ref.DecisionFingerprint, err = platformauthorization.Fingerprint(refJSON, snapshotJSON)
	if err != nil {
		return quantapplication.AuthorizationRef{}, fmt.Errorf("fingerprint Quant authorization decision: %w", err)
	}
	return ref, nil
}

func cloneAuthorizedPolicySnapshot(value quantapplication.PolicySnapshot) quantapplication.PolicySnapshot {
	value.Policies = append([]quantapplication.PolicyVersion{}, value.Policies...)
	return value
}

// GetRecord reads and verifies one immutable result. It is deliberately a
// concrete storage operation, not an unauthenticated Quant Application port.
func (repository *MySQLRepository) GetRecord(ctx context.Context, resultID string) (result.Record, error) {
	if repository == nil || repository.db == nil || ctx == nil || !validQuantResultRef(resultID) {
		return result.Record{}, ErrInvalidRepository
	}
	if err := ctx.Err(); err != nil {
		return result.Record{}, err
	}
	var manifestFingerprint, resultFingerprint string
	var manifestJSON, payloadJSON []byte
	err := repository.db.QueryRowContext(ctx, `SELECT manifest_fingerprint, result_fingerprint, manifest_json, payload_json
		FROM quant_results WHERE result_id = ?`, resultID).Scan(
		&manifestFingerprint, &resultFingerprint, &manifestJSON, &payloadJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return result.Record{}, ErrResultRecordNotFound
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result.Record{}, ctxErr
		}
		return result.Record{}, errors.Join(ErrStorageUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return result.Record{}, err
	}
	var record result.Record
	decoder := json.NewDecoder(bytes.NewReader(manifestJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record.Manifest); err != nil {
		return result.Record{}, ErrCorruptRecord
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return result.Record{}, ErrCorruptRecord
	}
	if record.Manifest.ResultID != resultID ||
		record.Manifest.ManifestFingerprint != manifestFingerprint ||
		record.Manifest.ResultFingerprint != resultFingerprint {
		return result.Record{}, ErrCorruptRecord
	}
	record.Payload = append(json.RawMessage(nil), payloadJSON...)
	snapshot, err := record.ValidatedSnapshot()
	if err != nil {
		return result.Record{}, errors.Join(ErrCorruptRecord, err)
	}
	return snapshot, nil
}
