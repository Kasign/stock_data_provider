package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func validISODate(value string, optional bool) bool {
	if value == "" { return optional }
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func validObservedQuality(value model.QualityStatus) bool {
	return value == model.QualityValid || value == model.QualityDegraded || value == model.QualityUnknown
}

func adjustmentFactorHash(value model.AdjustmentFactor) (string, error) {
	value.SystemObservedAt = nil
	value.FetchedAt = time.Time{}
	payload, err := json.Marshal(value)
	if err != nil { return "", err }
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// SaveAdjustmentFactors 追加保存复权因子观测版本；同内容重复采集不重复写入。
func (s *Store) SaveAdjustmentFactors(ctx context.Context, values []model.AdjustmentFactor) error {
	if s == nil || s.db == nil { return errors.New("MariaDB adjustment-factor storage disabled") }
	for _, value := range values {
		if strings.TrimSpace(value.Symbol) == "" || !validISODate(value.TradeDate, false) ||
			value.Factor <= 0 || math.IsNaN(value.Factor) || math.IsInf(value.Factor, 0) ||
			strings.TrimSpace(value.Source) == "" || value.FetchedAt.IsZero() || !validObservedQuality(value.QualityStatus) {
			return fmt.Errorf("invalid adjustment factor observation for %s on %s", value.Symbol, value.TradeDate)
		}
		hash, err := adjustmentFactorHash(value)
		if err != nil { return err }
		stored := value
		stored.SystemObservedAt = nil
		metadata, err := json.Marshal(stored)
		if err != nil { return err }
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil { return fmt.Errorf("begin adjustment-factor revision: %w", err) }
		var latest string
		lookupErr := tx.QueryRowContext(ctx, `SELECT revision_hash FROM market_data_adjustment_factor_versions
WHERE symbol=? AND trade_date=? AND source=? ORDER BY recorded_at DESC,id DESC LIMIT 1 FOR UPDATE`,
			value.Symbol, value.TradeDate, value.Source).Scan(&latest)
		if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) { tx.Rollback(); return lookupErr }
		if lookupErr != nil || latest != hash {
			_, err = tx.ExecContext(ctx, `INSERT INTO market_data_adjustment_factor_versions
(symbol,trade_date,source,revision_hash,recorded_at,fetched_at,available_at,quality_status,metadata)
VALUES (?,?,?,?,?,?,?,?,?)`, value.Symbol, value.TradeDate, value.Source, hash,
				time.Now().UTC(), value.FetchedAt.UTC(), nullableTime(value.AvailableAt), string(value.QualityStatus), metadata)
			if err != nil { tx.Rollback(); return fmt.Errorf("insert adjustment-factor revision: %w", err) }
		}
		if err := tx.Commit(); err != nil { return fmt.Errorf("commit adjustment-factor revision: %w", err) }
	}
	return nil
}

// ListAdjustmentFactorsAsOf 返回各交易日、各来源在系统观察时点之前的最新版本。
func (s *Store) ListAdjustmentFactorsAsOf(ctx context.Context, symbol, startDate, endDate string, asOf time.Time) ([]model.AdjustmentFactor, error) {
	if s == nil || s.db == nil { return nil, errors.New("MariaDB adjustment-factor storage disabled") }
	if strings.TrimSpace(symbol) == "" || !validISODate(startDate, true) || !validISODate(endDate, true) || asOf.IsZero() || asOf.After(time.Now().UTC()) {
		return nil, errors.New("invalid adjustment-factor as_of query")
	}
	asOfUTC := asOf.UTC()
	rows, err := s.db.QueryContext(ctx, `SELECT v.metadata, DATE_FORMAT(v.recorded_at,'%Y-%m-%d %H:%i:%s.%f')
FROM market_data_adjustment_factor_versions v
WHERE v.symbol=? AND (?='' OR v.trade_date>=?) AND (?='' OR v.trade_date<=?) AND v.recorded_at<=?
AND NOT EXISTS (
 SELECT 1 FROM market_data_adjustment_factor_versions n
 WHERE n.symbol=v.symbol AND n.trade_date=v.trade_date AND n.source=v.source AND n.recorded_at<=?
 AND (n.recorded_at>v.recorded_at OR (n.recorded_at=v.recorded_at AND n.id>v.id))
)
ORDER BY v.trade_date ASC,v.source ASC,v.recorded_at DESC`,
		symbol,startDate,startDate,endDate,endDate,asOfUTC,asOfUTC)
	if err != nil { return nil, fmt.Errorf("query adjustment-factor revisions: %w", err) }
	defer rows.Close()
	out := []model.AdjustmentFactor{}
	for rows.Next() {
		var raw []byte
		var observed string
		if err := rows.Scan(&raw,&observed); err != nil { return nil,err }
		var value model.AdjustmentFactor
		if err := json.Unmarshal(raw,&value); err != nil { return nil,fmt.Errorf("decode adjustment factor: %w",err) }
		at, err := time.ParseInLocation("2006-01-02 15:04:05.000000",observed,time.UTC)
		if err != nil { return nil,err }
		value.SystemObservedAt=&at
		out=append(out,value)
	}
	return out,rows.Err()
}

func nullableTime(value *time.Time) any {
	if value == nil { return nil }
	return value.UTC()
}

func nullableDate(value string) any {
	if value == "" { return nil }
	return value
}

func corporateActionHash(value model.CorporateAction) (string,error) {
	value.SystemObservedAt=nil
	value.FetchedAt=time.Time{}
	payload,err:=json.Marshal(value)
	if err!=nil{return "",err}
	sum:=sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]),nil
}

