import { getMarketBreadth, getMarketHotStocks, getMarketIndices, getMarketLimitPool, getMarketSectors } from "./api.js";
import { describeApiError, formatCompact, formatDateTime, formatPercent, formatPrice, qualityLabel } from "./format.js";
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
function count(v) { if(v===null||v===undefined||v==="")return "—"; const n=Number(v); return Number.isFinite(n)&&n>=0?Math.trunc(n).toLocaleString("zh-CN"):"—"; }
function signed(v, fmt) { if(v===null||v===undefined||v===""||!Number.isFinite(Number(v)))return "—";return (Number(v)>0?"+":"")+fmt(v); }
function dir(v) { const n=Number(v); if(v===null||v===undefined||v===""||!Number.isFinite(n))return "market-data-unknown";return n>0?"market-data-positive":n<0?"market-data-negative":"market-data-flat"; }
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
 if(key==="indices")empty($("#market-index-grid"),msg);if(key==="breadth")empty($("#market-breadth-values"),msg);if(key==="sectors")tableMessage($("#market-sector-body"),msg);if(key==="pool")tableMessage($("#market-pool-body"),msg);if(key==="hot")tableMessage($("#market-hot-stocks-body"),msg);
}
function errorMessage(err) { if(Number(err?.status)===404)return "当前部署的行情服务尚未提供此接口，请更新 market-data 服务后再刷新。";if([401,403].includes(Number(err?.status)))return "行情服务 API Key 验证失败，请检查连接与密钥设置。";return describeApiError(err,"读取市场数据失败。"); }
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
 if(!data||!fields.some(([,v])=>v!==null&&v!==undefined&&v!=="")){empty(root,"接口未返回可展示的涨跌分布字段。");badge("market-breadth-status","数据字段不完整","warning");return;}
 root.replaceChildren();fields.forEach(([label,value,cls])=>{const m=el("div","market-breadth-metric");m.append(el("small","",label),el("strong",cls,count(value)));root.append(m);});
 badge("market-breadth-status",qualityLabel(data.quality_status),"quality",data.quality_status);
 const amount=data.total_amount_cny===null||data.total_amount_cny===undefined?"全市场成交额未由此接口提供":"成交额 "+formatCompact(data.total_amount_cny)+" 元";
 meta("market-breadth-meta",["报告样本 "+count(data.total_securities)+" 只","覆盖样本 "+count(data.covered_securities)+" 只",amount,"来源 "+(data.source||"未报告"),"快照时间 "+formatDateTime(data.fetched_at)]);qualityReasons("market-breadth-reasons",data.quality_reasons);
}
function renderSectors(data) {
 const root=$("#market-sector-body"),items=Array.isArray(data?.items)?data.items.filter(v=>v&&typeof v==="object"):[ ];root.replaceChildren();
 if(!items.length){tableMessage(root,"接口已响应，但没有返回板块记录；这不代表市场不存在相关板块。");badge("market-sector-status","暂无板块明细","warning");}
 else{items.forEach(item=>{const tr=document.createElement("tr");cell(tr,item.rank===null||item.rank===undefined?"—":count(item.rank),"market-rank-cell");const name=el("td");name.append(el("strong","market-table-primary",item.name||"名称未报告"),el("small","market-table-secondary",item.code||"代码未报告"));tr.append(name);cell(tr,formatPercent(item.change_percent),dir(item.change_percent));cell(tr,item.leading_stock||"—");cell(tr,formatPercent(item.leader_change_percent),dir(item.leader_change_percent));root.append(tr);});badge("market-sector-status",qualityLabel(data.quality_status),"quality",data.quality_status);}
 meta("market-sector-meta",[(data?.category==="concept"?"概念":"行业")+"排行","返回 "+count(items.length)+" 条","源端总数 "+count(data?.total),"来源 "+(data?.source||"未报告"),"快照时间 "+formatDateTime(data?.fetched_at)]);qualityReasons("market-sector-reasons",data?.quality_reasons);
}
function renderPool(data) {
 const root=$("#market-pool-body"),items=Array.isArray(data?.items)?data.items.filter(v=>v&&typeof v==="object"):[ ];root.replaceChildren();
 if(!items.length){tableMessage(root,data?.has_data===false?"服务端未确认此池有可用数据；不能把当前空列表当作 0 只。":"接口未返回列表记录。");badge("market-pool-status",data?.has_data===false?"数据未确认":"暂无列表记录","warning");}
 else{items.forEach(item=>{const tr=document.createElement("tr"),stock=el("td");stock.append(el("strong","market-table-primary",item.name||"名称未报告"),el("small","market-table-secondary",item.symbol||item.code||"代码未报告"));tr.append(stock);cell(tr,formatPercent(item.change_percent),dir(item.change_percent));cell(tr,item.consecutive_boards===null||item.consecutive_boards===undefined?"—":count(item.consecutive_boards)+" 板");cell(tr,(item.first_touch_time||"—")+" / "+(item.last_touch_time||"—"),"market-touch-time");cell(tr,item.industry||"—");root.append(tr);});badge("market-pool-status",qualityLabel(data.quality_status),"quality",data.quality_status);}
 const label=data?.type==="down"?"跌停池":data?.type==="broken"?"炸板池":"涨停池";meta("market-pool-meta",[label,"交易日期 "+(data?.trade_date||"未报告"),"源端数量 "+count(data?.total),"当前返回 "+count(items.length)+" 条","来源 "+(data?.source||"未报告"),"快照时间 "+formatDateTime(data?.fetched_at)]);qualityReasons("market-pool-reasons",data?.quality_reasons);
}
function renderHotStocks(data) {
 const root=$("#market-hot-stocks-body"),items=Array.isArray(data?.items)?data.items.filter(item=>item&&typeof item==="object"):[];
 root.replaceChildren();
 if(!items.length) {
  tableMessage(root,"接口未返回人气榜/飙升榜记录；当前列表不是全市场证券的完整清单。");
  badge("market-hot-stocks-status","暂无列表记录","warning");
 } else {
  items.forEach(item=>{
   const tr=document.createElement("tr");
   cell(tr,item.rank===null||item.rank===undefined?"—":count(item.rank),"market-rank-cell");
   const stock=el("td"),button=el("button","market-hot-stock-link");
   button.type="button";button.dataset.hotSymbol=String(item.symbol||"");button.dataset.hotName=String(item.name||item.symbol||item.code||"");button.disabled=!String(item.symbol||"").trim();
   button.append(el("strong","market-table-primary",item.name||"名称未报告"),el("small","market-table-secondary",item.code||item.symbol||"代码未报告"));
   stock.append(button);tr.append(stock);
   const price=el("td");price.append(el("strong","market-table-primary",formatPrice(item.latest_price)),el("small","market-table-secondary","涨跌额 "+formatPrice(item.change)));tr.append(price);
   cell(tr,formatPercent(item.change_percent),dir(item.change_percent));
   cell(tr,item.rank_change===null||item.rank_change===undefined?"—":String(item.rank_change),"market-rank-cell");
   root.append(tr);
  });
  badge("market-hot-stocks-status",qualityLabel(data.quality_status),"quality",data.quality_status);
 }
 meta("market-hot-stocks-meta",[(data?.type==="surge"?"飙升榜":"人气榜"),"当前返回 "+count(items.length)+" 条（非全市场总数）","来源 "+(data?.source||"未报告"),"采集时间 "+formatDateTime(data?.fetched_at),"源端未提供可独立核验的行情快照时间"]);
 qualityReasons("market-hot-stocks-reasons",data?.quality_reasons);
}
export function bindMarketOverview(getSettings,onSelectStock) {
 const controllers={},generation={indices:0,breadth:0,sectors:0,pool:0,hot:0};let sectorType="industry",poolType="up",hotType="popular",refreshGeneration=0;
 function cancel(key){controllers[key]?.abort();delete controllers[key];generation[key]++;}
 async function load(key){cancel(key);const seq=generation[key],settings=typeof getSettings==="function"?getSettings():null;
  if(!settings?.apiKey){pending(key,"请先在连接与密钥设置中配置行情 API Key。","warning");return "needs_key";}
  const ctl=new AbortController();controllers[key]=ctl;pending(key,"正在读取…","pending");
  try{let data;if(key==="indices")data=await getMarketIndices(settings,ctl.signal);else if(key==="breadth")data=await getMarketBreadth(settings,ctl.signal);else if(key==="sectors")data=await getMarketSectors(settings,{type:sectorType,limit:10,signal:ctl.signal});else if(key==="pool")data=await getMarketLimitPool(settings,{type:poolType,limit:30,signal:ctl.signal});else if(key==="hot")data=await getMarketHotStocks(settings,{type:hotType,limit:20,signal:ctl.signal});else return "failed";
   if(ctl.signal.aborted||generation[key]!==seq)return "cancelled";if(key==="indices")renderIndices(data);else if(key==="breadth")renderBreadth(data);else if(key==="sectors")renderSectors(data);else if(key==="pool")renderPool(data);else renderHotStocks(data);return "received";
  }catch(err){
   if(ctl.signal.aborted||generation[key]!==seq)return "cancelled";
   pending(key,errorMessage(err),"error");
   const status=Number(err?.status);
   if(status===404)return "unsupported";
   if(status===503)return "provider_unavailable";
   if(status===401||status===403)return "auth_failed";
   return "failed";
  }finally{if(generation[key]===seq)delete controllers[key];}
 }
 async function refresh(){const currentGeneration=++refreshGeneration,status=$("#market-overview-updated"),settings=typeof getSettings==="function"?getSettings():null;if(status)status.textContent=settings?.apiKey?"正在刷新各模块…":"配置行情 API Key 后加载市场概况";const outcomes=await Promise.all(KEYS.map(load));if(currentGeneration!==refreshGeneration)return;if(status)status.textContent=summarizeMarketOverviewRefresh(outcomes,Boolean(settings?.apiKey));}
 $("#refresh-market-overview")?.addEventListener("click",()=>{void refresh();});
 document.querySelectorAll("[data-sector-type]").forEach(button=>button.addEventListener("click",()=>{sectorType=button.dataset.sectorType==="concept"?"concept":"industry";document.querySelectorAll("[data-sector-type]").forEach(item=>item.setAttribute("aria-pressed",String(item===button)));void load("sectors");}));
 document.querySelectorAll("[data-pool-type]").forEach(button=>button.addEventListener("click",()=>{poolType=["up","down","broken"].includes(button.dataset.poolType)?button.dataset.poolType:"up";document.querySelectorAll("[data-pool-type]").forEach(item=>item.setAttribute("aria-pressed",String(item===button)));void load("pool");}));
 document.querySelectorAll("[data-hot-type]").forEach(button=>button.addEventListener("click",()=>{hotType=["popular","surge"].includes(button.dataset.hotType)?button.dataset.hotType:"popular";document.querySelectorAll("[data-hot-type]").forEach(item=>item.setAttribute("aria-pressed",String(item===button)));void load("hot");}));
 $("#market-hot-stocks-body")?.addEventListener("click",event=>{const button=event.target.closest("button[data-hot-symbol]");if(!button||!button.dataset.hotSymbol||typeof onSelectStock!=="function")return;onSelectStock({symbol:button.dataset.hotSymbol,name:button.dataset.hotName||button.dataset.hotSymbol},"人气/飙升榜");});
 return {refresh};
}
