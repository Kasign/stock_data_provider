import { getMarketBreadth, getMarketCapitalFlow, getMarketHotStocks, getMarketIndices, getMarketLimitPool, getMarketSentimentHistory, getMarketSectors } from "./api.js";
import { describeApiError, finiteNumberOrNull, formatCompact, formatDateTime, formatPercent, formatPrice, qualityLabel } from "./format.js";
const $ = (s) => document.querySelector(s);
const KEYS = ["indices", "breadth", "sectors", "pool", "hot"];
const REFRESH_OUTCOMES = new Set(["received", "unsupported", "provider_unavailable", "auth_failed", "failed", "needs_key", "cancelled"]);
export function summarizeMarketOverviewRefresh(outcomes, configured = true) {
 if (!configured) return "请先配置行情 API Key 后刷新市场概况";
 const normalized = (Array.isArray(outcomes) ? outcomes : []).map(value => REFRESH_OUTCOMES.has(value) ? value : "failed");
 if (!normalized.length) return "本次未执行市场模块请求";
 const count = value => normalized.filter(item => item === value).length;
 const parts = [`本次收到 ${count("received")}/${normalized.length} 个模块响应`];
 if (count("unsupported")) parts.push(`接口尚未提供 ${count("unsupported")} 个`);
 if (count("provider_unavailable")) parts.push(`Provider 不可用 ${count("provider_unavailable")} 个`);
 if (count("auth_failed")) parts.push(`鉴权失败 ${count("auth_failed")} 个`);
 if (count("failed")) parts.push(`读取失败 ${count("failed")} 个`);
 if (count("needs_key")) parts.push(`缺少 API Key、未请求 ${count("needs_key")} 个`);
 if (count("cancelled")) parts.push(`已取消 ${count("cancelled")} 个`);
 if (count("received") === normalized.length) parts.push("请查看各模块来源时间与质量状态");
 return parts.join("；");
}
function el(tag, cls = "", text = "") { const n=document.createElement(tag); if(cls)n.className=cls; if(text!==undefined&&text!==null)n.textContent=String(text); return n; }
function isValidTradeDate(value) {
 const date=String(value??"");
 if(!/^\d{4}-\d{2}-\d{2}$/.test(date))return false;
 const timestamp=Date.parse(date+"T00:00:00Z");
 return Number.isFinite(timestamp)&&new Date(timestamp).toISOString().slice(0,10)===date;
}
function count(v) { const n=finiteNumberOrNull(v); return n!==null&&n>=0?Math.trunc(n).toLocaleString("zh-CN"):"—"; }
function signed(v, fmt) { const n=finiteNumberOrNull(v); if(n===null)return "—";return (n>0?"+":"")+fmt(n); }
function dir(v) { const n=finiteNumberOrNull(v); if(n===null)return "market-data-unknown";return n>0?"market-data-positive":n<0?"market-data-negative":"market-data-flat"; }
function badge(id,label,tone="pending",quality="") { const n=$("#"+id);if(!n)return;n.textContent=label;if(tone==="quality"){const q=["valid","degraded","stale","invalid","unknown"].includes(String(quality).toLowerCase())?String(quality).toLowerCase():"unknown";n.className="quality-pill quality-"+q;}else n.className="availability-pill "+(tone==="error"||tone==="warning"?"is-unavailable":tone==="success"?"is-local":"is-readonly"); }
function meta(id,parts) { const n=$("#"+id);if(n)n.textContent=parts.filter(v=>v!==null&&v!==undefined&&String(v).trim()).join(" · "); }
function qualityReasons(id,values) { const d=$("#"+id),ul=d?.querySelector("ul");if(!d||!ul)return;ul.replaceChildren();const rows=Array.isArray(values)?values.filter(v=>String(v??"").trim()):[];d.hidden=!rows.length;rows.forEach(v=>ul.append(el("li","",v)));const s=d.querySelector("summary");if(s&&rows.length)s.textContent="数据质量说明（"+rows.length+"）"; }
function empty(root,msg) { root.replaceChildren(el("p","market-empty-state",msg)); }
function tableMessage(root,msg) { root.replaceChildren();const tr=document.createElement("tr"),td=el("td","table-empty",msg);td.colSpan=5;tr.append(td);root.append(tr); }
function cell(tr,value,cls="") { tr.append(el("td",cls,value===null||value===undefined||value===""?"—":value)); }
function pending(key,msg,tone="pending") {
 const ids={indices:"market-indices-status",breadth:"market-breadth-status",sectors:"market-sector-status",pool:"market-pool-status",hot:"market-hot-stocks-status"};
 const mids={indices:"market-indices-meta",breadth:"market-breadth-meta",sectors:"market-sector-meta",pool:"market-pool-meta",hot:"market-hot-stocks-meta"};
 const rids={breadth:"market-breadth-reasons",sectors:"market-sector-reasons",pool:"market-pool-reasons",hot:"market-hot-stocks-reasons"};
 const label=tone==="error"?"读取失败":tone==="warning"&&/API Key/.test(msg)?"需配置 Key":tone==="pending"?"加载中":"不可用";
 badge(ids[key],label,tone);meta(mids[key],[]);if(rids[key])qualityReasons(rids[key],[]);
 if(key==="indices")empty($("#market-index-grid"),msg);
 if(key==="breadth"){
  empty($("#market-breadth-values"),msg);
  ["#market-sentiment-advancing","#market-sentiment-declining","#market-sentiment-unchanged","#market-money-turnover"].forEach(id=>{const node=$(id);if(node)node.textContent="—";});
  const note=$("#market-money-turnover-note");if(note)note.textContent="本次市场广度数据不可用";
  const sentimentMeta=$("#market-sentiment-meta");if(sentimentMeta)sentimentMeta.textContent=msg;
 }
 if(key==="sectors")tableMessage($("#market-sector-body"),msg);
 if(key==="pool"){tableMessage($("#market-pool-body"),msg);const node=$("#market-sentiment-pool");if(node)node.textContent="本次数据不可用";}
 if(key==="hot")tableMessage($("#market-hot-stocks-body"),msg);
}
export function describeMarketOverviewError(err) {
 return describeApiError(err, "读取市场数据失败。");
}

