package service

import (
	"context"
	"testing"
	"time"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)
type dragonTigerFakeProvider struct { snapshot *model.DragonTigerSnapshot }
func (p dragonTigerFakeProvider) Name() string { return "eastmoney" }
func (p dragonTigerFakeProvider) FetchDragonTiger(context.Context, string, int, int) (*model.DragonTigerSnapshot, error) { return p.snapshot, nil }

func TestDragonTigerValidatesDateAndPreservesSignedNetBuy(t *testing.T) {
	now := time.Now().UTC(); change, closePrice, netBuy := 2.5, 10.25, -12345.0
	item := model.DragonTigerItem{Symbol:"600519.SH", Code:"600519", Name:"贵州茅台", TradeDate:"2026-10-22", ChangePercent:&change, ClosePrice:&closePrice, NetBuyAmountCNY:&netBuy,
		Reason:"测试上榜原因", Source:"eastmoney", SourceVersion:"test", FetchedAt:now, QualityStatus:model.QualityDegraded, QualityReasons:[]string{"provider_snapshot_timestamp_not_exposed"}}
	snapshot := &model.DragonTigerSnapshot{TradeDate:"2026-10-22", ProviderReportDate:"2026-10-22", Page:1, PageSize:100, Total:1, Pages:1, Returned:1, HasData:true,
		HasMore:false, Items:[]model.DragonTigerItem{item}, Source:"eastmoney", SourceVersion:"test", FetchedAt:now, QualityStatus:model.QualityDegraded, QualityReasons:[]string{"provider_snapshot_timestamp_not_exposed"}}
	p:=dragonTigerFakeProvider{snapshot:snapshot}
	svc:=NewWithCapabilities([]provider.NamedProvider{p},nil,nil,cache.NewDisabled(),nil,time.Second,time.Minute,120,2000,ResilienceConfig{Retries:0,CircuitFailureThreshold:3,CircuitCooldown:time.Second},nil)
	svc.SetDragonTigerProviders([]provider.DragonTigerProvider{p})
	got,hit,err:=svc.DragonTiger(context.Background(),"2026-10-22",1,100)
	if err!=nil||hit { t.Fatalf("DragonTiger hit=%v err=%v",hit,err) }
	if got.Items[0].NetBuyAmountCNY==nil||*got.Items[0].NetBuyAmountCNY!=-12345 { t.Fatalf("negative net buy must survive: %+v",got.Items[0]) }
	if _,_,err=svc.DragonTiger(context.Background(),"2026-10-21",1,100);err==nil { t.Fatal("must reject provider data for mismatched requested date") }
}
func TestValidateDragonTigerSnapshotRejectsMissingRowsForNonzeroTotal(t *testing.T) {
	now:=time.Now().UTC()
	snapshot:=&model.DragonTigerSnapshot{TradeDate:"2026-10-22",ProviderReportDate:"2026-10-22",Page:1,PageSize:100,Total:5,Pages:1,Returned:0,HasData:true,Items:[]model.DragonTigerItem{},Source:"eastmoney",SourceVersion:"test",FetchedAt:now,QualityStatus:model.QualityDegraded}
	if err:=validateDragonTigerSnapshot("2026-10-22",1,100,snapshot);err==nil { t.Fatal("nonzero total and empty page must be rejected") }
}
