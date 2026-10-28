import { getCapitalFlow, getChipDistribution, getMarketValuations, getStockAnnouncements, getStockBoards, getStockFundamentals, getStockProfile } from "./api.js";
import { describeApiError, formatDateTime, formatPercent, qualityLabel } from "./format.js";
const $=(s)=>document.querySelector(s);
const QUALITY=["valid","degraded","stale","invalid","unknown"];
const CATEGORY={profitability:"盈利能力",operating:"营运能力",growth:"成长能力",solvency:"偿债能力",cash_flow:"现金流",dupont:"杜邦分析"};
const PANEL_CONFIG={
 profile:{root:"#stock-profile-fields",symbol:"stock-profile-symbol",meta:"stock-profile-meta",reasons:"stock-profile-reasons",quality:"profile-quality",error:"stock-profile-error",empty:"选择股票并加载后显示上市资料。"},
 boards:{root:"#stock-board-list",symbol:"stock-boards-symbol",meta:"stock-boards-meta",reasons:"stock-boards-reasons",quality:"boards-quality",error:"stock-boards-error",empty:"加载后显示所属行业与概念板块。"},
 fundamentals:{root:"#fundamentals-content",symbol:"fundamentals-symbol",meta:"fundamentals-meta",reasons:"fundamentals-reasons",quality:"fundamentals-quality",error:"fundamentals-error",empty:"选择报告期并加载财务指标。"},
 announcements:{root:"#announcement-list",symbol:"announcements-symbol",meta:"announcements-meta",reasons:"announcements-reasons",quality:"announcements-quality",error:"announcements-error",empty:"加载后显示数据源返回的公告。"},
 valuations:{root:"#valuation-content",symbol:"valuations-symbol",meta:"valuations-meta",reasons:"valuations-reasons",quality:"valuations-quality",error:"valuations-error",empty:"选择股票并读取估值指标。"},
 chip:{root:"#chip-distribution-content",symbol:"chip-symbol",meta:"chip-meta",reasons:"chip-reasons",quality:"chip-quality",error:"chip-error",empty:"选择股票并读取筹码分布。"},
 capitalFlow:{root:"#capital-flow-content",symbol:"capital-flow-symbol",meta:"capital-flow-meta",reasons:"capital-flow-reasons",quality:"capital-flow-quality",error:"capital-flow-error",empty:"选择股票并读取资金流摘要。"}
};
function el(tag,cls="",text=""){const n=document.createElement(tag);if(cls)n.className=cls;if(text!==undefined&&text!==null)n.textContent=String(text);return n;}
function setText(id,text){const n=$("#"+id);if(n)n.textContent=text===null||text===undefined||text===""?"—":String(text);}
function setQuality(id,status,label=""){const n=$("#"+id);if(!n)return;const q=QUALITY.includes(String(status).toLowerCase())?String(status).toLowerCase():"unknown";n.className="supplement-quality supplement-quality-"+q;n.textContent=label||qualityLabel(q);}
function setError(id,message){const n=$("#"+id);if(!n)return;n.hidden=!message;n.textContent=message||"";}
function makeDl(items){const dl=el("dl","supplemental-fields");items.forEach(([label,value])=>{dl.append(el("dt","",label),el("dd","",value===null||value===undefined||value===""?"—":value));});return dl;}
function reasons(id,rows){const root=$("#"+id);if(!root)return;root.replaceChildren();if(!Array.isArray(rows)||!rows.length){root.append(el("p","","接口未返回质量原因；请结合状态与时间判断。"));return;}rows.forEach(v=>root.append(el("p","",v)));}
function clearReasons(id){const root=$("#"+id);if(root)root.replaceChildren();}
function formatNumber(value){if(value===null||value===undefined||value===""||!Number.isFinite(Number(value)))return "—";return new Intl.NumberFormat("zh-CN",{maximumFractionDigits:4}).format(Number(value));}
function responseError(err){if(Number(err?.status)===404)return "当前部署的 market-data 尚未提供该接口，请更新数据服务版本。";if(Number(err?.status)===503)return "对应数据源未启用或未配置，请检查 market-data 的 Provider 配置。";return describeApiError(err,"请求失败。");}
function safeAnnouncementUrl(raw){try{const url=new URL(String(raw||""));return url.protocol==="https:"&&(url.hostname==="eastmoney.com"||url.hostname.endsWith(".eastmoney.com"))?url.href:"";}catch{return "";}}
function currentYearAndQuarter(){const now=new Date();let year=now.getFullYear(),quarter=Math.floor(now.getMonth()/3);if(quarter===0){year-=1;quarter=4;}return {year,quarter};}
export function bindStockResearch(getSettings,getSymbol){
 const state={symbol:"",generation:{profile:0,boards:0,fundamentals:0,announcements:0,valuations:0,chip:0,capitalFlow:0},controllers:{},requested:new Set(),announcementPage:0,announcementHasMore:false,announcementLoading:false,loadResearchGeneration:0};
 function resolvedSymbol(){try{return String(getSymbol?.()||"").trim();}catch{return "";}}
 function abort(key){state.controllers[key]?.abort();delete state.controllers[key];state.generation[key]++;}
 function clearPanel(key,placeholder,symbol=state.symbol){
  const panel=PANEL_CONFIG[key];if(!panel)return;
  const root=$(panel.root);if(root)root.replaceChildren(el("p","stock-research-empty",placeholder));
  setText(panel.symbol,symbol||"尚未选择");setText(panel.meta,"");clearReasons(panel.reasons);setError(panel.error,"");
 }
 function markApiKeyRequired(){
  Object.keys(state.generation).forEach(abort);
  state.announcementPage=0;state.announcementHasMore=false;state.announcementLoading=false;
  Object.keys(PANEL_CONFIG).forEach(key=>{
   const panel=PANEL_CONFIG[key];
   clearPanel(key,"需要配置行情 API Key 后才能读取。",state.symbol);
   setQuality(panel.quality,"unknown","需配置 Key");
   setError(panel.error,"请先在连接与密钥设置中配置行情 API Key。");
  });
  setText("stock-research-target",state.symbol?"当前标的："+state.symbol:"请先输入代码，或从中文名称搜索结果中选择股票");
  setText("stock-research-feedback","请先在连接与密钥设置中配置行情 API Key。");
  $("#load-more-announcements").disabled=true;$("#load-more-announcements").textContent="加载更多公告";
 }
 function reset(symbol){
  state.loadResearchGeneration+=1;
  Object.keys(state.generation).forEach(abort);state.symbol=symbol;state.requested.clear();state.announcementPage=0;state.announcementHasMore=false;state.announcementLoading=false;
  setText("stock-research-target",symbol?"当前标的："+symbol:"请先输入代码，或从中文名称搜索结果中选择股票");
  Object.keys(PANEL_CONFIG).forEach(key=>{
   const panel=PANEL_CONFIG[key];clearPanel(key,panel.empty,symbol);setQuality(panel.quality,"unknown","未加载");
  });
  setText("stock-research-feedback",symbol?"标的已切换；请重新加载对应资料，避免沿用上一只股票的结果。":"请先输入代码，或从中文名称搜索结果中选择股票。");
  [
   ["#load-stock-research",symbol?"重新加载公司资料":"加载公司资料"],
   ["#load-valuations","读取估值指标"],
   ["#load-chip-distribution","读取筹码分布"],
   ["#load-capital-flow","读取资金流"]
  ].forEach(([selector,label])=>{
   const button=$(selector);
   if(button){button.disabled=false;button.textContent=label;}
  });
  $("#load-more-announcements").disabled=true;$("#load-more-announcements").textContent="加载更多公告";
 }
 function syncSymbol(){
  const symbol=resolvedSymbol();
  if(symbol!==state.symbol)reset(symbol);
  return symbol;
 }
 function invalidate(message="行情连接配置已变更，请重新加载个股研究资料。"){
  reset(resolvedSymbol());
  const settings=typeof getSettings==="function"?getSettings():null;
  if(!settings?.apiKey){markApiKeyRequired();return;}
  setText("stock-research-feedback",message);
 }
 async function fetchOne(key,request,render){
  abort(key);const seq=state.generation[key],symbol=state.symbol;
  if(!symbol)return false;
  const settings=typeof getSettings==="function"?getSettings():null;
  const panel=PANEL_CONFIG[key];if(!panel)return false;
  if(!settings?.apiKey){markApiKeyRequired();return false;}
  state.requested.add(key);
  clearPanel(key,"正在读取本次请求，尚无新结果。",symbol);
  setQuality(panel.quality,"unknown","读取中…");
  const ctl=new AbortController();state.controllers[key]=ctl;
  try{const data=await request(settings,symbol,ctl.signal);if(ctl.signal.aborted||seq!==state.generation[key]||symbol!==state.symbol)return false;render(data,symbol);return true;}
  catch(err){if(ctl.signal.aborted||seq!==state.generation[key]||symbol!==state.symbol)return false;setQuality(panel.quality,"invalid","读取失败");setError(panel.error,responseError(err));return false;}
  finally{if(seq===state.generation[key])delete state.controllers[key];}
 }
 function renderProfile(data,symbol){
  setText("stock-profile-symbol",data.symbol||symbol);setQuality("profile-quality",data.quality_status);
  $("#stock-profile-fields").replaceChildren(makeDl([
   ["证券代码",data.symbol],["证券名称",data.name],["证券类型",data.security_type_label],["上市状态",data.listing_status_label],["上市日期",data.ipo_date],["退市日期",data.out_date],["数据源",data.source],["标准化版本",data.normalization_version],["获取时间",formatDateTime(data.fetched_at)]
  ]));
  reasons("stock-profile-reasons",data.quality_reasons);setText("stock-profile-meta","来源 "+(data.source||"未报告")+" · 获取时间 "+formatDateTime(data.fetched_at));
 }
 function renderBoards(data,symbol){
  setText("stock-boards-symbol",data.symbol||symbol);setQuality("boards-quality",data.quality_status);
  const root=$("#stock-board-list"),items=Array.isArray(data.items)?data.items:[ ];root.replaceChildren();
  if(!items.length)root.append(el("p","stock-research-empty",data.has_data===false?"数据源未确认该股票的所属板块，不能解释为确实没有板块。":"接口未返回板块记录。"));
  else items.forEach(item=>{const card=el("article","stock-board-item"),top=el("div","stock-board-item-top"),name=el("strong","",item.name||"板块名称未报告"),code=el("small","",item.code||"代码未报告");top.append(name,code);card.append(top);const metaLine=el("div","stock-board-item-meta");metaLine.append(el("span","",item.change_percent===null||item.change_percent===undefined?"涨跌幅 —":"涨跌幅 "+formatPercent(item.change_percent)),el("span","",item.provider_report_date?"源端日期 "+item.provider_report_date:"源端日期未报告"));card.append(metaLine);root.append(card);});
  setText("stock-boards-meta","源端返回 "+(data.total??items.length)+" 条 · "+(data.source||"未报告")+" · 获取时间 "+formatDateTime(data.fetched_at));reasons("stock-boards-reasons",data.quality_reasons);
 }
 async function loadAnnouncements(append=false){
  const symbol=syncSymbol();if(!symbol)return false;
  const settings=typeof getSettings==="function"?getSettings():null;
  if(!settings?.apiKey){markApiKeyRequired();return false;}
  if(!append)state.requested.add("announcements");
  if(state.announcementLoading)return false;
  if(append&&!state.announcementHasMore)return false;
  if(!append){
   state.announcementPage=0;state.announcementHasMore=false;
   clearPanel("announcements","正在读取本次公告，尚无新结果。",symbol);
   setQuality("announcements-quality","unknown","读取中…");
  }
  setError("announcements-error","");
  abort("announcements");const seq=state.generation.announcements,requestPage=append?state.announcementPage+1:1;
  state.announcementLoading=true;$("#load-more-announcements").disabled=true;$("#load-more-announcements").textContent="正在加载…";
  const ctl=new AbortController();state.controllers.announcements=ctl;
  try{
   const data=await getStockAnnouncements(settings,symbol,{page:requestPage,pageSize:20,signal:ctl.signal});
   if(ctl.signal.aborted||seq!==state.generation.announcements||symbol!==state.symbol)return false;
   const root=$("#announcement-list");
   if(!append)root.replaceChildren();
   const items=Array.isArray(data.items)?data.items:[ ];
   if(!items.length&&!append)root.append(el("p","stock-research-empty",data.has_data===false?"数据源在该查询范围内未返回公告记录；空列表不代表已确认没有公告。":"接口未返回公告明细。"));
   items.forEach(item=>{
    const article=el("article","stock-announcement-item"),header=el("div","stock-announcement-header"),title=item.title_chinese||item.title||item.title_english||"公告标题未报告",url=safeAnnouncementUrl(item.source_url);
    if(url){const link=el("a","stock-announcement-title",title);link.href=url;link.target="_blank";link.rel="noopener noreferrer";header.append(link);}else header.append(el("strong","stock-announcement-title",title));
    header.append(el("span","supplement-quality supplement-quality-"+(QUALITY.includes(String(item.quality_status).toLowerCase())?String(item.quality_status).toLowerCase():"unknown"),qualityLabel(item.quality_status)));
    article.append(header);
    const detail=el("p","stock-announcement-detail"),published=item.published_at?"发布时间 "+formatDateTime(item.published_at):"源端未提供精确发布时间";
    detail.textContent=(item.announcement_date||"公告日期未报告")+" · "+published+" · "+(item.category||item.category_code||"分类未报告");article.append(detail);root.append(article);
   });
   state.announcementPage=requestPage;state.announcementHasMore=Boolean(data.has_more);
   setQuality("announcements-quality",data.quality_status);
   setText("announcements-symbol",data.symbol||symbol);
   setText("announcements-meta","查询范围 "+(data.start_date||"默认起始日")+" 至 "+(data.end_date||"默认结束日")+" · 已返回 "+(data.returned??items.length)+" 条 / 源端总量 "+(data.total??"未报告")+" · "+(data.source||"未报告")+" · 获取时间 "+formatDateTime(data.fetched_at));
   reasons("announcements-reasons",data.quality_reasons);
   if(!items.length&&append)root.append(el("p","stock-research-empty","本页未返回新记录。"));
   return true;
  }catch(err){
   if(ctl.signal.aborted||seq!==state.generation.announcements||symbol!==state.symbol)return false;
   if(!append)setQuality("announcements-quality","invalid","读取失败");
   setError("announcements-error",responseError(err));return false;
  }finally{
   if(seq===state.generation.announcements){
    state.announcementLoading=false;delete state.controllers.announcements;
    $("#load-more-announcements").disabled=!state.announcementHasMore;
    $("#load-more-announcements").textContent=state.announcementHasMore?"加载更多公告":(state.announcementPage>0?"已加载当前可用公告":"加载更多公告");
   }
  }
 }
 async function loadResearch(){
  const symbol=syncSymbol(),settings=typeof getSettings==="function"?getSettings():null;
  if(!symbol){setText("stock-research-feedback","请先输入有效股票代码，或从中文名称搜索建议中选择股票。");return;}
  if(!settings?.apiKey){markApiKeyRequired();return;}
  const requestGeneration=++state.loadResearchGeneration;
  setText("stock-research-feedback","正在分别读取公司档案、所属板块和近期公告，各接口状态独立。");
  $("#load-stock-research").disabled=true;$("#load-stock-research").textContent="正在加载…";
  try{
   const results=await Promise.allSettled([
    fetchOne("profile",getStockProfile,renderProfile),
    fetchOne("boards",getStockBoards,renderBoards),
    loadAnnouncements(false)
   ]);
   if(requestGeneration!==state.loadResearchGeneration||symbol!==state.symbol)return;
   const failures=results.filter(r=>r.status==="rejected"||r.value===false).length;
   setText("stock-research-feedback",failures===0?"公司资料、所属板块和公告请求已完成，请查看各自质量与来源时间。":failures+" 项读取未成功；其它模块仍独立展示，请分别查看对应错误。");
  }finally{
   if(requestGeneration===state.loadResearchGeneration&&symbol===state.symbol){
    $("#load-stock-research").disabled=false;
    $("#load-stock-research").textContent="重新加载公司资料";
   }
  }
 }
 async function loadFundamentals(){
  const symbol=syncSymbol();if(!symbol){setText("stock-research-feedback","请先选择一只有效股票。");return;}
  const year=Number($("#fundamentals-year").value),quarter=Number($("#fundamentals-quarter").value);
  if(!Number.isInteger(year)||year<2007||!Number.isInteger(quarter)||quarter<1||quarter>4){abort("fundamentals");clearPanel("fundamentals","报告期参数无效；请修正后重新加载。",symbol);setQuality("fundamentals-quality","unknown","参数无效");setError("fundamentals-error","财务报告期不正确。");setText("stock-research-feedback","财务报告期不正确。");return;}
  const ok=await fetchOne("fundamentals",(settings,sym,signal)=>getStockFundamentals(settings,sym,{year,quarter,signal}),renderFundamentals);
  if(ok)setText("stock-research-feedback","季度财务指标已返回；请同时参考报告期、披露日期和质量状态。");
 }
 async function runSupplementRequest(buttonId,idleText,key,request,render,successMessage){
  const symbol=syncSymbol();
  if(!symbol){setText("stock-research-feedback","请先选择一只有效股票。");return false;}
  const button=$(buttonId);
  button.disabled=true;button.textContent="读取中…";
  const operation=fetchOne(key,request,render);
  const generation=state.generation[key];
  try{
   const ok=await operation;
   if(ok&&generation===state.generation[key]&&symbol===state.symbol)setText("stock-research-feedback",successMessage);
   return ok;
  }finally{
   if(generation===state.generation[key]&&symbol===state.symbol){
    button.disabled=false;
    button.textContent=idleText;
   }
  }
 }
 async function loadValuations(){
  return runSupplementRequest("#load-valuations","读取估值指标","valuations",
   (settings,sym,signal)=>getMarketValuations(settings,[sym],signal),renderValuations,
   "估值快照已返回；请同时参考每只股票质量、供应商时间及缺失指标。");
 }
 function renderValuations(data,symbol){
  const requestedSymbol=String(data.requested_symbols?.[0]||symbol);
  setText("valuations-symbol",requestedSymbol);
  setQuality("valuations-quality",data.quality_status);
  const root=$("#valuation-content");root.replaceChildren();
  const items=Array.isArray(data.items)?data.items.filter(item=>item&&typeof item==="object"):[];
  const item=items.find(value=>String(value.symbol||"")===requestedSymbol)||items[0];
  if(!item){
   const message=Array.isArray(data.missing_symbols)&&data.missing_symbols.includes(requestedSymbol)
    ?"服务端未返回该标的估值记录；不会将缺失记录解释为 0。"
    :"估值接口返回成功，但没有可展示的估值项目。";
   root.append(el("p","stock-research-empty",message));
  }else{
   const grid=el("div","stock-valuation-grid");
   const metrics=[
    ["市盈率 TTM","pe_ttm"],["市盈率 MRQ","pe_mrq"],["市净率 MRQ","pb_mrq"],
    ["市销率 TTM","ps_ttm"],["市现率 TTM","pcf_ttm"]
   ];
   metrics.forEach(([label,key])=>{
    const value=item[key];
    const card=el("div","stock-valuation-metric");
    card.append(el("span","",label),el("strong","",value===null||value===undefined||value===""?"未提供":formatNumber(value)+" 倍"));
    grid.append(card);
   });
   root.append(grid);
   if(item.quality_status)setQuality("valuations-quality",item.quality_status);
   reasons("valuations-reasons",Array.isArray(item.quality_reasons)&&item.quality_reasons.length?item.quality_reasons:data.quality_reasons);
  }
  const missing=Array.isArray(data.missing_symbols)&&data.missing_symbols.length?" · 未返回："+data.missing_symbols.join("、"):"";
  setText("valuations-meta","来源 "+(item?.source||data.source||"未报告")+" · 供应商时间 "+(data.provider_timestamp?formatDateTime(data.provider_timestamp):"未报告")+" · 获取时间 "+formatDateTime(data.fetched_at)+missing);
  if(!item)reasons("valuations-reasons",data.quality_reasons);
 }
 function fractionPercent(value){
  if(value===null||value===undefined||value===""||!Number.isFinite(Number(value)))return "未提供";
  return formatNumber(Number(value)*100)+"%";
 }
 function costBand(low,high){
  const lower=low===null||low===undefined||low===""?"未提供":formatNumber(low);
  const upper=high===null||high===undefined||high===""?"未提供":formatNumber(high);
  if(lower==="未提供"&&upper==="未提供")return "未提供";
  return lower+" – "+upper+" 元";
 }
 function formatYuan(value){
  if(value===null||value===undefined||value===""||!Number.isFinite(Number(value)))return "未提供";
  const amount=Number(value);
  return (amount>0?"+":"")+new Intl.NumberFormat("zh-CN",{minimumFractionDigits:2,maximumFractionDigits:2}).format(amount)+" 元";
 }
 async function loadChipDistribution(){
  return runSupplementRequest("#load-chip-distribution","读取筹码分布","chip",
   getChipDistribution,renderChipDistribution,
   "筹码分布快照已返回；请参考数据日期、质量状态及来源说明。");
 }
 function renderChipDistribution(data,symbol){
  setText("chip-symbol",data.symbol||symbol);setQuality("chip-quality",data.quality_status);
  const root=$("#chip-distribution-content");root.replaceChildren();
  const metrics=[
   ["平均持仓成本",data.average_cost===null||data.average_cost===undefined?"未提供":formatNumber(data.average_cost)+" 元"],
   ["90% 成本区间",costBand(data.cost_90_low,data.cost_90_high)],
   ["70% 成本区间",costBand(data.cost_70_low,data.cost_70_high)],
   ["获利比例",fractionPercent(data.profit_ratio)],
   ["90% 成本集中度",fractionPercent(data.concentration_90)],
   ["70% 成本集中度",fractionPercent(data.concentration_70)]
  ];
  const grid=el("div","stock-supplement-metric-grid");
  metrics.forEach(([label,value])=>{const item=el("div","stock-supplement-metric");item.append(el("span","",label),el("strong","",value));grid.append(item);});
  root.append(grid);
  setText("chip-meta","来源 "+(data.source||"未报告")+" · 数据日期 "+(data.date||"未报告")+" · 供应商日期 "+(data.provider_date||"未报告")+" · 获取时间 "+formatDateTime(data.fetched_at));
  reasons("chip-reasons",data.quality_reasons);
 }
 async function loadCapitalFlow(){
  return runSupplementRequest("#load-capital-flow","读取资金流","capitalFlow",
   getCapitalFlow,renderCapitalFlow,
   "资金流摘要已返回；请参考统计口径、观测数与质量状态。");
 }
 function renderCapitalFlow(data,symbol){
  setText("capital-flow-symbol",data.symbol||symbol);setQuality("capital-flow-quality",data.quality_status);
  const root=$("#capital-flow-content");root.replaceChildren();
  const observations=Number(data.observations);
  const metrics=[
   ["主力净流入",formatYuan(data.main_net_inflow_yuan)],
   ["5 日净流入合计",formatYuan(data.inflow_5d_yuan)],
   ["10 日净流入合计",formatYuan(data.inflow_10d_yuan)],
   ["观测数量",Number.isSafeInteger(observations)&&observations>=0?observations+" 条":"未报告"]
  ];
  const grid=el("div","stock-supplement-metric-grid");
  metrics.forEach(([label,value])=>{const item=el("div","stock-supplement-metric");item.append(el("span","",label),el("strong","",value));grid.append(item);});
  root.append(grid);
  setText("capital-flow-meta","来源 "+(data.source||"未报告")+" · 获取时间 "+formatDateTime(data.fetched_at));
  reasons("capital-flow-reasons",data.quality_reasons);
 }
 function renderFundamentals(data,symbol){
  setText("fundamentals-symbol",data.symbol||symbol);setQuality("fundamentals-quality",data.quality_status);
  const root=$("#fundamentals-content");root.replaceChildren();
  const sections=Array.isArray(data.sections)?data.sections:[ ];
  if(data.has_data===false||!sections.length){root.append(el("p","stock-research-empty","服务端未确认该报告期存在可用财务指标；这不代表指标值为 0。"));}
  sections.forEach(section=>{
   const card=el("section","fundamental-section"),header=el("div","fundamental-section-heading");
   header.append(el("strong","",CATEGORY[section.category]||section.category||"财务指标"),el("span","supplement-quality supplement-quality-"+(QUALITY.includes(String(section.quality_status).toLowerCase())?String(section.quality_status).toLowerCase():"unknown"),qualityLabel(section.quality_status)));
   const date=el("p","fundamental-period","报告期 "+(section.report_date||"未报告")+" · 披露日期 "+(section.publication_date||"未报告"));card.append(header,date);
   const metrics=Array.isArray(section.metrics)?section.metrics:[ ];
   if(!section.has_data||!metrics.length)card.append(el("p","stock-research-empty","该分类未返回指标。"));
   else{const grid=el("div","stock-research-metric-grid");metrics.forEach(metric=>{const item=el("div","stock-research-metric");item.append(el("span","",metric.name||metric.key||"指标"),el("strong","",formatNumber(metric.value)+(metric.unit?" "+metric.unit:"")));grid.append(item);});card.append(grid);}
   if(Array.isArray(section.quality_reasons)&&section.quality_reasons.length){const details=el("details","stock-research-quality-reasons"),summary=el("summary","","该分类质量说明");details.append(summary);const list=el("ul");section.quality_reasons.forEach(reason=>list.append(el("li","",reason)));details.append(list);card.append(details);}
   root.append(card);
  });
  setText("fundamentals-meta","报告期 "+data.year+" Q"+data.quarter+" · 来源 "+(data.source||"未报告")+" · 获取时间 "+formatDateTime(data.fetched_at));
  reasons("fundamentals-reasons",data.quality_reasons);
 }
 async function refresh(){
  const symbol=syncSymbol();
  if(!symbol){setText("stock-research-feedback","请先选择有效股票，再刷新研究数据。");return false;}
  const requested=new Set(state.requested);
  if(!requested.size)return loadResearch();
  const jobs=[];
  if(requested.has("profile"))jobs.push(fetchOne("profile",getStockProfile,renderProfile));
  if(requested.has("boards"))jobs.push(fetchOne("boards",getStockBoards,renderBoards));
  if(requested.has("announcements"))jobs.push(loadAnnouncements(false));
  if(requested.has("fundamentals"))jobs.push(loadFundamentals());
  if(requested.has("valuations"))jobs.push(loadValuations());
  if(requested.has("chip"))jobs.push(loadChipDistribution());
  if(requested.has("capitalFlow"))jobs.push(loadCapitalFlow());
  return Promise.allSettled(jobs);
 }
 function initPeriodSelectors(){
  const yearSelect=$("#fundamentals-year"),quarterSelect=$("#fundamentals-quarter");
  const currentYear=new Date().getFullYear();
  for(let year=currentYear;year>=2007;year--){const option=el("option","",String(year));option.value=String(year);yearSelect.append(option);}
  [1,2,3,4].forEach(q=>{const option=el("option","", "Q"+q);option.value=String(q);quarterSelect.append(option);});
  const period=currentYearAndQuarter();yearSelect.value=String(period.year);quarterSelect.value=String(period.quarter);
 }
 $("#load-stock-research")?.addEventListener("click",()=>{void loadResearch();});
 $("#load-fundamentals")?.addEventListener("click",()=>{void loadFundamentals();});
 $("#load-valuations")?.addEventListener("click",()=>{void loadValuations();});
 $("#load-chip-distribution")?.addEventListener("click",()=>{void loadChipDistribution();});
 $("#load-capital-flow")?.addEventListener("click",()=>{void loadCapitalFlow();});
 $("#load-more-announcements")?.addEventListener("click",()=>{void loadAnnouncements(true);});
 $("#symbol-input")?.addEventListener("input",()=>{syncSymbol();});
 $("#quote-form")?.addEventListener("submit",()=>{syncSymbol();});
 initPeriodSelectors();syncSymbol();
 return {refresh,syncSymbol,invalidate};
}