export function defaultMarketCapitalFlowDateRange(now = new Date()) {
 const parts = new Intl.DateTimeFormat("en-US", {
  timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit"
 }).formatToParts(now);
 const value = type => parts.find(part => part.type === type)?.value || "";
 const end = value("year") + "-" + value("month") + "-" + value("day");
 const [year, month, day] = end.split("-").map(Number);
 const startDate = new Date(Date.UTC(year, month - 1, day - 89));
 const start = startDate.getUTCFullYear() + "-" + String(startDate.getUTCMonth() + 1).padStart(2, "0") + "-" + String(startDate.getUTCDate()).padStart(2, "0");
 return { start, end };
}
function errorMessage(err) { return describeMarketOverviewError(err); }
function renderIndices(data) {
 const root=$("#market-index-grid"),items=Array.isArray(data?.items)?data.items.filter(v=>v&&typeof v==="object"):[ ];root.replaceChildren();
 if(!items.length){empty(root,"接口已响应，但未返回可展示的指数明细。");badge("market-indices-status","未返回指数明细","warning");meta("market-indices-meta",["来源 "+(data?.source||"未报告"),"快照时间 "+formatDateTime(data?.fetched_at)]);return;}
 items.forEach(item=>{const card=el("article","market-index-card"),head=el("div","market-index-card-heading"),identity=el("div","market-index-identity");
 identity.append(el("h4","",item.name||item.code||item.symbol||"指数名称未报告"),el("small","",item.code||item.symbol||"代码未报告"));
 const q=["valid","degraded","stale","invalid","unknown"].includes(String(item.quality_status||"").toLowerCase())?String(item.quality_status).toLowerCase():"unknown";
 head.append(identity,el("span","quality-pill quality-"+q,qualityLabel(item.quality_status)));
 const changes=el("div","market-index-changes");changes.append(el("span",dir(item.change),"涨跌 "+signed(item.change,formatPrice)),el("strong",dir(item.change_percent),formatPercent(item.change_percent)));
 const footer=el("div","market-index-card-footer"),foot=[];if(item.quote_time)foot.push("行情时间 "+formatDateTime(item.quote_time));foot.push("来源 "+(item.source||data.source||"未报告"));footer.append(el("small","",foot.join(" · ")));
 if(Array.isArray(item.quality_reasons)&&item.quality_reasons.length)footer.append(el("small","market-index-reasons",item.quality_reasons.join("；")));
 card.append(head,el("div","market-index-price",formatPrice(item.latest_price)),changes,footer);root.append(card);});
 badge("market-indices-status",qualityLabel(data.quality_status),"quality",data.quality_status);meta("market-indices-meta",["来源 "+(data.source||"未报告"),"快照时间 "+formatDateTime(data.fetched_at),"版本 "+(data.source_version||"未报告")]);
}
function renderBreadth(data) {
 const root=$("#market-breadth-values"),fields=[["上涨",data?.advancing_count,"market-data-positive"],["下跌",data?.declining_count,"market-data-negative"],["平盘",data?.unchanged_count,"market-data-flat"],["未分类",data?.unclassified_count,"market-data-unknown"]];
 if($("#market-sentiment-advancing"))$("#market-sentiment-advancing").textContent=count(data?.advancing_count);
 if($("#market-sentiment-declining"))$("#market-sentiment-declining").textContent=count(data?.declining_count);
 if($("#market-sentiment-unchanged"))$("#market-sentiment-unchanged").textContent=count(data?.unchanged_count);
 if($("#market-money-turnover"))$("#market-money-turnover").textContent=data?.total_amount_cny===null||data?.total_amount_cny===undefined?"未提供":formatCompact(data.total_amount_cny)+" 元";
 if($("#market-money-turnover-note"))$("#market-money-turnover-note").textContent="金额覆盖 "+count(data?.amount_coverage_count)+" / "+count(data?.covered_securities)+" 只 · 仅为成交额汇总";
 if($("#market-sentiment-meta"))$("#market-sentiment-meta").textContent="报告样本 "+count(data?.total_securities)+" 只 · 覆盖 "+count(data?.covered_securities)+" 只 · 来源 "+(data?.source||"未报告")+" · 快照 "+formatDateTime(data?.fetched_at);
 if(!data||!fields.some(([,v])=>v!==null&&v!==undefined&&v!=="")){empty(root,"接口未返回可展示的涨跌分布字段。");badge("market-breadth-status","数据字段不完整","warning");return;}
 root.replaceChildren();fields.forEach(([label,value,cls])=>{const m=el("div","market-breadth-metric");m.append(el("small","",label),el("strong",cls,count(value)));root.append(m);});
 badge("market-breadth-status",qualityLabel(data.quality_status),"quality",data.quality_status);
 const amount=data.total_amount_cny===null||data.total_amount_cny===undefined?"全市场成交额未由此接口提供":"成交额 "+formatCompact(data.total_amount_cny)+" 元";
 meta("market-breadth-meta",["报告样本 "+count(data.total_securities)+" 只","覆盖样本 "+count(data.covered_securities)+" 只",amount,"来源 "+(data.source||"未报告"),"快照时间 "+formatDateTime(data.fetched_at)]);qualityReasons("market-breadth-reasons",data.quality_reasons);
}
function sectorMetricSortBy(metric) {
 return metric==="amount"?"amount":metric==="five-day"?"change_5d":"change_percent";
}
function sectorMetricLabel(sortBy) {
 return sortBy==="amount"?"成交额":sortBy==="change_5d"?"近 5 日涨幅":"今日涨幅";
}
function sectorMetricValue(item,sortBy) {
 if(sortBy==="amount"){
  const amount=finiteNumberOrNull(item.amount);
  return amount===null?"—":formatCompact(amount)+(item.amount_unit==="CNY"?" 元":"（单位未确认）");
 }
 return formatPercent(sortBy==="change_5d"?item.change_percent_5d:item.change_percent);
}
function renderSectors(data,metric="change",direction="up",requestedLimit=10) {
 const root=$("#market-sector-body"),items=Array.isArray(data?.items)?data.items.filter(v=>v&&typeof v==="object"):[ ];root.replaceChildren();
 const sortBy=["change_percent","amount","change_5d"].includes(data?.sort_by)?data.sort_by:sectorMetricSortBy(metric);
 const sortLabel=sectorMetricLabel(sortBy),directionLabel=(data?.direction||direction)==="down"?"从低到高":"从高到低";
 const heading=$("#market-sector-metric-heading");if(heading)heading.textContent=sortLabel;
 if(!items.length){tableMessage(root,"接口已响应，但没有返回板块记录；这不代表市场不存在相关板块。");badge("market-sector-status","暂无板块明细","warning");}
 else{items.forEach(item=>{const tr=document.createElement("tr");cell(tr,item.rank===null||item.rank===undefined?"—":count(item.rank),"market-rank-cell");const name=el("td");name.append(el("strong","market-table-primary",item.name||"名称未报告"),el("small","market-table-secondary",item.code||"代码未报告"));tr.append(name);const metricValue=sortBy==="amount"?finiteNumberOrNull(item.amount):sortBy==="change_5d"?item.change_percent_5d:item.change_percent;cell(tr,sectorMetricValue(item,sortBy),sortBy==="amount"?"market-rank-cell":dir(metricValue));cell(tr,item.leading_stock||"—");cell(tr,formatPercent(item.leader_change_percent),dir(item.leader_change_percent));root.append(tr);});badge("market-sector-status",qualityLabel(data.quality_status),"quality",data.quality_status);}
 meta("market-sector-meta",[(data?.category==="concept"?"概念":"行业")+"排行","排序维度 "+sortLabel,directionLabel,"请求上限 "+count(requestedLimit)+" 条","实际返回 "+count(items.length)+" 条","源端总数 "+count(data?.total),"来源 "+(data?.source||"未报告"),"源端日期 "+(data?.provider_report_date||"未报告"),"快照时间 "+formatDateTime(data?.fetched_at)]);qualityReasons("market-sector-reasons",data?.quality_reasons);
}
function renderPool(data,requestedLimit=30) {
 const root=$("#market-pool-body"),items=Array.isArray(data?.items)?data.items.filter(v=>v&&typeof v==="object"):[ ];root.replaceChildren();
 if(!items.length){tableMessage(root,data?.has_data===false?"服务端未确认此池有可用数据；不能把当前空列表当作 0 只。":"接口未返回列表记录。");badge("market-pool-status",data?.has_data===false?"数据未确认":"暂无列表记录","warning");}
 else{items.forEach(item=>{const tr=document.createElement("tr"),stock=el("td"),symbol=String(item.symbol||item.code||"").trim(),name=String(item.name||symbol||"名称未报告");stock.append(el("strong","market-table-primary",name),el("small","market-table-secondary",symbol||"代码未报告"));if(symbol){const actions=el("div","market-stock-entry-actions");[["stocks","查看行情"],["research","深度研究"]].forEach(([target,label])=>{const button=el("button","button button-secondary market-stock-entry-action",label);button.type="button";button.dataset.poolSymbol=symbol;button.dataset.poolName=name;button.dataset.poolAction=target;actions.append(button);});stock.append(actions);}tr.append(stock);cell(tr,formatPercent(item.change_percent),dir(item.change_percent));cell(tr,item.consecutive_boards===null||item.consecutive_boards===undefined?"—":count(item.consecutive_boards)+" 板");cell(tr,(item.first_touch_time||"—")+" / "+(item.last_touch_time||"—"),"market-touch-time");cell(tr,item.industry||"—");root.append(tr);});badge("market-pool-status",qualityLabel(data.quality_status),"quality",data.quality_status);}
 const label=data?.type==="down"?"跌停池":data?.type==="broken"?"炸板池":"涨停池";
 if($("#market-sentiment-pool"))$("#market-sentiment-pool").textContent=data?.has_data===false?"数据未确认":label+" · "+count(data?.total)+" 只";
 meta("market-pool-meta",[label,"交易日期 "+(data?.trade_date||poolDate||"中国时区当日（默认）"),"源端数量 "+count(data?.total),"请求上限 "+count(requestedLimit)+" 条","当前返回 "+count(items.length)+" 条","来源 "+(data?.source||"未报告"),"快照时间 "+formatDateTime(data?.fetched_at)]);qualityReasons("market-pool-reasons",data?.quality_reasons);
}
function renderHotStocks(data,requestedLimit=20) {
 const root=$("#market-hot-stocks-body"),items=Array.isArray(data?.items)?data.items.filter(item=>item&&typeof item==="object"):[];
 root.replaceChildren();
 if(!items.length) {
  tableMessage(root,"接口未返回人气榜/飙升榜记录；当前列表不是全市场证券的完整清单。");
  badge("market-hot-stocks-status","暂无列表记录","warning");
 } else {
  items.forEach(item=>{
   const tr=document.createElement("tr");
   cell(tr,item.rank===null||item.rank===undefined?"—":count(item.rank),"market-rank-cell");
   const stock=el("td"),button=el("button","market-hot-stock-link"),symbol=String(item.symbol||item.code||"").trim(),name=String(item.name||symbol||"名称未报告");
   button.type="button";button.dataset.hotSymbol=symbol;button.dataset.hotName=name;button.dataset.hotAction="stocks";button.disabled=!symbol;
   button.append(el("strong","market-table-primary",name),el("small","market-table-secondary",symbol||"代码未报告"));
   stock.append(button);
   if(symbol){const research=el("button","button button-secondary market-stock-entry-action","深度研究");research.type="button";research.dataset.hotSymbol=symbol;research.dataset.hotName=name;research.dataset.hotAction="research";stock.append(research);}
   tr.append(stock);
   const price=el("td");price.append(el("strong","market-table-primary",formatPrice(item.latest_price)),el("small","market-table-secondary","涨跌额 "+formatPrice(item.change)));tr.append(price);
   cell(tr,formatPercent(item.change_percent),dir(item.change_percent));
   cell(tr,item.rank_change===null||item.rank_change===undefined?"—":String(item.rank_change),"market-rank-cell");
   root.append(tr);
  });
  badge("market-hot-stocks-status",qualityLabel(data.quality_status),"quality",data.quality_status);
 }
 meta("market-hot-stocks-meta",[(data?.type==="surge"?"飙升榜":"人气榜"),"请求上限 "+count(requestedLimit)+" 条","实际返回 "+count(items.length)+" 条（非全市场总数）","来源 "+(data?.source||"未报告"),"采集时间 "+formatDateTime(data?.fetched_at),"源端未提供可独立核验的行情快照时间"]);
 qualityReasons("market-hot-stocks-reasons",data?.quality_reasons);
}
function renderMarketCapitalFlow(data) {
 const root=$("#market-capital-flow-body"),items=Array.isArray(data?.items)?data.items.filter(item=>item&&typeof item==="object"):[];
 if(root){
  root.replaceChildren();
  if(!items.length)tableMessage(root,"接口已响应，但该日期范围内没有可展示的市场级资金流记录。");
  items.forEach(item=>{
   const row=document.createElement("tr");cell(row,item.trade_date||"—");
   const sh=el("td");sh.append(el("strong","market-table-primary",formatPrice(item.close_sh)),el("small","market-table-secondary","涨跌 "+formatPercent(item.change_percent_sh)));row.append(sh);
   const sz=el("td");sz.append(el("strong","market-table-primary",formatPrice(item.close_sz)),el("small","market-table-secondary","涨跌 "+formatPercent(item.change_percent_sz)));row.append(sz);
   const net=finiteNumberOrNull(item.net_amount_cny);cell(row,net===null?"—":(net>0?"+":"")+formatCompact(net)+" 元",dir(net));
   cell(row,formatPercent(item.net_amount_rate_percent),dir(item.net_amount_rate_percent));root.append(row);
  });
 }
 badge("market-capital-flow-status",items.length?qualityLabel(data?.quality_status):"暂无历史记录",items.length?"quality":"warning",data?.quality_status);
 meta("market-capital-flow-meta",["请求 "+(data?.start_date||"未报告")+" 至 "+(data?.end_date||"未报告"),"返回 "+count(items.length)+" / "+count(data?.total)+" 条","来源 "+(data?.source||"未报告"),"版本 "+(data?.source_version||"未报告"),"采集时间 "+formatDateTime(data?.fetched_at)]);
 qualityReasons("market-capital-flow-reasons",data?.quality_reasons);
}
export function bindMarketOverview(getSettings,onSelectStock) {
 const controllers={},generation={indices:0,breadth:0,sectors:0,pool:0,hot:0};let sectorType="industry",sectorMetric="change",sectorDirection="up",sectorLimit=10,poolType="up",poolDate="",poolLimit=30,hotType="popular",hotLimit=20,refreshGenerations=Object.create(null);
 let capitalFlowController=null,capitalFlowGeneration=0;
 function showSectorMetricUnavailable(metric=sectorMetric){
  const root=$("#market-sector-body");
  if(root)tableMessage(root,"板块资金净流入需要真实板块级资金流接口，当前尚未接入；不会用成交额替代资金净流入。");
  badge("market-sector-status","接口待接入","warning");
  meta("market-sector-meta",["维度 资金净流入","当前为能力占位，不代表没有符合条件的板块"]);
  qualityReasons("market-sector-reasons",[]);
 }
 function safeOverviewUi(operation){try{operation();return true;}catch{return false;}}
 function safeOverviewError(error){try{return errorMessage(error);}catch{return "未知异常";}}
 function cancel(key){const controller=controllers[key];delete controllers[key];generation[key]++;try{controller?.abort();}catch{}}
 async function load(key){
  let seq=null,settings=null,ctl=null;
  try{
   cancel(key);seq=generation[key];settings=typeof getSettings==="function"?getSettings():null;
   if(key==="sectors"&&sectorMetric==="flow"){showSectorMetricUnavailable();return "unsupported";}
   if(!settings?.apiKey){pending(key,"请先在连接与密钥设置中配置行情 API Key。","warning");return "needs_key";}
   ctl=new AbortController();controllers[key]=ctl;pending(key,"正在读取…","pending");
   let data;
   if(key==="indices")data=await getMarketIndices(settings,ctl.signal);
   else if(key==="breadth")data=await getMarketBreadth(settings,ctl.signal);
   else if(key==="sectors")data=await getMarketSectors(settings,{type:sectorType,limit:sectorLimit,direction:sectorDirection,sortBy:sectorMetricSortBy(sectorMetric),signal:ctl.signal});
   else if(key==="pool")data=await getMarketLimitPool(settings,{type:poolType,limit:poolLimit,date:poolDate,signal:ctl.signal});
   else if(key==="hot")data=await getMarketHotStocks(settings,{type:hotType,limit:hotLimit,signal:ctl.signal});
   else return "failed";
   if(ctl.signal.aborted||generation[key]!==seq)return "cancelled";
   if(key==="indices")renderIndices(data);else if(key==="breadth")renderBreadth(data);else if(key==="sectors")renderSectors(data,sectorMetric,sectorDirection,sectorLimit);else if(key==="pool")renderPool(data,poolLimit);else renderHotStocks(data,hotLimit);
   return "received";
  }catch(error){
   if(ctl?.signal.aborted||(seq!==null&&generation[key]!==seq))return "cancelled";
   safeOverviewUi(()=>pending(key,safeOverviewError(error),"error"));
   const status=Number(error?.status);
   if(status===404)return "unsupported";if(status===503)return "provider_unavailable";
   if(status===401||status===403)return "auth_failed";
   return "failed";
  }finally{
   if(seq!==null&&generation[key]===seq)safeOverviewUi(()=>{if(controllers[key]===ctl)delete controllers[key];});
  }
 }
 async function refresh(keys=KEYS,statusSelector="#market-overview-updated"){
  const requested=Array.isArray(keys)?keys.filter(key=>KEYS.includes(key)):KEYS;
  const currentGeneration=(refreshGenerations[statusSelector]||0)+1;
  refreshGenerations[statusSelector]=currentGeneration;
  let status=null,settings=null;
  try{
   settings=typeof getSettings==="function"?getSettings():null;
   safeOverviewUi(()=>{status=$(statusSelector);if(status)status.textContent=settings?.apiKey?"正在刷新市场模块…":"配置行情 API Key 后加载市场数据";});
   const outcomes=await Promise.all(requested.map(load));
   if(currentGeneration!==refreshGenerations[statusSelector])return outcomes;
   safeOverviewUi(()=>{if(status)status.textContent=summarizeMarketOverviewRefresh(outcomes,Boolean(settings?.apiKey));});
   return outcomes;
  }catch(error){
   if(currentGeneration!==refreshGenerations[statusSelector])return [];
   const message="市场模块刷新失败："+safeOverviewError(error);
   safeOverviewUi(()=>{if(!status)status=$(statusSelector);if(status)status.textContent=message;});
   return [];
  }
 }
 function invalidateConnection(){
  Object.keys(refreshGenerations).forEach(selector=>{refreshGenerations[selector]+=1;});
  KEYS.forEach(key=>{
   safeOverviewUi(()=>cancel(key));
   safeOverviewUi(()=>pending(key,"行情连接配置已变更，旧结果已清除，请重新刷新。","warning"));
  });
  capitalFlowGeneration++;
  try{capitalFlowController?.abort();}catch{}
  capitalFlowController=null;
  safeOverviewUi(()=>{
   const status=$("#market-capital-flow-status"),body=$("#market-capital-flow-body");
   if(status){status.textContent="连接已变更";status.className="availability-pill is-readonly";}
   const button=$("#load-market-capital-flow");
   if(button){button.disabled=false;button.textContent="读取历史资金流";}
   if(body)tableMessage(body,"行情连接配置已变更，请重新读取市场资金流。");
   meta("market-capital-flow-meta",[]);qualityReasons("market-capital-flow-reasons",[]);
  });
  ["#market-overview-updated","#market-data-updated"].forEach(selector=>safeOverviewUi(()=>{
   const node=$(selector);if(node)node.textContent="行情连接配置已变更，旧结果已清除，请重新刷新市场模块。";
  }));
 }

 $("#refresh-market-overview")?.addEventListener("click",()=>{void refresh(["indices","breadth"],"#market-overview-updated");});
 $("#refresh-market-data")?.addEventListener("click",()=>{void refresh(["sectors","pool","hot"],"#market-data-updated");});
 document.querySelectorAll("[data-sector-type]").forEach(button=>button.addEventListener("click",()=>{sectorType=button.dataset.sectorType==="concept"?"concept":"industry";document.querySelectorAll("[data-sector-type]").forEach(item=>item.setAttribute("aria-pressed",String(item===button)));void load("sectors");}));
 document.querySelectorAll("[data-sector-metric]").forEach(button=>button.addEventListener("click",()=>{sectorMetric=["change","amount","five-day","flow"].includes(button.dataset.sectorMetric)?button.dataset.sectorMetric:"change";document.querySelectorAll("[data-sector-metric]").forEach(item=>item.setAttribute("aria-pressed",String(item===button)));void load("sectors");}));
 document.querySelectorAll("[data-sector-direction]").forEach(button=>button.addEventListener("click",()=>{sectorDirection=button.dataset.sectorDirection==="down"?"down":"up";document.querySelectorAll("[data-sector-direction]").forEach(item=>item.setAttribute("aria-pressed",String(item===button)));void load("sectors");}));
 const sectorLimitControl=$("#market-sector-limit");
 sectorLimitControl?.addEventListener("change",event=>{const value=Number(event.target.value);sectorLimit=[10,20,50,100].includes(value)?value:10;if(sectorLimitControl.value!==String(sectorLimit))sectorLimitControl.value=String(sectorLimit);void load("sectors");});
 document.querySelectorAll("[data-pool-type]").forEach(button=>button.addEventListener("click",()=>{poolType=["up","down","broken"].includes(button.dataset.poolType)?button.dataset.poolType:"up";document.querySelectorAll("[data-pool-type]").forEach(item=>item.setAttribute("aria-pressed",String(item===button)));void load("pool");}));
 const poolLimitControl=$("#market-pool-limit");
 poolLimitControl?.addEventListener("change",event=>{const value=Number(event.target.value);poolLimit=[10,20,30,50,100].includes(value)?value:30;if(poolLimitControl.value!==String(poolLimit))poolLimitControl.value=String(poolLimit);void load("pool");});
 $("#market-pool-date")?.addEventListener("change",event=>{poolDate=String(event.target.value||"");void load("pool");});
 document.querySelectorAll("[data-hot-type]").forEach(button=>button.addEventListener("click",()=>{hotType=["popular","surge"].includes(button.dataset.hotType)?button.dataset.hotType:"popular";document.querySelectorAll("[data-hot-type]").forEach(item=>item.setAttribute("aria-pressed",String(item===button)));void load("hot");}));
 const hotLimitControl=$("#market-hot-stocks-limit");
 hotLimitControl?.addEventListener("change",event=>{const value=Number(event.target.value);hotLimit=[10,20,50,100].includes(value)?value:20;if(hotLimitControl.value!==String(hotLimit))hotLimitControl.value=String(hotLimit);void load("hot");});

 async function loadMarketCapitalFlow() {
  const status=$("#market-capital-flow-status"),body=$("#market-capital-flow-body");
  const start=$("#market-capital-flow-start")?.value||"",end=$("#market-capital-flow-end")?.value||"",button=$("#load-market-capital-flow");
  if(!start||!end){
   if(status){status.textContent="请选择开始和结束日期";status.className="availability-pill is-readonly";}
   if(body)tableMessage(body,"请选择开始和结束日期；本次未发起请求。");
   meta("market-capital-flow-meta",[]);qualityReasons("market-capital-flow-reasons",[]);
   return;
  }
  const settings=typeof getSettings==="function"?getSettings():null;
  if(!settings?.apiKey){
   if(status){status.textContent="请先配置行情服务 API Key";status.className="availability-pill is-unavailable";}
   if(body)tableMessage(body,"未配置行情服务 API Key，本次未发起请求。");
   meta("market-capital-flow-meta",[]);qualityReasons("market-capital-flow-reasons",[]);
   return;
  }
  const sequence=++capitalFlowGeneration;
  try{capitalFlowController?.abort();}catch{}
  const controller=new AbortController();capitalFlowController=controller;
  if(button){button.disabled=true;button.textContent="读取中…";}
  if(status){status.textContent="正在读取市场级资金流…";status.className="availability-pill is-readonly";}
  if(body)tableMessage(body,"正在读取服务端市场级资金流历史…");
  meta("market-capital-flow-meta",[]);qualityReasons("market-capital-flow-reasons",[]);
  try{
   const data=await getMarketCapitalFlow(settings,{start,end,signal:controller.signal});
   if(controller.signal.aborted||sequence!==capitalFlowGeneration)return;
   renderMarketCapitalFlow(data);
  }catch(error){
   if(controller.signal.aborted||sequence!==capitalFlowGeneration)return;
   if(status){status.textContent=Number(error?.status)===503?"Provider 未启用或不可用":Number(error?.status)===502?"Provider 返回失败或无效数据":"读取失败";status.className="availability-pill is-unavailable";}
   if(body)tableMessage(body,"市场级资金流读取失败："+safeOverviewError(error));
   meta("market-capital-flow-meta",[]);qualityReasons("market-capital-flow-reasons",[]);
  }finally{
   if(sequence===capitalFlowGeneration){
    if(capitalFlowController===controller)capitalFlowController=null;
    if(button){button.disabled=false;button.textContent="读取历史资金流";}
   }
  }
 }
 const defaultFlowRange = defaultMarketCapitalFlowDateRange();
 const flowStart=$("#market-capital-flow-start"),flowEnd=$("#market-capital-flow-end");
 if(flowStart&&!flowStart.value)flowStart.value=defaultFlowRange.start;
 if(flowEnd&&!flowEnd.value)flowEnd.value=defaultFlowRange.end;
 $("#load-market-capital-flow")?.addEventListener("click",()=>{void loadMarketCapitalFlow();});

 async function loadSentimentHistory() {
  const status=$("#market-sentiment-history-status");
  const body=$("#market-sentiment-history-body");
  const start=$("#market-sentiment-history-start")?.value || "";
  const end=$("#market-sentiment-history-end")?.value || "";
  if(!start||!end){if(status)status.textContent="请选择开始和结束日期";return;}
  const settings=typeof getSettings==="function"?getSettings():null;
  if(!settings?.apiKey){if(status)status.textContent="请先配置行情 API Key";return;}
  const button=$("#load-market-sentiment-history");
  const previous=button?.textContent;
  if(button){button.disabled=true;button.textContent="读取中…";}
  const controller=new AbortController();
  try{
   if(status)status.textContent="正在读取已持久化的情绪快照…";
   const data=await getMarketSentimentHistory(settings,{start,end,limit:120,signal:controller.signal});
   const items=Array.isArray(data?.items)?data.items:[];
   if(body){
    body.replaceChildren();
    if(!items.length){const row=document.createElement("tr"),cell=el("td","table-empty","该日期范围内没有已保存的情绪快照；不会用当前行情补造历史。");cell.colSpan=7;row.append(cell);body.append(row);}
    items.forEach(item=>{
     const row=document.createElement("tr");
     row.append(el("td","",item.trade_date||"未报告"));
     [count(item.advancing_count),count(item.declining_count),count(item.unchanged_count),count(item.limit_up_count),count(item.limit_down_count),qualityLabel(item.quality_status)].forEach(value=>row.append(el("td","",value)));
     const actions=el("td","market-date-drilldown-actions"),tradeDate=String(item.trade_date||"");
     if(isValidTradeDate(tradeDate)){
      [["pool","up","涨停池"],["pool","down","跌停池"],["pool","broken","炸板池"],["dragon-tiger","","龙虎榜"]].forEach(([target,type,label])=>{const action=el("button","button button-secondary market-stock-entry-action",label);action.type="button";action.dataset.marketDate=tradeDate;action.dataset.marketDateTarget=target;if(type)action.dataset.marketDateType=type;actions.append(action);});
     }else actions.textContent="日期未报告，无法联动";
     row.append(actions);body.append(row);
    });
   }
   if(status)status.textContent="已读取 "+items.length+"/"+count(data?.total)+" 条持久化快照";
   meta("market-sentiment-history-meta",["请求 "+(data?.start_date||start)+" 至 "+(data?.end_date||end),"数据条数 "+count(data?.total),"按服务端已保存记录展示"]);
  }catch(error){
   if(status)status.textContent="读取失败："+safeOverviewError(error);
   if(body){body.replaceChildren();const row=document.createElement("tr"),cell=el("td","table-empty","历史快照读取失败；请检查数据库历史存储与行情服务状态。");cell.colSpan=7;row.append(cell);body.append(row);}
  }finally{
   if(button){button.disabled=false;button.textContent=previous||"读取历史";}
  }
 }
 $("#load-market-sentiment-history")?.addEventListener("click",()=>{void loadSentimentHistory();});
 $("#market-sentiment-history-body")?.addEventListener("click",event=>{
  const button=event.target.closest("button[data-market-date-target]");if(!button)return;
  const date=String(button.dataset.marketDate||"");if(!isValidTradeDate(date))return;
  if(button.dataset.marketDateTarget==="pool"){
   poolDate=date;const dateInput=$("#market-pool-date");if(dateInput)dateInput.value=date;
   poolType=["up","down","broken"].includes(button.dataset.marketDateType)?button.dataset.marketDateType:"up";
   document.querySelectorAll("[data-pool-type]").forEach(item=>item.setAttribute("aria-pressed",String(item.dataset.poolType===poolType)));
   void load("pool");$("#market-lists-panel")?.scrollIntoView({behavior:"smooth",block:"start"});
  }else if(button.dataset.marketDateTarget==="dragon-tiger"){
   window.dispatchEvent(new CustomEvent("market-date-selected",{detail:{date}}));
   $("#market-activity-panel")?.scrollIntoView({behavior:"smooth",block:"start"});
  }
 });

 $("#market-hot-stocks-body")?.addEventListener("click",event=>{const button=event.target.closest("button[data-hot-symbol]");if(!button||!button.dataset.hotSymbol||typeof onSelectStock!=="function")return;const targetView=button.dataset.hotAction==="research"?"research":"stocks";onSelectStock({symbol:button.dataset.hotSymbol,name:button.dataset.hotName||button.dataset.hotSymbol},"人气/飙升榜",targetView);});
 $("#market-pool-body")?.addEventListener("click",event=>{const button=event.target.closest("button[data-pool-symbol]");if(!button||!button.dataset.poolSymbol||typeof onSelectStock!=="function")return;const targetView=button.dataset.poolAction==="research"?"research":"stocks";onSelectStock({symbol:button.dataset.poolSymbol,name:button.dataset.poolName||button.dataset.poolSymbol},"涨跌停池",targetView);});
 return {refresh,invalidateConnection};
}