// SaveCorporateActions 按事件标识追加保存公司行动修订，不覆盖此前已观察版本。
func (s *Store) SaveCorporateActions(ctx context.Context, values []model.CorporateAction) error {
	if s==nil || s.db==nil{return errors.New("MariaDB corporate-action storage disabled")}
	for _,value:=range values {
		if strings.TrimSpace(value.Symbol)=="" || strings.TrimSpace(value.ProviderEventID)=="" ||
			strings.TrimSpace(value.EventType)=="" || strings.TrimSpace(value.Source)=="" ||
			!validISODate(value.ExDate,false) || !validISODate(value.AnnouncementDate,true) ||
			!validISODate(value.RecordDate,true) || !validISODate(value.PayDate,true) ||
			value.FetchedAt.IsZero() || !validObservedQuality(value.QualityStatus) {
			return fmt.Errorf("invalid corporate-action event %s for %s",value.ProviderEventID,value.Symbol)
		}
		for _,number:=range []*float64{value.CashDividendCNY,value.BonusShareRatio,value.TransferShareRatio,value.RightsIssueRatio,value.RightsIssuePriceCNY} {
			if number!=nil && (math.IsNaN(*number)||math.IsInf(*number,0)||*number<0) {
				return fmt.Errorf("corporate-action event %s has invalid numeric terms",value.ProviderEventID)
			}
		}
		hash,err:=corporateActionHash(value);if err!=nil{return err}
		stored:=value
		stored.SystemObservedAt=nil
		metadata,err:=json.Marshal(stored);if err!=nil{return err}
		tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err}
		var latest string
		lookupErr:=tx.QueryRowContext(ctx,`SELECT revision_hash FROM market_data_corporate_action_versions
WHERE symbol=? AND provider_event_id=? AND source=? ORDER BY recorded_at DESC,id DESC LIMIT 1 FOR UPDATE`,
			value.Symbol,value.ProviderEventID,value.Source).Scan(&latest)
		if lookupErr!=nil && !errors.Is(lookupErr,sql.ErrNoRows){tx.Rollback();return lookupErr}
		if lookupErr!=nil || latest!=hash {
			_,err=tx.ExecContext(ctx,`INSERT INTO market_data_corporate_action_versions
(symbol,provider_event_id,source,ex_date,announcement_date,revision_hash,recorded_at,fetched_at,available_at,quality_status,metadata)
VALUES (?,?,?,?,?,?,?,?,?,?,?)`,value.Symbol,value.ProviderEventID,value.Source,value.ExDate,nullableDate(value.AnnouncementDate),hash,
				time.Now().UTC(),value.FetchedAt.UTC(),nullableTime(value.AvailableAt),string(value.QualityStatus),metadata)
			if err!=nil{tx.Rollback();return fmt.Errorf("insert corporate-action revision: %w",err)}
		}
		if err:=tx.Commit();err!=nil{return err}
	}
	return nil
}

// ListCorporateActionsAsOf 仅返回 as_of 时系统已经观察到的各事件最新修订。
func (s *Store) ListCorporateActionsAsOf(ctx context.Context,symbol,startDate,endDate string,asOf time.Time)([]model.CorporateAction,error){
	if s==nil||s.db==nil{return nil,errors.New("MariaDB corporate-action storage disabled")}
	if strings.TrimSpace(symbol)==""||!validISODate(startDate,true)||!validISODate(endDate,true)||asOf.IsZero()||asOf.After(time.Now().UTC()){return nil,errors.New("invalid corporate-action as_of query")}
	asOfUTC:=asOf.UTC()
	rows,err:=s.db.QueryContext(ctx,`SELECT v.metadata,DATE_FORMAT(v.recorded_at,'%Y-%m-%d %H:%i:%s.%f')
FROM market_data_corporate_action_versions v
WHERE v.symbol=? AND (?='' OR v.ex_date>=?) AND (?='' OR v.ex_date<=?) AND v.recorded_at<=?
AND NOT EXISTS (
 SELECT 1 FROM market_data_corporate_action_versions n
 WHERE n.symbol=v.symbol AND n.provider_event_id=v.provider_event_id AND n.source=v.source AND n.recorded_at<=?
 AND (n.recorded_at>v.recorded_at OR (n.recorded_at=v.recorded_at AND n.id>v.id))
)
ORDER BY v.ex_date ASC,v.source ASC,v.recorded_at DESC`,
		symbol,startDate,startDate,endDate,endDate,asOfUTC,asOfUTC)
	if err!=nil{return nil,fmt.Errorf("query corporate-action revisions: %w",err)}
	defer rows.Close()
	out:=[]model.CorporateAction{}
	for rows.Next(){
		var raw []byte;var observed string
		if err:=rows.Scan(&raw,&observed);err!=nil{return nil,err}
		var value model.CorporateAction
		if err:=json.Unmarshal(raw,&value);err!=nil{return nil,fmt.Errorf("decode corporate action: %w",err)}
		at,err:=time.ParseInLocation("2006-01-02 15:04:05.000000",observed,time.UTC);if err!=nil{return nil,err}
		value.SystemObservedAt=&at
		out=append(out,value)
	}
	return out,rows.Err()
}
