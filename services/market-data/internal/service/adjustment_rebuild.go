package service

import (
	"errors"
	"fmt"
	"sort"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// rebuildAdjustedDailyBars 仅使用同一来源版本、且系统观察与市场可用时间均已证明的因子重建价格。
func rebuildAdjustedDailyBars(raw []model.DailyBar, factors []model.AdjustmentFactor, basis model.AdjustmentBasis, requestedReferenceDate string) ([]model.DailyBar, error) {
	if basis != model.AdjustmentForward && basis != model.AdjustmentBackward { return nil, errors.New("adjustment rebuild requires forward or backward basis") }
	if len(raw)==0 || len(factors)==0 { return nil, errors.New("raw bars or adjustment factors are empty") }
	type key struct{ source,version string }
	groups:=map[key]map[string]model.AdjustmentFactor{}
	for _,factor:=range factors {
		if factor.Factor<=0 || factor.AvailableAt==nil || factor.SystemObservedAt==nil || factor.AvailableAt.IsZero() || factor.SystemObservedAt.IsZero() { continue }
		k:=key{factor.Source,factor.SourceVersion};if groups[k]==nil{groups[k]=map[string]model.AdjustmentFactor{}}
		if old,ok:=groups[k][factor.TradeDate];!ok || factor.SystemObservedAt.After(*old.SystemObservedAt){groups[k][factor.TradeDate]=factor}
	}
	keys:=make([]key,0,len(groups));for k:=range groups{keys=append(keys,k)}
	sort.Slice(keys,func(i,j int)bool{if keys[i].source==keys[j].source{return keys[i].version<keys[j].version};return keys[i].source<keys[j].source})
	var selected map[string]model.AdjustmentFactor;var reference model.AdjustmentFactor
	for _,k:=range keys{
		series:=groups[k];var candidate model.AdjustmentFactor
		for _,factor:=range series{if factor.TradeDate<=requestedReferenceDate&&(candidate.TradeDate==""||factor.TradeDate>candidate.TradeDate){candidate=factor}}
		if candidate.TradeDate==""{continue};complete:=true
		for _,bar:=range raw{if _,ok:=series[bar.TradeDate];!ok{complete=false;break}}
		if complete{selected=series;reference=candidate;break}
	}
	if selected==nil{return nil,fmt.Errorf("no single factor source/version covers every raw bar and reference date <= %s",requestedReferenceDate)}
	out:=make([]model.DailyBar,len(raw))
	for i,bar:=range raw{
		factor,ok:=selected[bar.TradeDate];if !ok{return nil,fmt.Errorf("adjustment factor missing for %s",bar.TradeDate)}
		scale:=factor.Factor;if basis==model.AdjustmentForward{scale/=reference.Factor}
		bar.Open*=scale;bar.High*=scale;bar.Low*=scale;bar.Close*=scale
		bar.MA5=nil;bar.MA20=nil;bar.MA120=nil;bar.MA250=nil;bar.ChangePercent=nil
		bar.AdjustmentBasis=basis;bar.AdjustmentMethod="cumulative_adj_factor_v1";bar.AdjustmentRefDate=reference.TradeDate
		bar.AdjustmentFactorSource=factor.Source;bar.AdjustmentFactorSourceVersion=factor.SourceVersion
		observed:=*factor.SystemObservedAt;bar.AdjustmentFactorSystemObservedAt=&observed
		out[i]=bar
	}
	for i:=1;i<len(out);i++{if out[i-1].Close>0{v:=(out[i].Close/out[i-1].Close-1)*100;out[i].ChangePercent=&v}}
	return out,nil
}
