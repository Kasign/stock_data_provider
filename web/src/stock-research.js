import { getMarketValuations, getStockAnnouncements, getStockBoards, getStockFundamentals, getStockProfile } from "./api.js";
import { describeApiError, formatDateTime, formatPercent, qualityLabel } from "./format.js";
const $=(s)=>document.querySelector(s);
const QUALITY=["valid","degraded","stale","invalid","unknown"];
const CATEGORY={profitability:"盈利能力",operating:"营运能力",growth:"成长能力",solvency:"偿债能力",cash_flow:"现金流",dupont:"杜邦分析"};
function el(tag,cls="",text=""){const n=document.createElement(tag);if(cls)n.className=cls;if(text!==undefined&&text!==null)n.textContent=String(text);return n;}
function setText(id,text){const n=$("#"+id);if(n)n.textContent=text===null||text===undefined||text===""?"—":String(text);}
function setQuality(id,status,label=""){const n=$("#"+id);if(!n)return;const q=QUALITY.includes(String(status).toLowerCase())?String(status).toLowerCase():"unknown";n.className="supplement-quality supplement-quality-"+q;n.textContent=label||qualityLabel(q);}
function setError(id,message){const n=$("#"+id);if(!n)return;n.hidden=!message;n.textContent=message||"";}
function makeDl(items){const dl=el("dl","supplemental-fields");items.forEach(([label,value])=>{dl.append(el("dt","",label),el("dd","",value===null||value===undefined||value===""?"—":value));});return dl;}
function reasons(id,rows){const root=$("#"+id);root.replaceChildren();if(!Array.isArray(rows)||!rows.length){root.append(el("p","","接口未返回质量原因；请结合状态与时间判断。"));return;}rows.forEach(v=>root.append(el("p","",v)));}
function formatNumber(value){if(value===null||value===undefined||value===""||!Number.isFinite(Number(value)))return "—";return new Intl.NumberFormat("zh-CN",{maximumFractionDigits:4}).format(Number(value));}
function responseError(err){if(Number(err?.status)===404)return "当前部署的 market-data 尚未提供该接口，请更新数据服务版本。";if(Number(err?.status)===503)return "对应数据源未启用或未配置，请检查 market-data 的 Provider 配置。";return describeApiError(err,"请求失败。");}
function safeAnnouncementUrl(raw){try{const url=new URL(String(raw||""));return url.protocol==="https:"&&(url.hostname==="eastmoney.com"||url.hostname.endsWith(".eastmoney.com"))?url.href:"";}catch{return "";}}
function currentYearAndQuarter(){const now=new Date();let year=now.getFullYear(),quarter=Math.floor(now.getMonth()/3);if(quarter===0){year-=1;quarter=4;}return {year,quarter};}
export function bindStockResearch(getSettings,getSymbol){
 const state={symbol:"",generation:{profile:0,boards:0,fundamentals:0,announcements:0,valuations:0},controllers:{},announcementPage:0,announcementHasMore:false,announcementLoading:false};
 function resolvedSymbol(){try{return String(getSymbol?.()||"").trim();}catch{return "";}}
 function abort(key){state.controllers[key]?.abort();delete state.controllers[key];state.generation[key]++;}
 function reset(symbol){
  Object.keys(state.generation).forEach(abort);state.symbol=symbol;state.announcementPage=0;state.announcementHasMore=false;state.announcementLoading=false;
  setText("stock-research-target",symbol?"当前标的："+symbol:"请先输入代码，或从中文名称搜索结果中选择股票");
  ["profile-quality","boards-quality","fundamentals-quality","announcements-quality","valuations-quality"].forEach(id=>setQuality(id,"unknown","未加载"));
  setText("stock-profile-symbol",symbol||"尚未选择");setText("stock-boards-symbol",symbol||"尚未选择");setText("fundamentals-symbol",symbol||"尚未选择");setText("announcements-symbol",symbol||"尚未选择");setText("valuations-symbol",symbol||"尚未选择");
  $("#stock-profile-fields").replaceChildren(el("p","stock-research-empty","选择股票并加载后显示上市资料。"));
  $("#stock-board-list").replaceChildren(el("p","stock-research-empty","加载后显示所属行业与概念板块。"));
  $("#fundamentals-content").replaceChildren(el("p","stock-research-empty","选择报告期并加载财务指标。"));
  $("#valuation-content").replaceChildren(el("p","stock-research-empty","选择股票并读取估值指标。"));
  $("#announcement-list").replaceChildren(el("p","stock-research-empty","加载后显示数据源返回的公告。"));
  ["stock-profile-error","stock-boards-error","fundamentals-error","announcements-error","valuations-error"].forEach(id=>setError(id,""));
  setText("stock-research-feedback",symbol?"标的已切换；请重新加载对应资料，避免沿用上一只股票的结果。":"请先输入代码，或从中文名称搜索结果中选择股票。");
  ["stock-profile-reasons","stock-boards-reasons","fundamentals-reasons","announcements-reasons"].forEach(id=>reasons(id,[]));
  setText("stock-profile-meta","");setText("stock-boards-meta","");setText("fundamentals-meta","");setText("announcements-meta","");setText("valuations-meta","");
  $("#load-more-announcements").disabled=true;$("#load-more-announcements").textContent="加载更多公告";
 }
 function syncSymbol(){
  const symbol=resolvedSymbol();
  if(symbol!==state.symbol)reset(symbol);
  return symbol;
 }
 async function fetchOne(key,request,render){
  abort(key);const seq=state.generation[key],symbol=state.symbol;
  if(!symbol)return false;
  const settings=typeof getSettings==="function"?getSettings():null;
  const ids={profile:"profile-quality",boards:"boards-quality",fundamentals:"fundamentals-quality",valuations:"valuations-quality"};
  const errorIds={profile:"stock-profile-error",boards:"stock-boards-error",fundamentals:"fundamentals-error",valuations:"valuations-error"};
  setError(errorIds[key],"");
  if(!settings?.apiKey){setQuality(ids[key],"unknown","需配置 Key");setError(errorIds[key],"请先在连接与密钥设置中配置行情 API Key。");return false;}
  setQuality(ids[key],"unknown","读取中…");
  const ctl=new AbortController();state.controllers[key]=ctl;
  try{const data=await request(settings,symbol,ctl.signal);if(ctl.signal.aborted||seq!==state.generation[key]||symbol!==state.symbol)return false;render(data,symbol);return true;}
  catch(err){if(ctl.signal.aborted||seq!==state.generation[key]||symbol!==state.symbol)return false;setQuality(ids[key],"invalid","读取失败");setError(errorIds[key]||"fundamentals-error",responseError(err));return false;}
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
  if(!settings?.apiKey){setError("announcements-error","请先在连接与密钥设置中配置行情 API Key。");return false;}
  if(state.announcementLoading)return false;
  if(append&&!state.announcementHasMore)return false;
  if(!append){state.announcementPage=0;$("#announcement-list").replaceChildren();setQuality("announcements-quality","unknown","读取中…");setError("announcements-error","");}
  abort("announcements");const seq=state.generation.announcements,requestPage=append?state.announcementPage+1:1;
  state.announcementLoading=true;$("#load-more-announcements").disabled=true;
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
  }catch(err){if(ctl.signal.aborted||seq!==state.generation.announcements||symbol!==state.symbol)return false;setQuality("announcements-quality","invalid","读取失败");setError("announcements-error",responseError(err));return false;}
  finally{if(seq===state.generation.announcements){state.announcementLoading=false;delete state.controllers.announcements;$("#load-more-announcements").disabled=!state.announcementHasMore;$("#load-more-announcements").textContent=state.announcementHasMore?"加载更多公告":"已加载当前可用公告";}}
 }
 async function loadResearch(){
  const symbol=syncSymbol(),settings=typeof getSettings==="function"?getSettings():null;
  if(!symbol){setText("stock-research-feedback","请先输入有效股票代码，或从中文名称搜索建议中选择股票。");return;}
  if(!settings?.apiKey){setText("stock-research-feedback","请先在连接与密钥设置中配置行情 API Key。");return;}
  setText("stock-research-feedback","正在分别读取公司档案、所属板块和近期公告，各接口状态独立。");
  $("#load-stock-research").disabled=true;$("#load-stock-research").textContent="正在加载…";
  const results=await Promise.allSettled([
   fetchOne("profile",getStockProfile,renderProfile),
   fetchOne("boards",getStockBoards,renderBoards),
   loadAnnouncements(false)
  ]);
  if(symbol===state.symbol){const failures=results.filter(r=>r.status==="rejected"||r.value===false).length;setText("stock-research-feedback",failures===0?"公司资料、所属板块和公告请求已完成，请查看各自质量与来源时间。":failures+" 项读取未成功；其它模块仍独立展示，请分别查看对应错误。");}
  $("#load-stock-research").disabled=false;$("#load-stock-research").textContent="重新加载公司资料";
 }
 async function loadFundamentals(){
  const symbol=syncSymbol();if(!symbol){setText("stock-research-feedback","请先选择一只有效股票。");return;}
  const year=Number($("#fundamentals-year").value),quarter=Number($("#fundamentals-quarter").value);
  if(!Number.isInteger(year)||year<2007||!Number.isInteger(quarter)||quarter<1||quarter>4){setText("stock-research-feedback","财务报告期不正确。");return;}
  const ok=await fetchOne("fundamentals",(settings,sym,signal)=>getStockFundamentals(settings,sym,{year,quarter,signal}),renderFundamentals);
  if(ok)setText("stock-research-feedback","季度财务指标已返回；请同时参考报告期、披露日期和质量状态。");
 }
 async function loadValuations(){
  const symbol=syncSymbol();
  if(!symbol){setText("stock-research-feedback","请先选择一只有效股票。");return;}
  const button=$("#load-valuations");
  button.disabled=true;button.textContent="读取中…";
  try{
   const requestedSymbol=symbol.includes(".")?symbol:symbol;
   const ok=await fetchOne("valuations",(settings,sym,signal)=>getMarketValuations(settings,[requestedSymbol],signal),renderValuations);
   if(ok)setText("stock-research-feedback","估值快照已返回；请同时参考每只股票质量、供应商时间及缺失指标。");
  }finally{button.disabled=false;button.textContent="读取估值指标";}
 }
 function renderValuations(data,symbol){
  setText("valuations-symbol",data.requested_symbols?.[0]||symbol);
  setQuality("valuations-quality",data.quality_status);
  const root=$("#valuation-content");root.replaceChildren();
  const items=Array.isArray(data.items)?data.items.filter(item=>item&&typeof item==="object"):[];
  const item=items.find(value=>String(value.symbol||"")===symbol)||items[0];
  if(!item){
   const message=Array.isArray(data.missing_symbols)&&data.missing_symbols.includes(symbol)
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
 $("#load-more-announcements")?.addEventListener("click",()=>{void loadAnnouncements(true);});
 $("#symbol-input")?.addEventListener("input",()=>{syncSymbol();});
 $("#quote-form")?.addEventListener("submit",()=>{syncSymbol();});
 initPeriodSelectors();syncSymbol();
 return {refresh:loadResearch,syncSymbol};
}
